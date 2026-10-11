# Optimistic-Lock Cascade Fix — Verification Note

Scope implemented (exactly the two requested changes; concurrency, worker pool,
Kafka partitioning and locks left untouched):

## Change 1 — re-fetch the execution version inside the retry loop
- Added `ExecutionServiceClient.UpdateExecutionWithRefetch(ctx, executionID, buildRequest)`
  in `internal/service/execution_service_client.go`. The GET-version-then-PUT pair is now
  the retried unit of work: each attempt re-fetches the current execution (via a new
  internal `getExecution` helper that shares the single `ExecuteAPICall` closure) and
  rebuilds the PUT body from the fresh version before sending. A version-mismatch 500
  (or a 409) therefore triggers a fresh GET before the next PUT, so the conflict
  self-heals instead of re-sending a stale version forever.
- `buildRequest` runs the business-rule validation (`validateFillMessage`) and builds
  `fill.ToUpdateRequest(execution.Version)`; a validation failure is a non-retryable
  `ValidationError` and aborts the cycle without a PUT, matching prior fast-fail behavior.
- Added the method to `ExecutionServiceClientInterface` (`internal/service/interfaces.go`)
  and to `MockExecutionServiceClient` (test file). The legacy `GetExecution`/`UpdateExecution`
  remain for interface compatibility but are no longer on the hot call path.
- `handleExecutionServiceCall` (`internal/service/confirmation_service.go`) now calls
  `UpdateExecutionWithRefetch`, preserving correlation IDs, logging, metrics
  (`RecordMessageFailed`), DLQ recording and duplicate-detection recording. Circuit
  breaker and max-attempts/backoff still apply (existing backoff kept; not over-engineered).

## Change 2 — stop the double retry cycle in resilience_manager.go
- `ResilienceManager.ExecuteWithResilience` previously ran one retry cycle inside the
  circuit breaker and then a SECOND full `retryer.Execute(...)` just to populate the DLQ
  (up to ~6 attempts). It now captures the single retry result and reuses it for the DLQ
  path, so a failing operation runs exactly ONE cycle (MaxAttempts). DLQ population and
  circuit-breaker semantics preserved. Handles the nil-result case (breaker open before fn ran).

## Tests added
- `internal/service/execution_service_client_test.go`
  - `TestUpdateExecutionWithRefetch_ReFetchesVersionBetweenAttempts`: httptest server;
    first PUT returns 500 and bumps the server version; the retry re-GETs the fresh
    version and the second PUT succeeds. Asserts 2 GETs, 2 PUTs, and that the retry PUT
    carried the incremented version (5 -> 6).
  - `TestUpdateExecutionWithRefetch_ValidationErrorAbortsBeforePut`: a buildRequest
    validation error aborts before any PUT (1 GET, 0 PUT).
- `internal/utils/resilience_manager_test.go`
  - `TestExecuteWithResilience_SingleRetryCycleOnFailure`: a failing op runs exactly 3
    times (one cycle, not 6) and is added to the DLQ once.
  - `TestExecuteWithResilience_SuccessRunsOnce`: happy path runs once, no DLQ entry.

Existing `HandleFillMessage` table tests were updated to drive the mock through
`UpdateExecutionWithRefetch` (the mock invokes the buildRequest callback so validation
coverage is retained). The GetExecutionError/ValidationError assertions now expect the
"failed to update execution" wrapper because the GET moved inside the update path.

## Commands run and results
- `go build ./...`  -> BUILD OK
- `go vet ./...`    -> VET OK (clean)
- `go test ./...`   -> all packages ok (internal/service, internal/utils, internal/api,
  internal/config, internal/domain, internal/metrics, internal/middleware, pkg/logger,
  pkg/metrics). No failures.
- `go test ./internal/service/ ./internal/utils/ -run 'UpdateExecutionWithRefetch|ExecuteWithResilience' -v`
  -> 4/4 new tests PASS.
