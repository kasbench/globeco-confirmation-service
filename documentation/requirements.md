# GlobeCo Confirmation Service

## Background

This document provides requirements for the GlobeCo Confirmation Service  This service receive "fill" messages from Kafka and updates the GlobeCo Execution Service.

This microservice will be deployed on Kubernetes 1.33.

This microservice is part of the GlobeCo suite of applications for benchmarking Kubernetes autoscaling.

- Name of service: Confirmation Service
- Host: globeco-confirmation-service
- Port: 8086 
- Author: Noah Krieger 
- Email: noah@kasbench.org
- Organization: KASBench
- Organization Domain: kasbench.org

## Technology

| Technology | Version | Notes |
|---------------------------|----------------|---------------------------------------|
| Go | 1.23.4 | |
| Kafka | 4.0.0 | |
---

These are the Go modules used in other GlobeCo microservices.  To the extent possible, we want to maintain consistency.

	github.com/go-chi/chi/v5 v5.2.1
	github.com/golang-migrate/migrate/v4 v4.18.3
	github.com/jmoiron/sqlx v1.4.0
	github.com/lib/pq v1.10.9
	github.com/prometheus/client_golang v1.22.0
	github.com/segmentio/kafka-go v0.4.48
	github.com/spf13/viper v1.20.1
	github.com/stretchr/testify v1.10.0
	github.com/testcontainers/testcontainers-go/modules/kafka v0.37.0
	github.com/testcontainers/testcontainers-go/modules/postgres v0.37.0
	go.opentelemetry.io/otel v1.36.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.36.0
	go.opentelemetry.io/otel/sdk v1.36.0
	go.opentelemetry.io/otel/trace v1.36.0
	go.uber.org/zap v1.27.0




## Other services

| Name | Host | Port | Description | OpenAPI Schema |
| --- | --- | --- | --- | --- |
| Kafka | globeco-execution-service-kafka | 9092 | Kafka cluster | |
| Execution Service | globeco-execution-service | 8084 | Trade execution service | [documentation/execution-service-openapi.json](execution-service-openapi.json) |

---

## Kafka
- Bootstrap Server: globeco-execution-service-kafka 
- Port: 9092 <br>
- Topic: fills (consumer)
- Consumer group: confirmation-service

## Sample Fill Message

```json
{"id":11,"executionServiceId":27,"isOpen":false,"executionStatus":"FULL","tradeType":"BUY","destination":"ML","securityId":"68336002fe95851f0a2aeda9","ticker":"IBM","quantity":1000,"receivedTimestamp":1748354367.509362,"sentTimestamp":1748354367.512467,"lastFilledTimestamp":1748354504.1602714,"quantityFilled":1000,"averagePrice":190.4096,"numberOfFills":3,"totalAmount":190409.6,"version":1}
```

## Logic

The Comfirmation microservice performs the following in a continuous loop.

1. Microservice listens for messages on the `fills` topic.  When a message is received, it proceeds to the next step. Message received will be in the following format:
```json
    {   "id":11, 
        "executionServiceId":27, 
        "isOpen":false, 
        "executionStatus":"FULL", 
        "tradeType":"BUY", 
        "destination":"ML", 
        "securityId":"68336002fe95851f0a2aeda9",
        "ticker":"IBM",
        "quantity":1000,
        "receivedTimestamp":1748354367.509362,
        "sentTimestamp":1748354367.512467,
        "lastFilledTimestamp":1748354504.1602714,
        "quantityFilled":1000,
        "averagePrice":190.4096,
        "numberOfFills":3,
        "totalAmount":190409.6,
        "version":1
    }
```
2. Call the GET api/v1/execution/{id} API on execution service to get the version number.  The {id} is executionServiceId from the Kafka message.  Reference the [OpenAPI Spec](execution-service-openapi.json) for details,
    - Sample Response
    ```json
    {
        "id": 0,
        "executionStatus": "string",
        "tradeType": "string",
        "destination": "string",
        "securityId": "string",
        "quantity": 0,
        "limitPrice": 0,
        "receivedTimestamp": "2025-05-27T23:11:31.910Z",
        "sentTimestamp": "2025-05-27T23:11:31.910Z",
        "tradeServiceExecutionId": 0,
        "quantityFilled": 0,
        "averagePrice": 0,
        "version": 0
    }
    ```
