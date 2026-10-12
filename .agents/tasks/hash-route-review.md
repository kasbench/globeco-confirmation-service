# Per-execution-id hash routing in the Kafka consumer

The consumer's single shared `messageCh` fan-out is replaced with one buffered channel per worker (`workerChs []chan kafka.Message`). The fetch loop parses the fill's `executionServiceId` out of each message value and routes to worker `uint64(id) % uint64(workerCount)`, so every fill for a given execution lands on exactly one worker and same-execution fills are processed strictly in order — closing the concurrent GET-then-PUT optimistic-lock race without any per-execution map, keyed mutex, or accumulating state. Routing is a pure, total helper (`workerIndexFor`), with an FNV-1a key hash (`workerIndexForKey`) as the fallback when the value can't be parsed, and malformed messages are still delivered so existing validation/DLQ handling runs. Ordered shutdown is preserved: Phase 1 stops the sole sender before Phase 2 closes the per-worker channels, so no send can hit a closed channel.

Watch for: fills carrying a literal `executionServiceId: 0` take the key-hash fallback rather than the id path (confirmed) — benign for routing determinism but a behavioral seam worth knowing. No other blocking concerns.

**Verdict**: APPROVED

## High-level view

The routing invariant is the core of the change: `Start` spawns workers with ids `0..workerCount-1`, and `workerChs` is allocated with exactly `workerCount` entries in the constructor, so both the fetch-side `workerChs[idx]` send and the worker-side `workerChs[workerID]` range are always in bounds. The modulo index is bounded by construction and the `uint64` conversion keeps negative ids in range, so there is no path to an index panic.

Ordering and serialization come for free from the existing single in-order fetch path: one goroutine reads the partition in offset order and pins each execution to one worker, so same-execution fills never run concurrently and never reorder. This is exactly the Option-4 approach from the investigation, and it holds the "no map, no keyed mutex, no per-execution tracking" non-goals.

Deliverability of malformed messages is preserved by routing on a fallback key hash instead of dropping. The routing-time unmarshal is a minimal struct that only reads `executionServiceId`; the full `domain.Fill` unmarshal and `Validate()` still happen downstream in `handleMessage` unchanged, so validation failures and DLQ handling are untouched.

Shutdown stays correct because the phase ordering is unchanged — the only edit closes N channels where one was closed before, in the same spot between `fetchWg.Wait()` and `workerWg.Wait()`, after the sole sender has returned.

The one behavioral seam: an `executionServiceId` of exactly 0 is indistinguishable from a parse miss here, so such a message routes via the key hash rather than the id modulo. Determinism is still preserved for that message, so it is not a correctness bug, just an asymmetry worth noting.

<details>
<summary>Issues (1)</summary>

1. **Zero execution-id falls through to key fallback** (confirmed) — `route.ExecutionServiceID != 0` treats a legitimate id of 0 as a parse miss and routes it by `message.Key` instead of by id. Harmless given determinism, but if 0 is ever a valid execution id, same-execution serialization for it would depend on the key rather than the id. No action required unless 0 is a valid id; worth a one-line comment.

</details>

<details>
<summary>Details</summary>

### Routing invariant and the index helpers

The safety of indexing both `workerChs[idx]` (sender) and `workerChs[workerID]` (worker) rests on channel count equalling worker count. The constructor sets `workerCount = config.Performance.WorkerPoolSize` and immediately allocates `workerChs = make([]chan kafka.Message, kcs.workerCount)`, and `Start` loops `for i := 0; i < kcs.workerCount; i++` spawning `workerLoop(ctx, i)`. So worker ids are exactly the valid slice indices, and `workerIndexFor` returns `int(uint64(id) % uint64(workerCount))` which is in `[0, workerCount)`. There is no path to an out-of-range index.

The `uint64` conversion on a negative id is the non-obvious decision: it reinterprets the bits rather than producing a negative Go modulo, keeping the result in range. The `workerCount <= 0` guard makes the helpers total even though config validation guarantees `workerCount >= 1` in production.

### Value-parsed routing key with key-hash fallback

