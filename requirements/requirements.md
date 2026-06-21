# Requirements Document

## Introduction

This feature adds a standardized set of custom Kafka consumer metrics to the GlobeCo FIX Engine service. The metrics provide observability into the Kafka consumer loop (`StartOrderIntakeLoop`) covering message processing outcomes, timing, polling behavior, and end-to-end latency. All metrics are emitted through the existing OpenTelemetry metrics pipeline and are scrapeable by Prometheus. The metric names, labels, units, histogram buckets, and semantic definitions are specified to be identical across all GlobeCo Kafka-consuming services so KASBench can compare services directly during benchmark analysis.

## Glossary

- **Consumer_Loop**: The `StartOrderIntakeLoop` goroutine in `ExecutionService` that calls `ReadMessage` on the kafka-go Reader, deserializes each record, and performs application-level processing.
- **Metrics_Registry**: The set of OTel metric instruments (counters and histograms) initialized once per process via the OTel Meter Provider, registered under the `globeco-fix-engine` service name.
- **Terminal_Outcome**: The final disposition of a consumed Kafka record — either successfully processed or permanently failed (retries exhausted, dead-lettered, or abandoned).
- **Processing_Duration**: Elapsed monotonic time between processing start and processing end for a single record.
- **Idle_Time**: Elapsed monotonic time during which the Consumer_Loop is running and available but not actively processing a record (includes time blocked in poll and time waiting between records).
- **Message_Creation_Time**: Wall-clock timestamp representing when the upstream event was created, resolved from (in priority order) a `created_at` Kafka header, a `createdAt`/`created_at` payload field, or the Kafka record timestamp.
- **Poll_Operation**: A single invocation of `ReadMessage` on the kafka-go Reader, from immediately before the call to immediately after it returns.
- **Common_Labels**: The set of labels attached to every metric: `service`, `consumer_group`, `topic`, `partition`.
- **Result_Label**: A conditional label with values `success` or `failure`, attached only to histogram observations.
- **OTel_Meter**: The `go.opentelemetry.io/otel/metric` Meter instance obtained from the global MeterProvider used to create all metric instruments.

## Requirements

### Requirement 1: Metric Instrument Initialization

**User Story:** As a platform engineer, I want all Kafka consumer metric instruments initialized once at process startup, so that metric recording is thread-safe and does not incur repeated allocation overhead.

#### Acceptance Criteria

1. WHEN the service process starts, THE Metrics_Registry SHALL create all eight metric instruments (six counters and two histograms) using the OTel_Meter, and all eight instruments SHALL be available before the Consumer_Loop begins processing messages. The six counters are: `kafka_consumer_messages_processed_total`, `kafka_consumer_messages_failed_total`, `kafka_consumer_processing_seconds_total`, `kafka_consumer_idle_seconds_total`, `kafka_consumer_records_polled_total`, `kafka_consumer_poll_seconds_total`. The two histograms are: `kafka_consumer_processing_duration_seconds`, `kafka_consumer_message_latency_seconds`.
2. THE Metrics_Registry SHALL initialize each of the six counters as a monotonically increasing Float64Counter with the exact metric name as listed in criterion 1.
3. THE Metrics_Registry SHALL initialize `kafka_consumer_processing_duration_seconds` as a Float64Histogram with explicit bucket boundaries: 0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5, 10, 30, 60.
4. THE Metrics_Registry SHALL initialize `kafka_consumer_message_latency_seconds` as a Float64Histogram with explicit bucket boundaries: 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5, 10, 30, 60, 120, 300, 600.
5. IF any metric instrument fails to be created during initialization, THEN THE Metrics_Registry SHALL return an error and the service process SHALL NOT start the Consumer_Loop.
6. THE Metrics_Registry SHALL be safe for concurrent use by multiple goroutines without external synchronization, as verified by the Go race detector producing zero data-race warnings under concurrent access.

### Requirement 2: Common Label Attachment

**User Story:** As a benchmark analyst, I want every metric observation to carry consistent identifying labels, so that I can filter and aggregate metrics by service, consumer group, topic, and partition.

#### Acceptance Criteria

