package platform

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"sealplatform/api/internal/seal"
)

var ErrNotFound = errors.New("record not found")
var ErrAmountMismatch = errors.New("order amount mismatch")
var ErrInvalidState = errors.New("invalid record state")

type User struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	PasswordHash    string     `json:"passwordHash"`
	MembershipLevel string     `json:"membershipLevel"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"createdAt"`
	VIPExpiresAt    *time.Time `json:"vipExpiresAt,omitempty"`
	Role            string     `json:"role"`
	AuthProvider    string     `json:"authProvider,omitempty"`
	AuthSubject     string     `json:"authSubject,omitempty"`
	DisplayName     string     `json:"displayName,omitempty"`
}

type SealConfigRecord struct {
	ID        string      `json:"id"`
	UserID    string      `json:"userId"`
	Name      string      `json:"name"`
	Config    seal.Config `json:"config"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

type Generation struct {
	ID              string      `json:"id"`
	UserID          string      `json:"userId"`
	Config          seal.Config `json:"config"`
	GenerationKey   string      `json:"generationKey"`
	RendererVersion string      `json:"rendererVersion"`
	Format          string      `json:"format"`
	Status          string      `json:"status"`
	Watermark       bool        `json:"watermark"`
	FileKey         string      `json:"fileKey,omitempty"`
	FailureReason   string      `json:"failureReason,omitempty"`
	CreatedAt       time.Time   `json:"createdAt"`
	StartedAt       *time.Time  `json:"startedAt,omitempty"`
	FinishedAt      *time.Time  `json:"finishedAt,omitempty"`
	AccessRevoked   bool        `json:"accessRevoked,omitempty"`
}

type DownloadToken struct {
	Hash         string     `json:"hash"`
	UserID       string     `json:"userId"`
	GenerationID string     `json:"generationId"`
	ExpiresAt    time.Time  `json:"expiresAt"`
	ConsumedAt   *time.Time `json:"consumedAt,omitempty"`
}

type AuditEvent struct {
	ID        string         `json:"id"`
	UserID    string         `json:"userId,omitempty"`
	Type      string         `json:"type"`
	TargetID  string         `json:"targetId,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

type Order struct {
	ID             string     `json:"id"`
	OrderNo        string     `json:"orderNo"`
	UserID         string     `json:"userId"`
	GenerationID   string     `json:"generationId,omitempty"`
	Product        string     `json:"product"`
	AmountCents    int        `json:"amountCents"`
	Status         string     `json:"status"`
	PaymentChannel string     `json:"paymentChannel,omitempty"`
	TransactionID  string     `json:"transactionId,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	PaidAt         *time.Time `json:"paidAt,omitempty"`
}

type RefundRequest struct {
	ID        string     `json:"id"`
	OrderID   string     `json:"orderId"`
	UserID    string     `json:"userId"`
	Reason    string     `json:"reason"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DecidedAt *time.Time `json:"decidedAt,omitempty"`
}

type Invoice struct {
	ID        string    `json:"id"`
	OrderID   string    `json:"orderId"`
	UserID    string    `json:"userId"`
	Title     string    `json:"title"`
	TaxNumber string    `json:"taxNumber,omitempty"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Resource struct {
	ID            string       `json:"id"`
	Type          string       `json:"type"`
	Key           string       `json:"key"`
	Name          string       `json:"name"`
	Version       string       `json:"version"`
	LicenseScope  string       `json:"licenseScope"`
	VIPOnly       bool         `json:"vipOnly"`
	ExportAllowed bool         `json:"exportAllowed"`
	Status        string       `json:"status"`
	Config        *seal.Config `json:"config,omitempty"`
	CreatedAt     time.Time    `json:"createdAt"`
	UpdatedAt     time.Time    `json:"updatedAt"`
}

type Asset struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	Mime      string    `json:"mime"`
	FileKey   string    `json:"fileKey"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"createdAt"`
}

type Session struct {
	ID            string     `json:"id"`
	TokenHash     string     `json:"tokenHash,omitempty"`
	UserID        string     `json:"userId"`
	UserAgentHash string     `json:"userAgentHash"`
	IPHash        string     `json:"ipHash"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	RevokedAt     *time.Time `json:"revokedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

type stateData struct {
	Users          map[string]User             `json:"users"`
	Configs        map[string]SealConfigRecord `json:"configs"`
	Generations    map[string]Generation       `json:"generations"`
	DownloadTokens map[string]DownloadToken    `json:"downloadTokens"`
	AuditEvents    []AuditEvent                `json:"auditEvents"`
	Orders         map[string]Order            `json:"orders"`
	Assets         map[string]Asset            `json:"assets"`
	Sessions       map[string]Session          `json:"sessions"`
	Refunds        map[string]RefundRequest    `json:"refunds"`
	Invoices       map[string]Invoice          `json:"invoices"`
	Resources      map[string]Resource         `json:"resources"`
}

type StateStore struct {
	mu       sync.RWMutex
	path     string
	database *pgxpool.Pool
	data     stateData
}

func NewStateStore(path string) (*StateStore, error) {
	store := &StateStore{path: path, data: stateData{Users: map[string]User{}, Configs: map[string]SealConfigRecord{}, Generations: map[string]Generation{}, DownloadTokens: map[string]DownloadToken{}, Orders: map[string]Order{}, Assets: map[string]Asset{}, Sessions: map[string]Session{}, Refunds: map[string]RefundRequest{}, Invoices: map[string]Invoice{}, Resources: map[string]Resource{}}}
	content, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(content, &store.data); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	store.ensureMaps()
	return store, nil
}

func NewPostgresStateStore(ctx context.Context, databaseURL string) (*StateStore, error) {
	database, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := database.Ping(ctx); err != nil {
		database.Close()
		return nil, err
	}
	if _, err := database.Exec(ctx, `CREATE TABLE IF NOT EXISTS platform_state (id SMALLINT PRIMARY KEY CHECK (id = 1), data JSONB NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		database.Close()
		return nil, err
	}
	store := &StateStore{database: database, data: stateData{}}
	var content []byte
	err = database.QueryRow(ctx, `SELECT data FROM platform_state WHERE id = 1`).Scan(&content)
	if err == nil {
		if err := json.Unmarshal(content, &store.data); err != nil {
			database.Close()
			return nil, err
		}
	} else {
		content, marshalErr := json.Marshal(store.data)
		if marshalErr != nil {
			database.Close()
			return nil, marshalErr
		}
		if _, insertErr := database.Exec(ctx, `INSERT INTO platform_state (id, data) VALUES (1, $1::jsonb) ON CONFLICT (id) DO NOTHING`, content); insertErr != nil {
			database.Close()
			return nil, insertErr
		}
	}
	store.ensureMaps()
	return store, nil
}

func (store *StateStore) Close() {
	if store.database != nil {
		store.database.Close()
	}
}

func (store *StateStore) ensureMaps() {
	if store.data.Users == nil {
		store.data.Users = map[string]User{}
	}
	if store.data.Configs == nil {
		store.data.Configs = map[string]SealConfigRecord{}
	}
	if store.data.Generations == nil {
		store.data.Generations = map[string]Generation{}
	}
	if store.data.DownloadTokens == nil {
		store.data.DownloadTokens = map[string]DownloadToken{}
	}
	if store.data.Orders == nil {
		store.data.Orders = map[string]Order{}
	}
	if store.data.Assets == nil {
		store.data.Assets = map[string]Asset{}
	}
	if store.data.Sessions == nil {
		store.data.Sessions = map[string]Session{}
	}
	if store.data.Refunds == nil {
		store.data.Refunds = map[string]RefundRequest{}
	}
	if store.data.Invoices == nil {
		store.data.Invoices = map[string]Invoice{}
	}
	if store.data.Resources == nil {
		store.data.Resources = map[string]Resource{}
	}
}

func (store *StateStore) persistLocked() error {
	if store.database != nil {
		content, err := json.Marshal(store.data)
		if err != nil {
			return err
		}
		_, err = store.database.Exec(context.Background(), `INSERT INTO platform_state (id, data, updated_at) VALUES (1, $1::jsonb, NOW()) ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data, updated_at = NOW()`, content)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(store.path), 0o750); err != nil {
		return err
	}
	content, err := json.MarshalIndent(store.data, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(store.path), ".state-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err = temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, store.path)
}

func (store *StateStore) reloadLocked(ctx context.Context) error {
	if store.database == nil {
		return nil
	}
	var content []byte
	if err := store.database.QueryRow(ctx, `SELECT data FROM platform_state WHERE id = 1`).Scan(&content); err != nil {
		store.data = stateData{}
		store.ensureMaps()
		return err
	}
	if err := json.Unmarshal(content, &store.data); err != nil {
		store.data = stateData{}
		store.ensureMaps()
		return err
	}
	store.ensureMaps()
	return nil
}

func (store *StateStore) lockForRead() func() {
	store.mu.Lock()
	_ = store.reloadLocked(context.Background())
	return store.mu.Unlock
}

func (store *StateStore) mutate(fn func(*stateData) error) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.database != nil {
		tx, err := store.database.Begin(context.Background())
		if err != nil {
			return err
		}
		defer tx.Rollback(context.Background())
		var content []byte
		if err := tx.QueryRow(context.Background(), `SELECT data FROM platform_state WHERE id = 1 FOR UPDATE`).Scan(&content); err != nil {
			return err
		}
		if err := json.Unmarshal(content, &store.data); err != nil {
			return err
		}
		store.ensureMaps()
		if err := fn(&store.data); err != nil {
			return err
		}
		content, err = json.Marshal(store.data)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), `UPDATE platform_state SET data = $1::jsonb, updated_at = NOW() WHERE id = 1`, content); err != nil {
			return err
		}
		return tx.Commit(context.Background())
	}
	if err := fn(&store.data); err != nil {
		return err
	}
	return store.persistLocked()
}

