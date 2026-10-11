# Optimistic-Lock Cascade Investigation — globeco-confirmation-service

**Mode:** Read-only investigation. No code was modified.
**Date of analysis:** against the current working tree.
**Scope:** Go confirmation-service (this repo) and its interaction with the Java/Spring execution-service (separate repo, source NOT present in this workspace).

---

## 1. Summary answer (the root cause)

The leading hypothesis is **CONFIRMED**. Symptoms 2, 3, and 4 form a single self-amplifying cascade driven by **concurrent, unserialized updates to the same execution record combined with a retry loop that never re-reads the version**:

1. The confirmation-service runs **5 concurrent Kafka workers** (`worker_pool_size: 5`) that pull fill messages off one buffered channel with **no per-execution-id serialization**. Multiple fills for the same `executionServiceId` are processed in parallel.
2. For each fill, the service does a **GET to fetch the current `version`, then a PUT with that version** — but the GET and PUT are two separate network round-trips with no lock between them (`handleExecutionServiceCall`, confirmation_service.go:339–360). Two workers reading the same version will both PUT the same version.
3. The execution-service uses JPA optimistic locking (`@Version`). The first PUT wins and increments the version; every concurrent/later PUT carrying the now-stale version is rejected as a version mismatch and returned as **HTTP 500** (`OptimisticLockingFailureException`).
4. The Go side classifies 500 as a **generic retryable server error** and retries up to 3× with 0.1s/0.2s backoff — but the **retry re-sends the identical PUT with the same stale version** because the GET-for-version lives *outside* the retried unit of work. The retry can therefore never succeed; it just re-loses the race. (execution_service_client.go `UpdateExecution`:171–270; retry.go `Execute`:65–140.)
5. Each exhausted retry marks the message as a **failed processed-message** in the in-memory duplicate cache. When Kafka redelivers (offset not advanced on handler failure — kafka_consumer.go:454) or when another near-identical fill for the same exec arrives, duplicate-detection sees `previous_success:false` and **reprocesses it**, feeding more concurrent PUTs back into the same contended record. That is the churn in symptom 3.

So: **concurrency without serialization → optimistic-lock conflicts (500) → blind same-version retries that cannot win → failed-message reprocessing → more concurrency on the same record.** Symptoms 2, 3, 4 are one cascade.

**Symptom 1 (FailedPreStopHook) is INDEPENDENT** of the locking cascade. The current `deployment.yaml` has **no** preStop hook and documents why (distroless/shell-less image). The events are explained by a previously-deployed manifest that still had an `sh -c` preStop hook running against the shell-less image. SIGTERM graceful shutdown is implemented correctly in Go. See section 5 (D).

---

## 2. Evidence (file / symbol citations)

### A/B. The version is fetched once, outside the retry loop — the crux

`internal/service/confirmation_service.go` — `handleExecutionServiceCall` (lines ~339–361):

```go
execution, err := cs.executionClient.GetExecution(ctx, fill.ExecutionServiceID) // GET version
...
updateRequest := fill.ToUpdateRequest(execution.Version)                         // snapshot version
updateResponse, err := cs.executionClient.UpdateExecution(ctx, fill.ExecutionServiceID, updateRequest) // PUT
```

- `fill.ToUpdateRequest(currentVersion)` (`internal/domain/execution.go`:217) copies the fetched `version` into the PUT body. The version comes from the **GET response**, not from the Kafka message — good, but it is captured **once**.
- `internal/service/execution_service_client.go` — `UpdateExecution` (lines ~171–270): the retried closure passed to `esc.resilienceManager.ExecuteAPICall(...)` **only marshals `updateReq` and sends the PUT**. It does **not** re-GET the version. The `updateReq` (with its stale `Version`) is captured by the closure and is byte-for-byte identical on every retry attempt.
- `internal/utils/retry.go` — `Execute` (lines ~65–140): loops `fn(ctx)` up to `MaxAttempts` with exponential backoff. It calls the **same** `fn` each time; there is no hook to refresh inputs between attempts.

**Conclusion for B:** The retry does NOT re-fetch the version. Concurrent fills for the same execution id can never succeed via retry alone — exactly as hypothesized.

### C. How the 500 is classified; conflict handling

