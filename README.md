# cache 🔗

[![License](https://img.shields.io/github/license/rakunlabs/cache?color=red&style=flat-square)](https://raw.githubusercontent.com/rakunlabs/cache/main/LICENSE)
[![Coverage](https://img.shields.io/sonar/coverage/rakunlabs_cache?logo=sonarcloud&server=https%3A%2F%2Fsonarcloud.io&style=flat-square)](https://sonarcloud.io/summary/overall?id=rakunlabs_cache)
[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/rakunlabs/cache/test.yml?branch=main&logo=github&style=flat-square&label=ci)](https://github.com/rakunlabs/cache/actions)
[![Go PKG](https://raw.githubusercontent.com/rakunlabs/.github/main/assets/badges/gopkg.svg)](https://pkg.go.dev/github.com/rakunlabs/cache)

Simple cache with using different libraries.

```sh
go get github.com/rakunlabs/cache
```

## Usage

Result of cache _get_, _set_ and _delete_ methods; stores could be force to types.

```go
Get(ctx context.Context, key K) (V, bool, error)
Set(ctx context.Context, key K, value V) error
Delete(ctx context.Context, key K) error
```

Additional function for help get-set together.

```go
GetSet(ctx context.Context, key K, fn func() (V, error)) (V, error)
```

`Take` gets and deletes a key atomically, so a value (for example a one-time
code) is consumed only once. The memory and redis stores support it; other
stores return `cache.ErrTakeNotSupported`.

```go
Take(ctx context.Context, key K) (V, bool, error)
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

Build a client with `redis.NewClient` (standalone or Redis Cluster, detected
automatically) or pass any existing `redis.UniversalClient`. The caller owns the
client and must close it.

```go
redisClient, err := redis.NewClient(ctx, redis.ConnConfig{
    Address: []string{"localhost:6379"},
})
if err != nil {
    // handle error
}
defer redisClient.Close()

c, err := cache.New(ctx,
    redis.Store(redisClient),
    cache.WithStoreConfig(redis.Config{
        TTL:       3 * time.Second,
        KeyPrefix: "app_",
    }),
)
if err != nil {
    // handle error
}
```

See [store/redis/README.md](store/redis/README.md) for cluster detection and tests.
