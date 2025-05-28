package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Fill represents a trade fill message received from Kafka
type Fill struct {
	ID                  int64   `json:"id" validate:"required"`
	ExecutionServiceID  int64   `json:"executionServiceId" validate:"required,min=1"`
	IsOpen              bool    `json:"isOpen"`
	ExecutionStatus     string  `json:"executionStatus" validate:"required,oneof=NEW SENT WORK PART FULL HOLD CNCL CNCLD CPART DEL"`
	TradeType           string  `json:"tradeType" validate:"required,oneof=BUY SELL"`
	Destination         string  `json:"destination" validate:"required"`
	SecurityID          string  `json:"securityId" validate:"required"`
	Ticker              string  `json:"ticker" validate:"required"`
	Quantity            int64   `json:"quantity" validate:"required,min=1"`
	ReceivedTimestamp   float64 `json:"receivedTimestamp" validate:"required"`
	SentTimestamp       float64 `json:"sentTimestamp" validate:"required"`
	LastFilledTimestamp float64 `json:"lastFilledTimestamp" validate:"required"`
	QuantityFilled      int64   `json:"quantityFilled" validate:"required,min=0"`
	AveragePrice        float64 `json:"averagePrice" validate:"required,min=0"`
	NumberOfFills       int     `json:"numberOfFills" validate:"required,min=0"`
	TotalAmount         float64 `json:"totalAmount" validate:"required,min=0"`
	Version             int     `json:"version" validate:"required,min=0"`
}

// Validate performs business rule validation on the Fill
func (f *Fill) Validate() error {
	// Validate that quantity filled doesn't exceed original quantity
	if f.QuantityFilled > f.Quantity {
		return fmt.Errorf("quantityFilled (%d) cannot exceed original quantity (%d)", f.QuantityFilled, f.Quantity)
	}

	// Validate that average price is reasonable (between 0 and 10000)
	if f.AveragePrice <= 0 || f.AveragePrice > 10000 {
		return fmt.Errorf("averagePrice (%.2f) must be between 0 and 10000", f.AveragePrice)
	}

	// Validate timestamp ordering
	if f.SentTimestamp < f.ReceivedTimestamp {
		return fmt.Errorf("sentTimestamp cannot be before receivedTimestamp")
	}

	if f.LastFilledTimestamp < f.SentTimestamp {
		return fmt.Errorf("lastFilledTimestamp cannot be before sentTimestamp")
	}

	return nil
}

// GetReceivedTime converts the received timestamp to time.Time
func (f *Fill) GetReceivedTime() time.Time {
	return time.Unix(int64(f.ReceivedTimestamp), int64((f.ReceivedTimestamp-float64(int64(f.ReceivedTimestamp)))*1e9))
}

// GetSentTime converts the sent timestamp to time.Time
func (f *Fill) GetSentTime() time.Time {
	return time.Unix(int64(f.SentTimestamp), int64((f.SentTimestamp-float64(int64(f.SentTimestamp)))*1e9))
}

// GetLastFilledTime converts the last filled timestamp to time.Time
func (f *Fill) GetLastFilledTime() time.Time {
	return time.Unix(int64(f.LastFilledTimestamp), int64((f.LastFilledTimestamp-float64(int64(f.LastFilledTimestamp)))*1e9))
}

// String returns a string representation of the Fill
func (f *Fill) String() string {
	data, _ := json.Marshal(f)
	return string(data)
}
