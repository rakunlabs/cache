package memory

import (
	"context"
	"time"

	"github.com/samber/hot"
)

var (
	DefaultMaxItems = 1_000
	DefaultTTL      = 10 * time.Minute
)

type Config struct {
	MaxItems int
	TTL      time.Duration
}

type Memory[K comparable, V any] struct {
	h *hot.HotCache[K, V]
}

func New[K comparable, V any](cfg Config) (*Memory[K, V], error) {
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
