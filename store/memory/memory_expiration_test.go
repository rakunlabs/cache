package memory

import (
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/rakunlabs/tummy"
)

// Call only while cache operations and background maintenance are stopped.
func checkExpirationHeap(t *testing.T, m *Memory[int, int]) {
	t.Helper()
	h := m.expirations
	if m.ttl <= 0 {
		if len(h) != 0 || cap(h) != 0 {
			t.Fatal("TTL-disabled cache allocated an expiration heap")
		}
		return
	}
	if len(h) != len(m.items) {
		t.Fatalf("heap size = %d, map size = %d", len(h), len(m.items))
	}
	for i, it := range h {
		if it == nil || it.heapIndex != i || m.items[it.key] != it || it.element == nil || it.element.Value != it {
			t.Fatalf("heap/map/list linkage mismatch at index %d", i)
		}
		if i > 0 && it.expiration.Before(h[(i-1)/2].expiration) {
			t.Fatalf("child %d expires before parent", i)
		}
	}
	for i, it := range h[:cap(h)][len(h):] {
		if it != nil {
			t.Fatalf("heap retains removed item at index %d", len(h)+i)
		}
	}
}

func TestExpirationHeapRefresh(t *testing.T) {
	freezeTime(t)
	m := testMemory(t, Config{TTL: time.Hour})
	_ = m.Close()
	for _, step := range []struct {
		advance   time.Duration
		key, root int
	}{
		{0, 1, 1}, {time.Minute, 2, 1}, {time.Minute, 1, 2},
		{-3 * time.Minute, 3, 3}, // Insertion before the current root.
		{-time.Minute, 1, 1},     // Refresh must Fix upwards, not only downwards.
	} {
		tummy.AddDuration(step.advance)
		_ = m.Set(t.Context(), step.key, step.key)
		checkExpirationHeap(t, m)
		if m.expirations[0].key != step.root {
			t.Fatalf("root = %d, want %d", m.expirations[0].key, step.root)
		}
	}
	for i := range 300 {
		tummy.AddDuration(time.Second)
		_ = m.Set(t.Context(), i%3+1, i)
		checkExpirationHeap(t, m)
		if len(m.expirations) != 3 {
			t.Fatal("refresh changed heap size")
		}
	}
}

func TestExpirationHeapNonRootRemoval(t *testing.T) {
	for _, eviction := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete", true: "LRU"}[eviction], func(t *testing.T) {
			freezeTime(t)
			m := testMemory(t, Config{TTL: time.Hour, MaxItems: 3})
			_ = m.Close()
			for key := 1; key <= 3; key++ {
				tummy.AddDuration(time.Minute)
				_ = m.Set(t.Context(), key, key)
				checkExpirationHeap(t, m)
			}
			old := m.items[2]
			if old.heapIndex == 0 {
				t.Fatal("expected non-root victim")
			}
			if eviction {
				_, _, _ = m.Get(t.Context(), 1) // Key 2 is LRU, but key 1 expires first.
				checkExpirationHeap(t, m)
				_ = m.Set(t.Context(), 4, 4)
			} else {
				_ = m.Delete(t.Context(), 2)
			}
			checkExpirationHeap(t, m)
			if m.items[2] != nil || old.heapIndex != -1 {
				t.Fatal("victim was not removed from map and heap")
			}
			_ = m.Set(t.Context(), 2, 42)
			checkExpirationHeap(t, m)
			if m.items[2] == old || m.items[2].value != 42 {
				t.Fatal("reinsertion reused removed item or lost value")
			}
		})
	}
}

