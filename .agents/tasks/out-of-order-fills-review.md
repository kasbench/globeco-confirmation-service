# Discard stale out-of-order fills and broaden ticker-format validation

Two defects are fixed in one change. First, out-of-order fills whose `quantityFilled` is below the already-applied quantity are now discarded on the update path instead of being written downward. The discard decision moved out of `validateFillMessage` (where it only logged a misleading "possible correction" WARN) into `handleExecutionServiceCall`, which does a read-only `GetExecution` before the PUT and returns a benign skip when the incoming quantity is strictly lower. Second, the ticker-format regex in `validateFormats` was widened from `^[A-Z]{1,5}$` to `^[A-Z]{1,6}([.\-][A-Z]{1,3})?$` so class-suffixed symbols like `BRK.B` and `BF.B` stop generating spurious warnings, while empty/lowercase/garbage still warn and the separate `>10` hard error is left intact.

Watch for: the discard path must not trigger Kafka redelivery or DLQ (confirmed — returns `nil`, records success); only strictly-less-than is discarded, equal still PUTs (confirmed); the extra `GetExecution` adds one read per fill (confirmed, by design).

**Verdict**: APPROVED

## High-level view

The discard decision was deliberately relocated out of the client's retry closure. The author's plan documents that returning a sentinel error through `buildRequest` would have flowed into `ExecuteWithResilience` and unconditionally produced a DLQ record, so a benign discard could not be represented that way. Instead the guard sits in `handleExecutionServiceCall` ahead of the PUT: a read-only `GetExecution` reads the authoritative current quantity, and `fill.QuantityFilled < current.QuantityFilled` returns `(nil, false, skipped=true, nil)`. This provably avoids every retry/DLQ side effect because `UpdateExecutionWithRefetch` is never reached.

The skip is wired through a new fourth return value (`skipped bool`). `HandleFillMessage` short-circuits on it: records `RecordMessageProcessed`, returns `nil`, and the deferred duplicate-detection recorder sees `processingError == nil` and marks the fill a success. The offset commits and the message is not reprocessed. Crucially the skip path does not call `logSuccess`, which would dereference the nil `updateResponse`.

The comparison is strict `<`, so equal and increasing quantities fall through to `UpdateExecutionWithRefetch` exactly as before, preserving the re-fetch-for-version optimistic-lock behavior. A `GetExecution` failure is treated as a genuine execution-service failure (DLQ + `RecordMessageFailed` + error), not a discard — the right call, and it reuses the existing `"failed to update execution %d"` wrapper so the error contract is unchanged.

The ticker regex is widened but not loosened to anything: it still anchors, requires at least one uppercase letter, permits at most one `.`/`-` separator with a 1-3 letter suffix, and rejects lowercase, digits, empty, and multi-separator garbage. Max matched length is 10, at/under the separate `>10` hard error that stays untouched. It remains a warning, not an error.

Tests assert the intended behavior rather than being weakened: the discard test sets up `GetExecution` once, asserts `UpdateExecutionWithRefetch` is never called, and verifies the fill is recorded as a success via a real duplicate-detection service. The equal-quantity test asserts the PUT does happen. The ticker test asserts no `INVALID_FORMAT` warning for `BRK.B`/`BF.B`/`MSFT`/`CMCSA`/`IBM`/`BRK-B`, and the invalid case (`ibm123`) still warns with the updated message.

<details>
<summary>Issues (1)</summary>

1. **Extra GET per fill** — every fill now incurs a read-only `GetExecution` before the PUT. By design and cheap, but it doubles execution-service read traffic on the hot path; worth noting for capacity, not a blocker.

</details>

<details>
<summary>Details</summary>

### Discard guard placement avoids the DLQ side effect

The decision to put the guard in `handleExecutionServiceCall` rather than inside the `buildRequest` closure is the load-bearing design choice. The closure runs inside `UpdateExecutionWithRefetch` → `ExecuteWithResilience`; a non-retryable error returned from there still ends the retry cycle as a failure and triggers `deadLetterQueue.Add`, so a sentinel-error discard would always leave a DLQ record. Moving the check before the PUT sidesteps that entirely — on discard, `UpdateExecutionWithRefetch` is simply never invoked (confirmed: it is the only PUT site in `internal/service`, now gated behind the GET and the `<` branch). (confirmed)

```
HandleFillMessage
  └─ handleExecutionServiceCall
       ├─ GetExecution(fill.ExecutionServiceID)        // read-only
       │     └─ err?  → DLQ + RecordMessageFailed + return (nil,true,false,err)   [genuine failure]
       ├─ fill.QF <  current.QF → INFO log + return (nil,false,true,nil)          [benign discard, no PUT]
       └─ fill.QF >= current.QF → UpdateExecutionWithRefetch(...)                 [equal/increasing: PUT as before]
```

