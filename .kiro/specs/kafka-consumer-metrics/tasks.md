# Implementation Plan: Kafka Consumer Metrics (Confirmation Service)

## Overview

Add standardized Kafka consumer metrics instrumentation to the GlobeCo Confirmation Service's worker-pool-based Kafka consumer. This involves creating a new `internal/metrics` package with `ConsumerMetrics` struct and creation time resolution, modifying `internal/utils/otel.go` to add a Prometheus exporter reader, instrumenting `fetchLoop` and `handleMessage` in `kafka_consumer.go`, and wiring everything together in `main.go`.

## Tasks

- [ ] 1. Add dependencies and create metrics package structure
  - [ ] 1.1 Add new Go dependencies
    - Run `go get go.opentelemetry.io/otel/exporters/prometheus` to add the OTel Prometheus bridge exporter
    - Run `go get github.com/leanovate/gopter` to add the property-based testing library (test dependency)
    - Run `go mod tidy` to clean up
    - _Requirements: 11.6_

  - [ ] 1.2 Create `internal/metrics/consumer_metrics.go` with ConsumerMetrics struct and constructor
    - Define the `ConsumerMetrics` struct with all eight OTel instruments: `messagesProcessed` (Int64Counter), `messagesFailed` (Int64Counter), `processingSeconds` (Float64Counter), `idleSeconds` (Float64Counter), `recordsPolled` (Int64Counter), `pollSeconds` (Float64Counter), `processingDuration` (Float64Histogram), `messageLatency` (Float64Histogram)
    - Store pre-computed `commonAttrs` with `service=globeco-confirmation-service` and `consumer_group` (fallback to `"unknown"` if empty)
    - Implement `NewConsumerMetrics(meter metric.Meter, consumerGroup string) (*ConsumerMetrics, error)` that creates all instruments with exact metric names: `kafka_consumer_messages_processed_total`, `kafka_consumer_messages_failed_total`, `kafka_consumer_processing_seconds_total`, `kafka_consumer_idle_seconds_total`, `kafka_consumer_records_polled_total`, `kafka_consumer_poll_seconds_total`, `kafka_consumer_processing_duration_seconds`, `kafka_consumer_message_latency_seconds`
    - Set explicit histogram bucket boundaries per design: processing duration (0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5, 10, 30, 60) and message latency (0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5, 10, 30, 60, 120, 300, 600)
    - Set unit to `"s"` for duration/seconds instruments and `"1"` for count instruments
    - Return error if any instrument creation fails
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 2.1, 2.2, 2.7, 12.1, 12.2, 12.4_

  - [ ] 1.3 Implement recording methods on ConsumerMetrics
    - Implement `RecordPollSuccess(ctx, pollDuration, topic, partition)` — increments `records_polled_total` by 1, adds pollDuration to `poll_seconds_total` and `idle_seconds_total`, with topic/partition labels
    - Implement `RecordPollError(ctx, pollDuration)` — adds pollDuration to `poll_seconds_total` and `idle_seconds_total`, uses `topic="unknown"` and `partition="unknown"` since no record metadata exists
    - Implement `RecordProcessingSuccess(ctx, processingDuration, latency *float64, topic, partition)` — increments `messages_processed_total`, adds processingDuration to `processing_seconds_total`, records `processing_duration_seconds` histogram with `result=success`, records `message_latency_seconds` histogram with `result=success` if latency != nil
    - Implement `RecordProcessingFailure(ctx, processingDuration, latency *float64, topic, partition)` — increments `messages_failed_total`, adds processingDuration to `processing_seconds_total`, records `processing_duration_seconds` histogram with `result=failure`, records `message_latency_seconds` histogram with `result=failure` if latency != nil
    - All methods wrap operations in `defer recover()` to suppress panics from OTel SDK
    - Use `strconv.Itoa` for partition label; fallback to `"unknown"` for invalid values
    - Attach `result` label only to histogram observations, not to counters
    - _Requirements: 2.3, 2.4, 2.5, 2.6, 2.8, 2.9, 3.1, 3.2, 4.1, 4.6, 5.1, 5.2, 6.1, 6.3, 7.1, 7.2, 7.4, 7.5, 8.1, 8.2, 9.1, 9.2, 10.1, 10.2, 10.3, 12.3, 12.7, 14.4, 14.5_

  - [ ] 1.4 Create `internal/metrics/creation_time.go` with message creation time resolution
    - Implement `ResolveMessageCreationTime(msg kafka.Message, payloadJSON []byte) (time.Time, bool)` following priority: (a) `created_at` Kafka header → (b) `createdAt`/`created_at` payload field → (c) `msg.Time`
    - For headers: parse as Unix epoch millis (integer string), Unix epoch seconds (decimal string), or RFC 3339
    - For payload fields: parse numeric JSON value as Unix epoch millis, string JSON value as RFC 3339
    - Skip sources that are absent, empty, non-parseable, or parse to non-positive epoch (≤ 0)
    - Implement `CalculateLatency(creationTime, completionTime time.Time) (float64, bool)` — compute `completionTime - creationTime` in seconds; if >= 0 record as-is, if -1s < value < 0 clamp to 0, if <= -1s skip (return false)
    - _Requirements: 8.3, 8.4, 8.5, 8.6, 8.7, 8.8, 12.5, 12.6_

