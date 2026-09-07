package redis_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rakunlabs/cache"
	store "github.com/rakunlabs/cache/store/redis"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcvalkey "github.com/testcontainers/testcontainers-go/modules/valkey"
)

func TestValkey(t *testing.T) {
	if testing.Short() {
		t.Skip("Valkey integration test requires Docker")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	container, err := tcvalkey.Run(ctx, "valkey/valkey:8.1.3-alpine")
	if container != nil {
		t.Cleanup(func() {
			if err := testcontainers.TerminateContainer(container); err != nil {
				t.Errorf("terminate Valkey: %v", err)
			}
		})
	}
	if err != nil {
		t.Fatalf("start Valkey (Docker must be running): %v", err)
	}
	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	opts, err := redis.ParseURL(uri)
	if err != nil {
		t.Fatal(err)
	}
	opts.DialTimeout = time.Second
	opts.ReadTimeout = time.Second
	opts.WriteTimeout = time.Second
	opts.ContextTimeoutEnabled = true
	opts.MaxRetries = -1
	client := redis.NewClient(opts)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	newCache := func(t *testing.T, ttl time.Duration) *cache.Cache[string, string] {
		t.Helper()
		c, err := cache.New(t.Context(), store.Store(client), cache.WithStoreConfig(store.Config{TTL: ttl}))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	t.Run("round-trip", func(t *testing.T) {
		c := newCache(t, 0)
		for _, value := range []string{"value", "", "binary\x00\xffvalue"} {
			if err := c.Set(ctx, t.Name(), value); err != nil {
				t.Fatal(err)
			}
			if got, ok, err := c.Get(ctx, t.Name()); err != nil || !ok || got != value {
				t.Fatalf("Get = %q, %v, %v; want %q, true, nil", got, ok, err, value)
			}
		}
	})
	t.Run("miss-and-delete", func(t *testing.T) {
		c := newCache(t, 0)
		key := t.Name()
		if got, ok, err := c.Get(ctx, key); err != nil || ok || got != "" {
			t.Fatalf("missing Get = %q, %v, %v", got, ok, err)
		}
		if err := c.Set(ctx, key, "value"); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := c.Delete(ctx, key); err != nil {
				t.Fatal(err)
			}
			if got, ok, err := c.Get(ctx, key); err != nil || ok || got != "" {
				t.Fatalf("deleted Get = %q, %v, %v", got, ok, err)
			}
		}
	})
	t.Run("expiration", func(t *testing.T) {
		const ttl = time.Second
		c := newCache(t, ttl)
		key := t.Name()
		if err := c.Set(ctx, key, "value"); err != nil {
			t.Fatal(err)
		}
		if remaining, err := client.PTTL(ctx, key).Result(); err != nil || remaining <= 0 || remaining > ttl {
			t.Fatalf("PTTL = %v, %v", remaining, err)
		}
		deadline, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			got, ok, err := c.Get(deadline, key)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				if got != "" {
					t.Fatalf("expired value = %q", got)
				}
				break
			}
			select {
			case <-deadline.Done():
				t.Fatal("key did not expire")
			case <-ticker.C:
			}
		}
	})
	t.Run("refresh-and-remove-TTL", func(t *testing.T) {
		c := newCache(t, time.Minute)
		key := t.Name()
		// Seed a longer deadline so a rewrite must replace it with the cache TTL.
		if err := client.Set(ctx, key, "old", time.Hour).Err(); err != nil {
			t.Fatal(err)
		}
		if err := c.Set(ctx, key, "new"); err != nil {
			t.Fatal(err)
		}
		if ttl, err := client.PTTL(ctx, key).Result(); err != nil || ttl <= 0 || ttl > time.Minute {
			t.Fatalf("refreshed PTTL = %v, %v", ttl, err)
		}
		if err := newCache(t, 0).Set(ctx, key, "persistent"); err != nil {
			t.Fatal(err)
		}
		// go-redis returns Redis's -1 sentinel directly as a time.Duration.
		if ttl, err := client.PTTL(ctx, key).Result(); err != nil || ttl != -1 {
			t.Fatalf("persistent PTTL = %v, %v", ttl, err)
		}
	})
	t.Run("wrong-type", func(t *testing.T) {
		key := t.Name()
		if err := client.LPush(ctx, key, "list-value").Err(); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := newCache(t, 0).Get(ctx, key); err == nil || ok || !strings.Contains(err.Error(), "WRONGTYPE") {
			t.Fatalf("wrong-type Get: found=%v, err=%v", ok, err)
		}
	})
	t.Run("get-set", func(t *testing.T) {
		c := newCache(t, 0)
		calls := 0
		for range 2 {
			value, err := c.GetSet(ctx, t.Name(), func() (string, error) {
				calls++
				return "loaded", nil
			})
			if err != nil || value != "loaded" {
				t.Fatalf("GetSet = %q, %v", value, err)
			}
		}
		if calls != 1 {
			t.Fatalf("loader calls = %d, want 1", calls)
		}
	})
}
