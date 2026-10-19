package memory

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/rakunlabs/tummy"
)

func testMemory(t *testing.T, cfg Config) *Memory[int, int] {
	t.Helper()
	c, err := Store[int, int](t.Context(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	m := c.(*Memory[int, int])
	t.Cleanup(func() { _ = m.Close() })
	return m
}

// These tests are not parallel: tummy.Enable/Disable replace a global function.
func freezeTime(t *testing.T) {
	t.Helper()
	tummy.Enable()
	tummy.Pause()
	tummy.SetTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	t.Cleanup(func() {
		tummy.Resume()
		tummy.Disable()
	})
}

func checkOrder(t *testing.T, m *Memory[int, int], want ...int) {
	t.Helper()
	checkExpirationHeap(t, m)
	var got []int
	for e := m.ll.Front(); e != nil; e = e.Next() {
		it := e.Value.(*item[int, int])
		if m.items[it.key] != it || it.element != e {
			t.Fatal("map/list linkage mismatch")
		}
		got = append(got, it.key)
	}
	if !reflect.DeepEqual(got, want) || len(m.items) != len(want) {
		t.Fatalf("order = %v, map size = %d; want %v", got, len(m.items), want)
	}
}

func TestExpirationIndependentOfLRU(t *testing.T) {
	for _, capacity := range []bool{false, true} {
		t.Run(map[bool]string{false: "cleanup", true: "capacity"}[capacity], func(t *testing.T) {
			freezeTime(t)
			m := testMemory(t, Config{TTL: time.Hour, MaxItems: 3, JanitorInterval: time.Hour})
			_ = m.Close() // Drive maintenance explicitly.
			_ = m.Set(t.Context(), 1, 1)
			_ = m.Set(t.Context(), 2, 2)
			tummy.AddDuration(30 * time.Minute)
			_ = m.Set(t.Context(), 3, 3)
			_, _, _ = m.Get(t.Context(), 1)
			_, _, _ = m.Get(t.Context(), 2)
			checkOrder(t, m, 2, 1, 3)
			tummy.AddDuration(31 * time.Minute)
			if capacity {
				_ = m.Set(t.Context(), 4, 4)
				checkOrder(t, m, 4, 3)
			} else {
				m.cleanup()
				checkOrder(t, m, 3)
			}
		})
	}
}

func TestDeleteAndLRU(t *testing.T) {
	m := testMemory(t, Config{MaxItems: 3})
	for i := range 3 {
		_ = m.Set(t.Context(), i, i)
	}
	_, _, _ = m.Get(t.Context(), 0)
	_ = m.Set(t.Context(), 3, 3)
	checkOrder(t, m, 3, 0, 2)
	_ = m.Set(t.Context(), 2, 42)
	checkOrder(t, m, 2, 3, 0)
	if v, ok, err := m.Get(t.Context(), 2); v != 42 || !ok || err != nil {
		t.Fatalf("Get = %d, %v, %v", v, ok, err)
	}
	_ = m.Delete(t.Context(), 3)
	_ = m.Delete(t.Context(), 3)
	checkOrder(t, m, 2, 0)
	_ = m.Set(t.Context(), 4, 4)
	checkOrder(t, m, 4, 2, 0)
}

func TestCompactionAfterRemovals(t *testing.T) {
	for _, removal := range []string{"delete-TTL0", "get", "cleanup", "capacity"} {
		t.Run(removal, func(t *testing.T) {
			freezeTime(t)
			cfg := Config{CompactThreshold: 4, TTL: time.Hour, JanitorInterval: time.Hour}
			if removal == "delete-TTL0" {
				cfg.TTL = 0
			}
			if removal == "capacity" {
				cfg.MaxItems = 12
			}
			m := testMemory(t, cfg)
			_ = m.Close()
			for i := range 12 {
				_ = m.Set(t.Context(), i, i)
			}
			oldMap := m.items
			if cfg.TTL == 0 {
				m.cleanup()
				if len(m.items) != 12 {
					t.Fatal("cleanup removed entries with TTL disabled")
				}
			}
			tummy.AddDuration(30 * time.Minute)
			for i := 7; i < 12; i++ {
				_ = m.Set(t.Context(), i, i)
			}
			tummy.AddDuration(31 * time.Minute)
			switch removal {
			case "delete-TTL0", "get":
				for i := range 6 {
					if removal == "get" {
						_, _, _ = m.Get(t.Context(), i)
					} else {
						_ = m.Delete(t.Context(), i)
					}
				}
				if m.peakItems != 12 {
					t.Fatal("compacted at half peak")
				}
				if removal == "get" {
					_, _, _ = m.Get(t.Context(), 6)
				} else {
					_ = m.Delete(t.Context(), 6)
				}
			case "cleanup":
				m.cleanup()
			case "capacity":
				_ = m.Set(t.Context(), 12, 12)
			}
			wantPeak := 5
			if removal == "capacity" {
				wantPeak = 6
				checkOrder(t, m, 12, 11, 10, 9, 8, 7)
			} else {
				checkOrder(t, m, 11, 10, 9, 8, 7)
			}
			if m.peakItems != wantPeak {
				t.Fatalf("peak = %d, want %d", m.peakItems, wantPeak)
			}
			delete(oldMap, 7)
			if _, ok := m.items[7]; !ok {
				t.Fatal("compaction did not replace map")
			}
		})
	}
}

func TestCompactionThreshold(t *testing.T) {
	m := testMemory(t, Config{CompactThreshold: 7})
	for i := range 12 {
		_ = m.Set(t.Context(), i, i)
	}
	for i := range 7 {
		_ = m.Delete(t.Context(), i)
	}
	if m.peakItems != 12 {
		t.Fatal("compacted when gap equals threshold")
	}
	_ = m.Delete(t.Context(), 7)
	if m.peakItems != 4 {
		t.Fatal("did not compact above threshold")
	}
	checkOrder(t, m, 11, 10, 9, 8)
}

func TestJanitorLifecycle(t *testing.T) {
	for _, mode := range []string{"close", "cancel", "already-canceled", "TTL0"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "already-canceled" {
				cancel()
			}
			cfg := Config{TTL: time.Hour, JanitorInterval: time.Hour}
			if mode == "TTL0" {
				cfg.TTL = 0
			}
			c, err := Store[int, int](ctx, &cfg)
			if err != nil {
				t.Fatal(err)
			}
			m := c.(*Memory[int, int])
			t.Cleanup(func() { _ = m.Close() })
			if mode == "cancel" {
				cancel()
			}
			if mode != "close" {
				select {
				case <-m.janitorDone:
				case <-time.After(5 * time.Second):
					t.Fatal("janitor did not stop")
				}
			}
			var wg sync.WaitGroup
			for range 32 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := m.Close(); err != nil {
						t.Error(err)
					}
					select {
					case <-m.janitorDone:
					default:
						t.Error("Close returned before janitor stopped")
					}
				}()
			}
			wg.Wait()
			_ = m.Set(t.Context(), 1, 42)
			if v, ok, err := m.Get(t.Context(), 1); v != 42 || !ok || err != nil {
				t.Fatalf("Get after stop = %d, %v, %v", v, ok, err)
			}
			_ = m.Delete(t.Context(), 1)
			checkOrder(t, m)
		})
	}
}