func (store *StateStore) FindUserByEmail(email string) (User, bool) {
	unlock := store.lockForRead()
	defer unlock()
	for _, user := range store.data.Users {
		if user.Email == email {
			return user, true
		}
	}
	return User{}, false
}

func (store *StateStore) FindUserByExternalIdentity(provider, subject string) (User, bool) {
	unlock := store.lockForRead()
	defer unlock()
	for _, user := range store.data.Users {
		if user.AuthProvider == provider && user.AuthSubject == subject {
			return user, true
		}
	}
	return User{}, false
}

func (store *StateStore) GetUser(id string) (User, bool) {
	unlock := store.lockForRead()
	defer unlock()
	user, ok := store.data.Users[id]
	return user, ok
}
func (store *StateStore) SaveUser(user User) error {
	return store.mutate(func(data *stateData) error { data.Users[user.ID] = user; return nil })
}
func (store *StateStore) ListUsers() []User {
	unlock := store.lockForRead()
	defer unlock()
	result := make([]User, 0, len(store.data.Users))
	for _, user := range store.data.Users {
		result = append(result, user)
	}
	return result
}
func (store *StateStore) SaveConfig(record SealConfigRecord) error {
	return store.mutate(func(data *stateData) error { data.Configs[record.ID] = record; return nil })
}
func (store *StateStore) GetConfig(id string) (SealConfigRecord, bool) {
	unlock := store.lockForRead()
	defer unlock()
	value, ok := store.data.Configs[id]
	return value, ok
}
func (store *StateStore) ListConfigs(userID string) []SealConfigRecord {
	unlock := store.lockForRead()
	defer unlock()
	result := []SealConfigRecord{}
	for _, value := range store.data.Configs {
		if value.UserID == userID {
			result = append(result, value)
		}
	}
	return result
}
func (store *StateStore) DeleteConfig(id, userID string) error {
	return store.mutate(func(data *stateData) error {
		value, ok := data.Configs[id]
		if !ok || value.UserID != userID {
			return ErrNotFound
		}
		delete(data.Configs, id)
		return nil
	})
}
func (store *StateStore) SaveGeneration(value Generation) error {
	return store.mutate(func(data *stateData) error { data.Generations[value.ID] = value; return nil })
}
func (store *StateStore) GetGeneration(id string) (Generation, bool) {
	unlock := store.lockForRead()
	defer unlock()
	value, ok := store.data.Generations[id]
	return value, ok
}
func (store *StateStore) ListGenerations(userID string) []Generation {
	unlock := store.lockForRead()
	defer unlock()
	result := []Generation{}
	for _, value := range store.data.Generations {
		if value.UserID == userID {
			result = append(result, value)
		}
	}
	return result
}
func (store *StateStore) AllGenerations() []Generation {
	unlock := store.lockForRead()
	defer unlock()
	result := make([]Generation, 0, len(store.data.Generations))
	for _, value := range store.data.Generations {
		result = append(result, value)
	}
	return result
}
func (store *StateStore) FindGenerationByKey(userID, key string) (Generation, bool) {
	unlock := store.lockForRead()
	defer unlock()
	for _, value := range store.data.Generations {
		if value.UserID == userID && value.GenerationKey == key && value.Status != "failed" {
			return value, true
		}
	}
	return Generation{}, false
}
func (store *StateStore) SaveDownloadToken(value DownloadToken) error {
	return store.mutate(func(data *stateData) error { data.DownloadTokens[value.Hash] = value; return nil })
}
func (store *StateStore) ConsumeDownloadToken(hash, userID string, now time.Time) (DownloadToken, error) {
	var result DownloadToken
	err := store.mutate(func(data *stateData) error {
		value, ok := data.DownloadTokens[hash]
		if !ok || value.UserID != userID || value.ConsumedAt != nil || !now.Before(value.ExpiresAt) {
			return ErrNotFound
		}
		value.ConsumedAt = &now
		data.DownloadTokens[hash] = value
		result = value
		return nil
	})
	return result, err
}
func (store *StateStore) AddAudit(event AuditEvent) error {
	return store.mutate(func(data *stateData) error {
		data.AuditEvents = append(data.AuditEvents, event)
		if len(data.AuditEvents) > 5000 {
			data.AuditEvents = data.AuditEvents[len(data.AuditEvents)-5000:]
		}
		return nil
	})
}
func (store *StateStore) ListAudit(limit int) []AuditEvent {
	unlock := store.lockForRead()
	defer unlock()
	if limit <= 0 || limit > len(store.data.AuditEvents) {
		limit = len(store.data.AuditEvents)
	}
	start := len(store.data.AuditEvents) - limit
	result := append([]AuditEvent(nil), store.data.AuditEvents[start:]...)
	return result
}

