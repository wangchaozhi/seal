package platform

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
)

var ErrQueueFull = errors.New("generation queue is full")

type GenerationQueue interface {
	Enqueue(context.Context, string) error
	Dequeue(context.Context) (string, error)
	Depth(context.Context) (int64, error)
}

type localGenerationQueue struct{ values chan string }

func newLocalGenerationQueue(size int) GenerationQueue {
	return &localGenerationQueue{values: make(chan string, size)}
}
func (queue *localGenerationQueue) Enqueue(_ context.Context, id string) error {
	select {
	case queue.values <- id:
		return nil
	default:
		return ErrQueueFull
	}
}
func (queue *localGenerationQueue) Dequeue(ctx context.Context) (string, error) {
	select {
	case value := <-queue.values:
		return value, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
func (queue *localGenerationQueue) Depth(context.Context) (int64, error) {
	return int64(len(queue.values)), nil
}

type RedisGenerationQueue struct {
	client *redis.Client
	key    string
}

func NewRedisGenerationQueue(client *redis.Client, key string) *RedisGenerationQueue {
	return &RedisGenerationQueue{client: client, key: key}
}
func (queue *RedisGenerationQueue) Enqueue(ctx context.Context, id string) error {
	return queue.client.LPush(ctx, queue.key, id).Err()
}
func (queue *RedisGenerationQueue) Dequeue(ctx context.Context) (string, error) {
	result, err := queue.client.BRPop(ctx, 0, queue.key).Result()
	if err != nil {
		return "", err
	}
	if len(result) != 2 {
		return "", errors.New("invalid redis queue result")
	}
	return result[1], nil
}
func (queue *RedisGenerationQueue) Depth(ctx context.Context) (int64, error) {
	return queue.client.LLen(ctx, queue.key).Result()
}
