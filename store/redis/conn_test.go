package redis_test

import (
	"testing"

	store "github.com/rakunlabs/cache/store/redis"
)

func TestNewClientRequiresAddress(t *testing.T) {
	for _, addrs := range [][]string{nil, {""}, {"localhost:6379", " "}} {
		if c, err := store.NewClient(t.Context(), store.ConnConfig{Address: addrs}); err == nil {
			_ = c.Close()
			t.Fatalf("NewClient(%q) succeeded, want error", addrs)
		}
	}
}

func TestTLSConfigDisabled(t *testing.T) {
	cfg, err := (store.TLSConfig{CAFile: "/does/not/exist"}).Generate()
	if err != nil || cfg != nil {
		t.Fatalf("Generate = %v, %v; want nil, nil", cfg, err)
	}
}
