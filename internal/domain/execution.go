package domain

import (
	"encoding/json"
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
	AveragePrice            float64   `json:"averagePrice"`
	Version                 int       `json:"version"`
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
		AveragePrice:            resp.AveragePrice,
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
