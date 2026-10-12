# Implementation Plan — Per-execution-id serialization via hash-routed worker channels

Goal: eliminate the concurrent GET-then-PUT optimistic-lock race by routing every
Kafka fill message to a single worker chosen by `executionServiceId`, so all messages
for one execution are processed serially by one goroutine. This is a dispatch/routing
change confined to `internal/service/kafka_consumer.go` plus a new unit-test file.

## Design decisions (made here, grounded in the code I read)

- **Replace the single shared `messageCh` fan-out with one channel per worker**
  (`workerChs []chan kafka.Message`). This is the chosen approach from Option 4 of
  `.agents/tasks/optimistic-lock-cascade-investigation.md`. Rationale: the fetch loop
  already reads one Kafka partition in offset order; routing each execution to a fixed
  worker preserves per-execution ordering AND serializes same-execution work without any
  map, keyed mutex, or per-execution tracking state (explicitly a non-goal).
- **Routing key = `fill.ExecutionServiceID` parsed from the message value.** The topic is
  partitioned by execution id upstream, but `message.Key` is not guaranteed and is not the
  domain source of truth; the message value is. Rationale: the investigation confirmed no
  partition key is set by this service, so we parse the value. To avoid paying for a full
  `domain.Fill` unmarshal (and its `validate` tags) just to route, use a minimal struct
  `struct{ ExecutionServiceID int64 \`json:"executionServiceId"\` }`. The full
  `domain.Fill` unmarshal + `Validate()` still happens later in `handleMessage` unchanged.
- **Index function is pure and total:** `workerIndexFor(executionID int64, workerCount int) int`
  returns `int(uint64(executionID) % uint64(workerCount))`. Using `uint64` conversion makes
  negative ids route deterministically without a negative modulo. Guard `workerCount <= 0`
  by returning 0 so the function is total and safe to unit-test in isolation.
- **Unparseable / zero-key fallback:** if the minimal unmarshal fails, route with an
  FNV-1a hash of `message.Key` bytes (`hash/fnv`), falling back to worker 0 when the key is
  empty. The message is still delivered to a worker so the existing `handleMessage` path
  runs validation and records the failure metric / DLQ — never dropped. Rationale: the task
  requires deliverability of malformed messages; routing determinism keeps any retries of
  the same bytes on the same worker.
- **Buffer sizing unchanged in aggregate semantics:** each per-worker channel gets capacity
  `config.Performance.MessageBufferSize`. The commit channel, commit flow, metrics,
  correlation-id logging, resilience, duplicate detection, and offset semantics are all
  untouched.

Confirmed facts from the current tree: struct field `messageCh chan kafka.Message`
(kafka_consumer.go), created in `NewKafkaConsumerService` with
`make(chan kafka.Message, config.Performance.MessageBufferSize)`; `workerCount int =
config.Performance.WorkerPoolSize`; `Start` spawns `go kcs.workerLoop(ctx, i)` in a loop;
`fetchLoop` sends via `select { case kcs.messageCh <- message ... }`; `workerLoop` does
`for message := range kcs.messageCh`; `Stop` Phase 2 does `close(kcs.messageCh)` then
`kcs.workerWg.Wait()`; `GetStats` reads `len/cap(kcs.messageCh)`. `domain.Fill` has
`ExecutionServiceID int64 \`json:"executionServiceId"\``. No existing `kafka_consumer_test.go`;
service tests use `package service` + testify (`stretchr/testify/assert` + `require`).
Base builds green (`go build ./...` exit 0).

---

- [ ] 1. Add the pure routing helpers and the `hash/fnv` import to `kafka_consumer.go`.
      Add `func workerIndexFor(executionID int64, workerCount int) int` returning
      `0` when `workerCount <= 0`, else `int(uint64(executionID) % uint64(workerCount))`.
      Add `func workerIndexForKey(key []byte, workerCount int) int` that returns `0` when
      `workerCount <= 0`, else FNV-1a hashes `key` (empty key hashes to 0 deterministically)
      and returns `int(h % uint64(workerCount))`. Add `"hash/fnv"` to the import block.
      Files: internal/service/kafka_consumer.go
      Verify: `go build ./...` from repo root succeeds (helpers compile, unused-import clean).

- [ ] 2. Replace the `messageCh` struct field with per-worker channels.
      In the `KafkaConsumerService` struct, change `messageCh chan kafka.Message` to
      `workerChs []chan kafka.Message` (keep `commitCh`, `workerCount`, all wait groups and
      control channels as-is). Leave the `// Parallel processing` grouping comment.
      Files: internal/service/kafka_consumer.go
      Verify: deferred to step 6 (`go build ./...` after all references are updated).

- [ ] 3. Allocate the per-worker channels in `NewKafkaConsumerService`.
      Replace `messageCh: make(chan kafka.Message, config.Performance.MessageBufferSize)` in
      the returned struct literal. Because `workerCount` is set in the same literal, build the
      slice after construction instead: assign the struct to a local `kcs`, then
      `kcs.workerChs = make([]chan kafka.Message, kcs.workerCount)` and loop
      `for i := range kcs.workerChs { kcs.workerChs[i] = make(chan kafka.Message, config.Performance.MessageBufferSize) }`,
      then `return kcs`. (`workerCount = config.Performance.WorkerPoolSize`, validated `>= 1`
      by config, so the slice is always non-empty.)
      Files: internal/service/kafka_consumer.go
      Verify: deferred to step 6.

