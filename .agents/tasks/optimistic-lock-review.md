# Re-fetch execution version inside the retry cycle and collapse the double retry

The change fixes the optimistic-lock cascade in `globeco-confirmation-service` by making the GET-version-then-PUT pair a single retried unit of work, so a version-mismatch 500 is followed by a fresh GET before the next PUT instead of re-sending the stale version forever. A new `ExecutionServiceClient.UpdateExecutionWithRefetch` wraps GET+build+PUT inside one `ExecuteAPICall` closure; `handleExecutionServiceCall` now calls it and passes validation + request construction as a callback. Separately, `ResilienceManager.ExecuteWithResilience` no longer runs a second full retry cycle to populate the DLQ — it captures the single cycle's result and reuses it, cutting worst-case attempts from ~6 to exactly `MaxAttempts`. The diff is confined to the two targeted files plus their tests, an interface entry, and a verification note.

Watch for: a comment/commit-message inaccuracy claiming 409 conflicts "self-heal" (409 maps to a non-retryable `ConflictError`, so only the 500 path actually retries) — **confirmed**, documentation-only, not a code defect.

**Verdict**: APPROVED

## High-level view

The core fix moves the version GET inside the retried closure. Each attempt re-reads the current version, rebuilds the PUT body from it, and sends — so when the execution service rejects a stale write with 500, the next attempt reads the winner's incremented version and succeeds. This directly breaks the "retry re-sends the same stale version" loop identified in the investigation. The resilience, circuit-breaker, backoff, correlation-ID, logging, metrics, and DLQ recording around the PUT are all preserved.

The second change removes the duplicate retry cycle in the resilience manager. The old code ran the retryer once inside the circuit breaker to get the result, then ran it a second time just to harvest attempt history for the DLQ — doubling load on the contended record. It now captures the first cycle's `RetryResult`, reuses it for the DLQ, and guards the nil case where the breaker rejected the call before `fn` ran.

Error classification is unchanged and behaves correctly with the new structure: a GET 404 (`NotFoundError`) and a validation failure (`ValidationError`) are both non-retryable, so they abort the cycle immediately without wasted attempts or a PUT. Only retryable `ExternalError` (500/502/503/504, timeouts, network) drives a retry — which is exactly the optimistic-lock case.

Nothing out of scope was touched: no worker-pool/concurrency change, no Kafka partition key, no per-execution lock, no Kubernetes manifest, and neither of the two unrelated validations.

<details>
<summary>Issues (2)</summary>

1. **409 self-heal claim is inaccurate** — comments in `handleExecutionServiceCall`/`UpdateExecutionWithRefetch` and the commit message say a 409 conflict self-heals on retry, but 409 maps to a non-retryable `ConflictError` and would abort the cycle. Only the 500 path retries. Non-blocking; fix the wording so the doc matches `handleErrorResponse`/`errors.go`.
2. **DLQ reason label unchanged for GET/validation aborts** — a GET failure or validation abort inside `UpdateExecutionWithRefetch` is still recorded to the DLQ under reason "execution-service failure" with `attempts: 1` from `handleExecutionServiceCall`. This matches prior behavior (the old code used the same label for both GET and validation failures), so it is not a regression; noted only for awareness.

</details>

<details>
<summary>Details</summary>

### Re-fetch inside the retried unit of work

`handleExecutionServiceCall` previously did GET → validate → `ToUpdateRequest(version)` → PUT as four separate steps, with the version captured once before the retry-wrapped PUT. The new version hands all of that to `UpdateExecutionWithRefetch` as a callback. Inside the closure passed to `ExecuteAPICall`, every attempt calls `getExecution` (a new non-retry-wrapped GET helper), then `buildRequest(execution)` (the callback that runs `validateFillMessage` and returns `fill.ToUpdateRequest(execution.Version)`), then marshals and sends the PUT. Because the GET sits at the top of the retried closure, a 500 from a lost optimistic-lock race is followed on the next attempt by a fresh GET that returns the winner's incremented version, and the rebuilt PUT carries that fresh version. That is the mechanism the investigation prescribed, and `TestUpdateExecutionWithRefetch_ReFetchesVersionBetweenAttempts` exercises it end to end (first PUT 500 + server version bump, second attempt re-GETs version 6 and succeeds; asserts 2 GETs, 2 PUTs, and version 5→6).

```
attempt N:  getExecution ──▶ buildRequest(exec) ──▶ PUT(version=exec.Version)
                 │                   │                      │
             GET fresh          validate +              500 → retry
             version            ToUpdateRequest         200 → done
```

The resilience wrapping is intact: the whole GET+PUT runs through `esc.resilienceManager.ExecuteAPICall`, so circuit breaker, `MaxAttempts`, and backoff still apply. Correlation ID is read once and threaded into both GET and PUT headers; the PUT path keeps its tracing span, debug/error logging, and response-version log. Metrics (`RecordMessageFailed`) and DLQ recording stay in `handleExecutionServiceCall` on the error path, and duplicate-detection recording is untouched upstream in `HandleFillMessage` — **confirmed** by reading the current `handleExecutionServiceCall`.

