# Implementation Plan: Kafka Consumer Metrics

## Overview

Add standardized Kafka consumer metrics instrumentation to the GlobeCo FIX Engine's `StartOrderIntakeLoop`. This involves creating a new `internal/metrics` package with metric instruments and helper functions, modifying `InitOTel` to add a Prometheus exporter reader, instrumenting the consumer loop, and wiring everything together in `main.go`.

## Tasks

- [x] 1. Add dependencies and create metrics package structure
  - [x] 1.1 Add new Go dependencies
    - Run `go get go.opentelemetry.io/otel/exporters/prometheus` to add the OTel Prometheus bridge exporter
    - Run `go get github.com/leanovate/gopter` to add the property-based testing library (test dependency)
    - Run `go mod tidy` to clean up
    - _Requirements: 11.6_

  - [x] 1.2 Create `internal/metrics/consumer_metrics.go` with ConsumerMetrics struct and constructor
    - Define the `ConsumerMetrics` struct with all eight OTel instruments (six Float64Counters and two Float64Histograms) and pre-computed common attributes
    - Implement `NewConsumerMetrics(meter metric.Meter, consumerGroup string) (*ConsumerMetrics, error)` that creates all instruments with exact metric names: `kafka_consumer_messages_processed_total`, `kafka_consumer_messages_failed_total`, `kafka_consumer_processing_seconds_total`, `kafka_consumer_idle_seconds_total`, `kafka_consumer_records_polled_total`, `kafka_consumer_poll_seconds_total`, `kafka_consumer_processing_duration_seconds`, `kafka_consumer_message_latency_seconds`
    - Set explicit histogram bucket boundaries per design: processing duration (0.005 to 60) and message latency (0.010 to 600)
    - Return error if any instrument creation fails
    - Pre-compute `commonAttrs` with `service=globeco-fix-engine` and `consumer_group` (fallback to `"unknown"` if empty)
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 2.1, 2.2, 2.7_

  - [x] 1.3 Implement recording methods on ConsumerMetrics
    - Implement `RecordPollSuccess(ctx, pollDuration, topic, partition)` — increments `records_polled_total` by 1, adds pollDuration to `poll_seconds_total` and `idle_seconds_total`, with topic/partition labels
    - Implement `RecordPollError(ctx, pollDuration)` — adds pollDuration to `poll_seconds_total` and `idle_seconds_total` without incrementing records polled
    - Implement `RecordProcessingSuccess(ctx, processingDuration, latency, topic, partition)` — increments `messages_processed_total`, adds processingDuration to `processing_seconds_total`, records histogram observations with `result=success`
    - Implement `RecordProcessingFailure(ctx, processingDuration, latency, topic, partition)` — increments `messages_failed_total`, adds processingDuration to `processing_seconds_total`, records histogram observations with `result=failure`
    - Ensure all methods suppress panics/errors from OTel SDK (recover if panic, no error propagation)
    - Use `strconv.Itoa` for partition label, fallback to `"unknown"` for invalid partition
    - _Requirements: 2.3, 2.4, 2.5, 2.6, 2.8, 3.1, 3.2, 4.1, 4.6, 5.1, 5.2, 6.1, 6.3, 7.1, 7.2, 7.5, 8.1, 8.2, 9.1, 9.2, 10.1, 10.2, 10.3, 12.1, 12.5, 12.6_

  - [x] 1.4 Create `internal/metrics/creation_time.go` with message creation time resolution
    - Implement `ResolveMessageCreationTime(msg kafka.Message, payloadJSON []byte) (time.Time, bool)` following priority: (a) `created_at` Kafka header → (b) `createdAt`/`created_at` payload field → (c) `msg.Time`
    - For headers: parse as Unix epoch millis (int string), Unix epoch seconds (decimal), or RFC 3339
    - For payload fields: parse numeric as Unix epoch millis, strings as RFC 3339
    - Skip sources that are absent, empty, non-parseable, or parse to non-positive epoch
    - Implement `CalculateLatency(creationTime, completionTime time.Time) (float64, bool)` — compute difference in seconds, clamp negative < 1s to 0, reject negative >= 1s
    - _Requirements: 8.3, 8.4, 8.5, 8.6, 8.7, 13.2_