func (store *StateStore) SaveOrder(value Order) error {
	return store.mutate(func(data *stateData) error { data.Orders[value.ID] = value; return nil })
}

func (store *StateStore) GetOrder(id string) (Order, bool) {
	unlock := store.lockForRead()
	defer unlock()
	value, ok := store.data.Orders[id]
	return value, ok
}

func (store *StateStore) ListOrders(userID string) []Order {
	unlock := store.lockForRead()
	defer unlock()
	result := []Order{}
	for _, value := range store.data.Orders {
		if value.UserID == userID {
			result = append(result, value)
		}
	}
	return result
}
func (store *StateStore) AllOrders() []Order {
	unlock := store.lockForRead()
	defer unlock()
	result := make([]Order, 0, len(store.data.Orders))
	for _, value := range store.data.Orders {
		result = append(result, value)
	}
	return result
}

func (store *StateStore) SaveRefund(value RefundRequest) error {
	return store.mutate(func(data *stateData) error {
		order, ok := data.Orders[value.OrderID]
		if !ok || order.UserID != value.UserID || order.Status != "paid" {
			return ErrInvalidState
		}
		for _, existing := range data.Refunds {
			if existing.OrderID == value.OrderID && (existing.Status == "pending" || existing.Status == "approved") {
				return ErrInvalidState
			}
		}
		data.Refunds[value.ID] = value
		return nil
	})
}

