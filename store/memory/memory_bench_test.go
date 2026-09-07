package memory

import (
	"fmt"
	"testing"
	"time"
)

// Benchmarks use the actual clock, not freezeTime. Keep each run (including
// population) below one hour so TTL entries remain live. Close immediately
// stops and joins the janitor without invalidating operations, isolating
// foreground work and explicit cleanup from background maintenance.
func benchmarkMemory(b *testing.B, size int, ttl time.Duration) *Memory[int, int] {
	b.Helper()
	c, err := Store[int, int](b.Context(), &Config{MaxItems: size, TTL: ttl})
	if err != nil {
		b.Fatal(err)
	}
	m := c.(*Memory[int, int])
	if err := m.Close(); err != nil {
		b.Fatal(err)
	}
	ctx := b.Context()
	for key := 0; key < size; key++ {
		if err := m.Set(ctx, key, key); err != nil {
			b.Fatal(err)
		}
	}
	return m
}

func BenchmarkMemoryGet(b *testing.B) {
	for _, size := range []int{1_000, 100_000} {
		for _, ttl := range []time.Duration{0, time.Hour} {
			b.Run(fmt.Sprintf("Capacity=%d/TTL=%s", size, ttl), func(b *testing.B) {
				m := benchmarkMemory(b, size, ttl)
				ctx := b.Context()
				b.ReportAllocs()
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					// Each worker cycles through all populated keys independently;
					// no shared counter or per-operation atomic is needed.
					key := 0
					for pb.Next() {
						value, ok, err := m.Get(ctx, key)
						if err != nil || !ok || value != key {
							b.Errorf("Get(%d) = %d, %v, %v; want %d, true, nil", key, value, ok, err, key)
							return
						}
						key++
						if key == size {
							key = 0
						}
					}
				})
			})
		}
	}
}

func BenchmarkMemorySetFull(b *testing.B) {
	for _, size := range []int{1_000, 10_000, 100_000} {
		for _, ttl := range []time.Duration{0, time.Hour} {
			b.Run(fmt.Sprintf("Size=%d/TTL=%s", size, ttl), func(b *testing.B) {
				if b.N > int(^uint(0)>>1)-size {
					b.Fatal("iteration count would overflow unique int keys")
				}
				m := benchmarkMemory(b, size, ttl)
				ctx := b.Context()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					// Never reuse a key: every insertion evicts at full capacity.
					// With TTL enabled, this also checks for expired entries first.
					key := size + i
					if err := m.Set(ctx, key, key); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				if len(m.items) != size {
					b.Fatalf("size = %d; want %d", len(m.items), size)
				}
				key := size + b.N - 1
				if value, ok, err := m.Get(ctx, key); err != nil || !ok || value != key {
					b.Fatalf("Get(%d) = %d, %v, %v; want %d, true, nil", key, value, ok, err, key)
				}
			})
		}
	}
}

func BenchmarkMemoryCleanup(b *testing.B) {
	for _, size := range []int{1_000, 10_000, 100_000} {
		b.Run(fmt.Sprintf("Size=%d/TTL=1h0m0s/AllLive", size), func(b *testing.B) {
			m := benchmarkMemory(b, size, time.Hour)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// All entries stay live; measure the no-expiration cleanup path.
				m.cleanup()
			}
			b.StopTimer()
			if len(m.items) != size {
				b.Fatalf("cleanup retained %d entries; want %d live entries", len(m.items), size)
			}
		})
	}
}
