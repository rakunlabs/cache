# cache

Simple cache with using different libraries.

```sh
go get github.com/worldline-go/cache
```

## Usage

```go
c := cache.New(context.Background(), cache.WithTTL(10*time.Second), cache.WithMaxSize(1000))

priceCache, err := cache.Port[string, int](c)
// ...

priceCache.Set("key", 100)
v, ok, err := vcache.Get("key")
```
