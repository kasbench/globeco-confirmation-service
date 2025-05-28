.PHONY: build test clean run docker-build docker-run lint fmt vet deps

# Variables
BINARY_NAME=confirmation-service
DOCKER_IMAGE=globeco-confirmation-service
DOCKER_TAG=latest

# Build the application
build:
	go build -o bin/$(BINARY_NAME) ./cmd/confirmation-service

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Run the application
run:
	go run ./cmd/confirmation-service

# Format code
fmt:
	go fmt ./...

# Vet code
vet:
	go vet ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Download dependencies
deps:
	go mod download
	go mod tidy

# Build Docker image
docker-build:
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

# Run Docker container
docker-run:
	docker run -p 8086:8086 \
		-e KAFKA_BROKERS=localhost:9092 \
		-e EXECUTION_SERVICE_URL=http://localhost:8084 \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

# Development setup
dev-setup: deps fmt vet test

# CI pipeline
ci: deps fmt vet lint test

# Help
help:
	@echo "Available targets:"
	@echo "  build         - Build the application"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  clean         - Clean build artifacts"
	@echo "  run           - Run the application"
	@echo "  fmt           - Format code"
	@echo "  vet           - Vet code"
	@echo "  lint          - Lint code (requires golangci-lint)"
	@echo "  deps          - Download and tidy dependencies"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run Docker container"
	@echo "  dev-setup     - Setup for development"
	@echo "  ci            - Run CI pipeline"
	@echo "  help          - Show this help" 