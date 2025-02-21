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
		cache.WithStoreConfig(memory.Config{
			MaxItems: 100,
			TTL:      10 * time.Minute,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	w := sync.WaitGroup{}

	for i := 0; i < 100; i++ {
		w.Add(1)
		go func(i int) {
			defer w.Done()

			if err := c.Set(t.Context(), fmt.Sprintf("key-%d", i), i); err != nil {
				t.Error(err)
			}
		}(i)
	}

	w.Wait()

	for i := 0; i < 100; i++ {
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
