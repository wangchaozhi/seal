package platform

import (
	"context"
	"errors"
	"testing"
)

func TestLocalGenerationQueueCapacityDepthAndCancellation(t *testing.T) {
	queue := newLocalGenerationQueue(1)
	if err := queue.Enqueue(context.Background(), "gen_1"); err != nil {
		t.Fatal(err)
	}
	if depth, err := queue.Depth(context.Background()); err != nil || depth != 1 {
		t.Fatalf("depth=%d err=%v", depth, err)
	}
	if err := queue.Enqueue(context.Background(), "gen_2"); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("full queue error %v", err)
	}
	if value, err := queue.Dequeue(context.Background()); err != nil || value != "gen_1" {
		t.Fatalf("dequeue=%q err=%v", value, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := queue.Dequeue(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error %v", err)
	}
}
