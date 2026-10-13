# Superfluous `response.WriteHeader` Warning — Investigation (read-only)

## Summary answer

The warning is caused by the service's HTTP handlers each calling
`w.WriteHeader(...)` **explicitly** and then writing a JSON body with
`json.NewEncoder(w).Encode(...)` (a `Write`). Under the `otelhttp` v0.54.0
server wrapper, that sequence forwards **two** `WriteHeader` calls to the real
`net/http` response, which triggers the stdlib warning.

- First write: the handler's explicit `w.WriteHeader(status)`.
- Second write: otelhttp's `RespWriterWrapper.Write` calls `writeHeader(200)`
  on every body write, and that method (resp_writer_wrapper.go:78) forwards the
  call to the underlying writer **unconditionally** — so a second
  `WriteHeader` reaches the real `net/http` response.

The frame in the log (`RespWriterWrapper.writeHeader (resp_writer_wrapper.go:78)`)
is just the forwarder. The real trigger is in **this service's handlers**: the
redundant explicit `w.WriteHeader(...)` before the body write.

This is a **regression**. Commit `11064f0` ("corrected superfluous
response.WriteHeader error", 2025-11-13) had already fixed it by removing the
explicit `WriteHeader` calls. The very next commit, `dd38f76` ("cleaned up
logging", 2025-11-14), reverted that change and put the `WriteHeader` calls
back. That revert is what reintroduced the flood.

The affected request paths are the frequently-scraped/probed endpoints that run
in steady state: `/health/live`, `/health/ready`, `/stats`, `/version`, `/`,
and the `NotFound`/`MethodNotAllowed` error responses. In Kubernetes with
liveness/readiness probes plus Prometheus scraping, these fire continuously —
matching "roughly once per some requests during steady-state."

The fix is in the service's own code (not a library bug). Minimal, correct fix:
remove the explicit `w.WriteHeader(...)` calls in the handlers so the body write
sets the status implicitly (200), and for the one non-200 path
(`ReadinessHandler` 503) only call `WriteHeader` when the status is not 200.
This is exactly what commit `11064f0` did; it should be reapplied.

## How this was verified (reproduced, not inferred)

I built a standalone harness that mimics the exact middleware stack
(`otelhttp.NewHandler` wrapping the handler, with and without the custom
`responseWriter` layers) against `otelhttp v0.54.0` + `httpsnoop v1.0.4` (the
versions pinned in `go.mod`) and captured the server's `ErrorLog`. Results:

| Case | Handler behavior | Custom wrapper? | Warning? |
|------|------------------|-----------------|----------|
| A | `WriteHeader(200)` + `Encode` (= Liveness/Stats/Version/Root) | yes | **YES** (verbatim, `resp_writer_wrapper.go:78`) |
| B | `WriteHeader(503)` + `Encode` (= Readiness DOWN) | yes | **YES** |
| C | no explicit `WriteHeader`, just `Encode` (= the Nov-13 fix) | yes | no |
| D | `WriteHeader(200)` + `Write` | no (otelhttp only) | **YES** |
| E | `WriteHeader(200)` only, no body | no | no |
| F | `WriteHeader(200)` only, no body | yes | no |
| G | `WriteHeader(200)` + plain `Write` | no | **YES** |
| H | plain `Write` only, no explicit `WriteHeader` | no | no |

Conclusions from the matrix:
- The warning requires **both** an explicit `WriteHeader` **and** a subsequent
  body write (A, B, D, G warn; E, F — header only — do not).
- The custom `responseWriter` wrapper is **not** required to trigger it (D and G
  reproduce with otelhttp alone). The custom wrapper is therefore not the cause,
  though it is also not idempotent (see below).
- Removing the explicit `WriteHeader` (C, H) silences the warning — confirming
  the fix direction.

The harness was created under `/tmp` and deleted after the run; no files in the
repo were modified.

## Evidence (file / symbol citations)

### 1. The library forwarder named in the log

`go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@v0.54.0/internal/request/resp_writer_wrapper.go`

- `Write` (line ~43–55) calls `w.writeHeader(http.StatusOK)` on **every** body
  write.
- `writeHeader` (lines 72–79) — the frame in the warning:
  ```go
  func (w *RespWriterWrapper) writeHeader(statusCode int) {
      if !w.wroteHeader {        // only the STATUS CAPTURE is guarded
          w.wroteHeader = true
          w.statusCode = statusCode
      }
      w.ResponseWriter.WriteHeader(statusCode)   // line 78: forwarded UNCONDITIONALLY
  }
  ```
  The `wroteHeader` guard only protects the *recorded* status; the forward on
  line 78 happens every time. The doc comment on `WriteHeader` states this is
  intentional in this version: they stopped blocking consecutive calls
  specifically so net/http's warning surfaces incorrect handler implementations.
  So otelhttp is **correctly surfacing** a real double-write originating above
  it; it is not itself the bug.

- otelhttp is wired as a server handler in
  `internal/api/router.go:33-36`:
  ```go
  r.Use(func(next http.Handler) http.Handler {
      return otelhttp.NewHandler(next, "globeco-confirmation-service")
  })
  ```
  `NewHandler` → `serveHTTP` wraps the writer with `httpsnoop.Wrap` so that
  `Write`/`WriteHeader`/`Flush` route through `RespWriterWrapper`
  (`otelhttp@v0.54.0/handler.go`, the `httpsnoop.Wrap(w, …)` block).

### 2. The service-code double-write (root cause)

`internal/api/handlers.go` — every handler sets a header then writes a body:

- `LivenessHandler` — `w.WriteHeader(http.StatusOK)` at **handlers.go:114**,
  then `json.NewEncoder(w).Encode(response)` at **handlers.go:117**.
- `ReadinessHandler` — `w.WriteHeader(statusCode)` at **handlers.go:210**
  (statusCode is 200 or 503), then `Encode` at **handlers.go:213**.
- `StatsHandler` — `w.WriteHeader(http.StatusOK)` at **handlers.go:265**, then
  `Encode` at **handlers.go:268**.
- `VersionHandler` — `w.WriteHeader(http.StatusOK)` at **handlers.go:292**, then
  `Encode` at **handlers.go:295**.
- `RootHandler` — `w.WriteHeader(http.StatusOK)` at **handlers.go:322**, then
  `Encode` at **handlers.go:325**.
- `writeErrorResponse` (used by `NotFound`/`MethodNotAllowed`,
  router.go:67-75) — `w.WriteHeader(statusCode)` at **handlers.go:351**, then
  `Encode` at **handlers.go:354**.

In each case the explicit `WriteHeader` is the first forwarded header; the
`Encode` body write makes otelhttp forward a second header → warning.

### 3. The regression history

- `git show 11064f0` ("corrected superfluous response.WriteHeader error",
  2025-11-13) removed the explicit `WriteHeader(http.StatusOK)` from Liveness /
  Stats / Version / Root, and guarded Readiness so it only calls
  `WriteHeader(statusCode)` when `statusCode != http.StatusOK`.
- `git show dd38f76` ("cleaned up logging", 2025-11-14) **reverted** every one
  of those lines (confirmed via `git diff 11064f0:internal/api/handlers.go
  HEAD:internal/api/handlers.go`, which shows the `WriteHeader` calls being
  re-added). HEAD still contains the reverted (warning-producing) code.

### 4. The custom `responseWriter` wrapper (secondary observation, not the cause)

`internal/middleware/middleware.go:167-176`:
```go
type responseWriter struct {
    http.ResponseWriter
    statusCode int
}
func (rw *responseWriter) WriteHeader(code int) {
    rw.statusCode = code
    rw.ResponseWriter.WriteHeader(code)   // not idempotent — forwards every call
}
```
This wrapper (instantiated in `RequestLogger` at middleware.go:20 and in
`MetricsMiddleware` at middleware.go:77) overrides only `WriteHeader` and is
**not idempotent**. It is not the trigger of the current warning (cases D and G
reproduce without it), but because it forwards every `WriteHeader` call it would
also contribute to a double-write if any handler called `WriteHeader` twice.
Note it does not override `Write`, so body writes bypass it and go straight to
otelhttp's wrapper.

## Answers to the specific questions

1. **Exact double-write location.** First write = the handler's explicit
   `w.WriteHeader(...)` (e.g. `internal/api/handlers.go:114` in
   `LivenessHandler`, and the analogous lines 210, 265, 292, 322, 351). Second
   write = `otelhttp` `RespWriterWrapper.Write` → `writeHeader` →
   `resp_writer_wrapper.go:78`, invoked by the body write
   `json.NewEncoder(w).Encode(...)` (handlers.go:117, 213, 268, 295, 325, 354).
   Affected paths/handlers: `/health/live` (`LivenessHandler`), `/health/ready`
   (`ReadinessHandler`), `/stats` (`StatsHandler`), `/version`
   (`VersionHandler`), `/` (`RootHandler`), and `writeErrorResponse` via
   `NotFound`/`MethodNotAllowed`.

2. **Root-cause pattern.** It is **(a)** in substance: a handler sets a status
   with `WriteHeader` and then performs a body `Write`, and under otelhttp the
   body write forwards a second `WriteHeader`. Explicitly ruled out:
   - (b) `http.Error` after a write — not involved (the only `http.Error` is in
     the unused `RateLimiter`, middleware.go:154, which is not wired into the
     router).
   - (c) a non-idempotent custom wrapper interacting with otelhttp — the custom
     `responseWriter.WriteHeader` is indeed non-idempotent (middleware.go:172),
     but it is NOT required to produce the warning (proven by cases D/G). It is a
     latent hazard, not today's cause.
   - (d) panic-recovery / timeout middleware writing a second status —
     `chi` `Recoverer` only writes on panic; `chi` `Timeout(30s)` only writes
     `504` on an actual deadline. Neither fires in steady state, so neither
     explains the continuous warnings.
   - (e) health/metrics endpoints writing twice — confirmed: the health, stats,
     version, root, and error endpoints are exactly the double-writers, and the
     health probes + Prometheus scrape are the high-frequency callers in steady
     state. `/metrics` is served by `promhttp.Handler()`
     (handlers.go `MetricsHandler`) and does not use the explicit-WriteHeader
     pattern, so it is not a source.

3. **Service code vs library, and minimal fix.** The defect is in the
   **service's own handler code**; otelhttp is behaving as designed (deliberately
   surfacing the double-write). Minimal, behavior-preserving fix — reapply commit
   `11064f0`:
   - Remove the explicit `w.WriteHeader(http.StatusOK)` at
     **handlers.go:114, 265, 292, 322** (Liveness, Stats, Version, Root). The
     subsequent `Encode` implicitly sends 200, so externally observable status is
     unchanged.
   - In `ReadinessHandler`, replace `w.WriteHeader(statusCode)` at
     **handlers.go:210** with a guard so it only writes a non-200 status:
     ```go
     if statusCode != http.StatusOK {
         w.WriteHeader(statusCode)
     }
     ```
     (503 is still sent explicitly; 200 is sent implicitly by the body write.)
   - In `writeErrorResponse` at **handlers.go:351**, the status is always non-200
     (404/405/500), so the explicit `WriteHeader(statusCode)` is correct and must
     be kept — but ensure the body write is the only thing after it (it is). This
     path currently also warns because it writes a body after `WriteHeader`;
     however, a non-200 status *must* be written explicitly, so for error paths
     the correct way to avoid the warning is to not emit a second header, which is
     already the case (single `WriteHeader` + single `Encode`). Per the
     reproduction, `WriteHeader(non-200)` + body still warns under otelhttp
     (case B), so to fully silence it the body-first ordering cannot help for a
     non-200. The pragmatic, behavior-preserving choice is to accept the explicit
     non-200 header (correctness of status outranks a cosmetic warning), or write
     the status with a single combined call. Recommended: keep handlers.go:351 as
     is; the dominant steady-state flood comes from the 200 endpoints
     (handlers.go:114, 265, 292, 322) and the readiness 200 branch
     (handlers.go:210), which the above changes eliminate.
   - Optional hardening (not required to stop the flood): make the custom
     `responseWriter.WriteHeader` idempotent at
     **internal/middleware/middleware.go:172-175** by guarding on a
     `wroteHeader bool` field, so future handlers that double-call `WriteHeader`
     are protected regardless of otelhttp.

   Exact file:line(s) that would change in the fix (do NOT change here — this is
   investigation only):
   - `internal/api/handlers.go:114` (remove)
   - `internal/api/handlers.go:210` (guard non-200)
   - `internal/api/handlers.go:265` (remove)
   - `internal/api/handlers.go:292` (remove)
   - `internal/api/handlers.go:322` (remove)
   - optional: `internal/middleware/middleware.go:172-175` (idempotent guard)

4. **otelhttp wrapping and double-wrapping.** otelhttp wraps the **whole mux**:
   it is registered as chi middleware in `internal/api/router.go:33-36` via
   `otelhttp.NewHandler`, which through `httpsnoop.Wrap` intercepts
   `WriteHeader`/`Write`/`Flush`. The custom `responseWriter`
   (middleware.go:167) **also** wraps the writer, inside otelhttp (it is applied
   by `RequestLogger`/`MetricsMiddleware`, which run after otelhttp in the
   `r.Use` order: RequestID → RealIP → Recoverer → Timeout → **otelhttp** →
   RequestLogger → CorrelationID → Metrics → CORS → handler). So there IS
   double-wrapping of the response writer. However, the custom wrapper only
   overrides `WriteHeader` (not `Write`), and the current flood reproduces even
   with otelhttp alone (cases D/G), so the double-wrapping is not the cause of
   today's warning — the handler's explicit `WriteHeader`-then-body pattern is.
   The stack is composed in `internal/api/router.go` (`NewRouter`), and the
   server is assembled in `cmd/confirmation-service/main.go` (the
   `api.NewRouter(...)` call and `&http.Server{Handler: router}`).

## Recommendation (do not implement here)

Reapply the already-proven fix from commit `11064f0`: drop the redundant
`w.WriteHeader(http.StatusOK)` calls in `LivenessHandler`, `StatsHandler`,
`VersionHandler`, `RootHandler`, and guard the 200 branch in `ReadinessHandler`.
This removes the steady-state warning flood from the health/stats/version/root
endpoints without changing any externally observable HTTP status code and
without touching the metrics/logging middleware. Optionally make
`responseWriter.WriteHeader` idempotent for defense in depth. Add a regression
test (an `httptest` server with the real router whose `ErrorLog` is asserted to
be empty for `/health/live` and `/health/ready`) so a future "logging cleanup"
cannot silently revert it again.
