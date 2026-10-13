# Implementation Plan: Out-of-Order Fill Discard + Ticker Format Validation

## Context discovered during exploration (read before implementing)

- **Build/test commands** come from the `Makefile` and standard Go tooling:
  - Build: `go build ./...`
  - Test (all): `go test ./...`
  - Test (changed package only): `go test ./internal/service/...`
  - Vet/format (CI uses these): `go vet ./...`, `go fmt ./...`
- **No `AGENTS.md`, `CONTRIBUTING.md`, or `.kiro/steering/*`** exist in this repo. The `Makefile` `ci` target is `deps fmt vet lint test`; `lint` needs `golangci-lint` which may not be installed, so verification below relies on `go build`, `go vet`, and `go test`.
- **Module path:** `github.com/kasbench/globeco-confirmation-service`. Tests use `testify` (`assert`, `require`, `mock`) — already a dependency.

### Call-path facts that drive the Defect 1 design (critical)

The stale-quantity comparison currently lives in `ConfirmationService.validateFillMessage`
(`internal/service/confirmation_service.go`, lines ~277-282, the `"Fill quantity is less than current quantity - possible correction"` WARN). This is the **only** such comparison in `internal/service` (grep for `QuantityFilled <` and `less than current quantity` confirms a single site; the `validation_service.go` hit is `QuantityFilled < 0`, unrelated).

`validateFillMessage` is invoked **inside the `buildRequest` closure** passed to
`executionClient.UpdateExecutionWithRefetch` from `handleExecutionServiceCall`. The closure's only outputs are `(*domain.ExecutionUpdateRequest, error)`:

- Returning an **error** does NOT just abort the PUT. The error flows through
  `ResilienceManager.ExecuteAPICall` → `ExecuteWithResilience`, and because the retry
  cycle ends with `retryResult.Success == false`, **`ExecuteWithResilience` itself calls
  `deadLetterQueue.Add`** (verified in `internal/utils/resilience_manager.go` lines ~80-108
  and `internal/utils/retry.go` `isRetryableError`: a non-retryable `*domain.DomainError`
  stops retries but is still a failure). The error then returns to
  `handleExecutionServiceCall`, which adds a **second** DLQ record and calls
  `RecordMessageFailed`.
- Returning a **non-nil request** performs the PUT.

**Therefore a sentinel error returned through `buildRequest` CANNOT represent a benign
discard** — it would always create at least one DLQ record, violating the "no DLQ / no
retry / no failed-message record" requirement. Approach (a) is rejected for this reason.

**Chosen approach: (c) — guard the discard in `handleExecutionServiceCall`, before the
PUT path, using a plain read-only GET.** `ExecutionServiceClientInterface` already exposes
`GetExecution(ctx, id)` (see `internal/service/interfaces.go`). We do a `GetExecution`
first; if `fill.QuantityFilled < current.QuantityFilled`, we log INFO and return a benign
skip (no PUT, no `UpdateExecutionWithRefetch`, no DLQ, no error). Otherwise we proceed to
`UpdateExecutionWithRefetch` exactly as today, so the equal (`==`) and increasing (`>`)
cases keep their current behavior, including the re-fetch-for-version optimistic-lock
self-heal. The extra GET is read-only and cheap; it is the authoritative
already-applied quantity (same source the old code compared against).

Why not approach (b) (bool out of validation): the validation runs inside the client's
retry closure, so a bool would have to be smuggled out of the closure via a captured
variable and still would not stop the inner-resilience DLQ add if combined with an error;
keeping the decision entirely outside the client (approach c) is cleaner and provably
avoids every DLQ/retry side effect.

### How success is recorded on the discard path (requirement check)

- `HandleFillMessage` keeps `processingError == nil` on discard (we return
  `(nil, false, nil)`-equivalent signaling — see step 2 for the exact mechanism that also
  avoids the `logSuccess` nil-dereference).
- The deferred `cs.duplicateDetection.RecordProcessedMessage(ctx, fill, processingError == nil, ...)`
  therefore records the fill as **success** (`success == true`), so it is not reprocessed.
