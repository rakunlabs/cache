package memory_test

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/rakunlabs/cache"
	"github.com/rakunlabs/cache/store/memory"
)

func Test_Memory(t *testing.T) {
	c, err := cache.New[string, int](t.Context(),
		memory.Store,
		cache.WithStoreConfig(&memory.Config{
			MaxItems: 100,
			TTL:      10 * time.Minute,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	w := sync.WaitGroup{}

	for i := range 100 {
		w.Add(1)
		go func(i int) {
			defer w.Done()

			if err := c.Set(t.Context(), fmt.Sprintf("key-%d", i), i); err != nil {
				t.Error(err)
			}
		}(i)
	}

	w.Wait()

	for i := range 100 {
		w.Add(1)
		go func(i int) {
			defer w.Done()
			v, ok, err := c.Get(t.Context(), fmt.Sprintf("key-%d", i))
			if err != nil {
				t.Error(err)
			}

			if !ok {
				t.Error("key not found")
			}

			if v != i {
				t.Error("value not match")
			}
		}(i)
	}
}

func Test_MemoryGetSet(t *testing.T) {
	c, err := cache.New[string, int](t.Context(),
		memory.Store,
		cache.WithStoreConfig(&memory.Config{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	count := 0

	wg := sync.WaitGroup{}

	for range 1000 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.GetSet(t.Context(), "key-1", func() (int, error) {
				count++
				return 42, nil
			})
			if err != nil {
				t.Error(err)
			}
			if v != 42 {
				t.Error("value not match")
			}
		}()
	}

	wg.Wait()

	if count != 1 {
		t.Errorf("function called %d times, want 1", count)
	}

	// Verify the value is set
	v, ok, err := c.Get(t.Context(), "key-1")
	if err != nil {
		t.Error(err)
	}
	if !ok {
		t.Error("key not found after GetSet")
	}
	if v != 42 {
		t.Error("value not match after GetSet")
	}
}

func Test_Memory_NoTTL_NoLimit(t *testing.T) {
	// Test with TTL=0 (no expiration) and MaxItems=0 (no limit)
	c, err := cache.New[string, int](t.Context(),
		memory.Store,
		cache.WithStoreConfig(&memory.Config{
			MaxItems: 0, // No limit
			TTL:      0, // No expiration
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Set many items (more than default limit)
	for i := 0; i < 2000; i++ {
		if err := c.Set(t.Context(), fmt.Sprintf("key-%d", i), i); err != nil {
			t.Error(err)
		}
	}

	// All items should still be there (no eviction)
	for i := range 2000 {
		v, ok, err := c.Get(t.Context(), fmt.Sprintf("key-%d", i))
		if err != nil {
			t.Error(err)
		}
		if !ok {
			t.Errorf("key-%d not found", i)
		}
		if v != i {
			t.Errorf("value mismatch for key-%d: got %d, want %d", i, v, i)
		}
	}

	// Wait a bit and check again - items should still be there (no expiration)
	time.Sleep(100 * time.Millisecond)
	for i := range 10 {
		v, ok, err := c.Get(t.Context(), fmt.Sprintf("key-%d", i))
		if err != nil {
			t.Error(err)
		}
		if !ok {
			t.Errorf("key-%d not found after wait", i)
		}
		if v != i {
			t.Errorf("value mismatch for key-%d after wait: got %d, want %d", i, v, i)
		}
	}
}

func Test_Memory_Compaction_ReleasesMemory(t *testing.T) {
	// This test demonstrates that map compaction releases memory after
	// a large number of items are inserted and then deleted.
	const (
		itemCount        = 100_000
		compactThreshold = 1_000
	)

	c, err := cache.New[string, []byte](t.Context(),
		memory.Store,
		cache.WithStoreConfig(&memory.Config{
			MaxItems:         0, // no limit
			TTL:              50 * time.Millisecond,
			JanitorInterval:  25 * time.Millisecond,
			CompactThreshold: compactThreshold,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Insert many items with large values to make memory usage obvious
	value := make([]byte, 256)
	for i := range itemCount {
		if err := c.Set(t.Context(), fmt.Sprintf("key-%d", i), value); err != nil {
			t.Fatal(err)
		}
	}

	// Measure memory at peak
	runtime.GC()
	var memPeak runtime.MemStats
	runtime.ReadMemStats(&memPeak)
	t.Logf("Peak HeapAlloc: %d MB", memPeak.HeapAlloc/(1024*1024))

	// Wait for TTL expiration + janitor cleanup + compaction
	time.Sleep(200 * time.Millisecond)

	// Force GC and measure memory after compaction
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)
	t.Logf("After compaction HeapAlloc: %d MB", memAfter.HeapAlloc/(1024*1024))

	// After compaction, memory should have dropped significantly.
	// We expect at least 50% reduction from peak since all items expired.
	if memAfter.HeapAlloc > memPeak.HeapAlloc/2 {
		t.Errorf("memory not released after compaction: peak=%d bytes, after=%d bytes (expected at least 50%% reduction)",
			memPeak.HeapAlloc, memAfter.HeapAlloc)
	}
}