The legacy `GetExecution` and `UpdateExecution` remain on the client and interface for compatibility but are off the hot path; the mock and interface gained `UpdateExecutionWithRefetch`.

### Error classification lines up with the new structure

A concern with folding GET into the retried closure is whether non-transient GET outcomes now trigger pointless retries. They do not. `getExecution` classifies a non-200 through the same `handleErrorResponse` as before: 404 → `NotFoundError` (`Retryable: false`), 409 → `ConflictError` (`Retryable: false`), 500/502/503/504 → `ExternalError(..., true)`. `isRetryableError` honors the domain flag, so a 404 or a validation `ValidationError` from `buildRequest` aborts the cycle immediately — no retry, no PUT. `TestUpdateExecutionWithRefetch_ValidationErrorAbortsBeforePut` confirms 1 GET / 0 PUT on a validation abort. Only the retryable `ExternalError` (the 500 optimistic-lock case) drives a retry, which is the intended behavior.

The only mismatch is documentation: the method/handler comments and the commit message assert that a 409 "self-heals on the next attempt," but 409 is non-retryable and would abort rather than retry. The observed failure is a 500, which does retry, so the fix works for the real symptom. The 409 wording is **confirmed** inaccurate against `handleErrorResponse` and `errors.go`, but it is comment-only with no behavioral impact.

### Single retry cycle in the resilience manager

The old `ExecuteWithResilience` ran `rm.retryer.Execute(...)` inside the circuit breaker to get the result, then — on failure — ran `rm.retryer.Execute(timeoutCtx, operation, fn)` a *second* full time solely to obtain `Attempts`/`ErrorHistory` for the DLQ. For a failing execution update that meant up to ~6 blind PUTs against the already-contended record. The new code declares `var retryResult *RetryResult`, assigns it inside the breaker closure, and on failure reads `Success`/`Attempts`/`ErrorHistory` from that single result. A failing operation now runs exactly one cycle (`MaxAttempts`). `TestExecuteWithResilience_SingleRetryCycleOnFailure` asserts exactly 3 calls and one DLQ entry; `TestExecuteWithResilience_SuccessRunsOnce` asserts a happy path runs once with no DLQ entry.

If the circuit breaker rejects the call before `fn` runs (breaker open), `retryResult` stays nil, and the code records the DLQ entry with `attempts: 0` and empty history rather than dereferencing nil. DLQ population on terminal failure and circuit-breaker semantics are preserved — **confirmed** by reading the current function.

### Scope

The commit touches only `confirmation_service.go`, `execution_service_client.go`, `interfaces.go`, `resilience_manager.go`, their test files, and the verification note. A targeted diff against `origin/main` for `k8s_aws/`, `config.yaml`, `internal/config/`, `kafka_consumer.go`, and `internal/domain/fill.go` returned nothing — **confirmed** no out-of-scope edits. Worker count / `worker_pool_size`, the concurrency model, Kafka partition keys, per-execution locks, Kubernetes manifests / preStop, and the two unrelated validations (averagePrice range, fill-quantity-less-than-current warn) are all untouched.

### Verification evidence

The verification note and commit message record `go build ./...` (OK), `go vet ./...` (clean), and `go test ./...` (all packages pass, including the 4 new tests), plus a targeted run of the two new test groups. Per task instructions the suites were not re-run; the evidence is specific and consistent with the diff (new tests exist in the diff and match the described assertions), leaving no articulable doubt that would warrant a spot-check.

</details>

<details>
<summary>File map</summary>

- `internal/service/confirmation_service.go` — `handleExecutionServiceCall` now calls `UpdateExecutionWithRefetch` with a validate+build callback; DLQ/metrics/error-wrapping preserved.
- `internal/service/execution_service_client.go` — new `UpdateExecutionWithRefetch` (GET+build+PUT as one retried unit) and `getExecution` helper.
- `internal/service/interfaces.go` — added `UpdateExecutionWithRefetch` to `ExecutionServiceClientInterface`.
- `internal/utils/resilience_manager.go` — `ExecuteWithResilience` captures the single retry result for the DLQ instead of running a second cycle; nil-guard for breaker-open.
- `internal/service/confirmation_service_test.go` — mock + table tests updated to drive the re-fetch callback.
- `internal/service/execution_service_client_test.go` — new: re-fetch-between-attempts and validation-abort tests.
- `internal/utils/resilience_manager_test.go` — new: single-cycle-on-failure and success-runs-once tests.
- `.agents/tasks/optimistic-lock-fix-verification.md` — verification note.

Full diff: `git diff origin/main HEAD`

</details>
