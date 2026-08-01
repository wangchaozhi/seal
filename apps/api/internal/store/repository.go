package store

import (
	"context"
	"encoding/json"
	"time"
)

type SealConfigRecord struct {
	ID            string
	UserID        string
	Name          string
	SchemaVersion int
	Config        json.RawMessage
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type SealConfigRepository interface {
	Create(ctx context.Context, record SealConfigRecord) (SealConfigRecord, error)
	Get(ctx context.Context, userID, id string) (SealConfigRecord, error)
	List(ctx context.Context, userID string, limit, offset int) ([]SealConfigRecord, error)
	Update(ctx context.Context, record SealConfigRecord) (SealConfigRecord, error)
	Delete(ctx context.Context, userID, id string) error
}