1. WHEN a Kafka-consumer metric observation is recorded, THE Metrics_Registry SHALL attach the `service` label with the literal value `globeco-fix-engine`.
2. WHEN a Kafka-consumer metric observation is recorded, THE Metrics_Registry SHALL attach the `consumer_group` label with the value of the configured Kafka consumer group ID (default: `fix_engine`).
3. WHEN a Kafka-consumer metric observation is recorded, THE Metrics_Registry SHALL attach the `topic` label with the topic name from the consumed record's metadata.
4. WHEN a Kafka-consumer metric observation is recorded, THE Metrics_Registry SHALL attach the `partition` label with the base-10 string representation of the partition number from the consumed record's metadata.
5. IF the consumed record does not contain a valid partition number, THEN THE Metrics_Registry SHALL use the literal string `unknown` for the `partition` label.
6. THE Metrics_Registry SHALL NOT attach high-cardinality labels such as message key, message ID, order ID, user ID, hostname, pod UID, or Kafka offset to any metric observation.
7. WHEN the configured Kafka consumer group ID is empty or not set, THE Metrics_Registry SHALL use the literal string `unknown` for the `consumer_group` label.
8. THE Metrics_Registry SHALL attach the four common labels (`service`, `consumer_group`, `topic`, `partition`) to every metric observation produced by the Kafka consumer processing path and to no other metric observations outside that path.

### Requirement 3: Messages Processed Counter

**User Story:** As a benchmark analyst, I want to count successfully processed Kafka messages, so that I can measure throughput and compare it across services.

#### Acceptance Criteria

1. WHEN a Kafka record completes all application-level processing successfully (deserialized, ticker looked up from security service, execution record created in PostgreSQL), THE Metrics_Registry SHALL increment `kafka_consumer_messages_processed_total` by 1.
2. WHEN a Kafka record fails processing at any stage (deserialization, ticker lookup, or database persistence), THE Metrics_Registry SHALL NOT increment `kafka_consumer_messages_processed_total`.
3. WHEN a Kafka record fails on a retry attempt but later succeeds, THE Metrics_Registry SHALL increment `kafka_consumer_messages_processed_total` exactly once upon the successful terminal outcome.
4. THE Metrics_Registry SHALL increment `kafka_consumer_messages_processed_total` before the Consumer_Loop proceeds to the next ReadMessage call.

### Requirement 4: Messages Failed Counter

**User Story:** As a platform engineer, I want to count permanently failed Kafka messages, so that I can monitor error rates and detect degradation.

#### Acceptance Criteria

1. WHEN a Kafka record reaches a Terminal_Outcome of failure (the Consumer_Loop permanently skips the record due to a processing error), THE Metrics_Registry SHALL increment `kafka_consumer_messages_failed_total` by 1.
2. WHEN a Kafka record fails transiently but later succeeds, THE Metrics_Registry SHALL NOT increment `kafka_consumer_messages_failed_total`.
3. WHEN a Kafka record cannot be deserialized and the Consumer_Loop skips the record permanently, THE Metrics_Registry SHALL increment `kafka_consumer_messages_failed_total` by 1.
4. WHEN a Kafka record is deserialized successfully but a downstream service call (security ticker lookup) fails and the Consumer_Loop skips the record permanently, THE Metrics_Registry SHALL increment `kafka_consumer_messages_failed_total` by 1.
5. WHEN a Kafka record is deserialized successfully but persistence (repository Create) fails and the Consumer_Loop skips the record permanently, THE Metrics_Registry SHALL increment `kafka_consumer_messages_failed_total` by 1.
6. IF `ReadMessage` returns an error (excluding context cancellation), THEN THE Metrics_Registry SHALL NOT increment `kafka_consumer_messages_failed_total`, as no record was successfully consumed.

### Requirement 5: Processing Seconds Counter

**User Story:** As a benchmark analyst, I want to measure cumulative active processing time, so that I can distinguish CPU-bound work from idle wait time.

#### Acceptance Criteria