- [ ] 2. Modify OTel initialization and instrument Kafka consumer
  - [ ] 2.1 Modify `internal/utils/otel.go` to add Prometheus exporter reader
    - Import `go.opentelemetry.io/otel/exporters/prometheus`
    - In `SetupOTel`, create a Prometheus exporter via `prometheus.New()` which returns an `metric.Reader`
    - Add the Prometheus exporter as a second reader on the `MeterProvider` using `metric.WithReader(promExporter)` alongside the existing OTLP periodic reader
    - Keep the existing OTLP periodic reader intact (dual-reader setup)
    - The Prometheus exporter auto-registers with `prometheus.DefaultRegisterer`, making OTel instruments visible via `promhttp.Handler()` at `/metrics`
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 11.7_

  - [ ] 2.2 Add `ConsumerMetrics` field to `KafkaConsumerService` and update constructor
    - Import `internal/metrics` package in `internal/service/kafka_consumer.go`
    - Add `consumerMetrics *metrics.ConsumerMetrics` field to the `KafkaConsumerService` struct
    - Add `ConsumerMetrics *metrics.ConsumerMetrics` field to `KafkaConsumerConfig`
    - In `NewKafkaConsumerService`, assign `config.ConsumerMetrics` to the new struct field
    - _Requirements: 1.5, 1.7_

  - [ ] 2.3 Instrument `fetchLoop` with poll/idle metric recording
    - Capture `pollStart = time.Now()` immediately before the `FetchMessage` call
    - Compute `pollDuration = time.Since(pollStart).Seconds()` immediately after `FetchMessage` returns
    - If error is `context.Canceled` or `context.DeadlineExceeded`: continue loop with no metrics recorded
    - If error is non-nil (other error): call `consumerMetrics.RecordPollError(ctx, pollDuration)`, log error, continue
    - If success (nil error): call `consumerMetrics.RecordPollSuccess(ctx, pollDuration, message.Topic, message.Partition)`, then dispatch to messageCh
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5, 9.1, 9.2, 9.3, 9.4, 10.1, 10.2, 10.3, 10.4, 10.5, 13.1, 13.4, 13.5_

  - [ ] 2.4 Instrument `handleMessage` with processing metric recording
    - Capture `processingStart = time.Now()` at entry of `handleMessage`
    - Store `rawValue := message.Value` for creation time resolution
    - At each terminal failure point (unmarshal error, validation error, resilience-wrapped handler error):
      - Compute `processingDuration = time.Since(processingStart).Seconds()`; clamp to 0 if non-positive
      - Compute `completionTime = time.Now()`
      - Call `ResolveMessageCreationTime(message, rawValue)` to get creation time
      - If valid creation time: call `CalculateLatency(creationTime, completionTime)` to get latency
      - Call `consumerMetrics.RecordProcessingFailure(ctx, processingDuration, latencyPtr, message.Topic, message.Partition)`
    - At successful terminal outcome (after `resilienceManager.ExecuteWithResilience` succeeds):
      - Compute `processingDuration = time.Since(processingStart).Seconds()`; clamp to 0 if non-positive
      - Compute `completionTime = time.Now()`
      - Resolve creation time and calculate latency as above
      - Call `consumerMetrics.RecordProcessingSuccess(ctx, processingDuration, latencyPtr, message.Topic, message.Partition)`
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 5.1, 5.2, 5.3, 5.4, 5.5, 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 8.1, 8.2, 8.6, 8.7, 8.8, 12.3, 13.2, 13.3, 14.1, 14.2, 14.6_

  - [ ] 2.5 Wire `ConsumerMetrics` into `cmd/confirmation-service/main.go`
    - Import `internal/metrics` package (aliased if needed to avoid conflict with `pkg/metrics`)
    - After `SetupOTel` returns successfully, obtain a `Meter` from `otel.GetMeterProvider().Meter("globeco-confirmation-service")`
    - Call `metrics.NewConsumerMetrics(meter, cfg.Kafka.ConsumerGroup)` — fatal on error
    - Pass the `ConsumerMetrics` instance to `NewKafkaConsumerService` via `KafkaConsumerConfig.ConsumerMetrics`
    - _Requirements: 1.1, 1.5, 1.7_

