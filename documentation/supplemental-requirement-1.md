# Supplemental Requirement 1: Allocation Service Integration

## Overview

The Confirmation Service currently processes fill messages received from Kafka and updates the Execution Service. This enhancement requires the Confirmation Service to **also call the Allocation Service for completed trades**. A trade is considered completed when the field `isOpen` is `false`.

- **Existing processing** for receiving the Kafka message and calling the Execution Service remains unchanged.
- **New step:** After calling the Execution Service (regardless of success or failure), the service must attempt to call the Allocation Service. Failures in this step should be logged but must not block further processing.

## Allocation Service Details

- **Host:** `globeco-execution-service`
- **Port:** `8089`
- **API Endpoint:** `POST /api/v1/executions`
- **OpenAPI Spec:** [allocation-service-openapi.yaml](allocation-service-openapi.yaml)

## Data Mapping

Each completed trade (where `isOpen` is `false`) should be mapped from the Confirmation Service's fill message to the Allocation Service Execution DTO as follows:

| Allocation Service Execution DTO | Confirmation Service Fill Message |
| --- | --- |
| executionServiceId | executionServiceId |
| isOpen | false |
| executionStatus | executionStatus |
| tradeType | tradeType |
| destination | destination |
| securityId | securityId |
| ticker | ticker |
| quantity | quantity |
| limitPrice | null |
| receivedTimestamp | receivedTimestamp |
| sentTimestamp | sentTimestamp |
| lastFillTimestamp | lastFilledTimestamp |
| quantityFilled | quantityFilled |
| totalAmount | totalAmount |
| averagePrice | averagePrice |

- **Note:** `limitPrice` should always be set to `null` in the Allocation Service request.

## Error Handling & Requirements

- The error handling, performance, message validation, and logging requirements for this enhancement are **identical to those in** [requirements.md](requirements.md). This includes:
  - Retry logic, circuit breaker, and dead letter queue for failed calls
  - Structured logging with correlation IDs
  - Input validation and business rule enforcement
  - Observability (metrics, tracing)

## Sample Fill Message

```json
{"id":11,"executionServiceId":27,"isOpen":false,"executionStatus":"FULL","tradeType":"BUY","destination":"ML","securityId":"68336002fe95851f0a2aeda9","ticker":"IBM","quantity":1000,"receivedTimestamp":1748354367.509362,"sentTimestamp":1748354367.512467,"lastFilledTimestamp":1748354504.1602714,"quantityFilled":1000,"averagePrice":190.4096,"numberOfFills":3,"totalAmount":190409.6,"version":1}
```

## Clarifications

1. **Allocation Service Response Handling:** Logging the response is sufficient; no further action is required.
2. **Partial Failures:** If both the Execution Service and Allocation Service calls fail, each failure should be sent to its respective dead letter queue (or clearly identified in a shared queue). There should be two records if both APIs fail.
3. **Idempotency:** The Confirmation Service does not need to implement idempotency for Allocation Service calls; the Allocation Service will handle this.
4. **limitPrice Field:** This field should always be set to null in the Allocation Service request.
5. **API Authentication:** No authentication or authorization headers are required for Allocation Service calls.
6. **Additional Fields:** No additional fields are required by the Allocation Service beyond those mapped from the fill message.
7. **Performance Requirements:** The throughput and latency requirements for Allocation Service calls are the same as for Execution Service calls.

## Execution Plan

Phase 0
- [x] Review and clarify requirements with stakeholders

Phase 1
- [x] Define/update domain models and DTOs for Allocation Service integration
- [x] Implement mapping logic from fill message to Allocation Service DTO
- [x] Implement HTTP client for Allocation Service (with retry, circuit breaker, and timeout)

Phase 2
- [x] Integrate Allocation Service call into message processing flow (after Execution Service call)
- [x] Ensure Allocation Service call is attempted even if Execution Service call fails
- [x] Log all Allocation Service call attempts and failures with correlation IDs

Phase 3
- [x] Update error handling to match requirements.md (retry, circuit breaker, DLQ)
- [x] Ensure dead letter queue handling is separate for Execution and Allocation Service failures, with clear identification

Phase 4
- [x] Add/extend unit tests for new logic and error scenarios
- [x] Update integration tests to cover Allocation Service flow

Phase 5
- [ ] Update documentation and runbooks
- [ ] Validate observability (metrics, tracing, logging)

Phase 6
- [ ] Review and refactor for code quality and consistency

---

*This document is based on and extends the requirements in [requirements.md](requirements.md). All clarifications above are confirmed by the project owner.*