func (store *StateStore) ListRefunds(userID string) []RefundRequest {
	unlock := store.lockForRead()
	defer unlock()
	result := []RefundRequest{}
	for _, value := range store.data.Refunds {
		if userID == "" || value.UserID == userID {
			result = append(result, value)
		}
	}
	return result
}

func (store *StateStore) DecideRefund(id, status string, now time.Time) (RefundRequest, error) {
	var result RefundRequest
	err := store.mutate(func(data *stateData) error {
		value, ok := data.Refunds[id]
		if !ok {
			return ErrNotFound
		}
		if value.Status != "pending" || (status != "approved" && status != "rejected") {
			return ErrInvalidState
		}
		value.Status, value.UpdatedAt, value.DecidedAt = status, now, &now
		data.Refunds[id] = value
		result = value
		if status != "approved" {
			return nil
		}
		order := data.Orders[value.OrderID]
		order.Status = "refunded"
		data.Orders[order.ID] = order
		if order.GenerationID != "" {
			generation := data.Generations[order.GenerationID]
			generation.AccessRevoked = true
			data.Generations[generation.ID] = generation
			for hash, token := range data.DownloadTokens {
				if token.GenerationID == generation.ID && token.ConsumedAt == nil {
					token.ExpiresAt = now
					data.DownloadTokens[hash] = token
				}
			}
		}
		if order.Product == "vip_monthly" {
			user := data.Users[order.UserID]
			if user.VIPExpiresAt != nil {
				expires := user.VIPExpiresAt.Add(-30 * 24 * time.Hour)
				user.VIPExpiresAt = &expires
				if !expires.After(now) {
					user.MembershipLevel = "free"
					user.VIPExpiresAt = nil
				}
			}
			data.Users[user.ID] = user
		}
		return nil
	})
	return result, err
}

