# Implementation Plan — Simplify duplicate-detection logic

## Goal

Now that per-execution-id ordering is guaranteed (all fills for an execution id are pinned to one worker and processed in Kafka-delivery order), each later fill for a given key is a legitimate successive UPDATE, not a correction. Replace the `hasSignificantChanges` heuristic with a simple identity check and eliminate the three per-message INFO log lines that currently fire for every ordinary update.

## Design decisions (made here, grounded in the code)

1. **Identity key for "exact duplicate" = `version` AND `lastFilledTimestamp` both unchanged.** The user's statement is authoritative: a record is a duplicate only when everything that advances (version, lastFilledTimestamp) is identical. Quantity/price are no longer part of the decision — they are derived data that an update happens to carry. Decision rule: identical version AND lastFilledTimestamp => skip (exact duplicate); otherwise => process as a normal update.

2. **`ProcessedMessage` must record `LastFilledTimestamp`.** It currently records Version, QuantityFilled, AveragePrice but not LastFilledTimestamp (`duplicate_detection_service.go` ~line 27). `domain.Fill.LastFilledTimestamp` is a `float64` (`internal/domain/fill.go` line 22), so add `LastFilledTimestamp float64`. Float equality is correct here: the value is copied verbatim from the same source struct on both record and compare, so no tolerance is needed (and the user wants exact-equality semantics).

3. **An updated message is NOT reported as a duplicate for logging/semantics.** When version/timestamp advanced, set `IsDuplicate = false` in the result even though a prior entry exists for the key. This keeps the caller's "duplicate" framing (and its INFO log at `confirmation_service.go` ~line 182) from firing on ordinary updates, which is exactly the noise being removed. `PreviousMessage` is still populated for diagnostics. The failed-retry path keeps `IsDuplicate = true` (it genuinely is the same message being retried).

4. **Logging levels.** All the per-message chatter drops to DEBUG or is removed. The exact-duplicate skip and the normal-update path log at DEBUG. The failed-retry path keeps a single concise log — DEBUG per the task's preference. The caller's "Processing duplicate message with changes" INFO (`confirmation_service.go` ~line 182) is removed since that branch no longer represents a meaningful event (updates are not duplicates). The caller's existing "Skipping duplicate message processing" INFO at ~line 116 stays as-is: it now only fires on true exact duplicates, which are rare, so it is not a noise source.

5. **Delete `hasSignificantChanges`.** Grep confirms its only callers are `CheckDuplicate` (`duplicate_detection_service.go` ~line 121) and its dedicated unit test `TestDuplicateDetectionService_hasSignificantChanges`. Once CheckDuplicate stops using it, remove both the method and its test.

## Verification commands (run from workspace root)

- `go build ./...` — succeeds, no errors.
- `go vet ./...` — clean, no diagnostics (catches an unused `hasSignificantChanges` if the delete is incomplete, unused imports, etc.).
- `go test ./...` — all packages pass.

Targeted test run for the changed package: `go test ./internal/service/ -run DuplicateDetection -v`.

---

## Plan items

- [ ] 1. Add `LastFilledTimestamp` to `ProcessedMessage` and populate it on record.
      In `ProcessedMessage` (struct ~line 27) add `LastFilledTimestamp float64 \`json:"lastFilledTimestamp"\`` alongside the existing Version/QuantityFilled/AveragePrice fields. In `RecordProcessedMessage` (~line 150) set `LastFilledTimestamp: fill.LastFilledTimestamp` in the `ProcessedMessage` literal. Leave QuantityFilled/AveragePrice recording in place (still useful for stats/diagnostics even though they no longer drive the decision).
      Files: internal/service/duplicate_detection_service.go
      Verify: `go build ./...` succeeds.

- [ ] 2. Rewrite `CheckDuplicate` decision logic (~line 82–140) to the new identity-based rule.
      Keep the early "not found in cache" path unchanged (IsDuplicate=false, ShouldProcess=true, Reason "New message, not previously processed", DEBUG log). Replace everything after the `if !exists` block:
      - Remove the unconditional INFO "Duplicate message detected" log (current ~line 105).
      - New branching on the existing `previousMessage`:
        - If `!previousMessage.Success`: this is a retry of a failed prior attempt. Set `IsDuplicate = true`, `ShouldProcess = true`, `Reason = "Previous processing failed, retrying"`. Keep one concise log at DEBUG (`fill_id`, `previous_error`).
        - Else if `fill.Version != previousMessage.Version || fill.LastFilledTimestamp != previousMessage.LastFilledTimestamp`: this is a legitimate next update. Set `IsDuplicate = false`, `ShouldProcess = true`, `Reason = "New/updated message, processing"`. Log at DEBUG only (e.g. `fill_id`, `message_key`, previous vs current version, previous vs current lastFilledTimestamp). Do NOT use "duplicate"/"significant changes"/"correction" wording.
        - Else (version AND lastFilledTimestamp both unchanged): exact duplicate. Set `IsDuplicate = true`, `ShouldProcess = false`, `Reason = "Exact duplicate (version and lastFilledTimestamp unchanged), skipping"`. Log at DEBUG (`fill_id`, `time_since_processed`).
      - The result struct is still initialized with `PreviousMessage: previousMessage`; just adjust `IsDuplicate`/`ShouldProcess`/`Reason` per branch (do not rely on the initial `IsDuplicate: exists` default — set it explicitly in each branch for clarity).
      Files: internal/service/duplicate_detection_service.go
      Verify: `go build ./...` succeeds (tests updated in item 5 before `go test`).

