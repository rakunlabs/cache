# Cache Benchmarks

## Reproduce

```sh
make bench
```

Equivalent command:

```sh
go test -p=1 -run '^$' -bench . -benchmem -benchtime=200ms -count=3 -cpu=1,8 ./...
```

Packages run sequentially so their benchmarks do not compete with one another.
Performance runs exclude tests and race instrumentation. Run correctness checks
separately with `make test` and `make lint`.

## Environment

- Measured on 2026-09-07, before and after expiration indexing in the worktree.
- Go: `go1.27.0 linux/amd64` (the module minimum and CI version are Go 1.24).
- CPU: AMD Ryzen 7 5800X, 8 cores / 16 logical CPUs, Microsoft hypervisor.
- GOMAXPROCS: 1 and 8; this is not a dedicated production host.
- Tables report the median of three 200 ms benchmark samples, rounded.
- Baseline output: `/tmp/opencode/cache-bench-20260907.txt`.
- Indexed-heap output: `/tmp/opencode/cache-bench-heap-20260907.txt`.

## GetSet Baseline

These are pre-indexing strategy comparisons using the same memory store, not a
checkout-to-checkout comparison. `Global` implements the previous global
miss-lock/double-check strategy. `Plain` implements Get/load/Set without duplicate
load suppression. `PerKey` uses the actual current GetSet, including its context
checks. The alternatives are benchmark-only and are not public APIs.

| Workload | GOMAXPROCS | PerKey | Global | Plain |
| --- | ---: | ---: | ---: | ---: |
| Serial hit | 1 | 29.2 ns/op | 16.9 ns/op | 17.7 ns/op |
| Serial cheap miss, including Delete | 1 | 396 ns/op | 215 ns/op | 145 ns/op |
| Parallel cheap misses, independent keys, including Delete | 8 | 559 ns/op | 141 ns/op | 173 ns/op |
| Parallel blocking misses, independent keys, 1 ms sleep | 8 | 138 us/op | 1,092 us/op | 137 us/op |
| Cold burst of 16 requests for one key, 1 ms sleep | 8 | 1.135 ms/burst | 1.117 ms/burst | 1.128 ms/burst |
| Loader executions per cold burst | 8 | 1 | 1 | 16 |

- All hit variants allocate zero bytes per operation.
- Serial cheap misses allocate 256 B / 4 allocations with PerKey versus
  96 B / 2 allocations for Global and Plain. This includes memory-store insertion.
- PerKey is about 7.9x faster in aggregate throughput than Global for the measured
  eight-worker blocking-load scenario, without Plain's duplicate backend loads.
- Coordination is not free: cheap-load benchmarks favor the simpler strategies.
  The differences above measure the full implementations, not just the loading map.
- Parallel ns/op is elapsed wall time divided by total completed operations. It
  is NOT the latency of one request: the 1 ms loader still takes about 1 ms.

The GetSet benchmarks use int keys/values, TTL disabled, and no capacity limit.
HitParallel targets one already-populated hot key. Miss benchmarks delete each
worker's own key before every load to keep memory bounded without eviction costs.
ColdBurst includes goroutine creation, deletion, and start/wait barriers; one
benchmark operation is all 16 requests, not a single request. Sleep simulates
blocking work, not CPU-intensive computation or a real database.

## Memory Store Baseline

The following results use GOMAXPROCS=1. Get uses RunParallel with one worker;
SetFull and Cleanup are serial. Population is outside the measured interval.

| Operation | Entries | TTL disabled | TTL enabled (1 hour) |
| --- | ---: | ---: | ---: |
| Get hit, cycling populated keys | 1,000 | 26.2 ns/op | 63.2 ns/op |
| Get hit, cycling populated keys | 100,000 | 42.1 ns/op | 80.7 ns/op |
| New-key Set at full capacity | 1,000 | 168 ns/op | 9.70 us/op |
| New-key Set at full capacity | 10,000 | 212 ns/op | 123 us/op |
| New-key Set at full capacity | 100,000 | 198 ns/op | 2.46 ms/op |
| Cleanup scan, all entries live | 1,000 | Not measured | 9.01 us/scan |
| Cleanup scan, all entries live | 10,000 | Not measured | 119 us/scan |
| Cleanup scan, all entries live | 100,000 | Not measured | 2.38 ms/scan |

At GOMAXPROCS=8, the 100,000-entry TTL-enabled Get benchmark measured
118 ns/op in aggregate, versus 80.7 ns/op with one worker. More readers do not
scale this workload: every hit acquires the same lock to maintain LRU order.

