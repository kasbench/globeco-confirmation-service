package domain

import (
	"encoding/json"
	"strconv"
	"time"
)

// ExecutionResponse represents the response from the Execution Service GET API
type ExecutionResponse struct {
	ID                      int64     `json:"id"`
	ExecutionStatus         string    `json:"executionStatus"`
	TradeType               string    `json:"tradeType"`
	Destination             string    `json:"destination"`
	SecurityID              string    `json:"securityId"`
	Quantity                int64     `json:"quantity"`
	LimitPrice              float64   `json:"limitPrice"`
	ReceivedTimestamp       time.Time `json:"receivedTimestamp"`
	SentTimestamp           time.Time `json:"sentTimestamp"`
	TradeServiceExecutionID int64     `json:"tradeServiceExecutionId"`
	QuantityFilled          int64     `json:"quantityFilled"`
	AveragePrice            *float64  `json:"averagePrice"`
	Version                 int       `json:"version"`
}

// UnmarshalJSON implements custom JSON unmarshaling for ExecutionResponse
func (e *ExecutionResponse) UnmarshalJSON(data []byte) error {
	// Define a temporary struct with string fields for problematic numeric fields
	type Alias ExecutionResponse
	aux := &struct {
		Quantity       interface{} `json:"quantity"`
		LimitPrice     interface{} `json:"limitPrice"`
		QuantityFilled interface{} `json:"quantityFilled"`
		AveragePrice   interface{} `json:"averagePrice"`
		*Alias
	}{
		Alias: (*Alias)(e),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Parse Quantity (handle both int, float, and scientific notation)
	if aux.Quantity != nil {
		if val, err := parseToInt64(aux.Quantity); err == nil {
			e.Quantity = val
		}
	}

	// Parse LimitPrice (handle scientific notation and null)
	if aux.LimitPrice != nil {
		if val, err := parseToFloat64(aux.LimitPrice); err == nil {
			e.LimitPrice = val
		}
	}

	// Parse QuantityFilled (handle scientific notation)
	if aux.QuantityFilled != nil {
		if val, err := parseToInt64(aux.QuantityFilled); err == nil {
			e.QuantityFilled = val
		}
	}

	// Parse AveragePrice (handle null and scientific notation)
	if aux.AveragePrice != nil {
		if val, err := parseToFloat64(aux.AveragePrice); err == nil {
			e.AveragePrice = &val
		}
	}

	return nil
}

// GetAveragePrice returns the average price value, handling null case
func (e *ExecutionResponse) GetAveragePrice() float64 {
	if e.AveragePrice == nil {
		return 0.0
	}
	return *e.AveragePrice
}

// parseToInt64 converts various numeric types to int64
func parseToInt64(value interface{}) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return int64(f), nil
		}
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, json.Unmarshal([]byte("0"), &value)
	}
}

// parseToFloat64 converts various numeric types to float64
func parseToFloat64(value interface{}) (float64, error) {
	switch v := value.(type) {
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, json.Unmarshal([]byte("0"), &value)
	}
}

// ExecutionUpdateRequest represents the request payload for updating an execution
type ExecutionUpdateRequest struct {
	QuantityFilled int64   `json:"quantityFilled" validate:"required,min=0"`
	AveragePrice   float64 `json:"averagePrice" validate:"required,min=0"`
	Version        int     `json:"version" validate:"required,min=0"`
}

// ExecutionUpdateResponse represents the response from the Execution Service PUT API
type ExecutionUpdateResponse struct {
	ID                      int64     `json:"id"`
	ExecutionStatus         string    `json:"executionStatus"`
	TradeType               string    `json:"tradeType"`
	Destination             string    `json:"destination"`
	SecurityID              string    `json:"securityId"`
	Quantity                int64     `json:"quantity"`
	LimitPrice              float64   `json:"limitPrice"`
	ReceivedTimestamp       time.Time `json:"receivedTimestamp"`
	SentTimestamp           time.Time `json:"sentTimestamp"`
	TradeServiceExecutionID int64     `json:"tradeServiceExecutionId"`
	QuantityFilled          int64     `json:"quantityFilled"`
	AveragePrice            float64   `json:"averagePrice"`
	Version                 int       `json:"version"`
}

// Execution represents the internal domain model for an execution
type Execution struct {
	ID                      int64
	ExecutionStatus         string
	TradeType               string
	Destination             string
	SecurityID              string
	Quantity                int64
	LimitPrice              float64
	ReceivedTimestamp       time.Time
	SentTimestamp           time.Time
	TradeServiceExecutionID int64
	QuantityFilled          int64
	AveragePrice            float64
	Version                 int
}

// ToUpdateRequest creates an ExecutionUpdateRequest from a Fill
func (f *Fill) ToUpdateRequest(currentVersion int) *ExecutionUpdateRequest {
	return &ExecutionUpdateRequest{
		QuantityFilled: f.QuantityFilled,
		AveragePrice:   f.AveragePrice,
		Version:        currentVersion,
	}
}

// FromExecutionResponse converts an ExecutionResponse to internal Execution model
func FromExecutionResponse(resp *ExecutionResponse) *Execution {
	return &Execution{
		ID:                      resp.ID,
		ExecutionStatus:         resp.ExecutionStatus,
		TradeType:               resp.TradeType,
		Destination:             resp.Destination,
		SecurityID:              resp.SecurityID,
		Quantity:                resp.Quantity,
		LimitPrice:              resp.LimitPrice,
		ReceivedTimestamp:       resp.ReceivedTimestamp,
		SentTimestamp:           resp.SentTimestamp,
		TradeServiceExecutionID: resp.TradeServiceExecutionID,
		QuantityFilled:          resp.QuantityFilled,
		AveragePrice:            resp.GetAveragePrice(),
		Version:                 resp.Version,
	}
}

// String returns a string representation of the ExecutionUpdateRequest
func (e *ExecutionUpdateRequest) String() string {
	data, _ := json.Marshal(e)
	return string(data)
}

// String returns a string representation of the ExecutionResponse
func (e *ExecutionResponse) String() string {
	data, _ := json.Marshal(e)
	return string(data)
}