func (store *StateStore) SaveInvoice(value Invoice) error {
	return store.mutate(func(data *stateData) error {
		order, ok := data.Orders[value.OrderID]
		if !ok || order.UserID != value.UserID || (order.Status != "paid" && order.Status != "refunded") {
			return ErrInvalidState
		}
		for _, existing := range data.Invoices {
			if existing.OrderID == value.OrderID {
				return ErrInvalidState
			}
		}
		data.Invoices[value.ID] = value
		return nil
	})
}

func (store *StateStore) ListInvoices(userID string) []Invoice {
	unlock := store.lockForRead()
	defer unlock()
	result := []Invoice{}
	for _, value := range store.data.Invoices {
		if userID == "" || value.UserID == userID {
			result = append(result, value)
		}
	}
	return result
}

func (store *StateStore) MarkInvoiceIssued(id string, now time.Time) (Invoice, error) {
	var result Invoice
	err := store.mutate(func(data *stateData) error {
		value, ok := data.Invoices[id]
		if !ok {
			return ErrNotFound
		}
		value.Status, value.UpdatedAt = "issued", now
		data.Invoices[id] = value
		result = value
		return nil
	})
	return result, err
}

func (store *StateStore) SaveResource(value Resource) error {
	return store.mutate(func(data *stateData) error {
		for id, existing := range data.Resources {
			if id != value.ID && existing.Type == value.Type && existing.Key == value.Key && existing.Version == value.Version {
				return ErrInvalidState
			}
		}
		data.Resources[value.ID] = value
		return nil
	})
}

func (store *StateStore) GetResource(id string) (Resource, bool) {
	unlock := store.lockForRead()
	defer unlock()
	value, ok := store.data.Resources[id]
	return value, ok
}

