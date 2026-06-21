Requirements: Standard Kafka Consumer Metrics for GlobeCo Services

1. Purpose

Implement a consistent set of custom Kafka consumer metrics in both GlobeCo services that consume Kafka messages. The metrics must be emitted with identical names, units, label keys, semantic definitions, and edge-case behavior so KASBench can compare the two services directly during benchmark analysis.

All metrics must be exported through the service’s OpenTelemetry-compatible metrics pipeline and must be scrapeable by Prometheus.

2. Scope

The implementation applies to every service instance that consumes Kafka records.

Each consuming service must emit the following metrics:

Metric name	Type	Unit
kafka_consumer_messages_processed_total	Counter	messages
kafka_consumer_messages_failed_total	Counter	messages
kafka_consumer_processing_seconds_total	Counter	seconds
kafka_consumer_idle_seconds_total	Counter	seconds
kafka_consumer_processing_duration_seconds	Histogram	seconds
kafka_consumer_message_latency_seconds	Histogram	seconds
kafka_consumer_records_polled_total	Counter	records
kafka_consumer_poll_seconds_total	Counter	seconds

3. Required Common Labels

Every metric must include the same label keys in both services.

Label	Required	Description
service	Yes	Logical service name, for example globeco-order-service.
consumer_group	Yes	Kafka consumer group ID.
topic	Yes	Kafka topic name.
partition	Yes	Kafka partition number as a string. Use "unknown" only when unavailable.
result	Conditional	Required only for histograms where useful. Values must be success or failure.

Do not add high-cardinality labels such as message key, message ID, exception message, stack trace, user ID, portfolio ID, order ID, hostname, pod UID, or Kafka offset.

The Kubernetes/OpenTelemetry pipeline may add resource attributes such as pod name, namespace, container name, and node name separately. Those should not be implemented as application metric labels unless already standardized across all GlobeCo services.

4. Timing Rules

All time values must be measured in seconds using a monotonic clock.

The implementation must not use wall-clock time for elapsed-duration calculations because wall-clock time can move backward or forward due to clock synchronization.

Use wall-clock timestamps only when computing end-to-end message latency from a message creation timestamp.

5. Message Lifecycle Definitions

A Kafka record has the following lifecycle points:

Point	Definition
Poll start	Immediately before invoking the Kafka poll operation.
Poll end	Immediately after the Kafka poll operation returns.
Processing start	Immediately before the service begins application-level handling of one record.
Processing end	Immediately after application-level handling finishes, whether successful or failed.
Message creation time	Timestamp representing when the event/message was created. Prefer an explicit message header or payload field. If absent, use the Kafka record timestamp.
Completion time	Wall-clock timestamp at processing end.

6. Metric Requirements

6.1 kafka_consumer_messages_processed_total

Type: Counter
Unit: messages

Increment by 1 when a Kafka record is successfully processed.

A message is successfully processed only when all application-level handling required by the service has completed and the service considers the message complete.

Do not increment this metric when processing fails, when a retry will occur, or when the message is routed to a dead-letter topic.

Acceptance tests:

* Given one successfully processed record, the counter increases by 1.
* Given a batch of 10 successfully processed records, the counter increases by 10.
* Given a record that fails processing, the counter does not increase.
* Given a record that fails once and later succeeds on retry, the counter increases exactly once, when the successful attempt completes.

6.2 kafka_consumer_messages_failed_total

Type: Counter
Unit: messages

Increment by 1 when processing of a Kafka record ultimately fails.

A failure means one of the following:

* Retries are exhausted.
* The message is routed to a dead-letter topic.
* The service permanently abandons the message.
* The service catches an unrecoverable processing exception and marks the message failed.

Do not increment this metric for transient failures that are retried and later succeed.

Acceptance tests:

* Given a record that throws an exception but succeeds on retry, the counter does not increase.
* Given a record that exhausts retries, the counter increases by 1.
* Given a record routed to a dead-letter topic, the counter increases by 1.
* Given 5 permanently failed records, the counter increases by 5.

6.3 kafka_consumer_processing_seconds_total

Type: Counter
Unit: seconds

Add the elapsed processing duration for every processing attempt that reaches a terminal outcome.

This metric measures active message-processing time only. It excludes time spent polling Kafka, waiting for records, sleeping, backoff delay, idle loops, and time when the consumer is available but not processing.

For concurrent processing, add the duration of each message independently. Therefore, this counter may increase faster than real elapsed time when multiple records are processed in parallel.

Acceptance tests:

* Given one message processed in 0.250 seconds, the counter increases by approximately 0.250.
* Given two messages processed concurrently for 1.000 second each, the counter increases by approximately 2.000.
* Given a Kafka poll that blocks for 5 seconds and returns no records, this counter does not increase.
* Given a failed message that reaches a terminal failure after 0.400 seconds of active processing, the counter increases by approximately 0.400.

6.4 kafka_consumer_idle_seconds_total

Type: Counter
Unit: seconds

