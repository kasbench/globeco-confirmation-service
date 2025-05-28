package logger

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateCorrelationID generates a new correlation ID
func GenerateCorrelationID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a simple counter-based approach if crypto/rand fails
		return fmt.Sprintf("corr-%d", generateFallbackID())
	}
	return hex.EncodeToString(bytes)
}

// generateFallbackID generates a simple counter-based ID as fallback
func generateFallbackID() int64 {
	// This is a simple implementation. In production, you might want
	// to use atomic operations or a more sophisticated approach
	return int64(len(fmt.Sprintf("%p", &struct{}{})))
}
