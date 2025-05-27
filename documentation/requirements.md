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


## Execution Plan    