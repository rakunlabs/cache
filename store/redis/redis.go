package redis

import (
	"context"
	"errors"
	"time"

	"github.com/rakunlabs/cache"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	TTL time.Duration `cfg:"ttl" json:"ttl"`
	// KeyPrefix is prepended to every key, so several caches can share one
	// Redis without colliding.
	KeyPrefix string `cfg:"key_prefix" json:"key_prefix"`
}

type Cache struct {
	client redis.UniversalClient
	cfg    Config
}

// Store uses a caller-owned client; the cache does not close it. Use NewClient
// to build one from ConnConfig, including Redis Cluster support.
func Store(v redis.UniversalClient) func(ctx context.Context, cfg Config) (cache.Cacher[string, string], error) {
	return func(ctx context.Context, cfg Config) (cache.Cacher[string, string], error) {
		return &Cache{
			client: v,
			cfg:    cfg,
		}, nil
	}
}

func (c *Cache) key(key string) string {
	return c.cfg.KeyPrefix + key
}

func (c *Cache) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := c.client.Get(ctx, c.key(key)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", false, nil
		}

		return "", false, err
	}

	return v, true, nil
}

func (c *Cache) Set(ctx context.Context, key, value string) error {
	cmd := c.client.Set(ctx, c.key(key), value, c.cfg.TTL)
	if err := cmd.Err(); err != nil {
		return err
	}

	return nil
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	cmd := c.client.Del(ctx, c.key(key))
	if err := cmd.Err(); err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}

		return err
	}

	return nil
}

var _ cache.Taker[string, string] = (*Cache)(nil)

// Take atomically gets and deletes key with GETDEL (Redis 6.2+), so only one
// caller can consume a value.
func (c *Cache) Take(ctx context.Context, key string) (string, bool, error) {
	v, err := c.client.GetDel(ctx, c.key(key)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", false, nil
		}

		return "", false, err
	}

	return v, true, nil
}
