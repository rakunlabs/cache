package redis_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	store "github.com/rakunlabs/cache/store/redis"
	"github.com/redis/go-redis/v9"
)

// Embedding leaves unexpected client calls unimplemented, without a Redis server.
type stubClient struct {
	redis.UniversalClient
	get func(context.Context, string) *redis.StringCmd
	set func(context.Context, string, interface{}, time.Duration) *redis.StatusCmd
	del func(context.Context, ...string) *redis.IntCmd
}

func (s *stubClient) Get(ctx context.Context, key string) *redis.StringCmd {
	return s.get(ctx, key)
}

func (s *stubClient) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.StatusCmd {
	return s.set(ctx, key, value, ttl)
}

func (s *stubClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	return s.del(ctx, keys...)
}

func TestGet(t *testing.T) {
	clientErr := errors.New("get failed")
	for _, tt := range []struct {
		name    string
		value   string
		err     error
		want    string
		found   bool
		wantErr error
	}{
		{name: "hit", value: "value", want: "value", found: true},
		{name: "empty hit", found: true},
		{name: "miss", err: redis.Nil},
		{name: "wrapped miss", err: fmt.Errorf("get: %w", redis.Nil)},
		{name: "error", value: "ignored", err: clientErr, wantErr: clientErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			calls := 0
			client := &stubClient{get: func(gotCtx context.Context, key string) *redis.StringCmd {
				calls++
				if gotCtx != ctx || key != "key" {
					t.Fatal("Get did not forward context and key")
				}
				return redis.NewStringResult(tt.value, tt.err)
			}}
			c, err := store.Store(client)(context.Background(), store.Config{})
			if err != nil {
				t.Fatal(err)
			}
			value, found, err := c.Get(ctx, "key")
			if value != tt.want || found != tt.found || !errors.Is(err, tt.wantErr) {
				t.Fatalf("Get = (%q, %v, %v), want (%q, %v, %v)", value, found, err, tt.want, tt.found, tt.wantErr)
			}
			if calls != 1 {
				t.Fatalf("Get calls = %d, want 1", calls)
			}
		})
	}
}

func TestSet(t *testing.T) {
	clientErr := errors.New("set failed")
	// TTL is passed through unchanged, including zero and negative values.
	for _, ttl := range []time.Duration{0, time.Minute, time.Millisecond, -time.Second} {
		for _, resultErr := range []error{nil, clientErr, redis.Nil} {
			t.Run(fmt.Sprintf("ttl=%s/error=%v", ttl, resultErr), func(t *testing.T) {
				ctx := t.Context()
				calls := 0
				client := &stubClient{set: func(gotCtx context.Context, key string, value interface{}, gotTTL time.Duration) *redis.StatusCmd {
					calls++
					if gotCtx != ctx || key != "key" || value != "value" || gotTTL != ttl {
						t.Fatalf("Set did not forward context, key, value, or TTL: key=%q value=%v TTL=%s", key, value, gotTTL)
					}
					return redis.NewStatusResult("OK", resultErr)
				}}
				c, err := store.Store(client)(context.Background(), store.Config{TTL: ttl})
				if err != nil {
					t.Fatal(err)
				}
				if err := c.Set(ctx, "key", "value"); !errors.Is(err, resultErr) {
					t.Fatalf("Set error = %v, want %v", err, resultErr)
				}
				if calls != 1 {
					t.Fatalf("Set calls = %d, want 1", calls)
				}
			})
		}
	}
}

func TestDelete(t *testing.T) {
	clientErr := errors.New("delete failed")
	for _, tt := range []struct {
		name    string
		count   int64
		err     error
		wantErr error
	}{
		{name: "deleted", count: 1},
		{name: "missing"},
		{name: "nil reply", err: redis.Nil},
		{name: "wrapped nil reply", err: fmt.Errorf("del: %w", redis.Nil)},
		{name: "error", err: clientErr, wantErr: clientErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			calls := 0
			client := &stubClient{del: func(gotCtx context.Context, keys ...string) *redis.IntCmd {
				calls++
				if gotCtx != ctx || len(keys) != 1 || keys[0] != "key" {
					t.Fatal("Delete did not forward context and key")
				}
				return redis.NewIntResult(tt.count, tt.err)
			}}
			c, err := store.Store(client)(context.Background(), store.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := c.Delete(ctx, "key"); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Delete error = %v, want %v", err, tt.wantErr)
			}
			if calls != 1 {
				t.Fatalf("Delete calls = %d, want 1", calls)
			}
		})
	}
}
