package main

import (
	"context"
	"fmt"

	"github.com/worldline-go/cache"
	"github.com/worldline-go/cache/store/memory"
)

func main() {
	ctx := context.Background()

	c, err := cache.New[string, int](ctx,
		memory.Store,
		cache.WithMaxItems(100),
		cache.WithTTL(60),
	)
	if err != nil {
		panic(err)
	}

	if err := c.Set(ctx, "key", 1); err != nil {
		panic(err)
	}

	v, ok, err := c.Get(ctx, "key")
	if err != nil {
		panic(err)
	}

	if !ok {
		panic("key not found")
	}

	fmt.Println(v)
}
