package platform

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"sealplatform/api/internal/raster"
	"sealplatform/api/internal/seal"
	"sealplatform/api/internal/storage"
)

const sessionCookie = "seal_session"

type Service struct {
	Store                  *StateStore
	Objects                *storage.Local
	Renderer               seal.Renderer
	Logger                 *slog.Logger
	MaxBodyBytes           int64
	TokenTTL               time.Duration
	SessionTTL             time.Duration
	PaymentSecret          string
	AllowPaymentSimulation bool
	Rasterizer             raster.Rasterizer
	ImageProcessor         raster.ImageProcessor
	AdminEmail             string
	AdminMFASecret         string
	RequireAdminMFA        bool
	queue                  GenerationQueue
}

func NewService(store *StateStore, objects *storage.Local, logger *slog.Logger, maxBodyBytes int64) *Service {
	return NewServiceWithQueue(store, objects, logger, maxBodyBytes, newLocalGenerationQueue(256))
}

func NewServiceWithQueue(store *StateStore, objects *storage.Local, logger *slog.Logger, maxBodyBytes int64, queue GenerationQueue) *Service {
	service := &Service{Store: store, Objects: objects, Renderer: seal.Renderer{}, Logger: logger, MaxBodyBytes: maxBodyBytes, TokenTTL: 120 * time.Second, SessionTTL: 24 * time.Hour, queue: queue}
	go service.worker()
	for _, generation := range store.AllGenerations() {
		if generation.Status == "queued" || generation.Status == "rendering" {
			generation.Status = "queued"
			generation.StartedAt = nil
			_ = store.SaveGeneration(generation)
			_ = service.queue.Enqueue(context.Background(), generation.ID)
		}
	}
	return service
}

func (service *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", service.login)
	mux.HandleFunc("POST /api/v1/auth/register", service.register)
	mux.HandleFunc("POST /api/v1/auth/logout", service.logout)
	mux.HandleFunc("GET /api/v1/auth/me", service.me)
	mux.HandleFunc("GET /api/v1/auth/sessions", service.listSessions)
	mux.HandleFunc("DELETE /api/v1/auth/sessions/{id}", service.revokeSession)
	mux.HandleFunc("GET /api/v1/seal-configs", service.listConfigs)
	mux.HandleFunc("POST /api/v1/seal-configs", service.createConfig)
	mux.HandleFunc("GET /api/v1/seal-configs/{id}", service.getConfig)
	mux.HandleFunc("PUT /api/v1/seal-configs/{id}", service.updateConfig)
	mux.HandleFunc("DELETE /api/v1/seal-configs/{id}", service.deleteConfig)
	mux.HandleFunc("GET /api/v1/generations", service.listGenerations)
	mux.HandleFunc("POST /api/v1/generations", service.createGeneration)
	mux.HandleFunc("GET /api/v1/generations/{id}", service.getGeneration)
	mux.HandleFunc("POST /api/v1/generations/{id}/retry", service.retryGeneration)
	mux.HandleFunc("POST /api/v1/generations/{id}/download-token", service.issueDownloadToken)
	mux.HandleFunc("GET /api/v1/downloads/{token}", service.download)
	mux.HandleFunc("GET /api/v1/orders", service.listOrders)
	mux.HandleFunc("POST /api/v1/orders", service.createOrder)
	mux.HandleFunc("GET /api/v1/orders/{id}", service.getOrder)
	mux.HandleFunc("GET /api/v1/refunds", service.listRefunds)
	mux.HandleFunc("POST /api/v1/orders/{id}/refund", service.requestRefund)
	mux.HandleFunc("GET /api/v1/invoices", service.listInvoices)
	mux.HandleFunc("POST /api/v1/orders/{id}/invoice", service.requestInvoice)
	mux.HandleFunc("POST /api/v1/orders/{id}/simulate-payment", service.simulatePayment)
	mux.HandleFunc("POST /api/v1/payments/callback", service.paymentCallback)
	mux.HandleFunc("POST /api/v1/uploads/images", service.uploadImage)
	mux.HandleFunc("GET /api/v1/assets/{id}", service.getAsset)
	mux.HandleFunc("GET /api/v1/resources", service.listResources)
	mux.HandleFunc("POST /api/v1/security/csp-report", service.cspReport)
	mux.HandleFunc("GET /api/v1/admin/users", service.adminUsers)
	mux.HandleFunc("PUT /api/v1/admin/users/{id}", service.adminUpdateUser)
	mux.HandleFunc("GET /api/v1/admin/orders", service.adminOrders)
	mux.HandleFunc("GET /api/v1/admin/generations", service.adminGenerations)
	mux.HandleFunc("POST /api/v1/admin/generations/{id}/retry", service.adminRetryGeneration)
	mux.HandleFunc("GET /api/v1/admin/audit-events", service.adminAudit)
	mux.HandleFunc("GET /api/v1/admin/refunds", service.adminRefunds)
	mux.HandleFunc("PUT /api/v1/admin/refunds/{id}", service.adminDecideRefund)
	mux.HandleFunc("GET /api/v1/admin/invoices", service.adminInvoices)
	mux.HandleFunc("PUT /api/v1/admin/invoices/{id}", service.adminIssueInvoice)
	mux.HandleFunc("GET /api/v1/admin/resources", service.adminResources)
	mux.HandleFunc("POST /api/v1/admin/resources", service.adminCreateResource)
	mux.HandleFunc("PUT /api/v1/admin/resources/{id}", service.adminUpdateResource)
	mux.HandleFunc("DELETE /api/v1/admin/resources/{id}", service.adminDeleteResource)
}

func randomID(prefix string) string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(value)
}
func tokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (service *Service) decode(request *http.Request, target any) error {
	reader := io.Reader(request.Body)
	if service.MaxBodyBytes > 0 {
		reader = io.LimitReader(request.Body, service.MaxBodyBytes+1)
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON value")
	}
	return nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func (service *Service) principal(request *http.Request) (User, bool) {
	cookie, err := request.Cookie(sessionCookie)
	if err != nil {
		return User{}, false
	}
	session, ok := service.Store.GetActiveSession(tokenHash(cookie.Value), time.Now().UTC())
	if !ok {
		return User{}, false
	}
	return service.Store.GetUser(session.UserID)
}

func (service *Service) requireUser(writer http.ResponseWriter, request *http.Request) (User, bool) {
	user, ok := service.principal(request)
	if !ok || user.Status != "active" {
		writeError(writer, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return User{}, false
	}
	return user, true
}

func (service *Service) requireAdmin(writer http.ResponseWriter, request *http.Request) (User, bool) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return User{}, false
	}
	if user.Role != "admin" {
		writeError(writer, http.StatusForbidden, "ADMIN_REQUIRED", "需要管理员权限")
		return User{}, false
	}
	return user, true
}

