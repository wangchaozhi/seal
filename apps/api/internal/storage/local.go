package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Local struct{ Root string }

func NewLocal(root string) (*Local, error) {
	if root == "" {
		return nil, errors.New("object storage root is required")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, err
	}
	return &Local{Root: root}, nil
}

func (store *Local) path(key string) (string, error) {
	clean := filepath.Clean(strings.TrimPrefix(key, "/"))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", errors.New("invalid object key")
	}
	root, err := filepath.Abs(store.Root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, clean)
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return "", errors.New("object key escapes storage root")
	}
	return target, nil
}

func (store *Local) Put(_ context.Context, key, contentType string, body io.Reader) error {
	target, err := store.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err = io.Copy(temporary, body); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Chmod(0o640); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, target); err != nil {
		return err
	}
	return os.WriteFile(target+".content-type", []byte(contentType), 0o640)
}

func (store *Local) Open(_ context.Context, key string) (io.ReadCloser, string, error) {
	target, err := store.path(key)
	if err != nil {
		return nil, "", err
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, "", err
	}
	contentType, _ := os.ReadFile(target + ".content-type")
	return file, string(contentType), nil
}

func (store *Local) Delete(_ context.Context, key string) error {
	target, err := store.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_ = os.Remove(target + ".content-type")
	return nil
}

func (store *Local) SignedDownloadURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", errors.New("local storage uses application-issued download tokens")
}
