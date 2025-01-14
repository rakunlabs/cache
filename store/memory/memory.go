package memory

import (
	"context"
	"time"

	"github.com/samber/hot"
	"github.com/worldline-go/cache"
)

var (
	DefaultMaxItems = 1_000
	DefaultTTL      = 10 * time.Minute
)

type Config struct {
	MaxItems int           `cfg:"max_items" json:"max_items"`
	TTL      time.Duration `cfg:"ttl"       json:"ttl"`
}

type Memory[K comparable, V any] struct {
	h *hot.HotCache[K, V]
}

func Store[K comparable, V any](ctx context.Context, cfg Config) (cache.Cacher[K, V], error) {
	if cfg.MaxItems == 0 {
		cfg.MaxItems = DefaultMaxItems
	}

	if cfg.TTL == 0 {
		cfg.TTL = DefaultTTL
	}

	cache := hot.NewHotCache[K, V](hot.LRU, cfg.MaxItems).
		WithTTL(cfg.TTL).
		WithJanitor().
		Build()

	return &Memory[K, V]{
		h: cache,
	}, nil
}

func (m *Memory[K, V]) Get(_ context.Context, key K) (V, bool, error) {
	return m.h.Get(key)
}

func (m *Memory[K, V]) Set(_ context.Context, key K, value V) error {
	m.h.Set(key, value)

	return nil
}