- `HandleFillMessage` returns `nil`, so the Kafka worker commits the offset normally
  (confirmed in `internal/service/kafka_consumer.go`; note the consumer commits even
  failed messages to avoid stranding, so offset commit is guaranteed either way — but we
  still want `nil` so no DLQ/failed-metric is recorded).
- No call to `cs.metrics.RecordMessageFailed()` on this path. We call
  `cs.metrics.RecordMessageProcessed()` (consistent with the existing duplicate-skip path
  in `HandleFillMessage`).

---

## Plan

- [ ] 1. Add a stale-fill discard signal to `handleExecutionServiceCall` return and detect
      the stale case with a read-only GET before the update.
      In `internal/service/confirmation_service.go`, change `handleExecutionServiceCall`'s
      signature from `(*domain.ExecutionUpdateResponse, bool, error)` to
      `(*domain.ExecutionUpdateResponse, bool, bool, error)` where the new middle `bool` is
      `skipped` (discarded stale fill). At the top of the method (before calling
      `UpdateExecutionWithRefetch`), call `current, err := cs.executionClient.GetExecution(ctx, fill.ExecutionServiceID)`.
      If that GET errors, treat it exactly like the existing update failure (DLQ +
      `RecordMessageFailed` + return `(nil, false, true, processingError)` with
      `execServiceFailed=true`). If the GET succeeds and
      `fill.QuantityFilled < current.QuantityFilled`, log at INFO
      `"Discarding stale out-of-order fill (quantityFilled below current)"` with fields
      `fill_id`, `execution_service_id`, `fill_quantity_filled`, `current_quantity_filled`,
      and return `(nil, false, true, nil)` (skipped=true, not failed, no error, no PUT).
      Otherwise call `UpdateExecutionWithRefetch` as today and return
      `(updateResponse, execServiceFailed, false, execErr)`.
      Files: internal/service/confirmation_service.go
      Verify: `go build ./...` succeeds (signature change compiles; callers updated in step 2).

- [ ] 2. Update `HandleFillMessage` to honor the `skipped` signal without logging success
      (avoids nil-dereference) and without failing.
      In `internal/service/confirmation_service.go`, change the call site to
      `updateResponse, execServiceFailed, skipped, execErr := cs.handleExecutionServiceCall(ctx, fill)`.
      When `skipped` is true: do NOT call `handleAllocationServiceCall`, do NOT call
      `logSuccess` (which dereferences `updateResponse.Version` and would panic on nil),
      call `cs.metrics.RecordMessageProcessed()`, leave `processingError` nil, and
      `return nil`. Keep the existing non-skipped flow unchanged: on
      `execServiceFailed` set `processingError = execErr`; otherwise call
      `handleAllocationServiceCall`, `logSuccess`, `RecordMessageProcessed`,
      `RecordMessageProcessingTime`.
      Files: internal/service/confirmation_service.go
      Verify: `go build ./...` succeeds; `go vet ./internal/service/...` clean.

- [ ] 3. Remove the obsolete stale-quantity WARN block from `validateFillMessage`.
      In `internal/service/confirmation_service.go`, delete the block (lines ~276-282):
      `// Check if quantity filled is not decreasing (unless it's a correction)` through
      the `cs.logger...Warn("Fill quantity is less than current quantity - possible correction", ...)`
      call. The authoritative discard now lives in `handleExecutionServiceCall` (step 1), so
      this log site is dead and its message ("possible correction") is wrong — there are no
      corrections. Do NOT touch the other checks in `validateFillMessage`
      (`execution_id_mismatch`, `trade_type_mismatch`, `destination_mismatch`,
      `security_id_mismatch`, `quantity_filled_exceeds_total`, `invalid_average_price`,
      high-price warn, timestamp-order errors) — they must behave exactly as before.
      Files: internal/service/confirmation_service.go
      Verify: `go build ./...` succeeds; grep confirms no remaining
      `"less than current quantity"` string in `internal/service/confirmation_service.go`.

