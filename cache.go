package cache

import (
	"context"
	"errors"
	"fmt"

	"github.com/worldline-go/struct2"
)

var ErrStoreNotExist = errors.New("store does not exist")

type Store[K comparable, V any, C any] func(ctx context.Context, config C) (Cacher[K, V], error)

// //////////////////////////////////////////////////////////////////////////

type Cacher[K comparable, V any] interface {
	Get(ctx context.Context, key K) (V, bool, error)
	Set(ctx context.Context, key K, value V) error
}

func New[K comparable, V any, C any](ctx context.Context, store Store[K, V, C], opts ...Option) (Cacher[K, V], error) {
	if store == nil {
		return nil, ErrStoreNotExist
	}

	o := &option{
		Config: make(map[string]interface{}),
	}
	for _, opt := range opts {
		opt(o)
	}

	var cfg C

	decoder := struct2.Decoder{TagName: "cfg"}
	if err := decoder.Decode(o, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	cacher, err := store(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get cacher: %w", err)
	}

	return cacher, nil
}