func (store *StateStore) ListResources(resourceType string) []Resource {
	unlock := store.lockForRead()
	defer unlock()
	result := []Resource{}
	for _, value := range store.data.Resources {
		if resourceType == "" || value.Type == resourceType {
			result = append(result, value)
		}
	}
	return result
}

func (store *StateStore) DeleteResource(id string) error {
	return store.mutate(func(data *stateData) error {
		if _, ok := data.Resources[id]; !ok {
			return ErrNotFound
		}
		delete(data.Resources, id)
		return nil
	})
}

func (store *StateStore) PayOrder(orderNo string, amountCents int, channel, transactionID string, now time.Time) (Order, string, bool, error) {
	var result Order
	var generationID string
	var requeue bool
	err := store.mutate(func(data *stateData) error {
		var id string
		for candidateID, value := range data.Orders {
			if value.OrderNo == orderNo {
				id = candidateID
				result = value
				break
			}
		}
		if id == "" {
			return ErrNotFound
		}
		if result.AmountCents != amountCents {
			return ErrAmountMismatch
		}
		if result.Status == "paid" {
			return nil
		}
		result.Status = "paid"
		result.PaymentChannel = channel
		result.TransactionID = transactionID
		result.PaidAt = &now
		data.Orders[id] = result
		if result.Product == "vip_monthly" {
			user := data.Users[result.UserID]
			base := now
			if user.VIPExpiresAt != nil && user.VIPExpiresAt.After(base) {
				base = *user.VIPExpiresAt
			}
			expires := base.Add(30 * 24 * time.Hour)
			user.MembershipLevel = "vip"
			user.VIPExpiresAt = &expires
			data.Users[user.ID] = user
		}
		if result.Product == "single_export" {
			generation, ok := data.Generations[result.GenerationID]
			if !ok {
				return ErrNotFound
			}
			generation.Status = "queued"
			generation.Watermark = false
			generation.FileKey = ""
			generation.FailureReason = ""
			generation.StartedAt = nil
			generation.FinishedAt = nil
			data.Generations[generation.ID] = generation
			generationID = generation.ID
			requeue = true
		}
		return nil
	})
	return result, generationID, requeue, err
}

func (store *StateStore) SaveAsset(value Asset) error {
	return store.mutate(func(data *stateData) error { data.Assets[value.ID] = value; return nil })
}

func (store *StateStore) GetAsset(id string) (Asset, bool) {
	unlock := store.lockForRead()
	defer unlock()
	value, ok := store.data.Assets[id]
	return value, ok
}

func (store *StateStore) SaveSession(value Session) error {
	return store.mutate(func(data *stateData) error {
		data.Sessions[value.TokenHash] = value
		return nil
	})
}

func (store *StateStore) GetActiveSession(hash string, now time.Time) (Session, bool) {
	unlock := store.lockForRead()
	defer unlock()
	value, ok := store.data.Sessions[hash]
	return value, ok && value.RevokedAt == nil && now.Before(value.ExpiresAt)
}

func (store *StateStore) ListSessions(userID string, now time.Time) []Session {
	unlock := store.lockForRead()
	defer unlock()
	result := []Session{}
	for _, value := range store.data.Sessions {
		if value.UserID == userID && value.RevokedAt == nil && now.Before(value.ExpiresAt) {
			value.TokenHash = ""
			result = append(result, value)
		}
	}
	return result
}

func (store *StateStore) RevokeSession(userID, id string, now time.Time) error {
	return store.mutate(func(data *stateData) error {
		for hash, value := range data.Sessions {
			if value.ID == id && value.UserID == userID {
				value.RevokedAt = &now
				data.Sessions[hash] = value
				return nil
			}
		}
		return ErrNotFound
	})
}

func (store *StateStore) RevokeSessionByHash(hash string, now time.Time) error {
	return store.mutate(func(data *stateData) error {
		value, ok := data.Sessions[hash]
		if !ok {
			return ErrNotFound
		}
		value.RevokedAt = &now
		data.Sessions[hash] = value
		return nil
	})
}
