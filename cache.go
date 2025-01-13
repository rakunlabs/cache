package cache

import (
	"context"
	"errors"
	"time"

	"github.com/worldline-go/cache/plugins/memory"
)

type Type int

const (
	TypeMemory Type = iota
)

var ErrInvalidType = errors.New("invalid type")

// //////////////////////////////////////////////////////////////////////////

type Cache struct {
	maxItems int
	ttl      time.Duration

	// Type is the type of cache to use.
	typeStorage Type
}

type Cacher[K comparable, V any] interface {
	Get(key K) (V, bool, error)
	Set(key K, value V) error
}

func New(_ context.Context, opts ...Option) *Cache {
	o := &option{}
	for _, opt := range opts {
		opt(o)
	}

	return &Cache{
		maxItems: o.MaxItems,
		ttl:      o.TTL,

		typeStorage: o.TypeStorage,
	}
}

func Port[K comparable, V any](c *Cache) (Cacher[K, V], error) {
	switch c.typeStorage {
	case TypeMemory:
		return memory.New[K, V](memory.Config{
			MaxItems: c.maxItems,
			TTL:      c.ttl,
		})
	}

	return nil, ErrInvalidType
}
