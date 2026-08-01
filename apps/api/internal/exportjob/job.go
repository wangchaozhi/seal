package exportjob

import (
	"context"
	"time"

	"sealplatform/api/internal/auth"
	"sealplatform/api/internal/seal"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRendering Status = "rendering"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type Job struct {
	ID        string
	UserID    string
	Config    seal.Config
	Policy    auth.ExportPolicy
	Status    Status
	FileKey   string
	CreatedAt time.Time
}

// Queue describes the production queue contract. The platform package provides
// Redis and bounded local implementations used by the assembled service.
type Queue interface {
	Enqueue(ctx context.Context, job Job) error
	Receive(ctx context.Context) (Job, error)
	Complete(ctx context.Context, id, fileKey string) error
	Fail(ctx context.Context, id, reason string) error
}