func (service *Service) audit(userID, eventType, targetID string, details map[string]any) {
	_ = service.Store.AddAudit(AuditEvent{ID: randomID("audit_"), UserID: userID, Type: eventType, TargetID: targetID, Details: details, CreatedAt: time.Now().UTC()})
}

type credentialsInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	MFACode  string `json:"mfaCode,omitempty"`
}
type userView struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	MembershipLevel string     `json:"membershipLevel"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"createdAt"`
	VIPExpiresAt    *time.Time `json:"vipExpiresAt,omitempty"`
	Role            string     `json:"role"`
}

func publicUser(user User) userView {
	return userView{ID: user.ID, Email: user.Email, MembershipLevel: user.MembershipLevel, Status: user.Status, CreatedAt: user.CreatedAt, VIPExpiresAt: user.VIPExpiresAt, Role: user.Role}
}
func normalizeEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	if !strings.Contains(email, "@") || len(email) > 254 {
		return "", errors.New("请输入有效邮箱")
	}
	return email, nil
}
func (service *Service) createSession(writer http.ResponseWriter, request *http.Request, user User) error {
	token := randomID("ses_")
	now := time.Now().UTC()
	session := Session{ID: randomID("session_"), TokenHash: tokenHash(token), UserID: user.ID, UserAgentHash: tokenHash(request.UserAgent()), IPHash: tokenHash(request.RemoteAddr), ExpiresAt: now.Add(service.SessionTTL), CreatedAt: now}
	if err := service.Store.SaveSession(session); err != nil {
		return err
	}
	http.SetCookie(writer, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: request.TLS != nil, MaxAge: int(service.SessionTTL.Seconds())})
	return nil
}

func (service *Service) register(writer http.ResponseWriter, request *http.Request) {
	var body credentialsInput
	if err := service.decode(request, &body); err != nil {
		writeError(writer, 400, "INVALID_BODY", err.Error())
		return
	}
	email, err := normalizeEmail(body.Email)
	if err != nil {
		writeError(writer, 422, "INVALID_EMAIL", err.Error())
		return
	}
	if _, exists := service.Store.FindUserByEmail(email); exists {
		writeError(writer, 409, "EMAIL_EXISTS", "该邮箱已注册")
		return
	}
	passwordHash, err := hashPassword(body.Password)
	if err != nil {
		writeError(writer, 422, "INVALID_PASSWORD", err.Error())
		return
	}
	role := "user"
	if service.AdminEmail != "" && email == strings.ToLower(strings.TrimSpace(service.AdminEmail)) {
		role = "admin"
	}
	if role == "admin" && service.RequireAdminMFA && !verifyTOTP(service.AdminMFASecret, body.MFACode, time.Now().UTC()) {
		writeError(writer, 401, "MFA_REQUIRED", "管理员动态验证码无效")
		return
	}
	user := User{ID: randomID("usr_"), Email: email, PasswordHash: passwordHash, MembershipLevel: "free", Status: "active", CreatedAt: time.Now().UTC(), Role: role}
	if err := service.Store.SaveUser(user); err != nil {
		writeError(writer, 500, "STORE_ERROR", "无法创建账户")
		return
	}
	if err := service.createSession(writer, request, user); err != nil {
		writeError(writer, 500, "STORE_ERROR", "无法创建登录会话")
		return
	}
	service.audit(user.ID, "auth.register", user.ID, nil)
	writeJSON(writer, 201, map[string]any{"user": publicUser(user)})
}

func (service *Service) login(writer http.ResponseWriter, request *http.Request) {
	var body credentialsInput
	if err := service.decode(request, &body); err != nil {
		writeError(writer, 400, "INVALID_BODY", err.Error())
		return
	}
	email, err := normalizeEmail(body.Email)
	if err != nil {
		writeError(writer, 422, "INVALID_EMAIL", err.Error())
		return
	}
	user, ok := service.Store.FindUserByEmail(email)
	if !ok || !verifyPassword(user.PasswordHash, body.Password) {
		writeError(writer, 401, "INVALID_CREDENTIALS", "邮箱或密码错误")
		return
	}
	if user.Role == "admin" && service.RequireAdminMFA && !verifyTOTP(service.AdminMFASecret, body.MFACode, time.Now().UTC()) {
		writeError(writer, 401, "MFA_REQUIRED", "管理员动态验证码无效")
		return
	}
	if err := service.createSession(writer, request, user); err != nil {
		writeError(writer, 500, "STORE_ERROR", "无法创建登录会话")
		return
	}
	service.audit(user.ID, "auth.login", user.ID, nil)
	writeJSON(writer, 200, map[string]any{"user": publicUser(user)})
}

func (service *Service) logout(writer http.ResponseWriter, request *http.Request) {
	if cookie, err := request.Cookie(sessionCookie); err == nil {
		_ = service.Store.RevokeSessionByHash(tokenHash(cookie.Value), time.Now().UTC())
	}
	http.SetCookie(writer, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: request.TLS != nil, MaxAge: -1})
	writer.WriteHeader(http.StatusNoContent)
}

func (service *Service) listSessions(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	sessions := service.Store.ListSessions(user.ID, time.Now().UTC())
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].CreatedAt.After(sessions[j].CreatedAt) })
	writeJSON(writer, http.StatusOK, map[string]any{"items": sessions})
}

func (service *Service) revokeSession(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	if err := service.Store.RevokeSession(user.ID, request.PathValue("id"), time.Now().UTC()); err != nil {
		writeError(writer, http.StatusNotFound, "NOT_FOUND", "会话不存在")
		return
	}
	service.audit(user.ID, "auth.session_revoked", request.PathValue("id"), nil)
	writer.WriteHeader(http.StatusNoContent)
}

func (service *Service) me(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.principal(request)
	if !ok {
		writeError(writer, 401, "AUTH_REQUIRED", "未登录")
		return
	}
	writeJSON(writer, 200, map[string]any{"user": publicUser(user)})
}

type configInput struct {
	Name   string      `json:"name"`
	Config seal.Config `json:"config"`
}

func validateConfigInput(input *configInput) error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 100 {
		return errors.New("配置名称长度必须为 1–100")
	}
	input.Config.ApplyDefaults()
	return seal.Validate(input.Config)
}

