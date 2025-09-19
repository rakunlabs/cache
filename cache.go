package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var ErrStoreNotExist = errors.New("store does not exist")

type Store[K comparable, V any, C any] func(ctx context.Context, config C) (Cacher[K, V], error)

// //////////////////////////////////////////////////////////////////////////

type Cache[K comparable, V any] struct {
	Cacher[K, V]

	m sync.Mutex
}

type Cacher[K comparable, V any] interface {
	Get(ctx context.Context, key K) (V, bool, error)
	Set(ctx context.Context, key K, value V) error
	Delete(ctx context.Context, key K) error
}

func New[K comparable, V any, C any](ctx context.Context, store Store[K, V, C], opts ...Option[C]) (*Cache[K, V], error) {
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

	return &Cache[K, V]{Cacher: cacher}, nil
}

func (c *Cache[K, V]) GetSet(ctx context.Context, key K, fn func() (V, error)) (V, error) {
	value, ok, err := c.Cacher.Get(ctx, key)
	if err != nil {
		return value, fmt.Errorf("failed to get key %v: %w", key, err)
	}
	if ok {
		return value, nil
	}

	c.m.Lock()
	defer c.m.Unlock()
	// Double check
	value, ok, err = c.Cacher.Get(ctx, key)
	if err != nil {
		return value, fmt.Errorf("failed to get key %v: %w", key, err)
	}
	if ok {
		return value, nil
	}

	// Call the function to get the value

	value, err = fn()
	if err != nil {
		return value, fmt.Errorf("failed to execute fn for key %v: %w", key, err)
	}

	if err := c.Cacher.Set(ctx, key, value); err != nil {
		return value, fmt.Errorf("failed to set key %v: %w", key, err)
	}

	return value, nil
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
