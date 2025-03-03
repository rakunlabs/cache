package redis

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/worldline-go/cache"
)

type Config struct {
	TTL time.Duration `cfg:"ttl" json:"ttl"`
}

type Cache struct {
	client redis.UniversalClient
	cfg    Config
}

func Store(v redis.UniversalClient) func(ctx context.Context, cfg Config) (cache.Cacher[string, string], error) {
	return func(ctx context.Context, cfg Config) (cache.Cacher[string, string], error) {
		return &Cache{
			client: v,
			cfg:    cfg,
		}, nil
	}
}

func (c *Cache) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", false, nil
		}

		return "", false, err
	}

	return v, true, nil
}

func (c *Cache) Set(ctx context.Context, key, value string) error {
	cmd := c.client.Set(ctx, key, value, c.cfg.TTL)
	if err := cmd.Err(); err != nil {
		return err
	}

	return nil
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	cmd := c.client.Del(ctx, key)
	if err := cmd.Err(); err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}

		return err
	}

	return nil
}