- `internal/service/execution_service_client.go` — `handleErrorResponse` (lines ~310–332):
  - `case http.StatusConflict (409)` → `domain.NewConflictError(...)` (a distinct, non-retryable conflict type).
  - `case http.StatusInternalServerError (500)` (and 502/503/504) → `domain.NewExternalError("execution-service", "server error: 500", nil, true)` — the `true` is the **retryable** flag.
- `internal/utils/retry.go` — `isRetryableError` (lines ~190–235): a `*domain.DomainError` returns its `IsRetryable()` flag; the ExternalError built above is retryable. (Even generic errors default to retryable, and the message "internal server error"/"server error" matches the retryable-pattern list.)
- **The execution-service returns 500, not 409**, for `OptimisticLockingFailureException` (per the user's log evidence; the Java source is not in this workspace to confirm directly). Because it is 500, the Go side treats it as a transient server error and retries blindly. The service **does** already have 409→`ConflictError` handling, but that branch is never exercised because the Java side never sends 409.
- `max_attempts: 3`, backoff `0.1s`→`0.2s` (`config.yaml` `execution_service.retry_backoff: 100ms`; `BackoffFactor: 2.0` in main.go) — matches the observed `attempt:1 delay:0.1`, `attempt:2 delay:0.2`, "failed after all retry attempts" logs.

> Note on resilience_manager.go `ExecuteWithResilience` (lines ~62–95): on failure it runs the retry block, and then on error runs `rm.retryer.Execute(...)` **a second time** to populate the DLQ. This means a failing API call is effectively retried **two full retry cycles** (up to 6 attempts of the same stale PUT), amplifying load against the contended record. Worth flagging for the fix task.

### Symptom 3 — duplicate detection reprocessing loop

`internal/service/duplicate_detection_service.go` — `CheckDuplicate` (lines ~82–147):

- Key = `fill_<fillID>_exec_<execID>` (`generateMessageKey`:238).
- `!previousMessage.Success` → `ShouldProcess = true`, reason "Previous processing failed, retrying" (line ~117). **Every failed optimistic-lock attempt arms a reprocess.**
- `hasSignificantChanges` (lines ~245–270) → process as correction if quantityFilled changed, price changed >0.1%, or version changed (line ~105/125 log sites).
- else → "Skipping exact duplicate message" (line ~136).
- `RecordProcessedMessage` (deferred in `HandleFillMessage`, confirmation_service.go:100) records `Success: processingError == nil`. So each exhausted-retry failure stores `success:false`, guaranteeing the next arrival of that key reprocesses — matching the observed "Reprocessing failed duplicate message" / "Processing duplicate with significant changes" / "Skipping exact duplicate" lines.

### Concurrency / serialization (the trigger)

`internal/service/kafka_consumer.go`:

- `Start` spawns `workerCount` goroutines: `for i := 0; i < kcs.workerCount; i++ { go kcs.workerLoop(ctx, i) }` (lines ~155–160). `workerCount = config.Performance.WorkerPoolSize` (NewKafkaConsumerService:119).
- **`worker_pool_size: 5`** in `config.yaml`; default is also 5 (config.go:172). This is the "worker_id:3" seen in logs.
- `workerLoop` (lines ~335–375) pulls from a **shared `messageCh`** (`for message := range kcs.messageCh`). Any worker can take any message. **There is no routing/affinity by execution id or partition.**
- **No partition key is set by this service** — it is a *consumer*, not the producer. The partition key on the `fills` topic is chosen by the execution-service producer (not in this workspace). A grep for `message.Key`, per-execution mutex, or any `lockFor`/`keyBy` in `internal/service` found **no per-execution serialization anywhere**. So even if the producer keyed by execution id (routing same-exec fills to one partition), the 5-worker shared-channel fan-out would still process them concurrently.
- On handler failure, `handleMessage` returns the error and the offset is **not** committed for that message (kafka_consumer.go:454, `// Don't commit the message if processing failed`), setting up Kafka redelivery → more reprocessing.

### E. The two unrelated log lines (confirmed independent)

- `averagePrice (0.00) must be between 0 and 10000` — `internal/domain/fill.go` `Validate` (lines ~38–40). A genuinely invalid inbound message rejected by input validation before any execution-service call. Independent of locking.
- `Fill quantity is less than current quantity - possible correction` — `internal/service/confirmation_service.go` `validateFillMessage` (lines ~283–288). A **WARN-only** branch (no error returned) for an expected business scenario. Independent of locking.

Both are orthogonal to the optimistic-lock cascade.

---

## 3. Symptom 1 (D) — FailedPreStopHook: independent, config/deploy drift

- `k8s_aws/deployment.yaml`: **no `lifecycle.preStop` hook**. An explicit comment (lines ~37–50) states a prior `sh -c` preStop was removed because the image is distroless and has no shell, and recommends the native `sleep` action if a delay is ever needed. The only `sh -c` present is the `wait-for-kafka` **initContainer** using `busybox:1.36` (which *does* have a shell) — unrelated to preStop.
- Running image tag: `kasbench/globeco-confirmation-service:1.0.6` (deployment.yaml:33).
- `Dockerfile`: final stage is `gcr.io/distroless/static-debian12:nonroot` — **shell-less, confirmed**.
- No other manifest under `k8s_aws/` (keda.yaml, hpa.yaml, service.yaml, vpa.yaml, deployment.yaml) defines a preStop hook.
- SIGTERM/graceful shutdown in `cmd/confirmation-service/main.go` (lines ~119–127, 240–270): traps `SIGINT`/`SIGTERM`, cancels the root context, then `kafkaConsumer.Stop` → `httpServer.Shutdown` → OTel shutdown within a 30s shutdown context. `terminationGracePeriodSeconds: 60` gives ample headroom. The Kafka shutdown is ordered (kafka_consumer.go `Stop`:185–240: stop fetch → drain workers → drain commits → close reader).

**Verdict:** FailedPreStopHook events are explained by an **older deployed manifest** (or an older image deploy) that still had an `sh -c` preStop hook executing against the shell-less distroless image — the hook binary (`/bin/sh`) does not exist, so the kubelet records `FailedPreStopHook`. The current manifest in the tree has already corrected this. This is **deployment/version drift, not a code defect in the current tree, and is independent of the locking cascade.** Confirming the actually-deployed Deployment spec (and whether tag 1.0.6 is really what is running) would close this out operationally; it cannot be verified from the source tree alone.

---

## 4. Confirmed root-cause chain

```
5 concurrent workers (worker_pool_size:5), shared messageCh, no per-exec serialization
  → two+ workers GET the same execution.version for the same execId
  → both PUT with that version
  → execution-service @Version optimistic lock: first write wins, version++
  → stale writers get Version mismatch → HTTP 500            [SYMPTOM 4 / SYMPTOM 2]
  → Go classifies 500 as retryable ExternalError
  → retry re-sends the SAME stale-version PUT (version fetched outside retry) → loses again
  → (resilience_manager double-runs the retry cycle for DLQ → up to ~6 blind attempts)
  → all attempts exhausted → handler returns error
  → offset not committed + duplicate cache marks success:false
  → redelivery / near-duplicate fill → duplicate-detection reprocesses  [SYMPTOM 3]
  → more concurrent PUTs on the same contended record → cascade sustains
```

Symptom 1 (FailedPreStopHook) sits outside this chain.

---

## 5. Fix options (NOT implemented — for the follow-up task)

Each option lists the file(s)/function(s) that would change. These are complementary; options 2 and (1 or 4) together give the most robust fix.

**Option 1 — Serialize updates per execution id.**
- In-process per-execution lock: add a keyed mutex (e.g. a `map[int64]*sync.Mutex` guarded by a mutex, or a sharded lock) acquired on `fill.ExecutionServiceID` around the GET-then-PUT.
  - File/func: `internal/service/confirmation_service.go` → `handleExecutionServiceCall` (wrap lines ~339–360), plus a new lock holder on the `ConfirmationService` struct.
  - Caveat: in-process locking only serializes within one pod; with multiple replicas you still need option 2 or a Kafka key = execution id so all fills for an exec land in one partition/consumer instance.
- Kafka partition key = execution id: this is a **producer-side** change in the execution-service (not in this repo); note it as a cross-repo dependency.

**Option 2 — Re-fetch the version inside the retry loop on conflict (strongly recommended).**
- Make the retried unit of work a full **GET-then-PUT**, so each attempt reads the fresh version before writing.
  - File/func: `internal/service/execution_service_client.go` → `UpdateExecution` (lines ~171–270): move the version GET inside the retried closure, or add an `UpdateExecutionWithRefetch` that does GET→build request→PUT inside `ExecuteAPICall`.
  - Alternatively restructure `internal/service/confirmation_service.go` `handleExecutionServiceCall` so the GET+PUT pair is the retried function passed to the resilience manager.
  - Also review `internal/utils/retry.go` `Execute` — it needs no change if the closure itself re-fetches, but consider a conflict-specific fast path (no long backoff needed for version conflicts).

**Option 3 — Make the conflict a 409 and treat 409 as re-fetch-and-retry.**
- Execution-service (Java, separate repo): return **409 Conflict** for `OptimisticLockingFailureException` instead of 500. Cross-repo change — flag as dependency.
- Confirmation-service already maps 409 → `domain.NewConflictError` (`execution_service_client.go` `handleErrorResponse`:318–320), which is currently **non-retryable**. Change the 409 handling so it triggers a **GET-refresh-then-retry** rather than a blind retry or a hard fail. This pairs naturally with option 2.

**Option 4 — Reduce worker concurrency / one worker per execution id.**
- Lower `worker_pool_size` (blunt; reduces but does not eliminate the race and hurts throughput).
  - File: `config.yaml` `performance.worker_pool_size`; default in `internal/config/config.go`:172.
- Better: hash-route messages to workers by `executionServiceId` so a given exec is always handled by the same worker (serialization without a global lock).
  - File/func: `internal/service/kafka_consumer.go` — replace the single shared `messageCh`/`workerLoop` fan-out (lines ~117–160, 335–375) with per-worker channels and a dispatcher keyed on `fill.ExecutionServiceID` (requires parsing the fill before dispatch, or keying on `message.Key`).

**Secondary cleanup (flagged, not a root cause):**
- `internal/utils/resilience_manager.go` `ExecuteWithResilience` (lines ~62–95) runs the retry cycle **twice** on failure (once for the result, once to populate the DLQ), roughly doubling the number of blind PUTs against the contended record. Fix to capture the single retry result and reuse it for the DLQ.

---

## 6. Concrete line/symbol index for the implementer

| Concern | File | Symbol / lines |
|---|---|---|
| GET version then PUT (not serialized) | internal/service/confirmation_service.go | `handleExecutionServiceCall` ~339–361 |
| Version snapshot into PUT body | internal/domain/execution.go | `(*Fill).ToUpdateRequest` ~217 |
| PUT retried without re-GET | internal/service/execution_service_client.go | `UpdateExecution` ~171–270 |
| 500 classified retryable; 409 branch unused | internal/service/execution_service_client.go | `handleErrorResponse` ~310–332 |
| Retry loop reuses same closure | internal/utils/retry.go | `Execute` ~65–140; `isRetryableError` ~190–235 |
| Double retry cycle on failure | internal/utils/resilience_manager.go | `ExecuteWithResilience` ~62–95 |
| 5 workers, shared channel, no affinity | internal/service/kafka_consumer.go | `Start` ~155–160; `workerLoop` ~335–375 |
| Worker count config | config.yaml / internal/config/config.go | `performance.worker_pool_size`; config.go:172 |
| Offset not committed on failure | internal/service/kafka_consumer.go | `handleMessage` ~454 |
| Failed-message reprocess trigger | internal/service/duplicate_detection_service.go | `CheckDuplicate` ~111–147; `RecordProcessedMessage` ~149–180 |
| preStop absent, distroless note | k8s_aws/deployment.yaml | ~37–50; image tag :1.0.6 line 33 |
| Shell-less base image | Dockerfile | `gcr.io/distroless/static-debian12:nonroot` |
| SIGTERM graceful shutdown | cmd/confirmation-service/main.go | signal handler ~119–127; shutdown ~240–270 |
| Unrelated: averagePrice validation | internal/domain/fill.go | `Validate` ~38–40 |
| Unrelated: quantity-correction warn | internal/service/confirmation_service.go | `validateFillMessage` ~283–288 |

---

## 7. Caveats / what could not be verified

- The **execution-service (Java/Spring) source is NOT in this workspace** (no `ExecutionServiceImpl` found). The 500-for-optimistic-lock behavior and `@Version` usage are inferred from the user-supplied logs and the Go-side classification, not read directly.
- The **actually-deployed** Kubernetes Deployment spec and the live image tag could not be inspected (read-only, source-tree only). The FailedPreStopHook root cause (older manifest with `sh -c` preStop) is the best explanation consistent with the current tree but should be confirmed against the running cluster.
- Line numbers are approximate (prefixed `~`) and anchored to the current working tree.
