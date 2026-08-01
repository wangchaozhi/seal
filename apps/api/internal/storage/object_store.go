package storage

import (
	"context"
	"io"
	"time"
)

// ObjectStore 用于适配 S3、OSS、COS 或 MinIO。正式文件必须放私有桶。
type ObjectStore interface {
	Put(ctx context.Context, key, contentType string, body io.Reader) error
	Open(ctx context.Context, key string) (io.ReadCloser, string, error)
	Delete(ctx context.Context, key string) error
	SignedDownloadURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}
