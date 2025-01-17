# cache

Simple cache with using different libraries.

```sh
go get github.com/worldline-go/cache
```

## Usage

```go
priceCache, err := cache.New[string, int](ctx,
    memory.Store,
    cache.WithMaxItems(100),
    cache.WithTTL(60 * time.Second),
)

err := priceCache.Set(ctx, "key", 100)
v, ok, err := vcache.Get(ctx, "key")
```