Routing parses a minimal `struct{ ExecutionServiceID int64 }` from `message.Value` rather than reusing `message.Key` as the source of truth — correct, since the investigation established no partition key is set upstream and the value is the domain source of truth. The minimal unmarshal avoids paying for a full `domain.Fill` decode plus validation just to route.

The fallback predicate is `err == nil && route.ExecutionServiceID != 0`. A value that fails to parse, or that parses to id 0, takes `workerIndexForKey(message.Key, ...)`. For genuinely unparseable messages this is exactly the required behavior: the message is still handed to a worker, so `handleMessage` runs the full unmarshal, records the validation failure metric, and drives DLQ handling — nothing is dropped. The id-0 case is the one seam: a fill whose execution id is literally 0 is treated as a parse miss. Routing stays deterministic (same bytes/key → same worker), so this is not a correctness defect, but if 0 were ever a valid execution id its serialization would key off `message.Key` instead of the id. Worth a comment at minimum.

### Ordered shutdown

The only change to `Stop` is replacing the single `close(kcs.messageCh)` with a loop closing every `workerChs[i]`, in the same position: after `fetchWg.Wait()` (Phase 1) and before `workerWg.Wait()`. Because `fetchLoop` is the sole sender on any worker channel and it has provably returned once `fetchWg.Wait()` unblocks, no send can race the closes — the "send on closed channel" panic is structurally impossible. Each worker then drains its own buffered messages, exits its `range`, and `workerWg.Wait()` returns. Phases 3–5 (commit ticker, commit drain, reader close) are untouched. The fetch-side `select` still has `stopFetchCh` and `ctx.Done()` escape arms, so a worker channel that fills during shutdown doesn't wedge the fetch loop.

### GetStats aggregation

`GetStats` sums `len`/`cap` across all worker channels into the existing `message_queue_length` / `message_queue_capacity` keys, preserving the downstream metric contract. The `kcs.workerChs != nil` guard matches the prior check on `messageCh`, and individual channels are allocated in the constructor alongside the slice, so there's no nil-element iteration risk.

### Test coverage

The new `kafka_consumer_test.go` covers both helpers: determinism across repeated calls, in-range `[0, n)` for n of 1/3/5, same-id-same-worker, negative and boundary ids (`MinInt64`/`MaxInt64`) without panic, the `workerCount <= 0` guard for both helpers, and the empty/nil-key fallback hashing to the same in-range worker. This directly backs the serialization guarantee and the "malformed still routes" requirement.

Not tested: the integration-level behavior — that `fetchLoop` actually sends to the computed `workerChs[idx]`, that a value with `executionServiceId: 0` takes the fallback branch, and that shutdown drains per-worker buffers. These are harder to unit-test given the live-reader `fetchLoop` and are reasonably left to the existing service tests plus the build/vet/test evidence, but the id-0 branch in particular has no direct test.

### Verification evidence

The coder's note records `go build ./...` (exit 0), `go vet ./...` (exit 0), `go test ./...` (all packages ok, including `internal/service`), and a focused `-run 'WorkerIndex'` run showing all seven routing tests passing. Per instructions I did not re-run the suites; I spot-confirmed the worker-id/channel-count invariant by reading `Start` and the constructor, which is the one claim the diff alone doesn't fully settle. Evidence is present and consistent with the diff.

</details>

<details>
<summary>File map</summary>

- `internal/service/kafka_consumer.go` — replaced shared `messageCh` with per-worker `workerChs`; added pure `workerIndexFor` / `workerIndexForKey` helpers and `hash/fnv` import; hash-routed each fetched message in `fetchLoop`; `workerLoop` ranges its own channel; `Stop` Phase 2 closes every worker channel; `GetStats` aggregates len/cap across channels.
- `internal/service/kafka_consumer_test.go` — new unit tests for both routing helpers (determinism, in-range, same-id-same-worker, negative/large ids, zero-worker guard, empty-key fallback).

Full diff: `git diff main` on `internal/service/kafka_consumer.go` plus untracked `internal/service/kafka_consumer_test.go`.

</details>
