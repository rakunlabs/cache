package cache

import (
	"context"
	"errors"
	"fmt"
)

var ErrStoreNotExist = errors.New("store does not exist")

type Store[K comparable, V any, C any] func(ctx context.Context, config C) (Cacher[K, V], error)

// //////////////////////////////////////////////////////////////////////////

type Cacher[K comparable, V any] interface {
	Get(ctx context.Context, key K) (V, bool, error)
	Set(ctx context.Context, key K, value V) error
	Delete(ctx context.Context, key K) error
}

func New[K comparable, V any, C any](ctx context.Context, store Store[K, V, C], opts ...Option[C]) (Cacher[K, V], error) {
	if store == nil {
		return nil, ErrStoreNotExist
	}

	o := &option[C]{}
	for _, opt := range opts {
		opt(o)
	}

	cacher, err := store(ctx, o.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to get cacher: %w", err)
	}

	return cacher, nil
}

// //////////////////////////////////////////////////////////////////////////

type option[T any] struct {
	Config T
}

type Option[T any] func(*option[T])

func WithStoreConfig[T any](cfg T) Option[T] {
	return func(o *option[T]) {
		o.Config = cfg
	}
}
