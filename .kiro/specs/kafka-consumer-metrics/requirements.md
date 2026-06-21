# Requirements Document

## Introduction

Implement standardized Kafka consumer metrics for the GlobeCo Confirmation Service. The metrics must be emitted with identical names, units, label keys, semantic definitions, and edge-case behavior as the GlobeCo FIX Engine implementation so that KASBench can compare the two services directly during benchmark analysis. All metrics are exported through the OpenTelemetry SDK and bridged to Prometheus via the OTel Prometheus exporter.

## Glossary

- **Consumer_Metrics_Package**: The new `internal/metrics` package containing the `ConsumerMetrics` struct and all metric instruments
- **Confirmation_Service**: The `globeco-confirmation-service` application that consumes fill messages from Kafka
- **Kafka_Consumer**: The `KafkaConsumerService` struct responsible for fetching messages from Kafka and dispatching them to workers
- **Fetch_Loop**: The single goroutine (`fetchLoop`) that calls `FetchMessage` to retrieve records from Kafka
- **Worker_Pool**: The set of `workerLoop` goroutines that process messages from the `messageCh` channel
- **Commit_Loop**: The goroutine (`commitLoop`) that batch-commits successfully processed message offsets
- **Message_Handler**: The `HandleFillMessage` method in `ConfirmationService` that executes application-level business logic
- **Resilience_Manager**: The `ResilienceManager` component that handles retries, circuit breaking, and dead-letter routing
- **OTel_Meter_Provider**: The OpenTelemetry `MeterProvider` configured with dual readers (OTLP periodic + Prometheus exporter)
- **Prometheus_Exporter**: The `go.opentelemetry.io/otel/exporters/prometheus` package that bridges OTel instruments to Prometheus scrapers
- **Message_Creation_Time**: The timestamp representing when the event/message was originally created, resolved from headers, payload, or Kafka record metadata
- **Terminal_Outcome**: The final processing result for a message — either success (all business logic completed) or failure (retries exhausted, dead-lettered, or permanently abandoned)
- **Poll_Duration**: The monotonic elapsed time from immediately before `FetchMessage` to immediately after it returns
- **Processing_Duration**: The monotonic elapsed time from processing start to terminal outcome within a worker
- **Common_Labels**: The set of labels (`service`, `consumer_group`, `topic`, `partition`) attached to every metric observation

## Requirements

### Requirement 1: Metric Instrument Initialization

**User Story:** As a platform engineer, I want all eight Kafka consumer metrics initialized at startup with correct names, units, and histogram boundaries, so that metrics are consistently defined and ready for recording.

#### Acceptance Criteria

1. WHEN the Confirmation_Service starts, THE Consumer_Metrics_Package SHALL create exactly eight OTel metric instruments with the following names and types: `kafka_consumer_messages_processed_total` (Int64Counter), `kafka_consumer_messages_failed_total` (Int64Counter), `kafka_consumer_processing_seconds_total` (Float64Counter), `kafka_consumer_idle_seconds_total` (Float64Counter), `kafka_consumer_records_polled_total` (Int64Counter), `kafka_consumer_poll_seconds_total` (Float64Counter), `kafka_consumer_processing_duration_seconds` (Float64Histogram), `kafka_consumer_message_latency_seconds` (Float64Histogram)
2. WHEN the Confirmation_Service starts, THE Consumer_Metrics_Package SHALL configure `kafka_consumer_processing_duration_seconds` with explicit histogram bucket boundaries: 0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5, 10, 30, 60
3. WHEN the Confirmation_Service starts, THE Consumer_Metrics_Package SHALL configure `kafka_consumer_message_latency_seconds` with explicit histogram bucket boundaries: 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5, 10, 30, 60, 120, 300, 600
4. THE Consumer_Metrics_Package SHALL set the unit to "s" for `kafka_consumer_processing_seconds_total`, `kafka_consumer_idle_seconds_total`, `kafka_consumer_poll_seconds_total`, `kafka_consumer_processing_duration_seconds`, and `kafka_consumer_message_latency_seconds`, and set the unit to "1" for `kafka_consumer_messages_processed_total`, `kafka_consumer_messages_failed_total`, and `kafka_consumer_records_polled_total`
5. IF instrument creation fails for any metric, THEN THE Consumer_Metrics_Package SHALL return an error wrapping the underlying OTel error and the Confirmation_Service SHALL NOT start the Kafka_Consumer
6. THE Consumer_Metrics_Package SHALL be safe for concurrent use by multiple Worker_Pool goroutines, such that concurrent calls to any recording method from up to 10 goroutines produce no data races and no panics
7. WHEN the Confirmation_Service starts, THE Consumer_Metrics_Package SHALL complete initialization of all eight instruments before the Kafka_Consumer begins consuming messages

