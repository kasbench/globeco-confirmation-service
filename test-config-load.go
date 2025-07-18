package main

import (
	"fmt"
	"log"

	"github.com/kasbench/globeco-confirmation-service/internal/config"
)

func main() {
	cfg, err := config.LoadFromEnvironment()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	fmt.Printf("Configuration loaded successfully:\n")
	fmt.Printf("Kafka Consumer Timeout: %v\n", cfg.Kafka.ConsumerTimeout)
	fmt.Printf("Kafka Connection Timeout: %v\n", cfg.Kafka.ConnectionTimeout)
	fmt.Printf("Kafka Fetch Timeout: %v\n", cfg.Kafka.FetchTimeout)
	fmt.Printf("Execution Service Timeout: %v\n", cfg.ExecutionService.Timeout)
	fmt.Printf("Allocation Service Timeout: %v\n", cfg.AllocationService.Timeout)
}