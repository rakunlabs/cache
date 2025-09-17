# cache 🔗

Simple cache with using different libraries.

```sh
go get github.com/worldline-go/cache
```

## Usage

Result of cache _get_, _set_ and _delete_ methods; stores could be force to types.

```go
Get(ctx context.Context, key K) (V, bool, error)
Set(ctx context.Context, key K, value V) error
Delete(ctx context.Context, key K) error
```

### memory

To not use max items and ttl, you can pass empty config `cache.WithStoreConfig(&memory.Config{})`.

```go
priceCache, err := cache.New[string, int](ctx,
    memory.Store,
    cache.WithStoreConfig(&memory.Config{
        MaxItems: 100,
        TTL:      10 * time.Minute,
    }),
)

err := priceCache.Set(ctx, "key", 100)
v, ok, err := priceCache.Get(ctx, "key")
```

### redis

Use [github.com/worldline-go/conn/connredis](https://github.com/worldline-go/conn/connredis) to create a redis client.

```go
redisClient, err := connredis.New(connredis.Config{
    Address: s.container.Address(),
})
if err != nil {
    // handle error
}

c, err := cache.New(s.T().Context(),
    redis.Store(redisClient),
    cache.WithStoreConfig(redis.Config{
        TTL: 3 * time.Second,
    }),
)
if err != nil {
    // handle error
}
```
