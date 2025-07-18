package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func main() {
	fmt.Println("Testing OpenTelemetry setup...")

	// Initialize OpenTelemetry
	shutdown, err := utils.SetupOTel(context.Background(), utils.OTelConfig{
		ServiceName:      "test-service",
		ServiceVersion:   "1.0.0",
		ServiceNamespace: "globeco",
		OTLPEndpoint:     "otel-collector-collector.monitoring.svc.cluster.local:4317",
		Enabled:          true,
	})
	if err != nil {
		log.Fatalf("Failed to setup OpenTelemetry: %v", err)
	}
	defer shutdown(context.Background())

	// Test tracing
	tracer := otel.Tracer("test-service")
	ctx, span := tracer.Start(context.Background(), "test-operation")
	span.SetAttributes(attribute.String("test.key", "test.value"))
	
	fmt.Println("Created test span")
	
	// Simulate some work
	time.Sleep(100 * time.Millisecond)
	
	span.End()
	fmt.Println("Ended test span")

	// Test metrics
	meter := otel.Meter("test-service")
	counter, err := meter.Int64Counter("test_counter")
	if err != nil {
		log.Printf("Failed to create counter: %v", err)
	} else {
		counter.Add(ctx, 1, metric.WithAttributes(attribute.String("test", "value")))
		fmt.Println("Recorded test metric")
	}

	// Give time for export
	time.Sleep(2 * time.Second)
	fmt.Println("Test completed")
}