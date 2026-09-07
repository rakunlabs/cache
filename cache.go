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

	m       sync.Mutex
	loading map[K]chan struct{}
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

// Close closes the store if it supports Close. Stores backed by caller-owned
// clients need not implement Close; in that case it is a no-op.
func (c *Cache[K, V]) Close() error {
	if closer, ok := c.Cacher.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

// GetSet loads missing values serially per key, without blocking other keys.
// Waiting callers can cancel via ctx. A failed load is retried by the next caller.
// fn must not recursively load the same key or create a cycle of key dependencies.
// Once fn starts, it must handle cancellation itself (for example by capturing ctx).
func (c *Cache[K, V]) GetSet(ctx context.Context, key K, fn func() (V, error)) (V, error) {
	for {
		if err := ctx.Err(); err != nil {
			var zero V
			return zero, err
		}
		value, ok, err := c.Get(ctx, key)
		if err != nil {
			return value, fmt.Errorf("failed to get key %v: %w", key, err)
		}
		if ok {
			return value, nil
		}

		c.m.Lock()
		if done, ok := c.loading[key]; ok {
			c.m.Unlock()
			select {
			case <-ctx.Done():
				var zero V
				return zero, ctx.Err()
			case <-done:
				continue
			}
		}
		if c.loading == nil {
			c.loading = make(map[K]chan struct{})
		}
		done := make(chan struct{})
		c.loading[key] = done
		c.m.Unlock()
		defer func() {
			c.m.Lock()
			delete(c.loading, key)
			close(done)
			c.m.Unlock()
		}()
		break
	}

	if err := ctx.Err(); err != nil {
		var zero V
		return zero, err
	}
	// Another load or Set may have completed before we reserved this key.
	value, ok, err := c.Get(ctx, key)
	if err != nil {
		return value, fmt.Errorf("failed to get key %v: %w", key, err)
	}
	if ok {
		return value, nil
	}

	value, err = fn()
	if err != nil {
		return value, fmt.Errorf("failed to execute fn for key %v: %w", key, err)
	}

	if err := ctx.Err(); err != nil {
		return value, err
	}
	if err := c.Set(ctx, key, value); err != nil {
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