func (service *Service) listConfigs(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	records := service.Store.ListConfigs(user.ID)
	sort.Slice(records, func(i, j int) bool { return records[i].UpdatedAt.After(records[j].UpdatedAt) })
	writeJSON(writer, 200, map[string]any{"items": records})
}
func (service *Service) createConfig(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	var input configInput
	if err := service.decode(request, &input); err != nil {
		writeError(writer, 400, "INVALID_BODY", err.Error())
		return
	}
	if err := validateConfigInput(&input); err != nil {
		writeError(writer, 422, "INVALID_CONFIG", err.Error())
		return
	}
	now := time.Now().UTC()
	record := SealConfigRecord{ID: randomID("cfg_"), UserID: user.ID, Name: input.Name, Config: input.Config, CreatedAt: now, UpdatedAt: now}
	if err := service.Store.SaveConfig(record); err != nil {
		writeError(writer, 500, "STORE_ERROR", "保存失败")
		return
	}
	service.audit(user.ID, "config.created", record.ID, nil)
	writeJSON(writer, 201, record)
}
func (service *Service) ownedConfig(writer http.ResponseWriter, request *http.Request) (User, SealConfigRecord, bool) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return User{}, SealConfigRecord{}, false
	}
	record, found := service.Store.GetConfig(request.PathValue("id"))
	if !found || record.UserID != user.ID {
		writeError(writer, 404, "NOT_FOUND", "配置不存在")
		return User{}, SealConfigRecord{}, false
	}
	return user, record, true
}
func (service *Service) getConfig(writer http.ResponseWriter, request *http.Request) {
	_, record, ok := service.ownedConfig(writer, request)
	if ok {
		writeJSON(writer, 200, record)
	}
}
func (service *Service) updateConfig(writer http.ResponseWriter, request *http.Request) {
	user, record, ok := service.ownedConfig(writer, request)
	if !ok {
		return
	}
	var input configInput
	if err := service.decode(request, &input); err != nil {
		writeError(writer, 400, "INVALID_BODY", err.Error())
		return
	}
	if err := validateConfigInput(&input); err != nil {
		writeError(writer, 422, "INVALID_CONFIG", err.Error())
		return
	}
	record.Name = input.Name
	record.Config = input.Config
	record.UpdatedAt = time.Now().UTC()
	if err := service.Store.SaveConfig(record); err != nil {
		writeError(writer, 500, "STORE_ERROR", "更新失败")
		return
	}
	service.audit(user.ID, "config.updated", record.ID, nil)
	writeJSON(writer, 200, record)
}
func (service *Service) deleteConfig(writer http.ResponseWriter, request *http.Request) {
	user, _, ok := service.ownedConfig(writer, request)
	if !ok {
		return
	}
	if err := service.Store.DeleteConfig(request.PathValue("id"), user.ID); err != nil {
		writeError(writer, 404, "NOT_FOUND", "配置不存在")
		return
	}
	service.audit(user.ID, "config.deleted", request.PathValue("id"), nil)
	writer.WriteHeader(204)
}

type generationInput struct {
	Config seal.Config `json:"config"`
	Format string      `json:"format"`
}

func generationKey(userID string, config seal.Config, format string, watermark bool) string {
	normalized, _ := json.Marshal(config)
	hash := sha256.New()
	hash.Write([]byte(userID))
	hash.Write(normalized)
	hash.Write([]byte(seal.RendererVersion))
	hash.Write([]byte(format))
	if watermark {
		hash.Write([]byte("watermarked"))
	} else {
		hash.Write([]byte("unlocked"))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (service *Service) createGeneration(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	var input generationInput
	if err := service.decode(request, &input); err != nil {
		writeError(writer, 400, "INVALID_BODY", err.Error())
		return
	}
	input.Config.ApplyDefaults()
	if err := seal.Validate(input.Config); err != nil {
		writeError(writer, 422, "INVALID_CONFIG", err.Error())
		return
	}
	if input.Format == "" {
		input.Format = "svg"
	}
	if input.Format != "svg" && input.Format != "png" {
		writeError(writer, 422, "FORMAT_UNAVAILABLE", "仅支持 SVG 或 PNG")
		return
	}
	if input.Format == "png" && service.Rasterizer == nil {
		writeError(writer, 503, "RASTERIZER_UNAVAILABLE", "PNG Worker 当前不可用")
		return
	}
	if err := service.validateResourceEntitlements(user, input.Config); err != nil {
		writeError(writer, 403, "RESOURCE_ENTITLEMENT_REQUIRED", err.Error())
		return
	}
	maxSize := 1200
	if user.MembershipLevel == "vip" {
		maxSize = 5000
	}
	if input.Config.Canvas.ExportWidth > maxSize {
		input.Config.Canvas.ExportWidth = maxSize
	}
	watermark := user.MembershipLevel != "vip"
	key := generationKey(user.ID, input.Config, input.Format, watermark)
	if existing, found := service.Store.FindGenerationByKey(user.ID, key); found {
		writeJSON(writer, 202, existing)
		return
	}
	generation := Generation{ID: randomID("gen_"), UserID: user.ID, Config: input.Config, GenerationKey: key, RendererVersion: seal.RendererVersion, Format: input.Format, Status: "queued", Watermark: watermark, CreatedAt: time.Now().UTC()}
	if err := service.Store.SaveGeneration(generation); err != nil {
		writeError(writer, 500, "STORE_ERROR", "任务创建失败")
		return
	}
	service.audit(user.ID, "generation.queued", generation.ID, map[string]any{"format": input.Format, "watermark": generation.Watermark})
	if err := service.queue.Enqueue(request.Context(), generation.ID); err != nil {
		generation.Status = "failed"
		generation.FailureReason = "queue unavailable"
		_ = service.Store.SaveGeneration(generation)
		writeError(writer, 503, "QUEUE_FULL", "生成队列繁忙")
		return
	}
	writeJSON(writer, 202, generation)
}

func (service *Service) listGenerations(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	items := service.Store.ListGenerations(user.ID)
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	writeJSON(writer, 200, map[string]any{"items": items})
}
func (service *Service) ownedGeneration(writer http.ResponseWriter, request *http.Request) (User, Generation, bool) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return User{}, Generation{}, false
	}
	value, found := service.Store.GetGeneration(request.PathValue("id"))
	if !found || value.UserID != user.ID {
		writeError(writer, 404, "NOT_FOUND", "生成任务不存在")
		return User{}, Generation{}, false
	}
	return user, value, true
}
func (service *Service) getGeneration(writer http.ResponseWriter, request *http.Request) {
	_, value, ok := service.ownedGeneration(writer, request)
	if ok {
		writeJSON(writer, 200, value)
	}
}

