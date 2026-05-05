package memory

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/rakunlabs/cache"
	"github.com/rakunlabs/tummy"
)

var (
	DefaultMaxItems         = 1_000
	DefaultTTL              = 10 * time.Minute
	DefaultJanitorInterval  = 1 * time.Minute
	DefaultCompactThreshold = 1_000
)

type Config struct {
	MaxItems int           `cfg:"max_items" json:"max_items"`
	TTL      time.Duration `cfg:"ttl"       json:"ttl"`

	JanitorInterval time.Duration `cfg:"janitor_interval" json:"janitor_interval"`

	// CompactThreshold is the minimum peak item count before map compaction
	// is considered. When the map shrinks to ≤25% of its peak size and the
	// peak exceeded this threshold, the map is rebuilt to release memory.
	// Default is 1000.
	CompactThreshold int `cfg:"compact_threshold" json:"compact_threshold"`
}

type item[K comparable, V any] struct {
	key        K
	value      V
	expiration time.Time
	element    *list.Element // reference to list element for O(1) removal
}

type Memory[K comparable, V any] struct {
	mu               sync.RWMutex
	items            map[any]*item[K, V]
	ll               *list.List // doubly-linked list for LRU order (front = MRU, back = LRU)
	maxItems         int
	ttl              time.Duration
	janitorTicker    *time.Ticker
	stopJanitor      chan struct{}
	peakItems        int // high-water mark for map compaction
	compactThreshold int // minimum peak before compaction kicks in
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

	if cfg.CompactThreshold <= 0 {
		cfg.CompactThreshold = DefaultCompactThreshold
	}

	m := &Memory[K, V]{
		items:            make(map[any]*item[K, V]),
		ll:               list.New(),
		maxItems:         cfg.MaxItems,
		ttl:              cfg.TTL,
		compactThreshold: cfg.CompactThreshold,
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

	now := tummy.Now()
	for m.ll.Len() > 0 {
		e := m.ll.Back()
		it := e.Value.(*item[K, V])
		if !it.expiration.Before(now) {
			break
		}
		m.removeItem(it)
	}

	m.compactIfNeeded()
}

func (m *Memory[K, V]) removeItem(it *item[K, V]) {
	delete(m.items, it.key)
	m.ll.Remove(it.element)
}

// compactIfNeeded rebuilds the map when it has shrunk significantly below
// its peak size, releasing memory held by unused hash buckets.
func (m *Memory[K, V]) compactIfNeeded() {
	current := len(m.items)
	if m.peakItems >= m.compactThreshold && current <= m.peakItems/4 {
		newItems := make(map[any]*item[K, V], current)
		for k, v := range m.items {
			newItems[k] = v
		}
		m.items = newItems
		m.peakItems = current
	}
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
	if m.ttl > 0 && tummy.Now().After(it.expiration) {
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
			it.expiration = tummy.Now().Add(m.ttl)
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
		it.expiration = tummy.Now().Add(m.ttl)
	}
	it.element = m.ll.PushFront(it)
	m.items[key] = it

	if n := len(m.items); n > m.peakItems {
		m.peakItems = n
	}

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
