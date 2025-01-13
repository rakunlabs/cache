package cache_test

import (
	"context"
	"fmt"
	"time"

	"github.com/worldline-go/cache"
)

func ExampleCache() {
	ctx := context.Background()
	cfg := cache.Config{
		MaxItems: 1_000,
		TTL:      30 * time.Minute,
	}

	c, err := cache.New(context.Background(), cfg.ToOption())
	if err != nil {
		panic(err)
	}

	vcache, err := cache.Port[string, int](c)
	if err != nil {
		panic(err)
	}

	if err := vcache.Set(ctx, "key", 42); err != nil {
		panic(err)
	}

	v, ok, err := vcache.Get(ctx, "key")
	if err != nil {
		panic(err)
	}

	if !ok {
		panic("key not found")
	}

	fmt.Println(v)
	// Output:
	// 42
}