1. WHEN a Kafka record reaches a Terminal_Outcome, THE Metrics_Registry SHALL add the Processing_Duration (in seconds, measured via monotonic clock from immediately after `ReadMessage` returns the record successfully to immediately after the record is persisted to PostgreSQL or the Consumer_Loop abandons the record due to an unrecoverable error) to `kafka_consumer_processing_seconds_total`.
2. WHEN the Consumer_Loop is blocked in a Poll_Operation (`ReadMessage` call), THE Metrics_Registry SHALL NOT add that elapsed time to `kafka_consumer_processing_seconds_total`.
3. WHEN multiple records are processed concurrently (if the Consumer_Loop is extended to parallel processing), THE Metrics_Registry SHALL add each record's Processing_Duration independently so that the counter may increase faster than wall-clock time; WHILE the Consumer_Loop is single-threaded, this criterion is satisfied implicitly by sequential addition.
4. WHEN a Kafka record fails processing at any stage (deserialization failure, external service call failure, or database persistence failure) and the Consumer_Loop moves to the next iteration, THE Metrics_Registry SHALL add the elapsed time from processing start to the point of failure to `kafka_consumer_processing_seconds_total`.

### Requirement 6: Idle Seconds Counter

**User Story:** As a benchmark analyst, I want to measure how long the consumer is idle, so that I can identify under-utilization and compare polling efficiency across services.

#### Acceptance Criteria

1. WHILE the Consumer_Loop is blocked inside a Poll_Operation (the `ReadMessage` call) and no record is being processed, THE Metrics_Registry SHALL add the elapsed time (measured via monotonic clock) to `kafka_consumer_idle_seconds_total`.
2. WHILE the Consumer_Loop is between the return of one record's processing and the start of the next `ReadMessage` call, THE Metrics_Registry SHALL add that elapsed time to `kafka_consumer_idle_seconds_total`.
3. WHILE the Consumer_Loop is actively processing a record (from processing start to Terminal_Outcome), THE Metrics_Registry SHALL NOT add that elapsed time to `kafka_consumer_idle_seconds_total`.
4. IF `ReadMessage` returns a record successfully but the record fails deserialization or is skipped without application-level processing, THEN THE Metrics_Registry SHALL NOT count the time spent in the error-handling path as idle time (it SHALL be counted toward `kafka_consumer_processing_seconds_total`).
5. WHEN the Consumer_Loop exits due to context cancellation, THE Metrics_Registry SHALL add any elapsed idle time accumulated since the last `ReadMessage` call began up to the point of cancellation to `kafka_consumer_idle_seconds_total` before stopping accumulation.
6. WHEN the service shuts down and the Consumer_Loop exits, THE Metrics_Registry SHALL stop accumulating Idle_Time and SHALL NOT record further observations.

### Requirement 7: Processing Duration Histogram

**User Story:** As a benchmark analyst, I want a distribution of per-message processing times, so that I can analyze latency percentiles and detect outliers.

#### Acceptance Criteria

1. WHEN a Kafka record reaches a Terminal_Outcome of success, THE Metrics_Registry SHALL record one observation in seconds to `kafka_consumer_processing_duration_seconds` with the Processing_Duration value measured from immediately after `ReadMessage` returns successfully to the point of successful persistence, and Result_Label `success`.
2. WHEN a Kafka record reaches a Terminal_Outcome of failure, THE Metrics_Registry SHALL record one observation in seconds to `kafka_consumer_processing_duration_seconds` with the Processing_Duration value measured from immediately after `ReadMessage` returns successfully to the point where the error is logged and the record is skipped, and Result_Label `failure`.
3. THE Metrics_Registry SHALL use monotonic clock measurements for all Processing_Duration calculations.
4. WHEN a Kafka record requires multiple processing attempts before reaching a Terminal_Outcome, THE Metrics_Registry SHALL record exactly one histogram observation reflecting the total elapsed time from the initial processing start to the terminal outcome.
5. THE Metrics_Registry SHALL record the Processing_Duration observation with Common_Labels and the Result_Label as the only additional label beyond Common_Labels.

### Requirement 8: Message Latency Histogram

**User Story:** As a benchmark analyst, I want to measure end-to-end message latency from creation to completion, so that I can assess the full pipeline delay.

#### Acceptance Criteria

