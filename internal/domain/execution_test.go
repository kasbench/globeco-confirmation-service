package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionResponse_UnmarshalJSON_ScientificNotation(t *testing.T) {
	// Test the actual JSON response that was causing the issue
	jsonData := `{
		"id": 5,
		"executionStatus": "NEW",
		"tradeType": "BUY",
		"destination": "ML",
		"securityId": "68336002fe95851f0a2aeda9",
		"quantity": 1000.00000000,
		"limitPrice": 0E-8,
		"receivedTimestamp": "2025-05-28T12:38:22.134746Z",
		"sentTimestamp": "2025-05-28T12:38:22.142663Z",
		"tradeServiceExecutionId": 5,
		"quantityFilled": 0E-8,
		"averagePrice": null,
		"version": 1
	}`

	var response ExecutionResponse
	err := json.Unmarshal([]byte(jsonData), &response)
	require.NoError(t, err)

	// Verify all fields are parsed correctly
	assert.Equal(t, int64(5), response.ID)
	assert.Equal(t, "NEW", response.ExecutionStatus)
	assert.Equal(t, "BUY", response.TradeType)
	assert.Equal(t, "ML", response.Destination)
	assert.Equal(t, "68336002fe95851f0a2aeda9", response.SecurityID)
	assert.Equal(t, int64(1000), response.Quantity)  // Should convert 1000.00000000 to 1000
	assert.Equal(t, float64(0), response.LimitPrice) // Should convert 0E-8 to 0
	assert.Equal(t, int64(5), response.TradeServiceExecutionID)
	assert.Equal(t, int64(0), response.QuantityFilled) // Should convert 0E-8 to 0
	assert.Nil(t, response.AveragePrice)               // Should handle null
	assert.Equal(t, 1, response.Version)

	// Test GetAveragePrice method
	assert.Equal(t, float64(0), response.GetAveragePrice())
}

func TestExecutionResponse_UnmarshalJSON_WithAveragePrice(t *testing.T) {
	// Test with a non-null average price
	jsonData := `{
		"id": 5,
		"executionStatus": "PARTIAL",
		"tradeType": "BUY",
		"destination": "ML",
		"securityId": "68336002fe95851f0a2aeda9",
		"quantity": 1000,
		"limitPrice": 100.50,
		"receivedTimestamp": "2025-05-28T12:38:22.134746Z",
		"sentTimestamp": "2025-05-28T12:38:22.142663Z",
		"tradeServiceExecutionId": 5,
		"quantityFilled": 500,
		"averagePrice": 99.75,
		"version": 2
	}`

	var response ExecutionResponse
	err := json.Unmarshal([]byte(jsonData), &response)
	require.NoError(t, err)

	assert.Equal(t, int64(5), response.ID)
	assert.Equal(t, "PARTIAL", response.ExecutionStatus)
	assert.Equal(t, int64(1000), response.Quantity)
	assert.Equal(t, float64(100.50), response.LimitPrice)
	assert.Equal(t, int64(500), response.QuantityFilled)
	assert.NotNil(t, response.AveragePrice)
	assert.Equal(t, float64(99.75), *response.AveragePrice)
	assert.Equal(t, 2, response.Version)

	// Test GetAveragePrice method
	assert.Equal(t, float64(99.75), response.GetAveragePrice())
}

func TestExecutionResponse_UnmarshalJSON_ScientificNotationStrings(t *testing.T) {
	// Test with scientific notation as strings
	jsonData := `{
		"id": 5,
		"executionStatus": "NEW",
		"tradeType": "BUY",
		"destination": "ML",
		"securityId": "68336002fe95851f0a2aeda9",
		"quantity": "1.5E+3",
		"limitPrice": "1.25E+2",
		"receivedTimestamp": "2025-05-28T12:38:22.134746Z",
		"sentTimestamp": "2025-05-28T12:38:22.142663Z",
		"tradeServiceExecutionId": 5,
		"quantityFilled": "2.5E+2",
		"averagePrice": "9.975E+1",
		"version": 1
	}`

	var response ExecutionResponse
	err := json.Unmarshal([]byte(jsonData), &response)
	require.NoError(t, err)

	assert.Equal(t, int64(1500), response.Quantity)      // 1.5E+3 = 1500
	assert.Equal(t, float64(125), response.LimitPrice)   // 1.25E+2 = 125
	assert.Equal(t, int64(250), response.QuantityFilled) // 2.5E+2 = 250
	assert.NotNil(t, response.AveragePrice)
	assert.Equal(t, float64(99.75), *response.AveragePrice) // 9.975E+1 = 99.75
}

func TestParseToInt64(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int64
		hasError bool
	}{
		{"int", 42, 42, false},
		{"int64", int64(42), 42, false},
		{"float64", 42.7, 42, false},
		{"string int", "42", 42, false},
		{"string float", "42.7", 42, false},
		{"scientific notation", "1.5E+3", 1500, false},
		{"zero scientific", "0E-8", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseToInt64(tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParseToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected float64
		hasError bool
	}{
		{"int", 42, 42.0, false},
		{"int64", int64(42), 42.0, false},
		{"float64", 42.7, 42.7, false},
		{"string int", "42", 42.0, false},
		{"string float", "42.7", 42.7, false},
		{"scientific notation", "1.25E+2", 125.0, false},
		{"zero scientific", "0E-8", 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseToFloat64(tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestFromExecutionResponse(t *testing.T) {
	// Test conversion from ExecutionResponse to Execution
	response := &ExecutionResponse{
		ID:                      5,
		ExecutionStatus:         "PARTIAL",
		TradeType:               "BUY",
		Destination:             "ML",
		SecurityID:              "68336002fe95851f0a2aeda9",
		Quantity:                1000,
		LimitPrice:              100.50,
		TradeServiceExecutionID: 5,
		QuantityFilled:          500,
		AveragePrice:            nil, // Test null case
		Version:                 2,
	}

	execution := FromExecutionResponse(response)

	assert.Equal(t, int64(5), execution.ID)
	assert.Equal(t, "PARTIAL", execution.ExecutionStatus)
	assert.Equal(t, "BUY", execution.TradeType)
	assert.Equal(t, "ML", execution.Destination)
	assert.Equal(t, "68336002fe95851f0a2aeda9", execution.SecurityID)
	assert.Equal(t, int64(1000), execution.Quantity)
	assert.Equal(t, float64(100.50), execution.LimitPrice)
	assert.Equal(t, int64(5), execution.TradeServiceExecutionID)
	assert.Equal(t, int64(500), execution.QuantityFilled)
	assert.Equal(t, float64(0), execution.AveragePrice) // Should be 0 for null
	assert.Equal(t, 2, execution.Version)
}
