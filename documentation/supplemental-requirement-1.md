# Supplemental Requirement 1

The Confirmation Service currently calls the Execution Service with fills received from Kafka.  This enhancement is to also call the Allocation Service for completed trades.  A trade is considered completed when the field isOpen is false.

Existing processing for receiving the Kafka message and calling the Execution Service is unchanged.  This extra step happens after the call to the Execution Service and is independent.  It should be attempted even if the call to the Execution Service fails.  Failures should be logged.

The Allocation Service is at host globeco-execution-service on port 8089.  The API is documented in [allocation-service-openapi.yaml](allocation-service-openapi.yaml).

The Confirmation Service will call the POST /api/v1/executions API on the Allocation Service


Each Record is mapped as follows:

| Allocation Service Execution DTO | Confirmation Service Fill Message (see sample fill message below) |
| --- | --- |
|executionServiceId | executionServiceId
|isOpen | false
|executionStatus | executionStatus
|tradeType | tradeType
|destination | destination
|securityId | securityId
|ticker | ticker
|quantity | quantity
|limitPrice | null |
|receivedTimestamp | receivedTimestamp
|sentTimestamp | sentTimestamp
|lastFillTimestamp | lastFilledTimestamp
|quantityFilled | quantityFilled
|totalAmount | totalAmount
|averagePrice | averagePrice

The error handling requirements, performance requirements, message validaton requirements, logging requirements, and other requirements are the same as in [requirements.md](requirements.md)


## Sample Fill Message

```json
{"id":11,"executionServiceId":27,"isOpen":false,"executionStatus":"FULL","tradeType":"BUY","destination":"ML","securityId":"68336002fe95851f0a2aeda9","ticker":"IBM","quantity":1000,"receivedTimestamp":1748354367.509362,"sentTimestamp":1748354367.512467,"lastFilledTimestamp":1748354504.1602714,"quantityFilled":1000,"averagePrice":190.4096,"numberOfFills":3,"totalAmount":190409.6,"version":1}
```