### Skip signal and success recording

The new `skipped` return threads to a short-circuit in `HandleFillMessage` that records `RecordMessageProcessed`, returns `nil`, and leaves `processingError` nil so the deferred `RecordProcessedMessage(..., processingError == nil, ...)` marks the fill a success. Returning `nil` means the Kafka worker commits the offset and the message is not reprocessed; no `RecordMessageFailed`, no DLQ. The skip branch returns before `logSuccess`, which would otherwise panic dereferencing the nil `updateResponse`. (confirmed)

### Strict less-than only; equal and greater unchanged

The branch condition is `fill.QuantityFilled < current.QuantityFilled`. Equal and greater quantities skip the discard and reach `UpdateExecutionWithRefetch` unchanged, including the re-fetch-for-version self-heal. No code path PUTs a lower quantity — the only writer is gated behind this check. The removed block in `validateFillMessage` was the misleading WARN-only "possible correction" site; its deletion does not touch any other check (`destination_mismatch`, `security_id_mismatch`, `quantity_filled_exceeds_total` which compares against `Quantity` not `QuantityFilled`, `invalid_average_price`, high-price warn, timestamp-order errors all remain). (confirmed)

### GET-failure maps to the existing failure contract

A `GetExecution` error produces `fmt.Errorf("failed to update execution %d: %w", ...)`, records a failed metric, adds a DLQ record, and returns `execServiceFailed=true`. This is the correct fail-closed behavior for a transport failure (as opposed to the fail-benign discard). The error wrapper text matches the pre-existing PUT-failure wrapper, so `TestConfirmationService_HandleFillMessage_GetExecutionError` still asserts `"failed to update execution 456"` and passes against the new pre-GET path. (confirmed)

### Ticker regex widened, not loosened

`^[A-Z]{1,6}([.\-][A-Z]{1,3})?$` accepts `BRK.B`, `BF.B`, `MSFT`, `CMCSA`, `IBM`, `BRK-B`. It rejects the empty string (≥1 letter required), lowercase/digit input such as `ibm123`, double-separator garbage such as `BRK..B`, and multi-suffix forms — the optional group matches at most one separator plus a 1-3 letter suffix. Maximum matched length is 10 (6+1+3), at/under the independent `len(fill.Ticker) > 10` hard error, which is left in place. The check stays a warning, and only the message text changed. (confirmed)

### Test coverage

`TestConfirmationService_HandleFillMessage_StaleFillDiscarded` mirrors a real log line (incoming 20, current 103), sets `GetExecution` to return once, asserts no error, asserts `UpdateExecutionWithRefetch` is never called, and uses a real `DuplicateDetectionService` to confirm the fill is subsequently reported as a non-processable duplicate — i.e. success was recorded, not just that no error surfaced. `TestConfirmationService_HandleFillMessage_EqualQuantityUpdates` asserts the PUT is called when quantities are equal. `setupRefetchUpdate` registers the pre-GET and marks the PUT `.Maybe()`, which is correct given the GET-error test path never reaches the PUT. The ticker test iterates `result.Warnings` and fails on any `ticker/INVALID_FORMAT`, and the invalid-ticker table case retains `ibm123` with the updated expected message. Assertions test behavior, not weakened to pass. (confirmed)

Not tested: a strictly-greater increasing fill is not covered by a dedicated new test (the equal case is), though pre-existing success-path tests exercise the `>=` branch via `setupRefetchUpdate`. Not a gap worth blocking on.

### Verification evidence

The plan records `go build ./...` (exit 0), `go vet ./...` (clean), `go test ./...` (all packages ok, `internal/service` 0.311s, no failures), and greps confirming no residual `possible correction` / `less than current quantity` strings. Per instructions these suites were not re-run. The regex accept/reject reasoning was spot-checked by inspection (a standalone runner was blocked by a local toolchain version mismatch unrelated to this repo); the behavior follows directly from the anchored pattern and is backed by the passing `go test` evidence.

</details>

<details>
<summary>Files changed</summary>

- `internal/service/confirmation_service.go` — discard guard + `GetExecution` pre-check in `handleExecutionServiceCall`; new `skipped` return threaded into `HandleFillMessage`; removed the obsolete "possible correction" WARN.
- `internal/service/confirmation_service_test.go` — `setupRefetchUpdate` models the pre-GET; new stale-discard and equal-quantity tests.
- `internal/service/validation_service.go` — widened ticker regex and updated warning message.
- `internal/service/validation_service_test.go` — updated invalid-ticker message expectation; new accepted-ticker test.

Full diff: `git diff` from repo root (base `main`).

</details>