All measured Get hits and all-live cleanup scans allocate zero bytes. SetFull
allocates two objects per insertion (approximately 96-100 B/op in this run).
These numbers are allocation traffic for int entries, not retained cache size.

The janitor is stopped with Close before population to isolate foreground costs.
All TTL entries remain live during measurement. SetFull always inserts a new key
at capacity and therefore includes eviction and, with TTL enabled, the full
expiration scan. Cleanup measures scanning, not expired-entry removal or map
compaction. Serial SetFull remains serial even in the GOMAXPROCS=8 runs.

## Baseline CPU Profile

A separate two-second profile of the 100,000-entry TTL-enabled SetFull scenario
measured 2.35 ms/op. `removeExpired` accounted for about 97% of cumulative CPU
samples, confirming that expiration scanning dominates this workload.

```sh
go test -run '^$' -bench '^BenchmarkMemorySetFull$/^Size=100000$/^TTL=1h0m0s$' \
  -benchmem -benchtime=2s -cpu=1 -count=1 \
  -cpuprofile=/tmp/opencode/cache-fullttl.cpu \
  -o /tmp/opencode/cache-memory-bench.test ./store/memory
go tool pprof -top /tmp/opencode/cache-memory-bench.test /tmp/opencode/cache-fullttl.cpu
```

## Expiration Index Results

The memory store now maintains an indexed min-heap ordered by expiration,
independently of LRU order. Updating TTL repairs the existing heap entry; removal
deletes it immediately, without stale records. The same benchmark workloads and
commands were rerun after this change. The table uses GOMAXPROCS=1 medians.

| TTL-enabled operation | Entries | Before | After | Approximate speedup |
| --- | ---: | ---: | ---: | ---: |
| New-key Set at full capacity | 1,000 | 9.70 us/op | 435 ns/op | 22x |
| New-key Set at full capacity | 10,000 | 123 us/op | 571 ns/op | 215x |
| New-key Set at full capacity | 100,000 | 2.46 ms/op | 776 ns/op | 3,172x |
| Cleanup, all entries live | 1,000 | 9.01 us/op | 50.6 ns/op | 178x |
| Cleanup, all entries live | 10,000 | 119 us/op | 48.2 ns/op | 2,474x |
| Cleanup, all entries live | 100,000 | 2.38 ms/op | 49.1 ns/op | 48,466x |
| Get hit, cycling populated keys | 100,000 | 80.7 ns/op | 78.5 ns/op | Similar |

With no expired entries, cleanup checks only the heap root: O(1), rather than
scanning n entries. TTL-enabled insertion, refresh, and removal have O(log n)
heap maintenance, rather than scanning all entries on a full-cache insertion.
Removing k expired entries costs O(k log n), plus any compaction. An expiration
burst can therefore still hold the mutex for substantial time.

Tradeoffs and checks:

- The heap stores one pointer per TTL-enabled entry, plus backing-array capacity.
  Each item also stores an index, including when TTL is disabled.
- Measured int-entry insertion allocations remain two objects, but grow from
  96 B to 112 B before amortized map allocation costs. TTL-disabled insertion
  at 100,000 entries measured 231 ns/op versus the baseline's 198 ns/op.
- Heap slots are cleared on removal. Compaction shrinks the heap backing array
  along with the map; remaining heap indices stay valid.
- Get hits still allocate zero bytes. Root GetSet behavior is unchanged; current
  serial misses allocate 272 B/op instead of 256 B/op due to the larger item.
- Tests cover refresh in both time directions, non-root deletion/eviction,
  reinsertion, expiration boundaries, compaction, mixed operations against a
  full-scan reference, and concurrent operations with background maintenance.

## Interpretation

Expiration indexing removes the measured full-cache insertion bottleneck while
preserving fixed TTL and LRU behavior. The shared LRU mutex, expiration bursts,
and compaction pauses remain separate scaling considerations. The very large
speedups apply to the specific all-live, full-cache workload, not all operations.

These microbenchmarks do not establish production capacity or latency guarantees.
They do not measure p95/p99 under mixed traffic, large values/string keys, heap
retention, GC pauses, expiration bursts, compaction pauses, or real Redis/network
behavior. The three short samples are directional evidence, not a statistical
claim about small timing differences. Repeat longer runs on target hardware and
the deployed Go version, then use realistic traffic and payloads before sizing.
