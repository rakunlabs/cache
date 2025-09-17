package example_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/worldline-go/cache"
	"github.com/worldline-go/cache/store/memory"
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