- [ ] 4. Update the test mock so `UpdateExecutionWithRefetch` is NOT expected on the
      discard path, and the new `GetExecution`-first flow is modeled.
      In `internal/service/confirmation_service_test.go`, the existing tests call
      `setupRefetchUpdate`, whose mock drives `buildRequest(m.refetchExecution)`. Because
      `handleExecutionServiceCall` now calls `GetExecution` first for EVERY fill, add a
      `mockClient.On("GetExecution", mock.Anything, executionID).Return(execution, nil)`
      expectation inside `setupRefetchUpdate` (returning the same `execution`/`fetchErr`
      used for the refetch) so all existing success/validation/allocation tests still pass
      with the added pre-GET. For the GET-error test
      (`TestConfirmationService_HandleFillMessage_GetExecutionError`) and the both-failures
      test, make the `GetExecution` mock return the `fetchErr`; since the discard guard
      treats a GET error as an execution-service failure, assert the same
      `"failed to update execution 456"` error wrapper still appears (keep the wrapper text
      identical in step 1's error path).
      Files: internal/service/confirmation_service_test.go
      Verify: `go test ./internal/service/... -run TestConfirmationService_HandleFillMessage` — all pass.

- [ ] 5. Add a focused unit test proving the stale fill is discarded (no PUT, no error,
      recorded as success).
      In `internal/service/confirmation_service_test.go`, add
      `TestConfirmationService_HandleFillMessage_StaleFillDiscarded`. Wire a
      `MockExecutionServiceClient` whose `GetExecution` returns a `currentExecution` with
      `QuantityFilled: 103` and a `fill` with `QuantityFilled: 20` (mirrors a real log line),
      matching trade type/destination/security/quantity so only the stale condition differs.
      Set up `GetExecution` to be called once; do NOT set up `UpdateExecutionWithRefetch`
      (and assert it is not called via `mockClient.AssertNotCalled(t, "UpdateExecutionWithRefetch", mock.Anything, mock.Anything, mock.Anything)`).
      Use a real `DuplicateDetectionService` (via `NewDuplicateDetectionService`) wired into
      the service, then after `HandleFillMessage` assert `err == nil` and that
      `CheckDuplicate` for the same fill now reports it as a prior success (e.g. reason is
      the exact-duplicate skip, i.e. `ShouldProcess == false`), proving success was
      recorded. Also add an equal-case test
      (`fill.QuantityFilled == current.QuantityFilled`) asserting the PUT still happens
      (reuse `setupRefetchUpdate`, expect `UpdateExecutionWithRefetch`, `err == nil`).
      Files: internal/service/confirmation_service_test.go
      Verify: `go test ./internal/service/... -run TestConfirmationService_HandleFillMessage_StaleFillDiscarded` and the equal-case test pass.

- [ ] 6. Broaden the ticker format regex in `validateFormats` and update the warning text.
      In `internal/service/validation_service.go`, replace
      `tickerRegex := regexp.MustCompile(\`^[A-Z]{1,5}$\`)` with
      `tickerRegex := regexp.MustCompile(\`^[A-Z]{1,6}([.\-][A-Z]{1,3})?$\`)` and change the
      warning message from
      `"ticker '%s' does not match expected format (1-5 uppercase letters)"` to
      `"ticker '%s' does not match expected format (1-6 uppercase letters, optional '.' or '-' class suffix)"`.
      This accepts `BRK.B`, `BF.B`, `MSFT`, `CMCSA`, and hyphen/period class suffixes while
      still warning on empty, lowercase (`ibm123`), and garbage. Max matched length is 10
      (6 + 1 + 3), which stays at/under the separate `len(fill.Ticker) > 10` hard error at
      line ~353 — leave that hard error untouched. Keep this a WARNING (do not change it to
      an error).
      Files: internal/service/validation_service.go
      Verify: `go build ./...` succeeds; `go vet ./internal/service/...` clean.

- [ ] 7. Update and extend the ticker-format tests for Defect 2.
      In `internal/service/validation_service_test.go`:
      (a) In `TestValidationService_ValidateFillMessage_FormatValidation`, update the
      `"invalid ticker format"` case's `expectedError` to the new message text from step 6
      (still using `Ticker: "ibm123"`, which must still warn).
      (b) Add a sub-test asserting `Ticker: "BRK.B"` (with otherwise-valid fields, e.g. the
      valid message shape used in `TestValidationService_ValidateFillMessage_ValidMessage`)
      produces NO ticker `INVALID_FORMAT` warning: iterate `result.Warnings` and assert none
      has `Field == "ticker"` and `Code == "INVALID_FORMAT"`.
      (c) Keep the `"ticker too long"` case (`"VERYLONGTICKER"`, 14 chars) asserting the
      existing `"ticker exceeds maximum length of 10 characters"` error — confirms the hard
      length rule is unchanged and does not conflict with the new regex.
      Files: internal/service/validation_service_test.go
      Verify: `go test ./internal/service/... -run TestValidationService_ValidateFillMessage_FormatValidation` passes.

- [ ] 8. Full verification of the changed package and whole build.
      Run the whole suite to confirm no regressions in duplicate-detection, resilience, or
      domain tests, and that both defects' tests pass together.
      Files: (none — verification only)
      Verify: `go build ./...` and `go test ./...` both succeed with all tests passing.
      Then `go vet ./...` is clean.

## Notes / assumptions

- The requirement text references `confirmation_service.go:166`, `:278`, and `:283-288`.
  The log lines in production map to this source; in the current source there is a SINGLE
  stale-comparison site at lines ~276-282 (grep-verified), not two. The `:166` WARN is the
  generic `"Fill message validation passed with warnings"` in `validateInitialFillMessage`
  (unrelated to the discard; left unchanged). No second discard site exists, so step 3
  removes the one and only block and step 1 adds the authoritative guard.
- The regex `^[A-Z]{1,6}([.\-][A-Z]{1,3})?$` is written with `\-` inside the character
  class to keep the hyphen literal and avoid a range; in a Go raw string literal
  (backticks) this is the exact pattern. It still rejects empty string, lowercase, digits,
  and multi-dot garbage, satisfying "do not loosen so far that it matches anything."

## Verification note (first iteration)

Implemented per plan. Changes:
- `internal/service/confirmation_service.go`: `handleExecutionServiceCall` now returns
  `(*domain.ExecutionUpdateResponse, bool, bool, error)` — the new middle bool is `skipped`.
  A read-only `GetExecution` runs before the update; when
  `fill.QuantityFilled < current.QuantityFilled` the fill is discarded (INFO log
  "Discarding stale out-of-order fill (quantityFilled below current)" with fields
  fill_id, execution_service_id, fill_quantity_filled, current_quantity_filled), with no
  PUT, no DLQ, no error. `HandleFillMessage` honors `skipped`: records
  `RecordMessageProcessed`, returns nil (offset-committable), and the deferred
  `RecordProcessedMessage` records it as success. Equal/greater quantities PUT exactly as
  before. The obsolete WARN "possible correction" block in `validateFillMessage` was
  removed.
- `internal/service/validation_service.go`: ticker regex broadened to
  `^[A-Z]{1,6}([.\-][A-Z]{1,3})?$`; warning text updated to
  "(1-6 uppercase letters, optional '.' or '-' class suffix)". The separate
  `len(fill.Ticker) > 10` hard error is unchanged.
- Tests: `setupRefetchUpdate` now registers the pre-`GetExecution` expectation (update
  marked `.Maybe()` since it is skipped on GET failure). Added
  `TestConfirmationService_HandleFillMessage_StaleFillDiscarded` (asserts no PUT, nil
  error, duplicate-detection records success) and
  `TestConfirmationService_HandleFillMessage_EqualQuantityUpdates` (equal quantity still
  PUTs). Added `TestValidationService_ValidateFillMessage_TickerFormatAccepted`
  (BRK.B/BF.B/MSFT/CMCSA/IBM/BRK-B produce no ticker INVALID_FORMAT warning); updated the
  invalid-ticker expected message.

Commands run from repo root (all passed):
- `go build ./...` → exit 0
- `go vet ./...` → clean (exit 0)
- `go test ./...` → all packages ok (internal/service 0.311s), no failures
- Grep for `possible correction` / `less than current quantity` in internal/service →
  no matches.

Changes left in the working tree (not committed), per instructions.
