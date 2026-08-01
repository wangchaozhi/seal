package platform

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sealplatform/api/internal/seal"
	"sealplatform/api/internal/storage"
)

type fakeRasterizer struct{}

func (fakeRasterizer) PNG(_ context.Context, _ []byte, _ int) ([]byte, error) {
	return []byte{137, 80, 78, 71, 13, 10, 26, 10, 1, 2, 3}, nil
}
func (fakeRasterizer) Reencode(_ context.Context, _ []byte, _ string) ([]byte, int, int, error) {
	content, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	return content, 1, 1, nil
}

func newTestServer(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	root := t.TempDir()
	state, err := NewStateStore(filepath.Join(root, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	objects, err := storage.NewLocal(filepath.Join(root, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(state, objects, slog.New(slog.NewTextHandler(io.Discard, nil)), 1<<20)
	service.PaymentSecret = "test-payment-secret"
	service.AllowPaymentSimulation = true
	service.Rasterizer = fakeRasterizer{}
	service.ImageProcessor = fakeRasterizer{}
	service.AdminEmail = "admin@example.com"
	mux := http.NewServeMux()
	service.Register(mux)
	server := httptest.NewServer(mux)
	jar, _ := cookiejar.New(nil)
	return server, &http.Client{Jar: jar}
}

func TestSecureImageUploadAndOwnedRender(t *testing.T) {
	server, client := newTestServer(t)
	defer server.Close()
	_, _ = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]any{"email": "asset@example.com", "password": "testing-password-123"})
	source, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	var form bytes.Buffer
	multipartWriter := multipart.NewWriter(&form)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="center.png"`)
	header.Set("Content-Type", "image/png")
	part, err := multipartWriter.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(source)
	_ = multipartWriter.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/uploads/images", bytes.NewReader(form.Bytes()))
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("upload status %d: %s", response.StatusCode, content)
	}
	var uploaded struct {
		Asset Asset `json:"asset"`
	}
	if err := json.Unmarshal(content, &uploaded); err != nil {
		t.Fatal(err)
	}
	config := fixtureConfig(t)
	for index := range config.Layers {
		if config.Layers[index].ID == "center" {
			config.Layers[index].Kind = "centerImage"
			config.Layers[index].AssetID = uploaded.Asset.ID
		}
	}
	response, content = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/generations", map[string]any{"format": "svg", "config": config})
	var generation Generation
	_ = json.Unmarshal(content, &generation)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response, content = requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/generations/"+generation.ID, nil)
		_ = json.Unmarshal(content, &generation)
		if generation.Status == "succeeded" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if generation.Status != "succeeded" {
		t.Fatalf("asset generation failed: %+v", generation)
	}
	response, content = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/generations/"+generation.ID+"/download-token", map[string]any{})
	var token struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(content, &token)
	response, content = requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/downloads/"+token.Token, nil)
	if response.StatusCode != http.StatusOK || !bytes.Contains(content, []byte(`data:image/png;base64,`)) {
		t.Fatalf("render did not embed owned image: %d", response.StatusCode)
	}
}

func TestPNGGenerationUsesRasterizer(t *testing.T) {
	server, client := newTestServer(t)
	defer server.Close()
	_, _ = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]any{"email": "png@example.com", "password": "testing-password-123"})
	response, content := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/generations", map[string]any{"format": "png", "config": fixtureConfig(t)})
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("PNG generation status %d: %s", response.StatusCode, content)
	}
	var generation Generation
	_ = json.Unmarshal(content, &generation)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response, content = requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/generations/"+generation.ID, nil)
		_ = json.Unmarshal(content, &generation)
		if generation.Status == "succeeded" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if generation.Status != "succeeded" || generation.Format != "png" {
		t.Fatalf("PNG generation failed: %+v", generation)
	}
	response, content = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/generations/"+generation.ID+"/download-token", map[string]any{})
	var token struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(content, &token)
	response, content = requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/downloads/"+token.Token, nil)
	if response.StatusCode != http.StatusOK || !bytes.HasPrefix(content, []byte{137, 80, 78, 71, 13, 10, 26, 10}) || response.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("invalid PNG download %d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
}

func requestJSON(t *testing.T, client *http.Client, method, url string, body any) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		content, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(content)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return response, content
}

func fixtureConfig(t *testing.T) seal.Config {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "testdata", "seal-config-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config seal.Config
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatal(err)
	}
	return config
}

func TestGenerationAndSingleUseDownload(t *testing.T) {
	server, client := newTestServer(t)
	defer server.Close()
	response, registerBody := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]any{"email": "tester@example.com", "password": "testing-password-123"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("login status %d", response.StatusCode)
	}
	if bytes.Contains(registerBody, []byte("passwordHash")) || bytes.Contains(registerBody, []byte("testing-password")) {
		t.Fatal("registration response leaked password material")
	}

	config := fixtureConfig(t)
	response, content := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/seal-configs", map[string]any{"name": "集成测试配置", "config": config})
	if response.StatusCode != 201 {
		t.Fatalf("config status %d: %s", response.StatusCode, content)
	}

	response, content = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/generations", map[string]any{"format": "svg", "config": config})
	if response.StatusCode != 202 {
		t.Fatalf("generation status %d: %s", response.StatusCode, content)
	}
	var generation Generation
	if err := json.Unmarshal(content, &generation); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for generation.Status != "succeeded" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		response, content = requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/generations/"+generation.ID, nil)
		if response.StatusCode != 200 {
			t.Fatalf("get generation status %d", response.StatusCode)
		}
		if err := json.Unmarshal(content, &generation); err != nil {
			t.Fatal(err)
		}
	}
	if generation.Status != "succeeded" {
		t.Fatalf("generation did not succeed: %+v", generation)
	}

	response, content = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/orders", map[string]any{"product": "single_export", "generationId": generation.ID})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("order status %d: %s", response.StatusCode, content)
	}
	var order Order
	if err := json.Unmarshal(content, &order); err != nil {
		t.Fatal(err)
	}
	response, content = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/orders/"+order.ID+"/simulate-payment", map[string]any{})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("simulate payment status %d: %s", response.StatusCode, content)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response, content = requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/generations/"+generation.ID, nil)
		if err := json.Unmarshal(content, &generation); err != nil {
			t.Fatal(err)
		}
		if generation.Status == "succeeded" && !generation.Watermark {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if generation.Status != "succeeded" || generation.Watermark {
		t.Fatalf("paid generation was not unlocked: %+v", generation)
	}

	response, content = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/generations/"+generation.ID+"/download-token", map[string]any{})
	if response.StatusCode != 200 {
		t.Fatalf("token status %d: %s", response.StatusCode, content)
	}
	var token struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(content, &token); err != nil {
		t.Fatal(err)
	}
	response, content = requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/downloads/"+token.Token, nil)
	if response.StatusCode != 200 || !bytes.Contains(content, []byte("<svg")) {
		t.Fatalf("download failed %d: %s", response.StatusCode, content)
	}
	response, _ = requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/downloads/"+token.Token, nil)
	if response.StatusCode != http.StatusGone {
		t.Fatalf("reused token status %d", response.StatusCode)
	}
	response, content = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/orders/"+order.ID+"/invoice", map[string]any{"title": "测试科技有限公司", "taxNumber": "91310000TEST", "email": "billing@example.com"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("invoice status %d: %s", response.StatusCode, content)
	}
	response, content = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/orders/"+order.ID+"/refund", map[string]any{"reason": "测试退款流程"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("refund status %d: %s", response.StatusCode, content)
	}
	var refund RefundRequest
	_ = json.Unmarshal(content, &refund)
	jar, _ := cookiejar.New(nil)
	admin := &http.Client{Jar: jar}
	_, _ = requestJSON(t, admin, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]any{"email": "admin@example.com", "password": "testing-password-123"})
	response, content = requestJSON(t, admin, http.MethodPut, server.URL+"/api/v1/admin/refunds/"+refund.ID, map[string]any{"status": "approved"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("refund decision status %d: %s", response.StatusCode, content)
	}
	response, _ = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/generations/"+generation.ID+"/download-token", map[string]any{})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("refunded generation token status %d", response.StatusCode)
	}
}

func TestPaymentCallbackSignatureAmountAndIdempotency(t *testing.T) {
	server, client := newTestServer(t)
	defer server.Close()
	_, _ = requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]any{"email": "vip@example.com", "password": "testing-password-123"})
	response, content := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/orders", map[string]any{"product": "vip_monthly"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("order status %d: %s", response.StatusCode, content)
	}
	var order Order
	_ = json.Unmarshal(content, &order)
	callback, _ := json.Marshal(map[string]any{"orderNo": order.OrderNo, "amountCents": order.AmountCents, "status": "paid", "transactionId": "gateway-123"})
	mac := hmac.New(sha256.New, []byte("test-payment-secret"))
	_, _ = mac.Write(callback)
	signature := hex.EncodeToString(mac.Sum(nil))
	call := func() (*http.Response, []byte) {
		request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/payments/callback", bytes.NewReader(callback))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Payment-Signature", signature)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		return response, body
	}
	response, content = call()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("callback status %d: %s", response.StatusCode, content)
	}
	response, content = call()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("idempotent callback status %d: %s", response.StatusCode, content)
	}
	response, content = requestJSON(t, client, http.MethodGet, server.URL+"/api/v1/auth/me", nil)
	if response.StatusCode != http.StatusOK || !bytes.Contains(content, []byte(`"membershipLevel":"vip"`)) {
		t.Fatalf("VIP entitlement missing: %s", content)
	}
}

func TestAdminAuthorizationAndCSPAudit(t *testing.T) {
	server, regular := newTestServer(t)
	defer server.Close()
	_, content := requestJSON(t, regular, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]any{"email": "regular@example.com", "password": "testing-password-123"})
	var registered struct {
		User userView `json:"user"`
	}
	_ = json.Unmarshal(content, &registered)
	response, _ := requestJSON(t, regular, http.MethodGet, server.URL+"/api/v1/admin/users", nil)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("regular admin access status %d", response.StatusCode)
	}
	response, _ = requestJSON(t, regular, http.MethodPost, server.URL+"/api/v1/security/csp-report", map[string]any{"csp-report": map[string]any{"violated-directive": "script-src"}})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("CSP report status %d", response.StatusCode)
	}
	jar, _ := cookiejar.New(nil)
	admin := &http.Client{Jar: jar}
	_, _ = requestJSON(t, admin, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]any{"email": "admin@example.com", "password": "testing-password-123"})
	response, content = requestJSON(t, admin, http.MethodGet, server.URL+"/api/v1/admin/users", nil)
	if response.StatusCode != http.StatusOK || bytes.Contains(content, []byte("passwordHash")) {
		t.Fatalf("admin users response invalid %d: %s", response.StatusCode, content)
	}
	response, content = requestJSON(t, admin, http.MethodPut, server.URL+"/api/v1/admin/users/"+registered.User.ID, map[string]any{"status": "banned"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("ban status %d: %s", response.StatusCode, content)
	}
	response, _ = requestJSON(t, regular, http.MethodGet, server.URL+"/api/v1/seal-configs", nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("banned user access status %d", response.StatusCode)
	}
	response, content = requestJSON(t, admin, http.MethodGet, server.URL+"/api/v1/admin/audit-events", nil)
	if response.StatusCode != http.StatusOK || !bytes.Contains(content, []byte("security.csp_report")) {
		t.Fatalf("CSP audit missing: %s", content)
	}
	response, content = requestJSON(t, admin, http.MethodPost, server.URL+"/api/v1/admin/resources", map[string]any{"type": "template", "key": "vip-company", "name": "VIP 企业章", "version": "1.0.0", "licenseScope": "internal-commercial", "vipOnly": true, "exportAllowed": true, "status": "active", "config": fixtureConfig(t)})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("resource create status %d: %s", response.StatusCode, content)
	}
	response, content = requestJSON(t, regular, http.MethodGet, server.URL+"/api/v1/resources?type=template", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("resource list status %d: %s", response.StatusCode, content)
	}
	if bytes.Contains(content, []byte(`"schemaVersion"`)) {
		t.Fatalf("VIP resource config leaked to non-VIP user: %s", content)
	}
	response, content = requestJSON(t, admin, http.MethodPost, server.URL+"/api/v1/admin/resources", map[string]any{"type": "font", "key": "vip-font", "name": "VIP 字体", "version": "1.0.0", "licenseScope": "commercial", "vipOnly": true, "exportAllowed": true, "status": "active"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("font resource status %d: %s", response.StatusCode, content)
	}
	jar, _ = cookiejar.New(nil)
	freeClient := &http.Client{Jar: jar}
	response, content = requestJSON(t, freeClient, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]any{"email": "font-user@example.com", "password": "testing-password-123"})
	var fontUser struct {
		User userView `json:"user"`
	}
	_ = json.Unmarshal(content, &fontUser)
	fontConfig := fixtureConfig(t)
	for index := range fontConfig.Layers {
		if fontConfig.Layers[index].Kind == "arcText" {
			fontConfig.Layers[index].FontID = "vip-font"
			break
		}
	}
	response, _ = requestJSON(t, freeClient, http.MethodPost, server.URL+"/api/v1/generations", map[string]any{"format": "svg", "config": fontConfig})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("free premium font generation status %d", response.StatusCode)
	}
	response, content = requestJSON(t, admin, http.MethodPut, server.URL+"/api/v1/admin/users/"+fontUser.User.ID, map[string]any{"membershipLevel": "vip"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("grant font VIP status %d: %s", response.StatusCode, content)
	}
	response, content = requestJSON(t, freeClient, http.MethodPost, server.URL+"/api/v1/generations", map[string]any{"format": "svg", "config": fontConfig})
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("VIP premium font generation status %d: %s", response.StatusCode, content)
	}
	var fontGeneration Generation
	_ = json.Unmarshal(content, &fontGeneration)
	deadline := time.Now().Add(2 * time.Second)
	for fontGeneration.Status != "succeeded" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		_, content = requestJSON(t, freeClient, http.MethodGet, server.URL+"/api/v1/generations/"+fontGeneration.ID, nil)
		_ = json.Unmarshal(content, &fontGeneration)
	}
	if fontGeneration.Status != "succeeded" {
		t.Fatalf("premium font generation did not finish: %+v", fontGeneration)
	}
}

func TestConfigOwnershipIsolation(t *testing.T) {
	server, first := newTestServer(t)
	defer server.Close()
	_, _ = requestJSON(t, first, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]any{"email": "first@example.com", "password": "testing-password-123"})
	response, content := requestJSON(t, first, http.MethodPost, server.URL+"/api/v1/seal-configs", map[string]any{"name": "私有配置", "config": fixtureConfig(t)})
	var record SealConfigRecord
	_ = json.Unmarshal(content, &record)
	jar, _ := cookiejar.New(nil)
	second := &http.Client{Jar: jar}
	_, _ = requestJSON(t, second, http.MethodPost, server.URL+"/api/v1/auth/register", map[string]any{"email": "second@example.com", "password": "testing-password-123"})
	response, _ = requestJSON(t, second, http.MethodGet, server.URL+"/api/v1/seal-configs/"+record.ID, nil)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-user read status %d", response.StatusCode)
	}
}

func TestSessionPersistsAcrossServiceRestartAndCanBeRevoked(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	objects, err := storage.NewLocal(filepath.Join(root, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStateStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	newServer := func(store *StateStore) *httptest.Server {
		service := NewService(store, objects, slog.New(slog.NewTextHandler(io.Discard, nil)), 1<<20)
		mux := http.NewServeMux()
		service.Register(mux)
		return httptest.NewServer(mux)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	first := newServer(store)
	response, content := requestJSON(t, client, http.MethodPost, first.URL+"/api/v1/auth/register", map[string]any{"email": "persistent@example.com", "password": "testing-password-123"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("register status %d: %s", response.StatusCode, content)
	}
	first.Close()

	reloaded, err := NewStateStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	second := newServer(reloaded)
	defer second.Close()
	response, content = requestJSON(t, client, http.MethodGet, second.URL+"/api/v1/auth/me", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("persisted session status %d: %s", response.StatusCode, content)
	}
	response, content = requestJSON(t, client, http.MethodGet, second.URL+"/api/v1/auth/sessions", nil)
	if response.StatusCode != http.StatusOK || bytes.Contains(content, []byte("tokenHash")) {
		t.Fatalf("session list invalid %d: %s", response.StatusCode, content)
	}
	var sessions struct {
		Items []Session `json:"items"`
	}
	if err := json.Unmarshal(content, &sessions); err != nil || len(sessions.Items) != 1 {
		t.Fatalf("unexpected sessions: %s", content)
	}
	response, content = requestJSON(t, client, http.MethodDelete, second.URL+"/api/v1/auth/sessions/"+sessions.Items[0].ID, nil)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke status %d: %s", response.StatusCode, content)
	}
	response, _ = requestJSON(t, client, http.MethodGet, second.URL+"/api/v1/auth/me", nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked session status %d", response.StatusCode)
	}
}
