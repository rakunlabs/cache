package cache_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rakunlabs/cache"
	"github.com/rakunlabs/cache/store/memory"
)

// The alternatives are benchmark-only: global matches the previous locking
// strategy; plain intentionally permits duplicate loads for concurrent misses.
func benchmarkCache(b *testing.B, strategy string) (*cache.Cache[int, int], func(int, func() (int, error)) (int, error)) {
	b.Helper()
	ctx := b.Context()
	c, err := cache.New[int, int](ctx, memory.Store, cache.WithStoreConfig(&memory.Config{}))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = c.Close() })
	var mu sync.Mutex
	return c, func(key int, load func() (int, error)) (int, error) {
		if strategy == "PerKey" {
			return c.GetSet(ctx, key, load)
		}
		v, ok, err := c.Get(ctx, key)
		if err != nil || ok {
			return v, err
		}
		if strategy == "Global" {
			mu.Lock()
			defer mu.Unlock()
			v, ok, err = c.Get(ctx, key)
			if err != nil || ok {
				return v, err
			}
		}
		v, err = load()
		if err != nil {
			return v, err
		}
		return v, c.Set(ctx, key, v)
	}
}

func BenchmarkGetSet(b *testing.B) {
	for _, workload := range []string{"HitSerial", "HitParallel", "MissSerial", "MissParallel", "SlowMissParallel"} {
		b.Run(workload, func(b *testing.B) {
			for _, strategy := range []string{"PerKey", "Global", "Plain"} {
				b.Run(strategy, func(b *testing.B) {
					c, getSet := benchmarkCache(b, strategy)
					ctx := b.Context()
					load := func() (int, error) { return 42, nil }
					if workload == "SlowMissParallel" {
						load = func() (int, error) {
							time.Sleep(time.Millisecond)
							return 42, nil
						}
					}
					hit := workload == "HitSerial" || workload == "HitParallel"
					if err := c.Set(ctx, 0, 42); err != nil {
						b.Fatal(err)
					}
					// Miss timings include Delete to keep every call cold without
					// growing the cache or introducing capacity-eviction costs.
					operation := func(key int) {
						if !hit {
							if err := c.Delete(ctx, key); err != nil {
								b.Error(err)
							}
						}
						v, err := getSet(key, load)
						if err != nil || v != 42 {
							b.Errorf("GetSet = %d, %v", v, err)
						}
					}
					var workers atomic.Int64
					b.ReportAllocs()
					b.ResetTimer()
					if workload == "HitSerial" || workload == "MissSerial" {
						for range b.N {
							operation(0)
						}
					} else {
						b.RunParallel(func(pb *testing.PB) {
							key := int(workers.Add(1))
							if hit {
								key = 0
							}
							for pb.Next() {
								operation(key)
							}
						})
					}
				})
			}
		})
	}
}

// One op is an entire cold-key burst of 16 requests, not one request.
// Goroutine creation, the start barrier, and deletion are included for all variants.
func BenchmarkGetSetColdBurst(b *testing.B) {
	const clients = 16
	for _, strategy := range []string{"PerKey", "Global", "Plain"} {
		b.Run(strategy, func(b *testing.B) {
			c, getSet := benchmarkCache(b, strategy)
			ctx := b.Context()
			var loads atomic.Int64
			load := func() (int, error) {
				loads.Add(1)
				time.Sleep(time.Millisecond)
				return 42, nil
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := c.Delete(ctx, 0); err != nil {
					b.Fatal(err)
				}
				start := make(chan struct{})
				var ready, done sync.WaitGroup
				ready.Add(clients)
				done.Add(clients)
				for range clients {
					go func() {
						defer done.Done()
						ready.Done()
						<-start
						v, err := getSet(0, load)
						if err != nil || v != 42 {
							b.Errorf("GetSet = %d, %v", v, err)
						}
					}()
				}
				ready.Wait()
				close(start)
				done.Wait()
			}
			b.StopTimer()
			b.ReportMetric(float64(loads.Load())/float64(b.N), "loads/burst")
			b.ReportMetric(clients, "requests/burst")
		})
	}
}
