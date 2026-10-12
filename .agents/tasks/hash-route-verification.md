# Verification note — hash-route by execution id

Implementation of per-execution-id serialization via hash-routed worker channels
(first iteration — no `hash-route-review.json` existed). All commands run from the
repo root `/Users/noahkrieger/projects/dissertation/kasbench/globeco-confirmation-service`.

## Files changed

- `internal/service/kafka_consumer.go` — replaced the single shared `messageCh`
  with per-worker channels (`workerChs []chan kafka.Message`); added pure routing
  helpers `workerIndexFor` (execution-id modulo) and `workerIndexForKey` (FNV-1a
  fallback on `message.Key`); hash-routed each fetched message in `fetchLoop`;
  `workerLoop` ranges over its own channel; `Stop` Phase 2 closes every per-worker
  channel; `GetStats` aggregates `message_queue_length`/`message_queue_capacity`
  across all worker channels. Added `hash/fnv` import.
- `internal/service/kafka_consumer_test.go` — new file; unit tests for both routing
  helpers (determinism, in-range, same-id-same-worker, negative/large ids,
  zero-worker guard, empty-key fallback).

No changes to config defaults, retry/resilience, execution-service client, duplicate
detection, commit flow, metrics, correlation-id logging, or producer/topic config.

## Commands and results

```
$ go build ./...
=== build exit: 0 ===

$ go vet ./...
=== vet exit: 0 ===

$ go test ./...
?   	.../cmd/confirmation-service	[no test files]
ok  	.../internal/api	0.296s
ok  	.../internal/config	(cached)
ok  	.../internal/domain	(cached)
ok  	.../internal/metrics	(cached)
ok  	.../internal/middleware	(cached)
ok  	.../internal/service	0.444s
ok  	.../internal/utils	(cached)
ok  	.../pkg/logger	(cached)
ok  	.../pkg/metrics	(cached)
?   	.../pkg/otelmetrics	[no test files]
=== test exit: 0 ===

$ go test ./internal/service/ -run 'WorkerIndex' -v
--- PASS: TestWorkerIndexFor_Determinism (0.00s)
--- PASS: TestWorkerIndexFor_SameIDSameWorker (0.00s)
--- PASS: TestWorkerIndexFor_NegativeAndLargeIDs (0.00s)
--- PASS: TestWorkerIndexFor_ZeroWorkerCountGuard (0.00s)
--- PASS: TestWorkerIndexForKey_Deterministic (0.00s)
--- PASS: TestWorkerIndexForKey_EmptyKey (0.00s)
--- PASS: TestWorkerIndexForKey_ZeroWorkerCountGuard (0.00s)
PASS
ok  	.../internal/service	0.218s
```

All build, vet, and test commands passed. Changes left in the working tree (not committed).
