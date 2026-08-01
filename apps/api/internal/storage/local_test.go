package storage

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestLocalPutOpenAndTraversal(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), "generations/a.svg", "image/svg+xml", strings.NewReader("<svg/>")); err != nil {
		t.Fatal(err)
	}
	body, contentType, err := store.Open(context.Background(), "generations/a.svg")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, _ := io.ReadAll(body)
	if string(data) != "<svg/>" || contentType != "image/svg+xml" {
		t.Fatalf("unexpected object: %q %q", data, contentType)
	}
	if err := store.Put(context.Background(), "../escape", "text/plain", strings.NewReader("x")); err == nil {
		t.Fatal("expected traversal rejection")
	}
}