1. WHEN a Kafka record reaches a Terminal_Outcome of success and a valid Message_Creation_Time is available, THE Metrics_Registry SHALL record one observation to `kafka_consumer_message_latency_seconds` with the value (completion wall-clock time minus Message_Creation_Time) and Result_Label `success`.
2. WHEN a Kafka record reaches a Terminal_Outcome of failure and a valid Message_Creation_Time is available, THE Metrics_Registry SHALL record one observation to `kafka_consumer_message_latency_seconds` with the value (completion wall-clock time minus Message_Creation_Time) and Result_Label `failure`.
3. THE Metrics_Registry SHALL resolve Message_Creation_Time using the following priority: (a) `created_at` Kafka header parsed as Unix epoch milliseconds (integer or decimal numeric string) or RFC 3339 string, (b) `createdAt` or `created_at` payload field parsed as Unix epoch milliseconds (numeric) or RFC 3339 string, (c) Kafka record timestamp. A source is skipped if it is absent, empty, non-parseable as a numeric or RFC 3339 value, or parses to a non-positive epoch value.
4. IF no valid Message_Creation_Time is available after evaluating all three sources, THEN THE Metrics_Registry SHALL NOT record a latency observation and SHALL log a structured warning indicating the record key or offset and the reason.
5. IF the calculated latency is negative and the absolute value is less than 1 second, THEN THE Metrics_Registry SHALL clamp the latency to 0 and record the observation.
6. IF the calculated latency is negative and the absolute value is 1 second or greater, THEN THE Metrics_Registry SHALL NOT record the observation and SHALL log a structured warning.
7. THE Metrics_Registry SHALL use a wall-clock timestamp (not monotonic) for the completion time in the latency calculation, since Message_Creation_Time originates from a different process.

### Requirement 9: Records Polled Counter

**User Story:** As a benchmark analyst, I want to count how many records are returned from each poll, so that I can analyze batching efficiency and consumer throughput at the poll level.

#### Acceptance Criteria

1. WHEN `ReadMessage` returns successfully (no error), THE Metrics_Registry SHALL increment `kafka_consumer_records_polled_total` by 1.
2. WHEN `ReadMessage` returns an error that is not a context cancellation (`context.Canceled` or `context.DeadlineExceeded`), THE Metrics_Registry SHALL NOT increment `kafka_consumer_records_polled_total`.
3. WHEN `ReadMessage` returns a context cancellation error, THE Metrics_Registry SHALL NOT increment `kafka_consumer_records_polled_total` and SHALL NOT treat the call as a completed Poll_Operation for any metric.

### Requirement 10: Poll Seconds Counter

**User Story:** As a benchmark analyst, I want to measure time spent inside poll calls, so that I can understand Kafka broker latency and consumer blocking behavior.

#### Acceptance Criteria

1. WHEN a Poll_Operation completes successfully, THE Metrics_Registry SHALL add the elapsed time (in seconds, measured via monotonic clock from immediately before to immediately after the poll call) to `kafka_consumer_poll_seconds_total`.
2. WHEN a Poll_Operation completes with an error (excluding context cancellation due to shutdown), THE Metrics_Registry SHALL add the elapsed time (in seconds, measured via monotonic clock) to `kafka_consumer_poll_seconds_total`.
3. WHEN the Consumer_Loop is processing a record after poll returns, THE Metrics_Registry SHALL NOT add that processing time to `kafka_consumer_poll_seconds_total`.
4. IF the Consumer_Loop is terminated via context cancellation, THEN THE Metrics_Registry SHALL NOT record the elapsed time of the interrupted Poll_Operation to `kafka_consumer_poll_seconds_total`.

### Requirement 11: Prometheus Export Compatibility

**User Story:** As a platform engineer, I want metrics exported in Prometheus-compatible format, so that existing scraping infrastructure works without configuration changes.

#### Acceptance Criteria