3. Microservice calls the Execution Service to update the execution record.
    - API: PUT api/v1/execution/{id}
    - [OpenAPI Spec](execution-service-openapi.json)
    - Sample Payload:
    ```json
    {
        "quantityFilled": 0,
        "averagePrice": 0,
        "version": 0
    }
    ```
   - Sample Response
   ```json
   {
        "id": 0,
        "executionStatus": "string",
        "tradeType": "string",
        "destination": "string",
        "securityId": "string",
        "quantity": 0,
        "limitPrice": 0,
        "receivedTimestamp": "2025-05-27T20:07:29.261Z",
        "sentTimestamp": "2025-05-27T20:07:29.261Z",
        "tradeServiceExecutionId": 0,
        "quantityFilled": 0,
        "averagePrice": 0,
        "version": 0
    }
   ```
   - Mapping
   
   | Kafka Message | PUT API |
   | --- | --- |
   | executionServiceId | id |
   | quantityFilled | quantityFilled |
   | averagePrice | averagePrice |
   | version | version from the previous step |
    ---

## Error Handling Requirements

- **Retry Strategy**: Failed Execution Service calls must be retried up to 3 times with exponential backoff (initial delay: 100ms, max delay: 5s)
- **Circuit Breaker**: Implement circuit breaker pattern for Execution Service calls (failure threshold: 5 consecutive failures, timeout: 30s)
- **Dead Letter Queue**: Messages that fail after all retries should be sent to a dead letter queue for manual investigation
- **Timeout Configuration**: 
  - Kafka consumer timeout: 30s
  - Execution Service API timeout: 10s
- **Error Logging**: All errors must be logged with correlation IDs for traceability
- **Graceful Degradation**: Service should continue processing other messages even if some fail

## Performance Requirements

- **Throughput**: Process minimum 1,000 messages per second under normal load
- **Response Time**: Execution Service API calls must complete within 100ms (95th percentile)
- **Concurrency**: Maximum 10 concurrent Execution Service API calls to prevent overwhelming the downstream service
- **Memory Usage**: Service should not exceed 512MB memory usage under normal load
- **CPU Usage**: Service should not exceed 500m CPU under normal load

## Message Validation Requirements

- **Required Fields**: Validate presence of all required fields in Kafka messages:
  - `executionServiceId` (must be positive integer)
  - `quantityFilled` (must be non-negative number)
  - `averagePrice` (must be positive number)
  - `version` (must be non-negative integer)
- **Data Types**: Ensure all numeric fields are valid numbers
- **Business Rules**: 
  - `quantityFilled` should not exceed original `quantity`
  - `averagePrice` should be reasonable (> 0 and < 10000)
- **Schema Validation**: Reject malformed JSON messages
- **Duplicate Detection**: Log duplicate message IDs but process them (idempotent operations)

## Health Check Requirements

- **Liveness Probe**: `/health/live` endpoint that returns 200 OK if service is running
- **Readiness Probe**: `/health/ready` endpoint that returns:
  - 200 OK if service can connect to Kafka and Execution Service
  - 503 Service Unavailable if dependencies are unreachable
- **Health Check Frequency**: Kubernetes probes should check every 10 seconds
- **Startup Grace Period**: Allow 30 seconds for service startup before health checks

## Logging Requirements

- **Structured Logging**: Use JSON format with consistent field names
- **Required Fields**: Every log entry must include:
  - `timestamp` (ISO 8601 format)
  - `level` (DEBUG, INFO, WARN, ERROR)
  - `service` ("globeco-confirmation-service")
  - `correlationId` (for request tracing)
  - `message` (human-readable description)
- **Request Logging**: Log every Kafka message received and API call made
- **Performance Logging**: Log processing time for each message
- **Error Context**: Include full error details and stack traces for debugging

### Other requirements

- OpenTelemetry instrumentation with trace propagation to Execution Service
- Prometheus metrics for message processing rates, error rates, and API response times



## Execution Plan    

The plan should be similar to the following plan from another GlobeCo project:

1. **Project Initialization and Repository Setup**
   - Initialize a new Go module and set up the project directory structure according to clean architecture principles
   - Create directory structure: `cmd/`, `internal/`, `pkg/`, `api/`, `config/`, `domain/`, `repository/`, `service/`, `middleware/`, `utils/`
   - Set up Go module dependencies (chi, sqlx, zap, viper, testify, kafka-go, etc.)
   - Initialize git repository and basic project files

2. **Domain Models and DTOs**
   - Define Go structs for Kafka fill messages
   - Define DTOs for Execution Service API requests/responses
   - Create domain models for internal business logic
   - Add JSON tags and validation annotations

3. **Configuration Management**
   - Implement configuration loading using Viper (supporting environment variables and config files)
   - Define configuration structs for Kafka, Execution Service, timeouts, retry policies, and app settings
   - Add configuration validation and default values
   - Support for different environments (dev, staging, prod)

4. **Logging and Observability Foundation**
   - Integrate zap for structured logging with required fields (timestamp, level, service, correlationId, message)
   - Set up Prometheus metrics endpoint and application metrics (message processing rates, error rates, API response times)
   - Integrate OpenTelemetry for distributed tracing with trace propagation
   - Implement correlation ID generation and propagation

5. **Error Handling and Resilience Strategy**
   - Implement retry mechanism with exponential backoff for Execution Service calls
   - Add circuit breaker pattern for external service calls
   - Create dead letter queue handling for failed messages
   - Define timeout configurations and error classification
   - Implement graceful error recovery and logging

6. **External Integrations**
   - **Kafka Consumer**: Set up Kafka consumer for the `fills` topic with consumer group configuration
   - **Execution Service Client**: Implement HTTP client for Execution Service with timeout, retry, and circuit breaker
   - Add connection health checks and monitoring
   - Implement message deserialization and API request/response handling

7. **Business Logic and Validation**
   - Implement core message processing logic (consume → validate → get version → update execution)
   - Add comprehensive input validation for Kafka messages
   - Implement business rule validation (quantity limits, price ranges, etc.)
   - Add duplicate detection and idempotent processing
   - Ensure proper error handling and logging throughout

8. **REST API Implementation**
   - Add health check endpoints (`/health/live`, `/health/ready`) with dependency checks
   - Implement metrics endpoint (`/metrics`) for Prometheus
   - Add any additional operational endpoints as needed
   - Ensure proper HTTP status codes and error responses

9. **Middleware and Utilities**
   - Implement HTTP middleware for logging, request tracing, CORS, and metrics collection
   - Add utility functions for correlation ID handling, time calculations, and data transformations
   - Implement request/response logging with performance metrics

10. **Unit Testing**
    - Write comprehensive unit tests for service and API layers using testify
    - Mock external dependencies (Kafka, Execution Service)
    - Test error scenarios, edge cases, and validation logic
    - Ensure high test coverage (>80%) and test for all business rules

11. **Integration Testing**
    - Add integration tests using testcontainers for Kafka
    - Test end-to-end message processing flow
    - Test error handling and retry scenarios
    - Validate health check endpoints and metrics collection

12. **Performance Testing**
    - Load test message processing to validate throughput requirements (1000 msg/sec)
    - Test concurrent processing limits and resource usage
    - Validate API response time requirements (100ms 95th percentile)
    - Test memory and CPU usage under load

13. **Graceful Shutdown and Robustness**
    - Implement context-based cancellation and graceful shutdown for all components
    - Ensure proper cleanup of Kafka consumers and HTTP connections
    - Add signal handling for container orchestration
    - Test startup and shutdown procedures

14. **Containerization and Deployment**
    - Write multi-stage Dockerfile for minimal image size and security
    - Add Docker Compose for local development and integration testing
    - Prepare Kubernetes manifests (Deployment, Service, ConfigMap, Secret, HPA)
    - Configure readiness and liveness probes with proper timeouts
    - Add resource limits and requests based on performance testing

15. **CI/CD Integration**
    - Set up CI pipeline for linting (golangci-lint), testing, and security scanning
    - Add automated testing with coverage reporting
    - Build and push Docker images with proper tagging
    - Add CD steps for deployment to Kubernetes environments

16. **Documentation and Operations**
    - Document API endpoints, configuration options, and operational procedures
    - Create runbooks for common operational tasks and troubleshooting
    - Update architecture and requirements documentation
    - Add monitoring and alerting recommendations

---