func TestSharedConfig(t *testing.T) {
	cfg := Config{TTL: time.Hour, MaxItems: 7}
	want := cfg
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := Store[int, int](t.Context(), &cfg)
			if err != nil {
				t.Error(err)
				return
			}
			m := c.(*Memory[int, int])
			defer func() { _ = m.Close() }()
			if m.compactThreshold != DefaultCompactThreshold || m.ttl != want.TTL || m.maxItems != want.MaxItems {
				t.Error("unexpected configuration")
			}
		}()
	}
	wg.Wait()
	if cfg != want {
		t.Fatalf("caller config changed: %+v", cfg)
	}
}

func TestTake(t *testing.T) {
	freezeTime(t)
	ctx := t.Context()
	m := testMemory(t, Config{TTL: time.Minute})
	_ = m.Close()

	if _, ok, err := m.Take(ctx, 1); err != nil || ok {
		t.Fatalf("missing Take = %v, %v", ok, err)
	}

	_ = m.Set(ctx, 1, 10)
	_ = m.Set(ctx, 2, 20)
	if v, ok, err := m.Take(ctx, 1); err != nil || !ok || v != 10 {
		t.Fatalf("Take = %d, %v, %v; want 10, true, nil", v, ok, err)
	}
	if _, ok, _ := m.Take(ctx, 1); ok {
		t.Fatal("second Take found the key")
	}
	checkOrder(t, m, 2)
	checkExpirationHeap(t, m)

	tummy.AddDuration(2 * time.Minute)
	if _, ok, err := m.Take(ctx, 2); err != nil || ok {
		t.Fatalf("expired Take = %v, %v; want miss", ok, err)
	}
	if len(m.items) != 0 || m.ll.Len() != 0 {
		t.Fatal("expired Take left the item behind")
	}
	checkExpirationHeap(t, m)
}

func TestTakeConcurrent(t *testing.T) {
	ctx := t.Context()
	m := testMemory(t, Config{})
	_ = m.Set(ctx, 1, 1)

	var wg sync.WaitGroup
	var mu sync.Mutex
	winners := 0
	for range 32 {
		wg.Go(func() {
			if _, ok, err := m.Take(ctx, 1); err != nil {
				t.Error(err)
			} else if ok {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if winners != 1 {
		t.Fatalf("winners = %d, want 1", winners)
	}
}