- [ ] 3. Checkpoint - Ensure build compiles and basic tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 4. Property-based tests for correctness properties
  - [ ]* 4.1 Write property test for Property 7: Message creation time priority resolution
    - **Property 7: Message creation time priority resolution**
    - **Validates: Requirements 8.3, 8.4, 8.5, 12.5**
    - Use `gopter` to generate arbitrary Kafka messages with random combinations of valid/invalid/absent `created_at` header values, payload `createdAt`/`created_at` fields, and `msg.Time` values
    - Assert that `ResolveMessageCreationTime` always returns the highest-priority valid source
    - Assert that lower-priority sources are ignored when a higher-priority source is valid
    - Minimum 100 iterations

  - [ ]* 4.2 Write property test for Property 8: Latency clamping rules
    - **Property 8: Latency observation recorded iff valid creation time and non-negative (or clampable) result**
    - **Validates: Requirements 8.1, 8.2, 8.6, 8.7, 8.8, 12.6**
    - Use `gopter` to generate arbitrary `creationTime` and `completionTime` pairs spanning positive, slightly-negative, and significantly-negative latencies
    - Assert: latency >= 0 → record as-is, -1s < latency < 0 → clamp to 0 and record, latency <= -1s → skip
    - Minimum 100 iterations

  - [ ]* 4.3 Write property test for Property 2: Success and failure mutual exclusivity
    - **Property 2: Success and failure counters are mutually exclusive and exhaustive**
    - **Validates: Requirements 3.1, 3.2, 3.4, 3.5, 4.1, 4.2, 4.6**
    - Use a recording/mock `metric.Meter` to capture observations
    - Generate random processing outcomes (success, failure, cancellation) and verify exactly one counter incremented per terminal outcome, never both and never neither
    - Minimum 100 iterations

  - [ ]* 4.4 Write property test for Property 3: Time accounting conservation
    - **Property 3: Time accounting conservation**
    - **Validates: Requirements 5.1, 5.2, 5.4, 5.5, 6.1, 6.2, 6.3, 14.3**
    - Generate random poll durations and processing durations
    - Verify `idle_seconds_total` receives poll duration and `processing_seconds_total` receives processing duration with no overlap or double-counting
    - Minimum 100 iterations

  - [ ]* 4.5 Write property test for Property 4: Poll counter increments iff FetchMessage succeeds
    - **Property 4: Poll counter increments iff FetchMessage succeeds**
    - **Validates: Requirements 9.1, 9.2, 9.3, 9.4**
    - Generate random poll outcomes (success, non-cancellation error, context cancellation)
    - Verify `records_polled_total` increments only on success
    - Minimum 100 iterations

  - [ ]* 4.6 Write property test for Property 5: Poll seconds accumulates for non-cancelled calls
    - **Property 5: Poll seconds accumulates for all non-cancelled FetchMessage calls**
    - **Validates: Requirements 10.1, 10.2, 10.3, 10.4, 10.5**
    - Generate random poll durations and outcomes (success, non-cancellation error, cancellation)
    - Verify `poll_seconds_total` accumulates for success and non-cancellation error, but not for cancellation
    - Minimum 100 iterations

  - [ ]* 4.7 Write property test for Property 1: Common labels always present
    - **Property 1: Common labels always present and correct**
    - **Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.6, 2.8, 2.9**
    - Generate random topic names, partition numbers, and consumer groups
    - Verify all four common labels (`service`, `consumer_group`, `topic`, `partition`) are attached with correct values to every observation
    - Minimum 100 iterations

  - [ ]* 4.8 Write property test for Property 9: Context cancellation produces zero observations
    - **Property 9: Context cancellation produces zero metric observations**
    - **Validates: Requirements 6.3, 6.4, 9.3, 10.4, 14.5**
    - Generate random poll durations with cancelled context
    - Verify zero metric observations of any kind are produced for cancelled calls
    - Minimum 100 iterations