### Requirement 2: Common Labels

**User Story:** As a benchmark analyst, I want every metric observation to carry consistent labels, so that I can filter and compare metrics across services, topics, and partitions.

#### Acceptance Criteria

1. THE Consumer_Metrics_Package SHALL attach the label `service` with value `globeco-confirmation-service` to every metric observation
2. THE Consumer_Metrics_Package SHALL attach the label `consumer_group` with the configured Kafka consumer group ID to every metric observation
3. WHEN a metric observation is associated with a specific Kafka message (the observation is recorded after `FetchMessage` returns a record successfully), THE Consumer_Metrics_Package SHALL attach the label `topic` with the topic name from the consumed record's metadata
4. WHEN a metric observation is associated with a specific Kafka message, THE Consumer_Metrics_Package SHALL attach the label `partition` with the base-10 string representation of the partition number from the consumed record's metadata
5. IF the consumed record does not contain a valid partition number, THEN THE Consumer_Metrics_Package SHALL use the literal string `unknown` for the `partition` label
6. THE Consumer_Metrics_Package SHALL attach the label `result` with value `success` or `failure` exclusively to histogram observations (`kafka_consumer_processing_duration_seconds` and `kafka_consumer_message_latency_seconds`) and SHALL NOT attach the `result` label to counter metrics
7. IF the consumer group configuration is empty or not set, THEN THE Consumer_Metrics_Package SHALL use the literal string `unknown` as the `consumer_group` label value
8. THE Consumer_Metrics_Package SHALL NOT attach high-cardinality labels such as message key, message ID, offset, hostname, pod UID, order ID, or portfolio ID to any metric observation
9. THE Consumer_Metrics_Package SHALL attach the four common labels (`service`, `consumer_group`, `topic`, `partition`) to every metric observation produced by the Kafka consumer processing path and to no other metric observations outside that path

### Requirement 3: Messages Processed Counter

**User Story:** As a benchmark analyst, I want to count successfully processed messages, so that I can measure throughput and success rates.

#### Acceptance Criteria

1. WHEN a Kafka fill message reaches a successful Terminal_Outcome (all business logic in Message_Handler completed without error), THE Consumer_Metrics_Package SHALL increment `kafka_consumer_messages_processed_total` by exactly 1
2. WHEN a Kafka fill message reaches a failed Terminal_Outcome (retries exhausted, dead-lettered, or permanently abandoned by the Resilience_Manager), THE Consumer_Metrics_Package SHALL NOT increment `kafka_consumer_messages_processed_total`
3. WHEN a message fails on a retry attempt but later succeeds through the Resilience_Manager, THE Consumer_Metrics_Package SHALL increment `kafka_consumer_messages_processed_total` exactly once upon the successful Terminal_Outcome
4. WHEN a `FetchMessage` call returns an error, THE Consumer_Metrics_Package SHALL NOT increment `kafka_consumer_messages_processed_total`
5. IF processing is interrupted by context cancellation before reaching a Terminal_Outcome, THEN THE Consumer_Metrics_Package SHALL NOT increment `kafka_consumer_messages_processed_total` for that message

### Requirement 4: Messages Failed Counter

**User Story:** As a benchmark analyst, I want to count terminally failed messages, so that I can measure error rates and identify reliability issues.

#### Acceptance Criteria

1. WHEN JSON deserialization of a Kafka fill message fails, THE Consumer_Metrics_Package SHALL increment `kafka_consumer_messages_failed_total` by exactly 1
2. WHEN validation of a fill message fails (missing required fields, invalid data types, or business rule violations), THE Consumer_Metrics_Package SHALL increment `kafka_consumer_messages_failed_total` by exactly 1
3. WHEN the Execution Service GET or PUT call fails and all 3 retry attempts are exhausted, THE Consumer_Metrics_Package SHALL increment `kafka_consumer_messages_failed_total` by exactly 1
4. WHEN a message is routed to the dead-letter queue by the Resilience_Manager, THE Consumer_Metrics_Package SHALL increment `kafka_consumer_messages_failed_total` by exactly 1 and SHALL NOT double-count if the same message processing path already incremented the counter
5. IF a message fails transiently on an Execution Service call but succeeds on a subsequent retry within the configured 3 retry attempts, THEN THE Consumer_Metrics_Package SHALL NOT increment `kafka_consumer_messages_failed_total`
6. THE Consumer_Metrics_Package SHALL increment `kafka_consumer_messages_failed_total` at most once per message, regardless of how many failure conditions a single message triggers during processing