- [x] 2. Modify OTel initialization and wire into service
  - [x] 2.1 Modify `internal/config/otel.go` to add Prometheus exporter reader
    - Import `go.opentelemetry.io/otel/exporters/prometheus`
    - Create a Prometheus exporter via `prometheus.New()` which acts as an `metric.Reader`
    - Add the Prometheus exporter as a second reader on the `MeterProvider` using `metric.WithReader(promExporter)`
    - Keep the existing OTLP periodic reader intact (dual-reader setup)
    - The Prometheus exporter auto-registers with `prometheus.DefaultRegisterer`, making OTel instruments visible via `promhttp.Handler()`
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 11.6_

  - [x] 2.2 Add `Metrics` field to `ExecutionService` and update constructor
    - Add `Metrics *metrics.ConsumerMetrics` field to the `ExecutionService` struct in `internal/service/execution_service.go`
    - Add corresponding parameter to `NewExecutionService` constructor
    - _Requirements: 1.5, 12.4_

  - [x] 2.3 Instrument `StartOrderIntakeLoop` with metric recording
    - Capture `time.Now()` before `ReadMessage` to measure poll/idle duration
    - On context cancellation from `ReadMessage`: exit loop with no metric observations (Property 9)
    - On non-cancellation error from `ReadMessage`: call `RecordPollError` with elapsed duration, then continue
    - On successful `ReadMessage`: call `RecordPollSuccess` with elapsed duration, topic, and partition
    - Capture `time.Now()` immediately after successful read as processing start
    - On each processing failure (unmarshal, security lookup, DB create): compute processing duration, resolve message creation time, calculate latency, call `RecordProcessingFailure`, then continue
    - On processing success: compute processing duration, resolve message creation time, calculate latency, call `RecordProcessingSuccess`
    - Use `time.Since()` for monotonic duration measurements
    - Use `time.Now()` (wall clock) for completion time in latency calculations
    - Clamp non-positive durations to 0
    - _Requirements: 3.1, 3.2, 3.4, 4.1, 4.3, 4.4, 4.5, 4.6, 5.1, 5.2, 5.4, 6.1, 6.2, 6.3, 6.4, 6.5, 7.1, 7.2, 7.3, 8.1, 8.2, 9.1, 9.2, 9.3, 10.1, 10.2, 10.3, 10.4, 12.5, 13.1, 13.3, 13.4, 14.1, 14.2, 14.3, 14.4, 14.5_

  - [x] 2.4 Wire `ConsumerMetrics` into `main.go`
    - Import `internal/metrics` package
    - After `InitOTel` returns successfully, obtain a `Meter` from `otel.GetMeterProvider().Meter("globeco-fix-engine")`
    - Call `metrics.NewConsumerMetrics(meter, cfg.Kafka.ConsumerGroup)` — fatal on error
    - Pass the `ConsumerMetrics` instance to `NewExecutionService`
    - _Requirements: 1.1, 1.5_