- [ ] 3. Delete the now-unused `hasSignificantChanges` method.
      Remove the method (~line 239–263) in its entirety. Grep first to reconfirm no remaining callers outside the test deleted in item 5: `grep -rn hasSignificantChanges internal/`.
      Files: internal/service/duplicate_detection_service.go
      Verify: `go vet ./...` clean (would flag the method only if a reference remained; it is a method so unused is allowed by the compiler, but vet + the grep confirm removal is clean). `go build ./...` succeeds.

- [ ] 4. Remove the caller's "Processing duplicate message with changes" INFO log in `checkForDuplicates`.
      In `confirmation_service.go` `checkForDuplicates` (~line 175–190): keep the skip contract — `if duplicateResult.IsDuplicate && !duplicateResult.ShouldProcess { return true, duplicateResult.Reason }`, else `return false, ""`. Delete the `if duplicateResult.IsDuplicate { cs.logger...Info("Processing duplicate message with changes" ...) }` block entirely (it only fired on the old significant-changes path, which no longer exists; updated messages now return IsDuplicate=false and failed-retries return skip=false and are processed normally). Confirm the sole caller at ~line 114 still compiles against the unchanged `(bool, string)` signature — the "Skipping duplicate message processing" INFO at ~line 116 is left as-is since it now only fires on rare true duplicates.
      Files: internal/service/confirmation_service.go
      Verify: `go build ./...` succeeds; `go vet ./...` clean.

- [ ] 5. Update the duplicate-detection tests to the new semantics.
      In `duplicate_detection_service_test.go`:
      - `TestDuplicateDetectionService_CheckDuplicate_NewMessage`: give the fill a `LastFilledTimestamp` (e.g. `3.0`); assertion for new message is unchanged (IsDuplicate false, ShouldProcess true, Reason "New message, not previously processed").
      - `TestDuplicateDetectionService_CheckDuplicate_ExactDuplicate` (case **a**): record then check the SAME fill with identical Version AND LastFilledTimestamp. Assert `IsDuplicate == true`, `ShouldProcess == false`, `Reason == "Exact duplicate (version and lastFilledTimestamp unchanged), skipping"`.
      - Replace `TestDuplicateDetectionService_CheckDuplicate_SignificantChanges` with `TestDuplicateDetectionService_CheckDuplicate_UpdatedMessage` (case **b**): record an original fill (Version 1, LastFilledTimestamp 3.0), then check an updated fill with a higher Version (2) and/or later LastFilledTimestamp (4.0) — add a sub-case for version-only bump, timestamp-only bump, and both. Assert `IsDuplicate == false`, `ShouldProcess == true`, `Reason == "New/updated message, processing"`, and explicitly assert the reason does NOT contain "significant"/"correction".
      - `TestDuplicateDetectionService_CheckDuplicate_FailedPrevious` (case **c**): record with `success=false` using some Version/LastFilledTimestamp, then check the same fill. Assert `IsDuplicate == true`, `ShouldProcess == true`, `Reason == "Previous processing failed, retrying"`.
      - Delete `TestDuplicateDetectionService_hasSignificantChanges` (its subject method is removed in item 3).
      - In `TestDuplicateDetectionService_RecordProcessedMessage`, add `LastFilledTimestamp` to the fill and assert `processedMessage.LastFilledTimestamp == fill.LastFilledTimestamp`.
      Files: internal/service/duplicate_detection_service_test.go
      Verify: `go test ./internal/service/ -run DuplicateDetection -v` — all duplicate-detection tests pass.

- [ ] 6. Full build, vet, and test sweep.
      Run the whole verification suite from the workspace root to confirm nothing else referenced the removed method or old reasons (notably `confirmation_service_test.go`, which already sets LastFilledTimestamp on its fills).
      Files: (none — verification only)
      Verify: `go build ./...` succeeds; `go vet ./...` clean; `go test ./...` all packages pass.

## Non-goals (do not touch)

Kafka consumer, worker routing, retry/resilience manager, execution-service client, the PUT update logic, the message key format (`fill_<fillID>_exec_<execID>`), cache cleanup/retention/maxEntries, and the "superfluous response.WriteHeader" otelhttp log. Only the duplicate-detection decision, its logging, the `ProcessedMessage` field addition, the caller's one INFO log, and the corresponding tests change.

## Notes / assumptions

- Float equality on `LastFilledTimestamp` is intentional and safe: both sides are the same `float64` copied from `domain.Fill`, and the desired semantic is exact-equality. No epsilon tolerance.
- `QuantityFilled`/`AveragePrice` remain on `ProcessedMessage` and in `RecordProcessedMessage` because `GetProcessedMessageStats` and diagnostics may use them; they simply no longer influence the duplicate decision. (They can be removed in a later cleanup if confirmed unused, but that is out of scope here.)

---

## Verification results (iteration 1)

Run from workspace root:

- `go build ./...` => succeeded (BUILD_OK, no errors).
- `go vet ./...` => clean (VET_OK, no diagnostics).
- `go test ./...` => all packages pass (`internal/service` ok 0.512s; all other packages ok or no test files).

Grep confirmations:
- The three INFO messages ("Duplicate message detected", "Processing duplicate with significant changes", "Processing duplicate message with changes") and the "significant changes" wording are no longer present anywhere in `*.go`.
- `hasSignificantChanges` has no remaining references in `*.go` (method and its test deleted).
- The only `.Info(` calls remaining in `duplicate_detection_service.go` are the out-of-scope cache-cleanup logs (`performCleanup`, `cleanupOldEntries`); the per-message duplicate decision path logs only at DEBUG.

Changes left uncommitted in the working tree per instructions.
