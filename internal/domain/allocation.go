package domain

// AllocationServiceExecutionDTO represents the payload for Allocation Service POST /api/v1/executions
// See documentation/supplemental-requirement-1.md for mapping details.
type AllocationServiceExecutionDTO struct {
	ExecutionServiceID int64    `json:"executionServiceId" validate:"required,min=1"`
	IsOpen             bool     `json:"isOpen" validate:"required,eq=false"`
	ExecutionStatus    string   `json:"executionStatus" validate:"required"`
	TradeType          string   `json:"tradeType" validate:"required"`
	Destination        string   `json:"destination" validate:"required"`
	SecurityID         string   `json:"securityId" validate:"required"`
	Ticker             string   `json:"ticker" validate:"required"`
	Quantity           int64    `json:"quantity" validate:"required,min=1"`
	LimitPrice         *float64 `json:"limitPrice"` // Always null
	ReceivedTimestamp  float64  `json:"receivedTimestamp" validate:"required"`
	SentTimestamp      float64  `json:"sentTimestamp" validate:"required"`
	LastFillTimestamp  float64  `json:"lastFillTimestamp" validate:"required"`
	QuantityFilled     int64    `json:"quantityFilled" validate:"required,min=0"`
	TotalAmount        float64  `json:"totalAmount" validate:"required,min=0"`
	AveragePrice       float64  `json:"averagePrice" validate:"required,min=0"`
}

// NewAllocationServiceExecutionDTO maps a Fill to AllocationServiceExecutionDTO
func NewAllocationServiceExecutionDTO(fill *Fill) *AllocationServiceExecutionDTO {
	return &AllocationServiceExecutionDTO{
		ExecutionServiceID: fill.ExecutionServiceID,
		IsOpen:             false, // Only for completed trades
		ExecutionStatus:    fill.ExecutionStatus,
		TradeType:          fill.TradeType,
		Destination:        fill.Destination,
		SecurityID:         fill.SecurityID,
		Ticker:             fill.Ticker,
		Quantity:           fill.Quantity,
		LimitPrice:         nil, // Always null
		ReceivedTimestamp:  fill.ReceivedTimestamp,
		SentTimestamp:      fill.SentTimestamp,
		LastFillTimestamp:  fill.LastFilledTimestamp,
		QuantityFilled:     fill.QuantityFilled,
		TotalAmount:        fill.TotalAmount,
		AveragePrice:       fill.AveragePrice,
	}
}
