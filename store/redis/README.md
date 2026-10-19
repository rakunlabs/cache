# Redis Adapter Tests

The adapter uses `go-redis/v9`. Its integration tests run against a real
`valkey/valkey:8.1.3-alpine` container managed by Testcontainers for Go.

From the repository root:

```sh
make test       # Unit and integration tests; requires a running Docker daemon
make test-unit  # Unit tests only; no Docker needed
```

Run only the Valkey suite:

```sh
go -C store/redis test -race -count=1 -timeout=5m -run '^TestValkey$' -v ./...
```

Each suite starts an isolated container with a dynamically mapped port and cleans
up the client and container afterward. The first run needs image-registry access
to pull Valkey and Testcontainers' resource-reaper image. Missing Docker or startup
failures fail the normal test run; only `-short` explicitly skips integration.
CI runs the full suite on the Docker-enabled `ubuntu-latest` runner.

Coverage includes binary/empty values, misses, deletion, actual TTL expiration,
TTL replacement/removal, WRONGTYPE errors, key prefixes, `Take` (GETDEL), and the
GetSet load/cache path.

`TestValkeyCluster` starts a three-master Valkey Cluster in one container and
checks `NewClient` cluster selection (detected from a single address, forced with
`Cluster`, and multiple addresses), that keys spread across every master, and
that reads, deletes and `Take` follow slot redirects. Sentinel, TLS and other
Redis versions are not covered.

Run only the cluster suite:

```sh
go -C store/redis test -race -count=1 -timeout=5m -run '^TestValkeyCluster$' -v ./...
```

## Connecting

`Store` takes a caller-owned `redis.UniversalClient`. `NewClient` builds one from
`ConnConfig` and pings it:

- more than one address, or `Cluster: true`, selects a cluster client;
- with a single address the server is asked `INFO cluster`; if INFO is denied
  (for example by an ACL) a standalone client is used.

```go
client, err := redis.NewClient(ctx, redis.ConnConfig{Address: []string{"redis:6379"}})
if err != nil { /* ... */ }
defer client.Close()

c, err := cache.New(ctx, redis.Store(client), cache.WithStoreConfig(redis.Config{
	TTL:       time.Minute,
	KeyPrefix: "app_",
}))
```

Testcontainers uses v0.44.0 and requires Go 1.25 or newer. Test dependencies are
confined to this nested module.
