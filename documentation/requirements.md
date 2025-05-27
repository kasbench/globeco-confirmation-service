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
| Go | 23.4 | |
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

### Other requirements

- OpenTelemetry instrumentation



## Execution Plan    

Teh plan should be similar to the following plan from another GlobeCo project:


1. **Project Initialization and Repository Setup**
   - Initialize a new Go module and set up the project directory structure according to clean architecture principles.
 
   - Set up Go module dependencies (chi, sqlx, zap, viper, testify, etc.).

2. **Configuration Management**
   - Implement configuration loading using Viper (supporting environment variables and config files).
   - Define configuration structs for Kafka, PostgreSQL, external services, and app settings.

3. **Logging and Observability Foundation**
   - Integrate zap for structured logging.
   - Set up Prometheus metrics endpoint and basic application metrics.
   - Integrate OpenTelemetry for distributed tracing (initial setup).


4. **Domain Models and DTOs**
   - Define Go structs for  DTOs 

5. **Kafka Integration**
   - Set up Kafka consumer for the `fills` topic.
   - Configure consumer group and error handling.

6. **External Service Integration**
   - Implement client for the Execution Service with 

7. **Business Logic: Service Layer**


8. **REST API Implementation**

   - Add health and readiness endpoints for Kubernetes.
   
9. **Middleware and Utilities**
    - Implement HTTP middleware for logging, request tracing, and CORS.
    - Add utility functions as needed (e.g., random fill logic, time calculations).

10. **Testing**
    - Write unit tests for service and API layers using testify.
    - Add integration tests for Kafka and database interactions.
    - Ensure high test coverage and test for edge cases.

11. **Graceful Shutdown and Robustness**
    - Implement context-based cancellation and graceful shutdown for all components.
    - Ensure proper error handling and retries for transient failures.

12. **Containerization and Deployment**
    - Write a multi-stage Dockerfile for minimal image size.
    - Add Docker Compose for local development and integration testing.
    - Prepare Kubernetes manifests for deployment (Deployment, Service, ConfigMap, Secret, etc.).
    - Configure readiness and liveness probes.

13. **CI/CD Integration**
    - Set up CI pipeline for linting, testing, and building Docker images.
    - Add CD steps for deployment to Kubernetes (if applicable).

14. **Documentation**
    - Document API endpoints, configuration, and operational procedures.
    - Update architecture and requirements documentation as needed.

---