func (service *Service) requeueGeneration(writer http.ResponseWriter, generation Generation, actorID, eventType string) {
	if generation.Status != "failed" {
		writeError(writer, 409, "RETRY_UNAVAILABLE", "仅失败任务可以重试")
		return
	}
	generation.Status, generation.FailureReason, generation.FileKey = "queued", "", ""
	generation.StartedAt, generation.FinishedAt = nil, nil
	if err := service.Store.SaveGeneration(generation); err != nil {
		writeError(writer, 500, "STORE_ERROR", "任务重试保存失败")
		return
	}
	if err := service.queue.Enqueue(context.Background(), generation.ID); err != nil {
		generation.Status = "failed"
		generation.FailureReason = "queue unavailable"
		_ = service.Store.SaveGeneration(generation)
		writeError(writer, 503, "QUEUE_FULL", "生成队列繁忙")
		return
	}
	service.audit(actorID, eventType, generation.ID, nil)
	writeJSON(writer, http.StatusAccepted, generation)
}

func (service *Service) retryGeneration(writer http.ResponseWriter, request *http.Request) {
	user, generation, ok := service.ownedGeneration(writer, request)
	if !ok {
		return
	}
	service.requeueGeneration(writer, generation, user.ID, "generation.retried")
}

func (service *Service) worker() {
	for {
		id, err := service.queue.Dequeue(context.Background())
		if err != nil {
			service.Logger.Error("generation queue read failed", "error", err)
			continue
		}
		service.renderGeneration(id)
	}
}
func (service *Service) renderGeneration(id string) {
	job, ok := service.Store.GetGeneration(id)
	if !ok || job.Status != "queued" {
		return
	}
	now := time.Now().UTC()
	job.Status = "rendering"
	job.StartedAt = &now
	if err := service.Store.SaveGeneration(job); err != nil {
		return
	}
	renderConfig, err := service.hydrateAssets(job.UserID, job.Config)
	var result []byte
	if err == nil {
		result, err = service.Renderer.SVG(renderConfig, job.Watermark)
	}
	contentType := "image/svg+xml; charset=utf-8"
	extension := "svg"
	if err == nil && job.Format == "png" {
		result, err = service.Rasterizer.PNG(context.Background(), result, job.Config.Canvas.ExportWidth)
		contentType = "image/png"
		extension = "png"
	}
	finished := time.Now().UTC()
	job.FinishedAt = &finished
	if err == nil {
		job.FileKey = "generations/" + job.ID + "." + extension
		err = service.Objects.Put(context.Background(), job.FileKey, contentType, bytes.NewReader(result))
	}
	if err != nil {
		job.Status = "failed"
		job.FailureReason = "render failed"
		service.Logger.Error("generation failed", "generation_id", job.ID, "error", err)
	} else {
		job.Status = "succeeded"
	}
	_ = service.Store.SaveGeneration(job)
	service.audit(job.UserID, "generation."+job.Status, job.ID, nil)
}

func (service *Service) hydrateAssets(userID string, config seal.Config) (seal.Config, error) {
	config.Layers = append([]seal.Layer(nil), config.Layers...)
	for index := range config.Layers {
		layer := &config.Layers[index]
		if layer.Kind != "centerImage" || layer.AssetID == "" {
			continue
		}
		asset, found := service.Store.GetAsset(layer.AssetID)
		if !found || asset.UserID != userID {
			return seal.Config{}, errors.New("center image asset is unavailable")
		}
		body, _, err := service.Objects.Open(context.Background(), asset.FileKey)
		if err != nil {
			return seal.Config{}, err
		}
		content, readErr := io.ReadAll(io.LimitReader(body, 6<<20))
		closeErr := body.Close()
		if readErr != nil {
			return seal.Config{}, readErr
		}
		if closeErr != nil {
			return seal.Config{}, closeErr
		}
		layer.AssetData = "data:image/png;base64," + base64.StdEncoding.EncodeToString(content)
	}
	return config, nil
}

