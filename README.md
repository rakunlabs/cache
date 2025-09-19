# cache 🔗

[![License](https://img.shields.io/github/license/rakunlabs/cache?color=red&style=flat-square)](https://raw.githubusercontent.com/rakunlabs/cache/main/LICENSE)
[![Coverage](https://img.shields.io/sonar/coverage/rakunlabs_cache?logo=sonarcloud&server=https%3A%2F%2Fsonarcloud.io&style=flat-square)](https://sonarcloud.io/summary/overall?id=rakunlabs_cache)
[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/rakunlabs/cache/test.yml?branch=main&logo=github&style=flat-square&label=ci)](https://github.com/rakunlabs/cache/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/rakunlabs/cache?style=flat-square)](https://goreportcard.com/report/github.com/rakunlabs/cache)
[![Go PKG](https://raw.githubusercontent.com/rakunlabs/guide/main/badge/custom/reference.svg)](https://pkg.go.dev/github.com/rakunlabs/cache)

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

Get a redis client and give it to the cache.

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