### Requirement 5: Processing Seconds Counter

**User Story:** As a benchmark analyst, I want to measure total active processing time, so that I can compute processing utilization across the worker pool.

#### Acceptance Criteria

1. WHEN a Kafka fill message reaches a Terminal_Outcome (success or failure), THE Consumer_Metrics_Package SHALL add the Processing_Duration (in seconds, measured via monotonic clock from immediately after the worker receives the message from the channel to immediately after the message reaches Terminal_Outcome) to `kafka_consumer_processing_seconds_total`
2. WHEN multiple Worker_Pool goroutines process messages concurrently, THE Consumer_Metrics_Package SHALL add each message's Processing_Duration independently so that the counter may increase faster than wall-clock time
3. THE Consumer_Metrics_Package SHALL measure Processing_Duration using a monotonic clock (Go's `time.Since()`)
4. WHEN a `FetchMessage` call blocks, THE Consumer_Metrics_Package SHALL NOT add that elapsed time to `kafka_consumer_processing_seconds_total`
5. WHEN a Kafka fill message fails processing at any stage (deserialization failure, Execution Service call failure, or any unrecoverable error), THE Consumer_Metrics_Package SHALL add the elapsed time from processing start to the point of failure to `kafka_consumer_processing_seconds_total`

### Requirement 6: Idle Seconds Counter

**User Story:** As a benchmark analyst, I want to measure consumer idle time, so that I can compute worker utilization and identify underutilized capacity.

#### Acceptance Criteria

1. WHEN the Fetch_Loop completes a `FetchMessage` call that returns successfully or with a non-cancellation error, THE Consumer_Metrics_Package SHALL add the elapsed Poll_Duration (measured via monotonic clock from immediately before the call to immediately after it returns) to `kafka_consumer_idle_seconds_total`
2. THE Consumer_Metrics_Package SHALL NOT add active message Processing_Duration to `kafka_consumer_idle_seconds_total`
3. IF `FetchMessage` returns due to context cancellation, THEN THE Consumer_Metrics_Package SHALL NOT add the elapsed Poll_Duration for that call to `kafka_consumer_idle_seconds_total`
4. WHEN the Confirmation_Service is shut down, THE Consumer_Metrics_Package SHALL stop accumulating idle time and SHALL NOT record Poll_Duration from any `FetchMessage` call that was interrupted by the shutdown signal
5. WHILE no `FetchMessage` call is in progress (e.g., between FetchMessage return and the next FetchMessage invocation), THE Consumer_Metrics_Package SHALL NOT attribute that inter-call time to `kafka_consumer_idle_seconds_total`

### Requirement 7: Processing Duration Histogram

**User Story:** As a benchmark analyst, I want a distribution of per-message processing times, so that I can analyze latency percentiles and detect performance regressions.

#### Acceptance Criteria

1. WHEN a Kafka fill message reaches a successful Terminal_Outcome, THE Consumer_Metrics_Package SHALL record one observation to `kafka_consumer_processing_duration_seconds` with `result=success` and Common_Labels
2. WHEN a Kafka fill message reaches a failed Terminal_Outcome, THE Consumer_Metrics_Package SHALL record one observation to `kafka_consumer_processing_duration_seconds` with `result=failure` and Common_Labels
3. THE Consumer_Metrics_Package SHALL measure the observation value as the monotonic elapsed time (using Go's `time.Since()`) from the point the Worker_Pool goroutine begins handling the message to Terminal_Outcome
4. IF the measured Processing_Duration is non-positive, THEN THE Consumer_Metrics_Package SHALL clamp the value to 0
5. THE Consumer_Metrics_Package SHALL record exactly one histogram observation per message per Terminal_Outcome
6. WHEN `FetchMessage` returns an error without delivering a message, THE Consumer_Metrics_Package SHALL NOT record a processing duration observation

### Requirement 8: Message Latency Histogram

**User Story:** As a benchmark analyst, I want to measure end-to-end message delivery latency, so that I can evaluate pipeline timeliness and detect queueing issues.

#### Acceptance Criteria

1. WHEN a Kafka fill message reaches a Terminal_Outcome and a valid Message_Creation_Time is available, THE Consumer_Metrics_Package SHALL record one observation to `kafka_consumer_message_latency_seconds` with the label `result` set to `success` if the Terminal_Outcome is success, or `failure` if the Terminal_Outcome is failure
2. THE Consumer_Metrics_Package SHALL compute the latency as `completion_wall_clock_time - message_creation_wall_clock_time` in seconds, where `completion_wall_clock_time` is the wall-clock time captured at the point of Terminal_Outcome (immediately after the final processing step completes or the record is permanently abandoned)
3. THE Consumer_Metrics_Package SHALL resolve Message_Creation_Time using the following priority: (a) `created_at` Kafka header, (b) `createdAt` or `created_at` payload JSON field, (c) Kafka record timestamp (`message.Time`). A source SHALL be skipped if it is absent, empty, non-parseable, or parses to a non-positive epoch value.
4. WHEN parsing the `created_at` header, THE Consumer_Metrics_Package SHALL support Unix epoch milliseconds (integer string), Unix epoch seconds (decimal string), and RFC 3339 format. IF the header value does not match any of these three formats, THEN THE Consumer_Metrics_Package SHALL skip the header source and proceed to the next priority source.
5. WHEN parsing payload timestamp fields, THE Consumer_Metrics_Package SHALL support numeric values as Unix epoch milliseconds and string values as RFC 3339. IF a payload field value is present but does not match these formats, THEN THE Consumer_Metrics_Package SHALL skip the payload source and proceed to the next priority source.
6. IF no valid Message_Creation_Time can be resolved from any source, THEN THE Consumer_Metrics_Package SHALL skip the latency observation and log a structured warning including the message partition, offset, and the reason no valid timestamp was found
7. IF computed latency is negative and the absolute value is less than 1 second, THEN THE Consumer_Metrics_Package SHALL clamp the latency to 0 and record the observation
8. IF computed latency is negative and the absolute value is 1 second or greater, THEN THE Consumer_Metrics_Package SHALL skip the observation and log a structured warning including the message partition, offset, and the computed latency value

### Requirement 9: Records Polled Counter

**User Story:** As a benchmark analyst, I want to count records returned by Kafka poll operations, so that I can measure fetch throughput independently of processing throughput.

#### Acceptance Criteria

1. WHEN `FetchMessage` returns a message successfully (nil error), THE Consumer_Metrics_Package SHALL increment `kafka_consumer_records_polled_total` by exactly 1
2. WHEN `FetchMessage` returns an error that is not a context cancellation (`context.Canceled` or `context.DeadlineExceeded`), THE Consumer_Metrics_Package SHALL NOT increment `kafka_consumer_records_polled_total`
3. WHEN `FetchMessage` returns a context cancellation error (`context.Canceled` or `context.DeadlineExceeded`), THE Consumer_Metrics_Package SHALL NOT increment `kafka_consumer_records_polled_total`
4. THE Consumer_Metrics_Package SHALL increment `kafka_consumer_records_polled_total` immediately after `FetchMessage` returns and before the message is dispatched for processing

### Requirement 10: Poll Seconds Counter

**User Story:** As a benchmark analyst, I want to measure total time spent in Kafka poll operations, so that I can assess broker responsiveness and consumer coordination overhead.

#### Acceptance Criteria

1. WHEN `FetchMessage` completes successfully, THE Consumer_Metrics_Package SHALL add the elapsed Poll_Duration in seconds to `kafka_consumer_poll_seconds_total`
2. WHEN `FetchMessage` returns an error that is not `context.Canceled`, THE Consumer_Metrics_Package SHALL add the elapsed Poll_Duration in seconds to `kafka_consumer_poll_seconds_total`
3. THE Consumer_Metrics_Package SHALL measure Poll_Duration using a monotonic clock (Go's `time.Since()`) from immediately before the `FetchMessage` call to immediately after it returns
4. WHEN `FetchMessage` returns an error that wraps `context.Canceled`, THE Consumer_Metrics_Package SHALL NOT add any time to `kafka_consumer_poll_seconds_total`
5. IF the measured Poll_Duration is zero or negative, THEN THE Consumer_Metrics_Package SHALL add 0 to `kafka_consumer_poll_seconds_total`

### Requirement 11: OpenTelemetry and Prometheus Integration

**User Story:** As a platform engineer, I want the metrics exposed via both OTLP and Prometheus endpoints, so that KASBench and existing monitoring infrastructure can scrape them.

#### Acceptance Criteria

1. THE Confirmation_Service SHALL configure the OTel_Meter_Provider with a Prometheus_Exporter reader in addition to the existing OTLP periodic reader
2. THE Prometheus_Exporter SHALL register with `prometheus.DefaultRegisterer` so that both OTel-sourced instruments and existing direct-registered Prometheus metrics are visible through the same `promhttp.Handler()` at `/metrics`
3. WHEN Prometheus scrapes the `/metrics` endpoint, THE Confirmation_Service SHALL expose counters with `_total` suffix and histograms with `_bucket`, `_count`, and `_sum` series
4. THE Confirmation_Service SHALL expose metrics using snake_case naming for both metric names and label keys, applying the OTel SDK's default Prometheus naming conventions (dots and hyphens converted to underscores)
5. THE Confirmation_Service SHALL NOT alter, remove, or cause name collisions with existing direct-registered Prometheus metrics when adding the OTel Prometheus exporter; all previously available metric series must remain present with identical names and label sets
6. THE Confirmation_Service SHALL add `go.opentelemetry.io/otel/exporters/prometheus` as a dependency
7. IF the OTLP collector endpoint is unreachable, THEN THE Confirmation_Service SHALL continue exposing OTel-sourced metrics through the Prometheus `/metrics` endpoint without error

### Requirement 12: Cross-Service Comparability

**User Story:** As a benchmark analyst, I want the confirmation service metrics to be directly comparable with the FIX Engine metrics, so that KASBench can perform apples-to-apples comparisons.

#### Acceptance Criteria

1. THE Consumer_Metrics_Package SHALL use identical metric names as the FIX Engine implementation: `kafka_consumer_messages_processed_total`, `kafka_consumer_messages_failed_total`, `kafka_consumer_processing_seconds_total`, `kafka_consumer_idle_seconds_total`, `kafka_consumer_records_polled_total`, `kafka_consumer_poll_seconds_total`, `kafka_consumer_processing_duration_seconds`, `kafka_consumer_message_latency_seconds`
2. THE Consumer_Metrics_Package SHALL use identical histogram bucket boundaries as the FIX Engine implementation: processing duration buckets of 0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5, 10, 30, 60 and message latency buckets of 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5, 10, 30, 60, 120, 300, 600
3. THE Consumer_Metrics_Package SHALL use the same Terminal_Outcome counting semantics as the FIX Engine: increment counters and record histogram observations only upon terminal outcomes (success or failure after retries exhausted), not upon individual retry attempts
4. THE Consumer_Metrics_Package SHALL use seconds as the unit for all duration metrics, matching the FIX Engine
5. THE Consumer_Metrics_Package SHALL use the same Message_Creation_Time resolution priority as the FIX Engine: (a) `created_at` Kafka header, (b) `createdAt` or `created_at` payload JSON field, (c) Kafka record timestamp; and SHALL support Unix epoch milliseconds, Unix epoch seconds, and RFC 3339 parsing formats
6. THE Consumer_Metrics_Package SHALL use the same latency clamping rules as the FIX Engine: clamp negative latency to 0 when the absolute value is less than 1 second, and skip the observation when the absolute value is 1 second or greater
7. THE Consumer_Metrics_Package SHALL use the same Common_Labels key set as the FIX Engine (`service`, `consumer_group`, `topic`, `partition`, `result`) so that metrics are filterable and groupable on identical dimensions across both services

### Requirement 13: Concurrency Model Adaptation

**User Story:** As a developer, I want the metrics correctly adapted for the confirmation service's worker pool architecture, so that measurements are accurate despite the multi-goroutine processing model.

#### Acceptance Criteria

1. WHILE the Fetch_Loop calls `FetchMessage`, THE Consumer_Metrics_Package SHALL measure Poll_Duration in the Fetch_Loop goroutine as the single measurement point for poll/idle metrics
2. WHEN a message is dispatched to a Worker_Pool goroutine, THE Consumer_Metrics_Package SHALL measure Processing_Duration as the elapsed time from the moment the worker receives the message from the channel to the moment the message handling function returns a final result (success or error after all retries are exhausted)
3. THE Consumer_Metrics_Package SHALL be safe for concurrent invocation from multiple Worker_Pool goroutines such that running the test suite with the Go race detector (`-race` flag) produces zero data race warnings
4. WHEN computing idle time in the worker pool model, THE Consumer_Metrics_Package SHALL report idle time as equal to the cumulative Poll_Duration (the time the Fetch_Loop goroutine spends blocked inside `FetchMessage`)
5. IF `FetchMessage` returns a timeout or error (not a valid message), THEN THE Consumer_Metrics_Package SHALL still record that Poll_Duration and count it toward idle time (per Requirement 6 criteria 1)

### Requirement 14: Nonfunctional — Performance and Safety

**User Story:** As a platform engineer, I want the metrics instrumentation to have negligible overhead, so that message processing throughput is not degraded during benchmarks.

#### Acceptance Criteria

1. THE Consumer_Metrics_Package SHALL NOT make network calls during metric recording; all metric recording operations SHALL write only to in-memory data structures, with export occurring asynchronously via a background goroutine
2. THE Consumer_Metrics_Package SHALL NOT increase the latency of any individual metric recording call on the Fetch_Loop or Worker_Pool goroutines by more than 1 millisecond under normal operation
3. THE Consumer_Metrics_Package SHALL NOT introduce memory growth that exceeds 50 MB above baseline when processing a sustained load of 1,000 messages per second for 10 minutes
4. IF an OTel SDK operation returns an error during metric recording, THEN THE Consumer_Metrics_Package SHALL discard the metric update, log the error at DEBUG level, and continue processing the current message without interruption
5. IF a metric recording operation panics, THEN THE Consumer_Metrics_Package SHALL recover the panic, log the panic details at ERROR level, and allow the in-progress message to continue processing through the remaining pipeline stages
6. WHILE the Consumer_Metrics_Package is recording metrics, THE Consumer_Metrics_Package SHALL NOT acquire locks held by the Fetch_Loop or Worker_Pool goroutines

### Requirement 15: Test Requirements

**User Story:** As a developer, I want comprehensive automated tests verifying metric correctness, so that regressions are caught before deployment.

#### Acceptance Criteria

1. THE Consumer_Metrics_Package SHALL include property-based tests (using `gopter`) validating that Message_Creation_Time resolution follows the priority order defined in Requirement 8 criterion 3: `created_at` header takes precedence over payload fields, which take precedence over Kafka record timestamp, for any combination of present/absent sources
2. THE Consumer_Metrics_Package SHALL include property-based tests validating that computed latency values that are negative with absolute value less than 1 second are clamped to 0, and latency values that are negative with absolute value of 1 second or greater produce no observation
3. THE Consumer_Metrics_Package SHALL include property-based tests validating that for any single message reaching a Terminal_Outcome, exactly one of `kafka_consumer_messages_processed_total` or `kafka_consumer_messages_failed_total` is incremented by 1, never both and never neither
4. THE Consumer_Metrics_Package SHALL include property-based tests validating that for any completed fetch-then-process cycle, the sum of time added to `kafka_consumer_idle_seconds_total` and `kafka_consumer_processing_seconds_total` accounts for all elapsed monotonic time with no unaccounted gaps
5. THE Consumer_Metrics_Package SHALL include property-based tests validating that `kafka_consumer_records_polled_total` increments by 1 only when `FetchMessage` returns a message successfully, and does not increment when `FetchMessage` returns an error or is cancelled by context cancellation
6. THE Consumer_Metrics_Package SHALL include property-based tests validating that when context cancellation occurs during a poll or processing operation, no observations are recorded to `kafka_consumer_idle_seconds_total`, `kafka_consumer_poll_seconds_total`, `kafka_consumer_records_polled_total`, `kafka_consumer_processing_duration_seconds`, or `kafka_consumer_message_latency_seconds` for that cancelled operation
7. THE Consumer_Metrics_Package SHALL include unit tests verifying that all 8 instrument names match the names defined in Requirement 1 criterion 1, and that the histogram bucket boundaries for `kafka_consumer_processing_duration_seconds` and `kafka_consumer_message_latency_seconds` match the values defined in Requirement 1 criteria 2 and 3 respectively
8. THE Consumer_Metrics_Package SHALL include an integration test that triggers at least one successful and one failed message processing cycle, then verifies that all 8 metrics are present on the `/metrics` endpoint with correct `_total` suffixes for counters and `_bucket`, `_count`, `_sum` series for histograms
9. THE Consumer_Metrics_Package SHALL configure all property-based tests to execute a minimum of 100 generated test cases per property
