package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFill_Validate(t *testing.T) {
	tests := []struct {
		name    string
		fill    Fill
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid fill",
			fill: Fill{
				ID:                  11,
				ExecutionServiceID:  27,
				IsOpen:              false,
				ExecutionStatus:     "FULL",
				TradeType:           "BUY",
				Destination:         "ML",
				SecurityID:          "68336002fe95851f0a2aeda9",
				Ticker:              "IBM",
				Quantity:            1000,
				ReceivedTimestamp:   1748354367.509362,
				SentTimestamp:       1748354367.512467,
				LastFilledTimestamp: 1748354504.1602714,
				QuantityFilled:      1000,
				AveragePrice:        190.4096,
				NumberOfFills:       3,
				TotalAmount:         190409.6,
				Version:             1,
			},
			wantErr: false,
		},
		{
			name: "quantity filled exceeds original quantity",
			fill: Fill{
				Quantity:            1000,
				QuantityFilled:      1500,
				AveragePrice:        100.0,
				ReceivedTimestamp:   1748354367.509362,
				SentTimestamp:       1748354367.512467,
				LastFilledTimestamp: 1748354504.1602714,
			},
			wantErr: true,
			errMsg:  "quantityFilled (1500) cannot exceed original quantity (1000)",
		},
		{
			name: "average price too low",
			fill: Fill{
				Quantity:            1000,
				QuantityFilled:      500,
				AveragePrice:        0.0,
				ReceivedTimestamp:   1748354367.509362,
				SentTimestamp:       1748354367.512467,
				LastFilledTimestamp: 1748354504.1602714,
			},
			wantErr: true,
			errMsg:  "averagePrice (0.00) must be between 0 and 10000",
		},
		{
			name: "average price too high",
			fill: Fill{
				Quantity:            1000,
				QuantityFilled:      500,
				AveragePrice:        15000.0,
				ReceivedTimestamp:   1748354367.509362,
				SentTimestamp:       1748354367.512467,
				LastFilledTimestamp: 1748354504.1602714,
			},
			wantErr: true,
			errMsg:  "averagePrice (15000.00) must be between 0 and 10000",
		},
		{
			name: "sent timestamp before received timestamp",
			fill: Fill{
				Quantity:            1000,
				QuantityFilled:      500,
				AveragePrice:        100.0,
				ReceivedTimestamp:   1748354367.512467,
				SentTimestamp:       1748354367.509362,
				LastFilledTimestamp: 1748354504.1602714,
			},
			wantErr: true,
			errMsg:  "sentTimestamp cannot be before receivedTimestamp",
		},
		{
			name: "last filled timestamp before sent timestamp",
			fill: Fill{
				Quantity:            1000,
				QuantityFilled:      500,
				AveragePrice:        100.0,
				ReceivedTimestamp:   1748354367.509362,
				SentTimestamp:       1748354367.512467,
				LastFilledTimestamp: 1748354367.510000,
			},
			wantErr: true,
			errMsg:  "lastFilledTimestamp cannot be before sentTimestamp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fill.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFill_TimeConversions(t *testing.T) {
	fill := Fill{
		ReceivedTimestamp:   1748354367.509362,
		SentTimestamp:       1748354367.512467,
		LastFilledTimestamp: 1748354504.1602714,
	}

	// Test received time conversion
	receivedTime := fill.GetReceivedTime()
	assert.Equal(t, int64(1748354367), receivedTime.Unix())
	assert.True(t, receivedTime.Nanosecond() > 0)

	// Test sent time conversion
	sentTime := fill.GetSentTime()
	assert.Equal(t, int64(1748354367), sentTime.Unix())
	assert.True(t, sentTime.Nanosecond() > 0)

	// Test last filled time conversion
	lastFilledTime := fill.GetLastFilledTime()
	assert.Equal(t, int64(1748354504), lastFilledTime.Unix())
	assert.True(t, lastFilledTime.Nanosecond() > 0)

	// Verify time ordering
	assert.True(t, sentTime.After(receivedTime))
	assert.True(t, lastFilledTime.After(sentTime))
}

func TestFill_JSONSerialization(t *testing.T) {
	fill := Fill{
		ID:                  11,
		ExecutionServiceID:  27,
		IsOpen:              false,
		ExecutionStatus:     "FULL",
		TradeType:           "BUY",
		Destination:         "ML",
		SecurityID:          "68336002fe95851f0a2aeda9",
		Ticker:              "IBM",
		Quantity:            1000,
		ReceivedTimestamp:   1748354367.509362,
		SentTimestamp:       1748354367.512467,
		LastFilledTimestamp: 1748354504.1602714,
		QuantityFilled:      1000,
		AveragePrice:        190.4096,
		NumberOfFills:       3,
		TotalAmount:         190409.6,
		Version:             1,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(fill)
	require.NoError(t, err)

	// Test JSON unmarshaling
	var unmarshaled Fill
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	// Verify all fields are preserved
	assert.Equal(t, fill.ID, unmarshaled.ID)
	assert.Equal(t, fill.ExecutionServiceID, unmarshaled.ExecutionServiceID)
	assert.Equal(t, fill.IsOpen, unmarshaled.IsOpen)
	assert.Equal(t, fill.ExecutionStatus, unmarshaled.ExecutionStatus)
	assert.Equal(t, fill.TradeType, unmarshaled.TradeType)
	assert.Equal(t, fill.Destination, unmarshaled.Destination)
	assert.Equal(t, fill.SecurityID, unmarshaled.SecurityID)
	assert.Equal(t, fill.Ticker, unmarshaled.Ticker)
	assert.Equal(t, fill.Quantity, unmarshaled.Quantity)
	assert.Equal(t, fill.ReceivedTimestamp, unmarshaled.ReceivedTimestamp)
	assert.Equal(t, fill.SentTimestamp, unmarshaled.SentTimestamp)
	assert.Equal(t, fill.LastFilledTimestamp, unmarshaled.LastFilledTimestamp)
	assert.Equal(t, fill.QuantityFilled, unmarshaled.QuantityFilled)
	assert.Equal(t, fill.AveragePrice, unmarshaled.AveragePrice)
	assert.Equal(t, fill.NumberOfFills, unmarshaled.NumberOfFills)
	assert.Equal(t, fill.TotalAmount, unmarshaled.TotalAmount)
	assert.Equal(t, fill.Version, unmarshaled.Version)
}

func TestFill_String(t *testing.T) {
	fill := Fill{
		ID:                 11,
		ExecutionServiceID: 27,
		ExecutionStatus:    "FULL",
		TradeType:          "BUY",
		Ticker:             "IBM",
		Quantity:           1000,
		QuantityFilled:     1000,
		AveragePrice:       190.4096,
		Version:            1,
	}

	str := fill.String()
	assert.Contains(t, str, `"id":11`)
	assert.Contains(t, str, `"executionServiceId":27`)
	assert.Contains(t, str, `"executionStatus":"FULL"`)
	assert.Contains(t, str, `"tradeType":"BUY"`)
	assert.Contains(t, str, `"ticker":"IBM"`)
}

func TestFill_ToUpdateRequest(t *testing.T) {
	fill := Fill{
		QuantityFilled: 1000,
		AveragePrice:   190.4096,
	}

	currentVersion := 5
	updateReq := fill.ToUpdateRequest(currentVersion)

	assert.Equal(t, fill.QuantityFilled, updateReq.QuantityFilled)
	assert.Equal(t, fill.AveragePrice, updateReq.AveragePrice)
	assert.Equal(t, currentVersion, updateReq.Version)
}
