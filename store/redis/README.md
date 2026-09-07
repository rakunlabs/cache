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
TTL replacement/removal, WRONGTYPE errors, and the GetSet load/cache path. This is
standalone Valkey coverage, not Cluster, Sentinel, TLS, or Redis-version coverage.

Testcontainers uses v0.44.0 and requires Go 1.25 or newer. Test dependencies are
confined to this nested module.
