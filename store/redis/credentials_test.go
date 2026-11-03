package redis_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	store "github.com/rakunlabs/cache/store/redis"
	"github.com/testcontainers/testcontainers-go"
	tcvalkey "github.com/testcontainers/testcontainers-go/modules/valkey"
)

func TestNewClientCredentialsProvider(t *testing.T) {
	if testing.Short() {
		t.Skip("Valkey integration test requires Docker")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	container, err := tcvalkey.Run(ctx, "valkey/valkey:8.1.3-alpine",
		testcontainers.WithCmd("valkey-server", "--requirepass", "secret-token"))
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
	addr := strings.TrimPrefix(uri, "redis://")

	if _, err := store.NewClient(ctx, store.ConnConfig{Address: []string{addr}}); err == nil {
		t.Fatal("expected NOAUTH without credentials")
	}

	var calls atomic.Int32
	client, err := store.NewClient(ctx, store.ConnConfig{
		Address:  []string{addr},
		Password: "wrong",
		CredentialsProvider: func(context.Context) (string, string, error) {
			calls.Add(1)

			return "default", "secret-token", nil
		},
	})
	if err != nil {
		t.Fatalf("connect with provider: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Set(ctx, "k", "v", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if calls.Load() == 0 {
		t.Fatal("credentials provider was not called")
	}
}