- [ ] 5. Unit tests and integration tests
  - [ ]* 5.1 Write unit tests for ConsumerMetrics initialization
    - Verify all 8 instruments created with correct names and types
    - Verify histogram bucket boundaries match spec for both histograms
    - Verify error returned on instrument creation failure
    - Verify empty consumer group falls back to `"unknown"`
    - Verify invalid partition falls back to `"unknown"`
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 2.5, 2.7, 15.7_

  - [ ]* 5.2 Write unit tests for specific failure scenarios
    - Test deserialization error increments `messages_failed_total` exactly once
    - Test validation error increments `messages_failed_total` exactly once
    - Test Execution Service error (retries exhausted) increments `messages_failed_total` exactly once
    - Test DLQ routing increments `messages_failed_total` exactly once with no double-count
    - Test transient failure + retry success does NOT increment `messages_failed_total`
    - Test successful processing increments `messages_processed_total` exactly once
    - _Requirements: 3.1, 3.3, 4.1, 4.2, 4.3, 4.4, 4.5, 4.6_

  - [ ]* 5.3 Write integration test for `/metrics` endpoint exposure
    - Configure a test service with Prometheus exporter enabled
    - Trigger at least one successful and one failed message processing cycle
    - Scrape `/metrics` endpoint and verify all 8 OTel-registered metrics appear in Prometheus text format with `_total` suffixes for counters and `_bucket`, `_count`, `_sum` series for histograms
    - Verify existing direct-registered Prometheus metrics remain present and unchanged
    - Verify HTTP 200 with correct Content-Type
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 11.7, 15.8_

- [ ] 6. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document using `gopter` with minimum 100 iterations each
- Unit tests validate specific examples and edge cases
- The implementation uses Go 1.23 with existing project dependencies plus `go.opentelemetry.io/otel/exporters/prometheus` and `github.com/leanovate/gopter`
- No business logic changes are required — metrics are purely observational
- OTel instruments are thread-safe; no additional synchronization is needed for the `ConsumerMetrics` struct
- The confirmation service uses a worker-pool model: `fetchLoop` measures poll/idle metrics, each `workerLoop` measures processing metrics independently
- Import alias may be needed for `internal/metrics` vs `pkg/metrics` in files that use both

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2", "1.4"] },
    { "id": 2, "tasks": ["1.3", "2.1"] },
    { "id": 3, "tasks": ["2.2", "2.5"] },
    { "id": 4, "tasks": ["2.3", "2.4"] },
    { "id": 5, "tasks": ["4.1", "4.2", "4.7"] },
    { "id": 6, "tasks": ["4.3", "4.4", "4.5", "4.6", "4.8"] },
    { "id": 7, "tasks": ["5.1", "5.2", "5.3"] }
  ]
}
```