func TestExpirationHeapBoundary(t *testing.T) {
	freezeTime(t)
	m := testMemory(t, Config{TTL: time.Hour})
	_ = m.Close()
	_ = m.Set(t.Context(), 1, 42)
	checkExpirationHeap(t, m)
	old := m.items[1]
	tummy.AddDuration(time.Hour)
	m.cleanup()
	checkExpirationHeap(t, m)
	if v, ok, err := m.Get(t.Context(), 1); v != 42 || !ok || err != nil {
		t.Fatalf("Get at expiration = %d, %v, %v", v, ok, err)
	}
	checkExpirationHeap(t, m)
	tummy.AddDuration(time.Nanosecond)
	if v, ok, err := m.Get(t.Context(), 1); v != 0 || ok || err != nil {
		t.Fatalf("expired Get = %d, %v, %v", v, ok, err)
	}
	checkExpirationHeap(t, m)
	if len(m.items) != 0 || old.heapIndex != -1 {
		t.Fatal("expired Get did not remove item")
	}
}

func TestExpirationHeapCompaction(t *testing.T) {
	freezeTime(t)
	m := testMemory(t, Config{TTL: time.Hour, CompactThreshold: 4})
	_ = m.Close()
	for i := range 128 {
		_ = m.Set(t.Context(), i, i)
		checkExpirationHeap(t, m)
	}
	old := m.expirations[:cap(m.expirations)]
	tummy.AddDuration(time.Minute)
	_ = m.Set(t.Context(), 127, 127)
	checkExpirationHeap(t, m)
	tummy.AddDuration(time.Hour)
	m.cleanup()
	checkExpirationHeap(t, m)
	if len(m.items) != 1 || m.items[127] == nil || cap(m.expirations) != 1 || m.peakItems != 1 {
		t.Fatalf("compaction: items=%d, heap capacity=%d, peak=%d", len(m.items), cap(m.expirations), m.peakItems)
	}
	if &old[0] == &m.expirations[0] {
		t.Fatal("compaction did not replace backing array")
	}
	for _, it := range old {
		if it != nil && it != m.items[127] {
			t.Fatal("old backing array retains an expired item")
		}
	}
	_ = m.Delete(t.Context(), 127)
	checkExpirationHeap(t, m)
	_ = m.Set(t.Context(), 127, 42)
	checkExpirationHeap(t, m)
}

func TestExpirationHeapMixedOperations(t *testing.T) {
	freezeTime(t)
	m := testMemory(t, Config{TTL: time.Minute, MaxItems: 64, CompactThreshold: 4})
	_ = m.Close()
	rng := rand.New(rand.NewPCG(1, 2))
	for range 2_000 {
		tummy.AddDuration(time.Duration(rng.IntN(15)-5) * time.Second)
		key := rng.IntN(128)
		switch rng.IntN(4) {
		case 0:
			_ = m.Set(t.Context(), key, key)
		case 1:
			_ = m.Delete(t.Context(), key)
		case 2:
			it, exists := m.items[key]
			want := exists && !it.expiration.Before(tummy.Now())
			v, ok, err := m.Get(t.Context(), key)
			if err != nil || ok != want || (ok && v != key) {
				t.Fatalf("Get(%d) = %d, %v, %v; want hit=%v", key, v, ok, err, want)
			}
		case 3:
			now := tummy.Now()
			live := make(map[int]bool)
			for key, it := range m.items {
				if !it.expiration.Before(now) {
					live[key] = true
				}
			}
			m.cleanup()
			if len(m.items) != len(live) {
				t.Fatal("cleanup differs from full-scan reference")
			}
			for key := range m.items {
				if !live[key] {
					t.Fatal("cleanup retained an expired entry")
				}
			}
		}
		checkExpirationHeap(t, m)
	}
}

func TestExpirationHeapConcurrentMaintenance(t *testing.T) {
	m := testMemory(t, Config{
		TTL: time.Microsecond, JanitorInterval: time.Microsecond,
		MaxItems: 128, CompactThreshold: 4,
	})
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 500 {
				key := (i + worker*17) % 128
				_ = m.Set(t.Context(), key, key)
				v, ok, err := m.Get(t.Context(), key)
				if err != nil || (ok && v != key) {
					t.Errorf("Get(%d) = %d, %v, %v", key, v, ok, err)
				}
				_ = m.Delete(t.Context(), (key+1)%128)
				if i%13 == 0 {
					m.cleanup()
				}
			}
		}()
	}
	wg.Wait()
	_ = m.Close()
	checkExpirationHeap(t, m)
}
