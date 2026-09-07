package memory

import (
	"container/heap"
	"container/list"
	"context"
	"maps"
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

	// CompactThreshold is the minimum unused item capacity before compaction.
	// After removals, even with TTL disabled, the map is rebuilt when the
	// peak minus its size exceeds this threshold and its size is below
	// half the peak (using integer division). The expiration index is also shrunk.
	// Default is 1000.
	CompactThreshold int `cfg:"compact_threshold" json:"compact_threshold"`
}

type item[K comparable, V any] struct {
	key        K
	value      V
	expiration time.Time
	element    *list.Element // reference to list element for O(1) removal
	heapIndex  int
}

// expirationHeap is independent of LRU order and holds exactly one entry per
// TTL-enabled item. Indices allow updates and removals without stale entries.
type expirationHeap[K comparable, V any] []*item[K, V]

func (h expirationHeap[K, V]) Len() int { return len(h) }

func (h expirationHeap[K, V]) Less(i, j int) bool {
	return h[i].expiration.Before(h[j].expiration)
}

func (h expirationHeap[K, V]) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIndex, h[j].heapIndex = i, j
}

func (h *expirationHeap[K, V]) Push(v any) {
	it := v.(*item[K, V])
	it.heapIndex = len(*h)
	*h = append(*h, it)
}

func (h *expirationHeap[K, V]) Pop() any {
	n := len(*h) - 1
	it := (*h)[n]
	(*h)[n] = nil // Do not retain removed values in the backing array.
	*h = (*h)[:n]
	it.heapIndex = -1
	return it
}

type Memory[K comparable, V any] struct {
	mu               sync.RWMutex
	items            map[K]*item[K, V]
	ll               *list.List // doubly-linked list for LRU order (front = MRU, back = LRU)
	expirations      expirationHeap[K, V]
	maxItems         int
	ttl              time.Duration
	janitorTicker    *time.Ticker
	stopJanitor      chan struct{}
	janitorDone      chan struct{}
	closeOnce        sync.Once
	peakItems        int // high-water mark for map compaction
	compactThreshold int // minimum gap from peak before compaction kicks in
}

// Store creates a cache. Canceling ctx stops background maintenance without
// invalidating cache operations, just like Close.
func Store[K comparable, V any](ctx context.Context, cfg *Config) (cache.Cacher[K, V], error) {
	if cfg == nil {
		cfg = &Config{
			MaxItems: DefaultMaxItems,
			TTL:      DefaultTTL,
		}
	} else {
		copyConfig := *cfg
		cfg = &copyConfig
	}

	if cfg.JanitorInterval <= 0 {
		cfg.JanitorInterval = DefaultJanitorInterval
	}

	if cfg.CompactThreshold <= 0 {
		cfg.CompactThreshold = DefaultCompactThreshold
	}

	m := &Memory[K, V]{
		items:            make(map[K]*item[K, V]),
		ll:               list.New(),
		maxItems:         cfg.MaxItems,
		ttl:              cfg.TTL,
		compactThreshold: cfg.CompactThreshold,
		stopJanitor:      make(chan struct{}),
		janitorDone:      make(chan struct{}),
	}

	// Only start janitor if TTL is enabled (TTL > 0)
	if cfg.TTL > 0 {
		m.janitorTicker = time.NewTicker(cfg.JanitorInterval)
		go m.janitor(ctx)
	} else {
		close(m.janitorDone)
	}

	return m, nil
}

// Close stops background maintenance and waits for it to finish. It does not
// invalidate cache operations. Close is safe to call repeatedly and concurrently.
func (m *Memory[K, V]) Close() error {
	m.closeOnce.Do(func() { close(m.stopJanitor) })
	<-m.janitorDone
	return nil
}

func (m *Memory[K, V]) janitor(ctx context.Context) {
	defer close(m.janitorDone)
	defer m.janitorTicker.Stop()
	for {
		select {
		case <-m.janitorTicker.C:
			m.cleanup()
		case <-m.stopJanitor:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (m *Memory[K, V]) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.removeExpired()
	m.compactIfNeeded()
}

// removeExpired requires mu to be held; expiration order is independent of LRU.
func (m *Memory[K, V]) removeExpired() {
	if m.ttl <= 0 {
		return
	}
	now := tummy.Now()
	for len(m.expirations) > 0 {
		it := m.expirations[0]
		if !it.expiration.Before(now) {
			break
		}
		m.removeItem(it)
	}
}

func (m *Memory[K, V]) removeItem(it *item[K, V]) {
	delete(m.items, it.key)
	m.ll.Remove(it.element)
	if m.ttl > 0 {
		heap.Remove(&m.expirations, it.heapIndex)
	}
}

// compactIfNeeded releases unused map and expiration-index capacity after a
// significant drop from the peak size.
func (m *Memory[K, V]) compactIfNeeded() {
	current := len(m.items)
	gap := m.peakItems - current
	// Compact when the wasted space exceeds the threshold AND
	// the current size is less than half of peak (meaningful relative waste).
	if gap > m.compactThreshold && current < m.peakItems/2 {
		newItems := make(map[K]*item[K, V], current)
		maps.Copy(newItems, m.items)
		m.items = newItems
		if m.ttl > 0 {
			expirations := make(expirationHeap[K, V], current)
			copy(expirations, m.expirations)
			m.expirations = expirations
		}
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
		m.compactIfNeeded()
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
			heap.Fix(&m.expirations, it.heapIndex)
		}
		m.moveToFront(it)

		return nil
	}

	// Reclaim expired entries before evicting a live LRU entry.
	if m.maxItems > 0 && len(m.items) >= m.maxItems {
		m.removeExpired()
		if len(m.items) >= m.maxItems {
			m.removeItem(m.ll.Back().Value.(*item[K, V]))
		}
		m.compactIfNeeded()
	}

	// New item
	it = &item[K, V]{
		key:   key,
		value: value,
	}
	if m.ttl > 0 {
		it.expiration = tummy.Now().Add(m.ttl)
		heap.Push(&m.expirations, it)
	}
	it.element = m.ll.PushFront(it)
	m.items[key] = it

	if n := len(m.items); n > m.peakItems {
		m.peakItems = n
	}

	return nil
}

func (m *Memory[K, V]) Delete(_ context.Context, key K) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	it, ok := m.items[key]
	if ok {
		m.removeItem(it)
		m.compactIfNeeded()
	}

	return nil
}