- [x] 3. Checkpoint - Ensure build compiles and basic tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 4. Property-based tests for correctness properties
  - [ ]* 4.1 Write property test for Property 7: Message creation time priority resolution
    - **Property 7: Message creation time priority resolution**
    - **Validates: Requirements 8.3**
    - Use `gopter` to generate arbitrary Kafka messages with random combinations of `created_at` header values, payload `createdAt`/`created_at` fields, and `msg.Time` values
    - Assert that `ResolveMessageCreationTime` always returns the highest-priority valid source
    - Assert that lower-priority sources are ignored when higher-priority source is valid

  - [ ]* 4.2 Write property test for Property 8: Latency clamping rules
    - **Property 8: Latency observation recorded iff valid creation time and non-negative (or clampable) result**
    - **Validates: Requirements 8.1, 8.2, 8.4, 8.5, 8.6**
    - Use `gopter` to generate arbitrary `creationTime` and `completionTime` pairs
    - Assert: latency >= 0 → record as-is, -1s < latency < 0 → clamp to 0 and record, latency <= -1s → skip

  - [ ]* 4.3 Write property test for Property 2: Success and failure mutual exclusivity
    - **Property 2: Success and failure counters are mutually exclusive and exhaustive**
    - **Validates: Requirements 3.1, 3.2, 4.1, 4.3, 4.4, 4.5, 4.6**
    - Use a mock/recording meter to capture observations
    - Generate random processing outcomes and verify exactly one counter incremented per terminal outcome

  - [ ]* 4.4 Write property test for Property 3: Time accounting conservation
    - **Property 3: Time accounting conservation**
    - **Validates: Requirements 5.1, 5.2, 5.4, 6.1, 6.2, 6.3, 14.3**
    - Generate random poll durations and processing durations
    - Verify idle_seconds receives poll duration and processing_seconds receives processing duration, no overlap

  - [ ]* 4.5 Write property test for Property 4: Poll counter increments iff ReadMessage succeeds
    - **Property 4: Poll counter increments iff ReadMessage succeeds**
    - **Validates: Requirements 9.1, 9.2, 14.1, 14.2**
    - Generate random poll outcomes (success, error, cancellation)
    - Verify counter increments only on success

  - [ ]* 4.6 Write property test for Property 5: Poll seconds accumulates for non-cancelled calls
    - **Property 5: Poll seconds accumulates for all non-cancelled ReadMessage calls**
    - **Validates: Requirements 10.1, 10.2, 10.3, 10.4, 14.4**
    - Generate random poll durations and outcomes
    - Verify `poll_seconds_total` accumulates for success and non-cancellation error, but not for cancellation

  - [ ]* 4.7 Write property test for Property 1: Common labels always present
    - **Property 1: Common labels always present and correct**
    - **Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.6, 2.8**
    - Generate random topic names, partitions, and consumer groups
    - Verify all four common labels are attached with correct values to every observation

  - [ ]* 4.8 Write property test for Property 9: Context cancellation produces zero observations
    - **Property 9: Context cancellation produces zero metric observations**
    - **Validates: Requirements 9.3, 10.4, 14.5**
    - Generate random poll durations with cancelled context
    - Verify no metric observations produced

- [ ] 5. Unit tests for specific scenarios
  - [ ]* 5.1 Write unit tests for ConsumerMetrics initialization
    - Verify all 8 instruments created with correct names
    - Verify histogram bucket boundaries match spec
    - Verify error returned on instrument creation failure
    - Verify empty consumer group falls back to `"unknown"`
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 2.7_

  - [ ]* 5.2 Write unit tests for specific failure scenarios in instrumented loop
    - Test deserialization error increments `messages_failed_total`
    - Test security service error increments `messages_failed_total`
    - Test DB persistence error increments `messages_failed_total`
    - Test successful processing increments `messages_processed_total`
    - _Requirements: 4.3, 4.4, 4.5, 3.1_

  - [ ]* 5.3 Write integration test for `/metrics` endpoint exposure
    - Start service with Prometheus exporter enabled
    - Produce a test message and wait for consumption
    - Scrape `/metrics` and verify OTel-registered metrics appear in Prometheus text format with `_total`, `_bucket`, `_count`, `_sum` suffixes
    - Verify HTTP 200 with correct Content-Type
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 11.6_

- [x] 6. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document using `gopter`
- Unit tests validate specific examples and edge cases
- The implementation uses Go 1.24 with existing project dependencies plus `go.opentelemetry.io/otel/exporters/prometheus` and `github.com/leanovate/gopter`
- No business logic changes are required — metrics are purely observational

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2", "1.4"] },
    { "id": 2, "tasks": ["1.3", "2.1"] },
    { "id": 3, "tasks": ["2.2"] },
    { "id": 4, "tasks": ["2.3", "2.4"] },
    { "id": 5, "tasks": ["4.1", "4.2", "4.7"] },
    { "id": 6, "tasks": ["4.3", "4.4", "4.5", "4.6", "4.8"] },
    { "id": 7, "tasks": ["5.1", "5.2", "5.3"] }
  ]
}
```
