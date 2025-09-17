package memory

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/worldline-go/cache"
)

var (
	DefaultMaxItems        = 1_000
	DefaultTTL             = 10 * time.Minute
	DefaultJanitorInterval = 1 * time.Minute
)

type Config struct {
	MaxItems int           `cfg:"max_items" json:"max_items"`
	TTL      time.Duration `cfg:"ttl"       json:"ttl"`

	JanitorInterval time.Duration `cfg:"janitor_interval" json:"janitor_interval"`
}

type item[K comparable, V any] struct {
	key        K
	value      V
	expiration time.Time
	element    *list.Element // reference to list element for O(1) removal
}

type Memory[K comparable, V any] struct {
	mu            sync.RWMutex
	items         map[any]*item[K, V]
	ll            *list.List // doubly-linked list for LRU order (front = MRU, back = LRU)
	maxItems      int
	ttl           time.Duration
	janitorTicker *time.Ticker
	stopJanitor   chan struct{}
}

func Store[K comparable, V any](_ context.Context, cfg *Config) (cache.Cacher[K, V], error) {
	if cfg == nil {
		cfg = &Config{
			MaxItems: DefaultMaxItems,
			TTL:      DefaultTTL,
		}
	}

	if cfg.JanitorInterval <= 0 {
		cfg.JanitorInterval = DefaultJanitorInterval
	}

	m := &Memory[K, V]{
		items:    make(map[any]*item[K, V]),
		ll:       list.New(),
		maxItems: cfg.MaxItems,
		ttl:      cfg.TTL,
	}

	// Only start janitor if TTL is enabled (TTL > 0)
	if cfg.TTL > 0 {
		m.stopJanitor = make(chan struct{})
		m.janitorTicker = time.NewTicker(cfg.JanitorInterval)
		go m.janitor()
	}

	return m, nil
}

func (m *Memory[K, V]) janitor() {
	for {
		select {
		case <-m.janitorTicker.C:
			m.cleanup()
		case <-m.stopJanitor:
			m.janitorTicker.Stop()

			return
		}
	}
}

func (m *Memory[K, V]) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for m.ll.Len() > 0 {
		e := m.ll.Back()
		it := e.Value.(*item[K, V])
		if !it.expiration.Before(now) {
			break
		}
		m.removeItem(it)
	}
}

func (m *Memory[K, V]) removeItem(it *item[K, V]) {
	delete(m.items, it.key)
	m.ll.Remove(it.element)
}

func (m *Memory[K, V]) moveToFront(it *item[K, V]) {
	m.ll.MoveToFront(it.element)
}

func (m *Memory[K, V]) Get(_ context.Context, key K) (V, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	it, ok := m.items[key]
	if !ok {
		var zero V

		return zero, false, nil
	}

	// Check expiration only if TTL is enabled
	if m.ttl > 0 && time.Now().After(it.expiration) {
		m.removeItem(it)
		var zero V

		return zero, false, nil
	}

	m.moveToFront(it)

	return it.value, true, nil
}

func (m *Memory[K, V]) Set(_ context.Context, key K, value V) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	it, ok := m.items[key]
	if ok {
		it.value = value
		if m.ttl > 0 {
			it.expiration = time.Now().Add(m.ttl)
		}
		m.moveToFront(it)

		return nil
	}

	// New item
	it = &item[K, V]{
		key:   key,
		value: value,
	}
	if m.ttl > 0 {
		it.expiration = time.Now().Add(m.ttl)
	}
	it.element = m.ll.PushFront(it)
	m.items[key] = it

	// Evict if over capacity (only if maxItems is set)
	if m.maxItems > 0 && len(m.items) > m.maxItems {
		e := m.ll.Back()
		if e != nil {
			m.removeItem(e.Value.(*item[K, V]))
		}
	}

	return nil
}

func (m *Memory[K, V]) Delete(_ context.Context, key K) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	it, ok := m.items[key]
	if ok {
		m.removeItem(it)
	}

	return nil
}