1. WHEN Prometheus scrapes the `/metrics` endpoint, THE Metrics_Registry SHALL expose all OTel-registered counter metrics with the `_total` suffix in the Prometheus text exposition format.
2. WHEN Prometheus scrapes the `/metrics` endpoint, THE Metrics_Registry SHALL expose all OTel-registered histogram metrics with `_bucket`, `_count`, and `_sum` series in the Prometheus text exposition format.
3. THE Metrics_Registry SHALL use snake_case for all metric names and label names.
4. THE Metrics_Registry SHALL respond to HTTP GET requests on the `/metrics` endpoint with a `Content-Type` of `text/plain` or `application/openmetrics-text` and HTTP status 200 within 5 seconds.
5. IF the `/metrics` endpoint encounters an internal error during metric collection, THEN THE Metrics_Registry SHALL respond with HTTP status 500 and an empty or partial body, without terminating the service process.
6. THE Metrics_Registry SHALL expose metrics registered through the OpenTelemetry MeterProvider via the Prometheus bridge alongside any metrics registered directly with the Prometheus client library.

### Requirement 12: Non-Functional Performance Constraints

**User Story:** As a platform engineer, I want the metrics implementation to have negligible impact on consumer performance, so that benchmark results reflect actual processing behavior.

#### Acceptance Criteria

1. THE Metrics_Registry SHALL NOT perform network calls during metric recording operations.
2. THE Metrics_Registry SHALL NOT add more than 1 millisecond of synchronous latency to any single metric recording operation under normal operating conditions (no resource exhaustion).
3. THE Metrics_Registry SHALL allocate metric instrument state only at initialization time and SHALL NOT allocate per-message heap objects during metric recording, ensuring memory consumption remains proportional to label cardinality (Common_Labels × Result_Label combinations) rather than to the number of messages processed.
4. THE Metrics_Registry SHALL be safe for concurrent use by multiple goroutines without external locking, verifiable via Go race detector with no data races reported.
5. THE Metrics_Registry SHALL NOT alter existing business logic, Kafka commit behavior, or message processing outcomes.
6. IF a metric recording operation encounters an internal error (instrument nil, attribute error, or SDK panic), THEN THE Metrics_Registry SHALL suppress the error without propagating it to the Consumer_Loop and SHALL continue processing subsequent records.

### Requirement 13: Timing Measurement Accuracy

**User Story:** As a benchmark analyst, I want timing measurements to use monotonic clocks, so that results are not affected by system clock adjustments.

#### Acceptance Criteria

1. THE Metrics_Registry SHALL use a monotonic clock source (Go `time.Now()` difference via `time.Since()` or `time.Duration`) for all elapsed-duration calculations (Processing_Duration, Idle_Time, poll elapsed time).
2. THE Metrics_Registry SHALL use wall-clock timestamps only for Message_Creation_Time to completion-time latency calculations.
3. THE Metrics_Registry SHALL record all duration values in seconds as float64 numbers with nanosecond-level granularity (no rounding or truncation coarser than 1 nanosecond).
4. IF a monotonic duration measurement yields a non-positive value, THEN THE Metrics_Registry SHALL clamp the value to 0 before recording.

### Requirement 14: kafka-go ReadMessage Adaptation

**User Story:** As a developer, I want the metrics instrumentation to work with the existing kafka-go `ReadMessage` API (which combines poll and single-message fetch), so that the implementation integrates cleanly with the current consumer architecture.

#### Acceptance Criteria

1. WHEN the Consumer_Loop calls `ReadMessage`, THE Metrics_Registry SHALL treat the entire `ReadMessage` call as a single Poll_Operation returning exactly 1 record (on success) or 0 records (on error), measuring elapsed time via monotonic clock from immediately before the call to immediately after it returns.
2. WHEN `ReadMessage` returns successfully, THE Metrics_Registry SHALL increment `kafka_consumer_records_polled_total` by 1.
3. WHEN `ReadMessage` returns successfully, THE Metrics_Registry SHALL add the elapsed `ReadMessage` duration (in seconds) to both `kafka_consumer_poll_seconds_total` and `kafka_consumer_idle_seconds_total`.
4. IF `ReadMessage` returns an error that is not a context cancellation (i.e., `ctx.Err()` is nil), THEN THE Metrics_Registry SHALL add the elapsed `ReadMessage` duration (in seconds) to both `kafka_consumer_poll_seconds_total` and `kafka_consumer_idle_seconds_total`, and SHALL NOT increment `kafka_consumer_records_polled_total`.
5. IF `ReadMessage` returns an error due to context cancellation (i.e., `ctx.Err()` is non-nil), THEN THE Metrics_Registry SHALL NOT record any metric observations for that call.