- [ ] 4. Route each fetched message to its worker channel in `fetchLoop`.
      Replace the single `select { case kcs.messageCh <- message: ... }` send with:
      compute `idx` by unmarshalling `message.Value` into a local
      `var route struct { ExecutionServiceID int64 \`json:"executionServiceId"\` }`; on success
      and `route.ExecutionServiceID != 0` use `workerIndexFor(route.ExecutionServiceID, kcs.workerCount)`,
      otherwise use `workerIndexForKey(message.Key, kcs.workerCount)`; then
      `select { case kcs.workerChs[idx] <- message: case <-kcs.stopFetchCh: return case <-ctx.Done(): return }`.
      Keep the existing poll-success metric call and the stop/ctx escape arms. Do not add
      per-message error logging for the route-parse failure (the full unmarshal in
      `handleMessage` already records the failure); a Debug log is acceptable but optional.
      Files: internal/service/kafka_consumer.go
      Verify: deferred to step 6.

- [ ] 5. Make each worker range over its own channel in `workerLoop`.
      Change `for message := range kcs.messageCh` to `for message := range kcs.workerChs[workerID]`.
      Leave the body (handleMessage, commit-channel send, failure logging) unchanged. Update the
      "worker stopping" log message text that references `messageCh` to reference the worker's
      own channel.
      Files: internal/service/kafka_consumer.go
      Verify: deferred to step 6.

- [ ] 6. Close every per-worker channel in `Stop` Phase 2 and keep ordered shutdown correct.
      Replace `close(kcs.messageCh)` with a loop `for i := range kcs.workerChs { close(kcs.workerChs[i]) }`
      placed exactly where `close(kcs.messageCh)` was — after `kcs.fetchWg.Wait()` (Phase 1,
      so the fetch loop can no longer send on any `workerChs[i]`) and before `kcs.workerWg.Wait()`
      (so each worker drains its own buffered messages then exits its `range`). This preserves
      the stop-fetch → drain-workers → drain-commits → close-reader ordering; no send on a closed
      channel is possible because the only sender (fetchLoop) has already returned.
      Files: internal/service/kafka_consumer.go
      Verify: `go build ./...` and `go vet ./...` from repo root both succeed (all `messageCh`
      references are gone; no copy-of-lock or shadow warnings).

- [ ] 7. Aggregate per-worker channel stats in `GetStats`.
      Replace the `if kcs.messageCh != nil { ... len/cap ... }` block with a loop over
      `kcs.workerChs` summing `len` into `message_queue_length` and `cap` into
      `message_queue_capacity` (total across all workers). Keep the existing keys so downstream
      metrics/consumers are unchanged; leave `commit_queue_*` and `reader_stats` as-is.
      Files: internal/service/kafka_consumer.go
      Verify: `go build ./...` succeeds; `go vet ./...` clean.

- [ ] 8. Add unit tests for the routing helpers.
      Create `internal/service/kafka_consumer_test.go` with `package service` using testify,
      matching the style of `duplicate_detection_service_test.go`. Cover:
      (a) determinism — `workerIndexFor(id, n)` returns the same index across many calls for the
      same `id`, and the index is always in `[0, n)` for a range of ids and worker counts
      (e.g. n=5 like the default, plus n=1 and n=3);
      (b) distinct ids that collide mod n still map consistently (same id → same worker; that is
      the serialization guarantee the test asserts, not uniqueness);
      (c) negative/large ids route in-range without panic (exercises the `uint64` conversion);
      (d) `workerIndexFor(id, 0)` returns 0 (total-function guard);
      (e) fallback path — `workerIndexForKey([]byte("exec-123"), n)` is deterministic and in
      `[0, n)`, and `workerIndexForKey(nil, n)` / `workerIndexForKey([]byte{}, n)` returns a valid
      in-range index (0), confirming an unparseable/empty-key message still routes to a real
      worker rather than being dropped.
      Files: internal/service/kafka_consumer_test.go
      Verify: `go test ./internal/service/ -run 'WorkerIndex' -v` — new tests pass.

- [ ] 9. Full verification from repo root.
      Run the project's standard build/vet/test commands and confirm no regressions in the
      service package (existing confirmation/duplicate-detection/validation/execution-client
      tests still pass alongside the new routing tests).
      Files: (none — verification only)
      Verify (run from /Users/noahkrieger/projects/dissertation/kasbench/globeco-confirmation-service):
      `go build ./...` (exit 0), `go vet ./...` (exit 0), `go test ./...` (all packages pass,
      including the new `internal/service/kafka_consumer_test.go`).

---

## Shutdown-correctness check (explicit)

- Phase 1 closes `stopFetchCh` and `fetchWg.Wait()` returns only after `fetchLoop` exits, so
  after Phase 1 there is no goroutine that can send on any `workerChs[i]`.
- Phase 2 closes every `workerChs[i]`; each `workerLoop` finishes draining its buffered
  messages, exits its `range`, and `workerWg.Wait()` returns. No "send on closed channel"
  is possible because the sole sender already stopped.
- Phases 3–5 (commit ticker stop, commit drain, reader close) are untouched and still run in
  the same order.

## Non-goals honored

No per-execution map/tracking state, no keyed mutex, no change to `worker_pool_size` defaults,
and no changes to retry/resilience, the execution-service client, duplicate detection, or the
producer/topic config. Ordering is preserved: the single fetch path reads the partition in
offset order and each execution id is pinned to one worker.
