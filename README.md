# GlobeCo Confirmation Service

A Go microservice that processes trade fill messages from Kafka, updates the GlobeCo Execution Service, and notifies the GlobeCo Allocation Service.

## Overview

The Confirmation Service is part of the GlobeCo suite of applications for benchmarking Kubernetes autoscaling. It consumes fill messages from a Kafka topic and updates execution records via the Execution Service API.

## Architecture

This service follows clean architecture principles with clear separation of concerns:

```
├── cmd/                    # Application entry points
│   └── confirmation-service/
├── internal/               # Private application code
│   ├── api/               # HTTP handlers and routes
│   ├── config/            # Configuration management
│   ├── domain/            # Business domain models
│   ├── middleware/        # HTTP middleware
│   ├── repository/        # Data access layer
│   ├── service/           # Business logic
│   └── utils/             # Utility functions
└── pkg/                   # Reusable public packages
    ├── client/            # External service clients
    ├── logger/            # Logging utilities
    └── metrics/           # Metrics collection
```

## Technology Stack

- **Go**: 1.23.4
- **Kafka**: 4.0.0 (segmentio/kafka-go)
- **HTTP Router**: Chi v5
- **Logging**: Zap
- **Configuration**: Viper
- **Metrics**: Prometheus
- **Tracing**: OpenTelemetry
- **Testing**: Testify + Testcontainers

## Configuration

The service supports configuration via environment variables and config files:

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `KAFKA_BROKERS` | Kafka bootstrap servers | `globeco-execution-service-kafka:9092` |
| `KAFKA_TOPIC` | Kafka topic to consume | `fills` |
| `KAFKA_CONSUMER_GROUP` | Kafka consumer group | `confirmation-service` |
| `EXECUTION_SERVICE_URL` | Execution Service base URL | `http://globeco-execution-service:8084` |
| `ALLOCATION_SERVICE_URL` | Allocation Service base URL | `http://globeco-allocation-service:8089` |
| `HTTP_PORT` | HTTP server port | `8086` |
| `LOG_LEVEL` | Logging level | `info` |

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health/live` | GET | Liveness probe |
| `/health/ready` | GET | Readiness probe |
| `/metrics` | GET | Prometheus metrics |

## Development

### Prerequisites

- Go 1.23.4+
- Docker and Docker Compose
- Access to Kafka cluster
- Access to Execution Service

### Setup

1. Clone the repository:
```bash
git clone https://github.com/kasbench/globeco-confirmation-service.git
cd globeco-confirmation-service
```

2. Install dependencies:
```bash
go mod tidy
```

3. Build the application:
```bash
go build ./cmd/confirmation-service
```

4. Run tests:
```bash
go test ./...
```

### Running Locally

```bash
# Set environment variables
export KAFKA_BROKERS=localhost:9092
export EXECUTION_SERVICE_URL=http://localhost:8084

# Run the service
./confirmation-service
```

## Docker

### Build Image

```bash
docker build -t globeco-confirmation-service .
```

### Run Container

```bash
docker run -p 8086:8086 \
  -e KAFKA_BROKERS=kafka:9092 \
  -e EXECUTION_SERVICE_URL=http://execution-service:8084 \
  globeco-confirmation-service
```

## Kubernetes Deployment

The service is designed for Kubernetes deployment with:

- Health check endpoints for liveness and readiness probes
- Graceful shutdown handling
- Prometheus metrics integration
- OpenTelemetry tracing

## Monitoring

### Metrics

The service exposes Prometheus metrics at `/metrics`:

- `confirmation_messages_processed_total` - Total messages processed
- `confirmation_messages_failed_total` - Total messages failed
- `confirmation_api_requests_duration_seconds` - API request duration
- `confirmation_api_requests_total` - Total API requests

### Logging

Structured JSON logging with correlation IDs for request tracing.

### Tracing

OpenTelemetry integration for distributed tracing across the GlobeCo platform.

## Contributing

1. Follow Go coding standards and run `gofmt`
2. Write tests for new functionality
3. Ensure all tests pass: `go test ./...`
4. Update documentation as needed

## License

See [LICENSE](LICENSE) file for details.

## Contact

- **Author**: Noah Krieger
- **Email**: noah@kasbench.org
- **Organization**: KASBench
