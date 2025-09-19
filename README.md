# cache 🔗

[![License](https://img.shields.io/github/license/worldline-go/cache?color=red&style=flat-square)](https://raw.githubusercontent.com/worldline-go/cache/main/LICENSE)
[![Coverage](https://img.shields.io/sonar/coverage/worldline-go_cache?logo=sonarcloud&server=https%3A%2F%2Fsonarcloud.io&style=flat-square)](https://sonarcloud.io/summary/overall?id=worldline-go_cache)
[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/worldline-go/cache/test.yml?branch=main&logo=github&style=flat-square&label=ci)](https://github.com/worldline-go/cache/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/worldline-go/cache?style=flat-square)](https://goreportcard.com/report/github.com/worldline-go/cache)
[![Go PKG](https://raw.githubusercontent.com/worldline-go/guide/main/badge/custom/reference.svg)](https://pkg.go.dev/github.com/worldline-go/cache)

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

Additonal function for help get-set together.

```go
GetSet(ctx context.Context, key K, fn func() (V, error)) (V, error)
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