Add time during which the consumer is running and available but not actively processing records.

Idle time includes:

* Time blocked inside Kafka poll.
* Time between poll return and the next processing start when no record is being processed.
* Time waiting for work.
* Intentional sleep/backoff while the consumer remains available.

Idle time excludes:

* Active message processing.
* Service startup before the consumer is ready.
* Time after shutdown begins.
* Time when the consumer is unhealthy or intentionally paused.

For single-threaded consumers, elapsed time should be classified as either idle or processing, except that poll_seconds_total is also separately measured.

For concurrent consumers, idle time should be measured per consumer worker or consumer loop. Document which concurrency model is used and keep it identical across both services where practical.

Acceptance tests:

* Given a healthy consumer polling for 2 seconds with no records, idle time increases by approximately 2 seconds.
* Given a consumer actively processing a message for 1 second, idle time does not increase for that same worker during the processing interval.
* Given a consumer that is shut down, idle time stops increasing.
* Given a consumer that is paused and not available for work, idle time does not increase.

6.5 kafka_consumer_processing_duration_seconds

Type: Histogram
Unit: seconds

Record one observation for each message-processing attempt that reaches a terminal outcome.

The observed value is:

processing_end_monotonic_time - processing_start_monotonic_time

The histogram must include both successful and terminally failed messages. Use the result label with values success and failure.

Recommended bucket boundaries:

0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5, 10, 30, 60

Acceptance tests:

* Given a successful message processed in 0.100 seconds, one histogram observation is recorded with result="success".
* Given a terminally failed message processed in 0.200 seconds, one histogram observation is recorded with result="failure".
* Given a message that fails once and succeeds on retry, only the terminal successful processing attempt is recorded unless the service explicitly treats attempts as separately processed units. Both services must follow the same rule.
* Histogram bucket counts, sum, and count are exported correctly to Prometheus.

6.6 kafka_consumer_message_latency_seconds

Type: Histogram
Unit: seconds

Record one observation for each message that reaches a terminal outcome.

The observed value is:

processing_completion_wall_clock_time - message_creation_wall_clock_time

Message creation time must be resolved in this order:

1. A standardized Kafka header, recommended name: created_at.
2. A standardized payload field, recommended name: createdAt or created_at.
3. Kafka record timestamp.

The timestamp format for headers or payload fields must be Unix epoch milliseconds or RFC 3339. Both consuming services must support the same format.

If no valid creation timestamp is available, do not record this histogram observation and increment/log an internal warning counter or structured warning log. Do not emit negative latency. If calculated latency is negative due to clock skew, clamp to 0 only if the skew is less than 1 second; otherwise skip the observation and log a warning.

Use the result label with values success and failure.

Recommended bucket boundaries:

```text
0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5, 10, 30, 60, 120, 300, 600
```

Acceptance tests:

* Given a message created 3 seconds before completion, the histogram records approximately 3 seconds.
* Given a message with a valid created_at header, the header timestamp is used instead of the Kafka record timestamp.
* Given a message with no valid timestamp, no latency observation is recorded and a warning is logged.
* Given a terminally failed message, latency is still recorded with result="failure" if creation time is available.

6.7 kafka_consumer_records_polled_total

Type: Counter
Unit: records

Increment by the number of records returned from each Kafka poll operation.

This metric counts records returned by Kafka, not records successfully processed.

Acceptance tests:

* Given a poll returning 0 records, the counter does not increase.
* Given a poll returning 5 records, the counter increases by 5.
* Given 5 polled records where 2 later fail processing, the counter still increases by 5.
* Given records from multiple partitions, the counter is incremented per topic and partition according to the returned records.

6.8 kafka_consumer_poll_seconds_total

Type: Counter
Unit: seconds

Add the elapsed time spent inside Kafka poll calls.

The measured interval starts immediately before invoking poll and ends immediately after poll returns, regardless of whether records are returned.

This metric is expected to overlap with kafka_consumer_idle_seconds_total because time blocked in poll is idle time from the application-processing perspective.

Acceptance tests:

* Given a poll call that blocks for 1 second and returns no records, this counter increases by approximately 1.
* Given a poll call that returns immediately with records after 0.010 seconds, this counter increases by approximately 0.010.
* Given processing time after poll returns, this counter does not include processing time.

7. Retry and Dead-Letter Behavior

Both services must use the same terminal-outcome semantics.

A message is not considered processed or failed until the service reaches a final decision for that message.

Required behavior:

* Retryable failure followed by success: count as one processed message.
* Retryable failure followed by exhausted retries: count as one failed message.
* Dead-letter routing: count as one failed message.
* Deserialization failure before application processing: count as failed if the service can associate the failure with a consumed record.
* Poll failure with no records returned: do not count as message failure; log separately.

8. Offset Commit Semantics

Metric updates must reflect application-level processing outcomes, not merely Kafka offset commits.

Recommended order for successful processing:

1. Record processing start time.
2. Execute application logic.
3. Complete application logic successfully.
4. Increment messages_processed_total.
5. Record processing duration and message latency.
6. Commit offset according to the service’s existing commit strategy.

For failed processing:

1. Record processing start time.
2. Execute application logic.
3. Exhaust retries or route to dead letter.
4. Increment messages_failed_total.
5. Record processing duration and message latency if available.
6. Commit, skip, or leave offset according to the service’s existing error-handling strategy.

9. OpenTelemetry Requirements

Each service must expose these metrics using its language’s OpenTelemetry metrics SDK or a compatible framework.

Requirements:

* Metric names must match exactly.
* Units must be seconds for all duration metrics.
* Counters must be monotonic.
* Histograms must use consistent bucket boundaries across both services.
* Metric instruments must be initialized once per process.
* Metric recording must be thread-safe.
* Metrics must be emitted continuously while the service is running.
* Metrics must be visible at the same endpoint already used by the service’s metrics pipeline.

10. Prometheus Naming Expectations

When exported to Prometheus:

* Counters should appear with _total suffix.
* Histograms should expose _bucket, _count, and _sum series.
* Label names must use snake_case.
* Metric names must use snake_case.

Example Prometheus series:

kafka_consumer_messages_processed_total{
  service="globeco-example-service",
  consumer_group="example-consumer-group",
  topic="orders",
  partition="0"
}
kafka_consumer_processing_duration_seconds_bucket{
  service="globeco-example-service",
  consumer_group="example-consumer-group",
  topic="orders",
  partition="0",
  result="success",
  le="0.25"
}

11. Cross-Service Comparability Requirements

Both Kafka-consuming services must implement the metrics according to the same rules.

The implementation is not acceptable if:

* One service counts retry attempts and the other counts terminal outcomes.
* One service uses milliseconds and the other uses seconds.
* One service includes polling time in processing time.
* One service records latency from poll time while the other records latency from message creation time.
* One service emits additional required labels that the other does not.
* One service uses different histogram buckets.
* One service omits failed messages from histograms while the other includes them.

12. Test Requirements

Each service must include automated tests covering:

1. Successful message processing.
2. Failed message processing.
3. Retry followed by success.
4. Retry exhaustion.
5. Dead-letter routing, if supported.
6. Poll returning zero records.
7. Poll returning multiple records.
8. Multiple topics or partitions, if supported.
9. Processing duration measurement.
10. End-to-end message latency measurement.
11. Missing or invalid message creation timestamp.
12. Concurrent processing, if supported.
13. Graceful shutdown.
14. Metric label consistency.
15. Prometheus/OpenTelemetry export format.

13. Suggested Shared Test Fixtures

Create a common test specification that both services must satisfy.

Minimum fixture scenarios:

Scenario A: Successful single message

Input:

* Topic: orders
* Partition: 0
* Consumer group: test-consumer
* Message creation timestamp: now minus 2 seconds
* Processing result: success

Expected:

* messages_processed_total increases by 1.
* messages_failed_total does not increase.
* processing_seconds_total increases by processing duration.
* processing_duration_seconds records one success observation.
* message_latency_seconds records approximately 2 seconds.
* records_polled_total increases by 1.
* poll_seconds_total increases by measured poll duration.

Scenario B: Terminal failure

Expected:

* messages_processed_total does not increase.
* messages_failed_total increases by 1.
* processing_seconds_total increases by failed processing duration.
* processing_duration_seconds records one failure observation.
* message_latency_seconds records one failure observation if timestamp is valid.

Scenario C: Empty poll

Expected:

* records_polled_total does not increase.
* poll_seconds_total increases.
* idle_seconds_total increases.
* Processing metrics do not increase.

Scenario D: Retry then success

Expected:

* messages_processed_total increases by 1.
* messages_failed_total does not increase.
* Histograms record the terminal successful outcome according to the shared retry rule.

Scenario E: Retry exhaustion

Expected:

* messages_processed_total does not increase.
* messages_failed_total increases by 1.
* Histograms record the terminal failed outcome.

14. Nonfunctional Requirements

The metrics implementation must:

* Add negligible overhead to Kafka processing.
* Avoid blocking the consumer loop.
* Avoid network calls during metric recording.
* Avoid unbounded memory growth.
* Avoid high-cardinality labels.
* Be safe under concurrent message processing.
* Continue emitting valid metrics during long benchmark trials.
* Not change existing business behavior, Kafka commit behavior, or retry behavior except where instrumentation requires clearly isolated hooks.

15. Definition of Done

Implementation is complete when:

* Both Kafka-consuming services emit all eight required metrics.
* Metric names, labels, units, histogram buckets, and retry semantics are identical across both services.
* Unit tests pass for all required scenarios.
* Integration tests confirm metrics are visible through the service metrics endpoint.
* Prometheus can scrape the metrics.
* A short README section documents the metric definitions and any service-specific implementation notes.
* KASBench can query the metrics consistently across both services. 