func (service *Service) issueDownloadToken(writer http.ResponseWriter, request *http.Request) {
	user, generation, ok := service.ownedGeneration(writer, request)
	if !ok {
		return
	}
	if generation.Status != "succeeded" || generation.FileKey == "" {
		writeError(writer, 409, "NOT_READY", "文件尚未生成完成")
		return
	}
	if generation.AccessRevoked {
		writeError(writer, 403, "ENTITLEMENT_REVOKED", "该文件权益已因退款失效")
		return
	}
	raw := randomID("dlt_")
	record := DownloadToken{Hash: tokenHash(raw), UserID: user.ID, GenerationID: generation.ID, ExpiresAt: time.Now().UTC().Add(service.TokenTTL)}
	if err := service.Store.SaveDownloadToken(record); err != nil {
		writeError(writer, 500, "STORE_ERROR", "令牌签发失败")
		return
	}
	service.audit(user.ID, "download.token_issued", generation.ID, map[string]any{"expiresAt": record.ExpiresAt})
	writeJSON(writer, 200, map[string]any{"token": raw, "expiresAt": record.ExpiresAt, "downloadUrl": "/api/v1/downloads/" + raw})
}
func (service *Service) download(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	record, err := service.Store.ConsumeDownloadToken(tokenHash(request.PathValue("token")), user.ID, time.Now().UTC())
	if err != nil {
		service.audit(user.ID, "download.token_rejected", "", nil)
		writeError(writer, 410, "TOKEN_GONE", "下载令牌无效、过期或已消费")
		return
	}
	generation, found := service.Store.GetGeneration(record.GenerationID)
	if !found || generation.UserID != user.ID || generation.AccessRevoked {
		writeError(writer, 404, "NOT_FOUND", "文件不存在")
		return
	}
	body, contentType, err := service.Objects.Open(request.Context(), generation.FileKey)
	if err != nil {
		writeError(writer, 404, "NOT_FOUND", "文件不存在")
		return
	}
	defer body.Close()
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="seal.%s"`, generation.Format))
	writer.Header().Set("Cache-Control", "private, no-store")
	service.audit(user.ID, "download.consumed", generation.ID, nil)
	_, _ = io.Copy(writer, body)
}

type orderInput struct {
	Product      string `json:"product"`
	GenerationID string `json:"generationId,omitempty"`
}

var productPrices = map[string]int{"single_export": 199, "vip_monthly": 2999}

func (service *Service) validateResourceEntitlements(user User, config seal.Config) error {
	for _, layer := range config.Layers {
		if layer.Kind == "border" || layer.Kind == "innerRing" || layer.Kind == "centerImage" {
			continue
		}
		if layer.FontID == "" || layer.FontID == "system-serif" || layer.FontID == "system-sans" {
			continue
		}
		allowed := false
		for _, resource := range service.Store.ListResources("font") {
			if resource.Key != layer.FontID || resource.Status != "active" || !resource.ExportAllowed {
				continue
			}
			if resource.VIPOnly && user.MembershipLevel != "vip" {
				return errors.New("该字体仅限 VIP 导出")
			}
			allowed = true
			break
		}
		if !allowed {
			return errors.New("字体未安装、未授权或已停用")
		}
	}
	for _, resource := range service.Store.ListResources("texture") {
		if resource.Key == config.Texture.Type && resource.Status == "active" {
			if !resource.ExportAllowed {
				return errors.New("该纹理不允许导出")
			}
			if resource.VIPOnly && user.MembershipLevel != "vip" {
				return errors.New("该纹理仅限 VIP 导出")
			}
		}
	}
	return nil
}

func (service *Service) createOrder(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	var input orderInput
	if err := service.decode(request, &input); err != nil {
		writeError(writer, 400, "INVALID_BODY", err.Error())
		return
	}
	amount, valid := productPrices[input.Product]
	if !valid {
		writeError(writer, 422, "INVALID_PRODUCT", "不支持的商品")
		return
	}
	if input.Product == "single_export" {
		generation, found := service.Store.GetGeneration(input.GenerationID)
		if !found || generation.UserID != user.ID {
			writeError(writer, 404, "NOT_FOUND", "生成任务不存在")
			return
		}
		if err := service.validateResourceEntitlements(user, generation.Config); err != nil {
			writeError(writer, 403, "RESOURCE_ENTITLEMENT_REQUIRED", err.Error())
			return
		}
	}
	now := time.Now().UTC()
	order := Order{ID: randomID("ord_"), OrderNo: fmt.Sprintf("SP%s%s", now.Format("20060102150405"), randomID("")[:8]), UserID: user.ID, GenerationID: input.GenerationID, Product: input.Product, AmountCents: amount, Status: "pending", CreatedAt: now}
	if err := service.Store.SaveOrder(order); err != nil {
		writeError(writer, 500, "STORE_ERROR", "订单创建失败")
		return
	}
	service.audit(user.ID, "order.created", order.ID, map[string]any{"product": order.Product, "amountCents": order.AmountCents})
	writeJSON(writer, 201, order)
}

func (service *Service) listOrders(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	items := service.Store.ListOrders(user.ID)
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	writeJSON(writer, 200, map[string]any{"items": items})
}

func (service *Service) ownedOrder(writer http.ResponseWriter, request *http.Request) (User, Order, bool) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return User{}, Order{}, false
	}
	order, found := service.Store.GetOrder(request.PathValue("id"))
	if !found || order.UserID != user.ID {
		writeError(writer, 404, "NOT_FOUND", "订单不存在")
		return User{}, Order{}, false
	}
	return user, order, true
}

func (service *Service) getOrder(writer http.ResponseWriter, request *http.Request) {
	_, order, ok := service.ownedOrder(writer, request)
	if ok {
		writeJSON(writer, 200, order)
	}
}

type refundInput struct {
	Reason string `json:"reason"`
}

func (service *Service) requestRefund(writer http.ResponseWriter, request *http.Request) {
	user, order, ok := service.ownedOrder(writer, request)
	if !ok {
		return
	}
	var input refundInput
	if err := service.decode(request, &input); err != nil {
		writeError(writer, 400, "INVALID_BODY", err.Error())
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if len([]rune(input.Reason)) < 2 || len([]rune(input.Reason)) > 300 {
		writeError(writer, 422, "INVALID_REASON", "退款原因长度必须为 2–300")
		return
	}
	now := time.Now().UTC()
	refund := RefundRequest{ID: randomID("refund_"), OrderID: order.ID, UserID: user.ID, Reason: input.Reason, Status: "pending", CreatedAt: now, UpdatedAt: now}
	if err := service.Store.SaveRefund(refund); err != nil {
		writeError(writer, 409, "REFUND_UNAVAILABLE", "订单当前不能申请退款或已有退款申请")
		return
	}
	service.audit(user.ID, "refund.requested", refund.ID, map[string]any{"orderId": order.ID})
	writeJSON(writer, http.StatusCreated, refund)
}

func (service *Service) listRefunds(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	items := service.Store.ListRefunds(user.ID)
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	writeJSON(writer, http.StatusOK, map[string]any{"items": items})
}

type invoiceInput struct {
	Title     string `json:"title"`
	TaxNumber string `json:"taxNumber,omitempty"`
	Email     string `json:"email"`
}

func (service *Service) requestInvoice(writer http.ResponseWriter, request *http.Request) {
	user, order, ok := service.ownedOrder(writer, request)
	if !ok {
		return
	}
	var input invoiceInput
	if err := service.decode(request, &input); err != nil {
		writeError(writer, 400, "INVALID_BODY", err.Error())
		return
	}
	input.Title, input.TaxNumber = strings.TrimSpace(input.Title), strings.TrimSpace(input.TaxNumber)
	email, err := normalizeEmail(input.Email)
	if input.Title == "" || len([]rune(input.Title)) > 120 || len(input.TaxNumber) > 30 || err != nil {
		writeError(writer, 422, "INVALID_INVOICE", "发票抬头、税号或邮箱无效")
		return
	}
	now := time.Now().UTC()
	invoice := Invoice{ID: randomID("invoice_"), OrderID: order.ID, UserID: user.ID, Title: input.Title, TaxNumber: input.TaxNumber, Email: email, Status: "requested", CreatedAt: now, UpdatedAt: now}
	if err := service.Store.SaveInvoice(invoice); err != nil {
		writeError(writer, 409, "INVOICE_UNAVAILABLE", "订单当前不能开票或已提交过发票")
		return
	}
	service.audit(user.ID, "invoice.requested", invoice.ID, map[string]any{"orderId": order.ID})
	writeJSON(writer, http.StatusCreated, invoice)
}

func (service *Service) listInvoices(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	items := service.Store.ListInvoices(user.ID)
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	writeJSON(writer, http.StatusOK, map[string]any{"items": items})
}

func (service *Service) settleOrder(orderNo string, amountCents int, channel, transactionID string) (Order, error) {
	order, generationID, requeue, err := service.Store.PayOrder(orderNo, amountCents, channel, transactionID, time.Now().UTC())
	if err != nil {
		return Order{}, err
	}
	if requeue {
		if queueErr := service.queue.Enqueue(context.Background(), generationID); queueErr != nil {
			if generation, found := service.Store.GetGeneration(generationID); found {
				generation.Status = "failed"
				generation.FailureReason = "queue unavailable after payment"
				_ = service.Store.SaveGeneration(generation)
			}
		}
	}
	service.audit(order.UserID, "order.paid", order.ID, map[string]any{"channel": channel, "transactionId": transactionID})
	return order, nil
}

func (service *Service) simulatePayment(writer http.ResponseWriter, request *http.Request) {
	if !service.AllowPaymentSimulation {
		writeError(writer, 404, "NOT_FOUND", "接口不存在")
		return
	}
	_, order, ok := service.ownedOrder(writer, request)
	if !ok {
		return
	}
	paid, err := service.settleOrder(order.OrderNo, order.AmountCents, "simulation", randomID("txn_"))
	if err != nil {
		writeError(writer, 409, "PAYMENT_FAILED", err.Error())
		return
	}
	writeJSON(writer, 200, paid)
}

type paymentCallbackInput struct {
	OrderNo       string `json:"orderNo"`
	AmountCents   int    `json:"amountCents"`
	Status        string `json:"status"`
	TransactionID string `json:"transactionId"`
}

func (service *Service) paymentCallback(writer http.ResponseWriter, request *http.Request) {
	content, err := io.ReadAll(io.LimitReader(request.Body, service.MaxBodyBytes+1))
	if err != nil || int64(len(content)) > service.MaxBodyBytes {
		writeError(writer, 400, "INVALID_BODY", "回调内容无效")
		return
	}
	provided, err := hex.DecodeString(request.Header.Get("X-Payment-Signature"))
	if err != nil {
		service.audit("", "payment.callback_rejected", "", map[string]any{"reason": "signature_format"})
		writeError(writer, 401, "INVALID_SIGNATURE", "支付签名无效")
		return
	}
	mac := hmac.New(sha256.New, []byte(service.PaymentSecret))
	_, _ = mac.Write(content)
	if !hmac.Equal(provided, mac.Sum(nil)) {
		service.audit("", "payment.callback_rejected", "", map[string]any{"reason": "signature"})
		writeError(writer, 401, "INVALID_SIGNATURE", "支付签名无效")
		return
	}
	var input paymentCallbackInput
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, 400, "INVALID_BODY", "回调内容无效")
		return
	}
	if input.Status != "paid" {
		writeError(writer, 422, "INVALID_STATUS", "只接受已支付状态")
		return
	}
	order, err := service.settleOrder(input.OrderNo, input.AmountCents, "callback", input.TransactionID)
	if errors.Is(err, ErrAmountMismatch) {
		service.audit("", "payment.callback_rejected", input.OrderNo, map[string]any{"reason": "amount"})
		writeError(writer, 409, "AMOUNT_MISMATCH", "支付金额不匹配")
		return
	}
	if errors.Is(err, ErrNotFound) {
		writeError(writer, 404, "NOT_FOUND", "订单不存在")
		return
	}
	if err != nil {
		writeError(writer, 500, "PAYMENT_FAILED", "支付处理失败")
		return
	}
	writeJSON(writer, 200, map[string]any{"processed": true, "order": order})
}

func (service *Service) uploadImage(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	if service.ImageProcessor == nil {
		writeError(writer, 503, "IMAGE_PROCESSOR_UNAVAILABLE", "图片处理服务当前不可用")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 6<<20)
	if err := request.ParseMultipartForm(6 << 20); err != nil {
		writeError(writer, 400, "INVALID_UPLOAD", "上传内容无效或超过限制")
		return
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		writeError(writer, 400, "FILE_REQUIRED", "请选择图片文件")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, (5<<20)+1))
	if err != nil || len(content) == 0 || len(content) > 5<<20 {
		writeError(writer, 422, "FILE_TOO_LARGE", "图片不能超过 5MB")
		return
	}
	declared := strings.Split(header.Header.Get("Content-Type"), ";")[0]
	detected := http.DetectContentType(content)
	allowed := map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true}
	if !allowed[declared] || !allowed[detected] || declared != detected {
		writeError(writer, 422, "UNSUPPORTED_IMAGE", "仅支持真实的 PNG、JPEG 或 WebP 图片")
		return
	}
	png, width, height, err := service.ImageProcessor.Reencode(request.Context(), content, detected)
	if err != nil {
		writeError(writer, 422, "IMAGE_DECODE_FAILED", "图片无法安全解码")
		return
	}
	sum := sha256.Sum256(png)
	assetID := randomID("ast_")
	fileKey := "assets/" + user.ID + "/" + assetID + ".png"
	if err := service.Objects.Put(request.Context(), fileKey, "image/png", bytes.NewReader(png)); err != nil {
		writeError(writer, 500, "STORE_ERROR", "图片保存失败")
		return
	}
	asset := Asset{ID: assetID, UserID: user.ID, Mime: "image/png", FileKey: fileKey, Width: width, Height: height, SHA256: hex.EncodeToString(sum[:]), CreatedAt: time.Now().UTC()}
	if err := service.Store.SaveAsset(asset); err != nil {
		_ = service.Objects.Delete(request.Context(), fileKey)
		writeError(writer, 500, "STORE_ERROR", "图片记录保存失败")
		return
	}
	service.audit(user.ID, "asset.created", asset.ID, map[string]any{"width": width, "height": height, "sourceMime": detected})
	writeJSON(writer, 201, map[string]any{"asset": asset, "url": "/api/v1/assets/" + asset.ID})
}

func (service *Service) getAsset(writer http.ResponseWriter, request *http.Request) {
	user, ok := service.requireUser(writer, request)
	if !ok {
		return
	}
	asset, found := service.Store.GetAsset(request.PathValue("id"))
	if !found || asset.UserID != user.ID {
		writeError(writer, 404, "NOT_FOUND", "图片不存在")
		return
	}
	body, contentType, err := service.Objects.Open(request.Context(), asset.FileKey)
	if err != nil {
		writeError(writer, 404, "NOT_FOUND", "图片不存在")
		return
	}
	defer body.Close()
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(writer, body)
}

func (service *Service) listResources(writer http.ResponseWriter, request *http.Request) {
	user, _ := service.principal(request)
	resourceType := request.URL.Query().Get("type")
	if resourceType != "" && resourceType != "template" && resourceType != "font" && resourceType != "texture" {
		writeError(writer, 422, "INVALID_RESOURCE_TYPE", "资源类型无效")
		return
	}
	items := service.Store.ListResources(resourceType)
	filtered := make([]Resource, 0, len(items))
	for _, item := range items {
		if item.Status != "active" {
			continue
		}
		if item.VIPOnly && user.MembershipLevel != "vip" {
			item.Config = nil
		}
		filtered = append(filtered, item)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name < filtered[j].Name })
	writeJSON(writer, http.StatusOK, map[string]any{"items": filtered})
}

func (service *Service) cspReport(writer http.ResponseWriter, request *http.Request) {
	content, err := io.ReadAll(io.LimitReader(request.Body, (64<<10)+1))
	if err != nil || len(content) > 64<<10 {
		writeError(writer, 413, "REPORT_TOO_LARGE", "报告超过限制")
		return
	}
	var report any
	if err := json.Unmarshal(content, &report); err != nil {
		writeError(writer, 400, "INVALID_REPORT", "报告格式无效")
		return
	}
	user, _ := service.principal(request)
	service.audit(user.ID, "security.csp_report", "", map[string]any{"bytes": len(content), "contentType": request.Header.Get("Content-Type")})
	writer.WriteHeader(http.StatusNoContent)
}

func (service *Service) adminUsers(writer http.ResponseWriter, request *http.Request) {
	if _, ok := service.requireAdmin(writer, request); !ok {
		return
	}
	users := service.Store.ListUsers()
	sort.Slice(users, func(i, j int) bool { return users[i].CreatedAt.After(users[j].CreatedAt) })
	items := make([]userView, 0, len(users))
	for _, user := range users {
		items = append(items, publicUser(user))
	}
	writeJSON(writer, 200, map[string]any{"items": items})
}

type adminUserInput struct {
	Status          string `json:"status,omitempty"`
	MembershipLevel string `json:"membershipLevel,omitempty"`
}

func (service *Service) adminUpdateUser(writer http.ResponseWriter, request *http.Request) {
	admin, ok := service.requireAdmin(writer, request)
	if !ok {
		return
	}
	target, found := service.Store.GetUser(request.PathValue("id"))
	if !found {
		writeError(writer, 404, "NOT_FOUND", "用户不存在")
		return
	}
	var input adminUserInput
	if err := service.decode(request, &input); err != nil {
		writeError(writer, 400, "INVALID_BODY", err.Error())
		return
	}
	if input.Status != "" && input.Status != "active" && input.Status != "banned" {
		writeError(writer, 422, "INVALID_STATUS", "用户状态无效")
		return
	}
	if input.MembershipLevel != "" && input.MembershipLevel != "free" && input.MembershipLevel != "vip" {
		writeError(writer, 422, "INVALID_MEMBERSHIP", "会员等级无效")
		return
	}
	if target.ID == admin.ID && input.Status == "banned" {
		writeError(writer, 409, "SELF_BAN_FORBIDDEN", "不能封禁当前管理员")
		return
	}
	if input.Status != "" {
		target.Status = input.Status
	}
	if input.MembershipLevel != "" {
		target.MembershipLevel = input.MembershipLevel
		if input.MembershipLevel == "vip" {
			expires := time.Now().UTC().Add(30 * 24 * time.Hour)
			target.VIPExpiresAt = &expires
		} else {
			target.VIPExpiresAt = nil
		}
	}
	if err := service.Store.SaveUser(target); err != nil {
		writeError(writer, 500, "STORE_ERROR", "用户更新失败")
		return
	}
	service.audit(admin.ID, "admin.user_updated", target.ID, map[string]any{"status": input.Status, "membershipLevel": input.MembershipLevel})
	writeJSON(writer, 200, map[string]any{"user": publicUser(target)})
}

func (service *Service) adminOrders(writer http.ResponseWriter, request *http.Request) {
	if _, ok := service.requireAdmin(writer, request); !ok {
		return
	}
	items := service.Store.AllOrders()
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	writeJSON(writer, 200, map[string]any{"items": items})
}
func (service *Service) adminGenerations(writer http.ResponseWriter, request *http.Request) {
	if _, ok := service.requireAdmin(writer, request); !ok {
		return
	}
	items := service.Store.AllGenerations()
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	writeJSON(writer, 200, map[string]any{"items": items})
}
func (service *Service) adminRetryGeneration(writer http.ResponseWriter, request *http.Request) {
	admin, ok := service.requireAdmin(writer, request)
	if !ok {
		return
	}
	generation, found := service.Store.GetGeneration(request.PathValue("id"))
	if !found {
		writeError(writer, 404, "NOT_FOUND", "生成任务不存在")
		return
	}
	service.requeueGeneration(writer, generation, admin.ID, "admin.generation_retried")
}
func (service *Service) adminAudit(writer http.ResponseWriter, request *http.Request) {
	if _, ok := service.requireAdmin(writer, request); !ok {
		return
	}
	writeJSON(writer, 200, map[string]any{"items": service.Store.ListAudit(500)})
}

func (service *Service) adminRefunds(writer http.ResponseWriter, request *http.Request) {
	if _, ok := service.requireAdmin(writer, request); !ok {
		return
	}
	items := service.Store.ListRefunds("")
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	writeJSON(writer, http.StatusOK, map[string]any{"items": items})
}

type refundDecisionInput struct {
	Status string `json:"status"`
}

func (service *Service) adminDecideRefund(writer http.ResponseWriter, request *http.Request) {
	admin, ok := service.requireAdmin(writer, request)
	if !ok {
		return
	}
	var input refundDecisionInput
	if err := service.decode(request, &input); err != nil {
		writeError(writer, 400, "INVALID_BODY", err.Error())
		return
	}
	refund, err := service.Store.DecideRefund(request.PathValue("id"), input.Status, time.Now().UTC())
	if err != nil {
		writeError(writer, 409, "REFUND_DECISION_FAILED", "退款申请不存在或已处理")
		return
	}
	service.audit(admin.ID, "admin.refund_"+input.Status, refund.ID, map[string]any{"orderId": refund.OrderID})
	writeJSON(writer, http.StatusOK, refund)
}

func (service *Service) adminInvoices(writer http.ResponseWriter, request *http.Request) {
	if _, ok := service.requireAdmin(writer, request); !ok {
		return
	}
	items := service.Store.ListInvoices("")
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	writeJSON(writer, http.StatusOK, map[string]any{"items": items})
}

func (service *Service) adminIssueInvoice(writer http.ResponseWriter, request *http.Request) {
	admin, ok := service.requireAdmin(writer, request)
	if !ok {
		return
	}
	invoice, err := service.Store.MarkInvoiceIssued(request.PathValue("id"), time.Now().UTC())
	if err != nil {
		writeError(writer, 404, "NOT_FOUND", "发票申请不存在")
		return
	}
	service.audit(admin.ID, "admin.invoice_issued", invoice.ID, map[string]any{"orderId": invoice.OrderID})
	writeJSON(writer, http.StatusOK, invoice)
}

type resourceInput struct {
	Type          string       `json:"type"`
	Key           string       `json:"key"`
	Name          string       `json:"name"`
	Version       string       `json:"version"`
	LicenseScope  string       `json:"licenseScope"`
	VIPOnly       bool         `json:"vipOnly"`
	ExportAllowed bool         `json:"exportAllowed"`
	Status        string       `json:"status"`
	Config        *seal.Config `json:"config,omitempty"`
}

func validateResourceInput(input *resourceInput) error {
	input.Key, input.Name, input.Version, input.LicenseScope = strings.TrimSpace(input.Key), strings.TrimSpace(input.Name), strings.TrimSpace(input.Version), strings.TrimSpace(input.LicenseScope)
	if input.Type != "template" && input.Type != "font" && input.Type != "texture" {
		return errors.New("资源类型无效")
	}
	validKey := regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(input.Key)
	if !validKey || len(input.Key) > 100 || input.Name == "" || len([]rune(input.Name)) > 100 || input.Version == "" || len(input.Version) > 40 {
		return errors.New("资源标识、名称或版本无效")
	}
	if input.Status != "active" && input.Status != "draft" && input.Status != "disabled" {
		return errors.New("资源状态无效")
	}
	if input.Config != nil {
		input.Config.ApplyDefaults()
		if err := seal.Validate(*input.Config); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) adminResources(writer http.ResponseWriter, request *http.Request) {
	if _, ok := service.requireAdmin(writer, request); !ok {
		return
	}
	items := service.Store.ListResources(request.URL.Query().Get("type"))
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	writeJSON(writer, http.StatusOK, map[string]any{"items": items})
}

func (service *Service) saveAdminResource(writer http.ResponseWriter, request *http.Request, existing *Resource) {
	admin, ok := service.requireAdmin(writer, request)
	if !ok {
		return
	}
	var input resourceInput
	if err := service.decode(request, &input); err != nil {
		writeError(writer, 400, "INVALID_BODY", err.Error())
		return
	}
	if err := validateResourceInput(&input); err != nil {
		writeError(writer, 422, "INVALID_RESOURCE", err.Error())
		return
	}
	now := time.Now().UTC()
	resource := Resource{ID: randomID("resource_"), CreatedAt: now}
	status := http.StatusCreated
	event := "admin.resource_created"
	if existing != nil {
		resource, status, event = *existing, http.StatusOK, "admin.resource_updated"
	}
	resource.Type, resource.Key, resource.Name, resource.Version, resource.LicenseScope = input.Type, input.Key, input.Name, input.Version, input.LicenseScope
	resource.VIPOnly, resource.ExportAllowed, resource.Status, resource.Config, resource.UpdatedAt = input.VIPOnly, input.ExportAllowed, input.Status, input.Config, now
	if err := service.Store.SaveResource(resource); err != nil {
		writeError(writer, 409, "RESOURCE_CONFLICT", "相同类型、标识和版本的资源已存在")
		return
	}
	service.audit(admin.ID, event, resource.ID, map[string]any{"type": resource.Type, "key": resource.Key, "version": resource.Version})
	writeJSON(writer, status, resource)
}

func (service *Service) adminCreateResource(writer http.ResponseWriter, request *http.Request) {
	service.saveAdminResource(writer, request, nil)
}

func (service *Service) adminUpdateResource(writer http.ResponseWriter, request *http.Request) {
	if _, ok := service.requireAdmin(writer, request); !ok {
		return
	}
	existing, ok := service.Store.GetResource(request.PathValue("id"))
	if !ok {
		writeError(writer, 404, "NOT_FOUND", "资源不存在")
		return
	}
	service.saveAdminResource(writer, request, &existing)
}

func (service *Service) adminDeleteResource(writer http.ResponseWriter, request *http.Request) {
	admin, ok := service.requireAdmin(writer, request)
	if !ok {
		return
	}
	if err := service.Store.DeleteResource(request.PathValue("id")); err != nil {
		writeError(writer, 404, "NOT_FOUND", "资源不存在")
		return
	}
	service.audit(admin.ID, "admin.resource_deleted", request.PathValue("id"), nil)
	writer.WriteHeader(http.StatusNoContent)
}

func (service *Service) WriteMetrics(writer io.Writer) {
	statuses := map[string]int{"queued": 0, "rendering": 0, "succeeded": 0, "failed": 0}
	var duration time.Duration
	var durationCount int
	for _, generation := range service.Store.AllGenerations() {
		statuses[generation.Status]++
		if generation.StartedAt != nil && generation.FinishedAt != nil {
			duration += generation.FinishedAt.Sub(*generation.StartedAt)
			durationCount++
		}
	}
	for _, status := range []string{"queued", "rendering", "succeeded", "failed"} {
		_, _ = fmt.Fprintf(writer, "seal_generation_jobs{status=\"%s\"} %d\n", status, statuses[status])
	}
	depth, err := service.queue.Depth(context.Background())
	if err == nil {
		_, _ = fmt.Fprintf(writer, "seal_generation_queue_depth %d\n", depth)
	}
	_, _ = fmt.Fprintf(writer, "seal_generation_duration_seconds_count %d\n", durationCount)
	_, _ = fmt.Fprintf(writer, "seal_generation_duration_seconds_sum %.6f\n", duration.Seconds())
	var tokenFailures, paymentFailures int
	for _, event := range service.Store.ListAudit(5000) {
		switch event.Type {
		case "download.token_rejected":
			tokenFailures++
		case "payment.callback_rejected":
			paymentFailures++
		}
	}
	_, _ = fmt.Fprintf(writer, "seal_download_token_failures_total %d\n", tokenFailures)
	_, _ = fmt.Fprintf(writer, "seal_payment_callback_failures_total %d\n", paymentFailures)
}
