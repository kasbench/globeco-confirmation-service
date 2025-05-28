package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDataUtils(t *testing.T) {
	du := NewDataUtils()
	assert.NotNil(t, du)
}

func TestDataUtils_CalculatePercentageChange(t *testing.T) {
	du := NewDataUtils()

	tests := []struct {
		name     string
		oldValue float64
		newValue float64
		expected float64
	}{
		{
			name:     "positive change",
			oldValue: 100,
			newValue: 120,
			expected: 20,
		},
		{
			name:     "negative change",
			oldValue: 100,
			newValue: 80,
			expected: -20,
		},
		{
			name:     "no change",
			oldValue: 100,
			newValue: 100,
			expected: 0,
		},
		{
			name:     "from zero to positive",
			oldValue: 0,
			newValue: 50,
			expected: 100,
		},
		{
			name:     "from zero to zero",
			oldValue: 0,
			newValue: 0,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.CalculatePercentageChange(tt.oldValue, tt.newValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataUtils_CalculateTotalAmount(t *testing.T) {
	du := NewDataUtils()

	tests := []struct {
		name         string
		quantity     int64
		averagePrice float64
		expected     float64
	}{
		{
			name:         "normal calculation",
			quantity:     1000,
			averagePrice: 190.41,
			expected:     190410,
		},
		{
			name:         "zero quantity",
			quantity:     0,
			averagePrice: 190.41,
			expected:     0,
		},
		{
			name:         "zero price",
			quantity:     1000,
			averagePrice: 0,
			expected:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.CalculateTotalAmount(tt.quantity, tt.averagePrice)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataUtils_ValidateTotalAmount(t *testing.T) {
	du := NewDataUtils()

	tests := []struct {
		name         string
		quantity     int64
		averagePrice float64
		totalAmount  float64
		tolerance    float64
		expected     bool
	}{
		{
			name:         "exact match",
			quantity:     1000,
			averagePrice: 190.41,
			totalAmount:  190410,
			tolerance:    0.01,
			expected:     true,
		},
		{
			name:         "within tolerance",
			quantity:     1000,
			averagePrice: 190.41,
			totalAmount:  190409.99,
			tolerance:    1.0,
			expected:     true,
		},
		{
			name:         "outside tolerance",
			quantity:     1000,
			averagePrice: 190.41,
			totalAmount:  190400,
			tolerance:    0.01,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.ValidateTotalAmount(tt.quantity, tt.averagePrice, tt.totalAmount, tt.tolerance)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataUtils_CalculateAveragePrice(t *testing.T) {
	du := NewDataUtils()

	tests := []struct {
		name        string
		totalAmount float64
		quantity    int64
		expected    float64
	}{
		{
			name:        "normal calculation",
			totalAmount: 190410,
			quantity:    1000,
			expected:    190.41,
		},
		{
			name:        "zero quantity",
			totalAmount: 190410,
			quantity:    0,
			expected:    0,
		},
		{
			name:        "zero amount",
			totalAmount: 0,
			quantity:    1000,
			expected:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.CalculateAveragePrice(tt.totalAmount, tt.quantity)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataUtils_RoundToDecimalPlaces(t *testing.T) {
	du := NewDataUtils()

	tests := []struct {
		name     string
		value    float64
		places   int
		expected float64
	}{
		{
			name:     "round to 2 places",
			value:    190.4096,
			places:   2,
			expected: 190.41,
		},
		{
			name:     "round to 0 places",
			value:    190.4096,
			places:   0,
			expected: 190,
		},
		{
			name:     "round to 4 places",
			value:    190.4096,
			places:   4,
			expected: 190.4096,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.RoundToDecimalPlaces(tt.value, tt.places)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataUtils_FormatCurrency(t *testing.T) {
	du := NewDataUtils()

	tests := []struct {
		name          string
		amount        float64
		decimalPlaces int
		expected      string
	}{
		{
			name:          "format with 2 decimal places",
			amount:        190.41,
			decimalPlaces: 2,
			expected:      "$190.41",
		},
		{
			name:          "format with 0 decimal places",
			amount:        190.41,
			decimalPlaces: 0,
			expected:      "$190",
		},
		{
			name:          "format large amount",
			amount:        1234567.89,
			decimalPlaces: 2,
			expected:      "$1234567.89",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.FormatCurrency(tt.amount, tt.decimalPlaces)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataUtils_ParseCurrency(t *testing.T) {
	du := NewDataUtils()

	tests := []struct {
		name        string
		currencyStr string
		expected    float64
		shouldError bool
	}{
		{
			name:        "parse simple currency",
			currencyStr: "$190.41",
			expected:    190.41,
			shouldError: false,
		},
		{
			name:        "parse currency with commas",
			currencyStr: "$1,234,567.89",
			expected:    1234567.89,
			shouldError: false,
		},
		{
			name:        "parse without dollar sign",
			currencyStr: "190.41",
			expected:    190.41,
			shouldError: false,
		},
		{
			name:        "parse with whitespace",
			currencyStr: " $190.41 ",
			expected:    190.41,
			shouldError: false,
		},
		{
			name:        "invalid currency",
			currencyStr: "$abc",
			expected:    0,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := du.ParseCurrency(tt.currencyStr)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestDataUtils_ValidateFormats(t *testing.T) {
	du := NewDataUtils()

	// Test ValidateTickerFormat
	tickerTests := []struct {
		ticker   string
		expected bool
	}{
		{"IBM", true},
		{"AAPL", true},
		{"GOOGL", true},
		{"A", true},
		{"ABCDE", true},
		{"ABCDEF", false}, // Too long
		{"abc", false},    // Lowercase
		{"123", false},    // Numbers
		{"", false},       // Empty
	}

	for _, tt := range tickerTests {
		t.Run("ticker_"+tt.ticker, func(t *testing.T) {
			result := du.ValidateTickerFormat(tt.ticker)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Test ValidateSecurityIDFormat
	securityIDTests := []struct {
		securityID string
		expected   bool
	}{
		{"68336002fe95851f0a2aeda9", true},
		{"ABC123", true},
		{"123456", true},
		{"abc123", true},
		{"", false},        // Empty
		{"ABC-123", false}, // Special characters
	}

	for _, tt := range securityIDTests {
		t.Run("securityID_"+tt.securityID, func(t *testing.T) {
			result := du.ValidateSecurityIDFormat(tt.securityID)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Test ValidateDestinationFormat
	destinationTests := []struct {
		destination string
		expected    bool
	}{
		{"ML", true},
		{"NYSE", true},
		{"NASD", true},
		{"A", false},     // Too short
		{"ABCDE", false}, // Too long
		{"ml", false},    // Lowercase
		{"M1", false},    // Numbers
		{"", false},      // Empty
	}

	for _, tt := range destinationTests {
		t.Run("destination_"+tt.destination, func(t *testing.T) {
			result := du.ValidateDestinationFormat(tt.destination)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataUtils_NormalizeString(t *testing.T) {
	du := NewDataUtils()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "trim and uppercase",
			input:    " ibm ",
			expected: "IBM",
		},
		{
			name:     "already uppercase",
			input:    "IBM",
			expected: "IBM",
		},
		{
			name:     "mixed case",
			input:    "IbM",
			expected: "IBM",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.NormalizeString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataUtils_CalculateRates(t *testing.T) {
	du := NewDataUtils()

	// Test CalculateSuccessRate
	successRate := du.CalculateSuccessRate(95, 100)
	assert.Equal(t, 95.0, successRate)

	successRateZero := du.CalculateSuccessRate(0, 0)
	assert.Equal(t, 0.0, successRateZero)

	// Test CalculateErrorRate
	errorRate := du.CalculateErrorRate(5, 100)
	assert.Equal(t, 5.0, errorRate)

	errorRateZero := du.CalculateErrorRate(0, 0)
	assert.Equal(t, 0.0, errorRateZero)
}

func TestDataUtils_IsWithinTolerance(t *testing.T) {
	du := NewDataUtils()

	tests := []struct {
		name      string
		value1    float64
		value2    float64
		tolerance float64
		expected  bool
	}{
		{
			name:      "within tolerance",
			value1:    190.41,
			value2:    190.42,
			tolerance: 0.1,
			expected:  true,
		},
		{
			name:      "outside tolerance",
			value1:    190.41,
			value2:    190.52,
			tolerance: 0.1,
			expected:  false,
		},
		{
			name:      "exact match",
			value1:    190.41,
			value2:    190.41,
			tolerance: 0.01,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.IsWithinTolerance(tt.value1, tt.value2, tt.tolerance)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataUtils_CalculateMovingAverage(t *testing.T) {
	du := NewDataUtils()

	tests := []struct {
		name       string
		values     []float64
		windowSize int
		expected   []float64
	}{
		{
			name:       "normal moving average",
			values:     []float64{1, 2, 3, 4, 5},
			windowSize: 3,
			expected:   []float64{2, 3, 4},
		},
		{
			name:       "window size equals length",
			values:     []float64{1, 2, 3},
			windowSize: 3,
			expected:   []float64{2},
		},
		{
			name:       "window size larger than length",
			values:     []float64{1, 2},
			windowSize: 3,
			expected:   nil,
		},
		{
			name:       "zero window size",
			values:     []float64{1, 2, 3},
			windowSize: 0,
			expected:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.CalculateMovingAverage(tt.values, tt.windowSize)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataUtils_GetStatistics(t *testing.T) {
	du := NewDataUtils()

	tests := []struct {
		name     string
		values   []float64
		expected map[string]float64
	}{
		{
			name:   "empty values",
			values: []float64{},
			expected: map[string]float64{
				"count": 0,
			},
		},
		{
			name:   "single value",
			values: []float64{5},
			expected: map[string]float64{
				"count":    1,
				"sum":      5,
				"mean":     5,
				"median":   5,
				"min":      5,
				"max":      5,
				"variance": 0,
				"std_dev":  0,
				"range":    0,
			},
		},
		{
			name:   "multiple values",
			values: []float64{1, 2, 3, 4, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.GetStatistics(tt.values)

			if tt.expected != nil {
				for key, expectedValue := range tt.expected {
					assert.Equal(t, expectedValue, result[key])
				}
			} else {
				// For multiple values test, just verify structure
				assert.Contains(t, result, "count")
				assert.Contains(t, result, "sum")
				assert.Contains(t, result, "mean")
				assert.Contains(t, result, "median")
				assert.Contains(t, result, "min")
				assert.Contains(t, result, "max")
				assert.Contains(t, result, "variance")
				assert.Contains(t, result, "std_dev")
				assert.Contains(t, result, "range")

				// Verify basic calculations
				assert.Equal(t, float64(5), result["count"])
				assert.Equal(t, float64(15), result["sum"])
				assert.Equal(t, float64(3), result["mean"])
				assert.Equal(t, float64(3), result["median"])
				assert.Equal(t, float64(1), result["min"])
				assert.Equal(t, float64(5), result["max"])
				assert.Equal(t, float64(4), result["range"])
			}
		})
	}
}

func TestDataUtils_CalculatePercentile(t *testing.T) {
	du := NewDataUtils()

	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	tests := []struct {
		name       string
		values     []float64
		percentile float64
		expected   float64
	}{
		{
			name:       "50th percentile (median)",
			values:     values,
			percentile: 50,
			expected:   5.5,
		},
		{
			name:       "0th percentile (min)",
			values:     values,
			percentile: 0,
			expected:   1,
		},
		{
			name:       "100th percentile (max)",
			values:     values,
			percentile: 100,
			expected:   10,
		},
		{
			name:       "empty values",
			values:     []float64{},
			percentile: 50,
			expected:   0,
		},
		{
			name:       "invalid percentile",
			values:     values,
			percentile: -10,
			expected:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.CalculatePercentile(tt.values, tt.percentile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataUtils_SanitizeString(t *testing.T) {
	du := NewDataUtils()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal string",
			input:    "IBM",
			expected: "IBM",
		},
		{
			name:     "string with whitespace",
			input:    "  IBM  ",
			expected: "IBM",
		},
		{
			name:     "string with control characters",
			input:    "IBM\x00\x01",
			expected: "IBM",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.SanitizeString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataUtils_TruncateString(t *testing.T) {
	du := NewDataUtils()

	tests := []struct {
		name      string
		input     string
		maxLength int
		expected  string
	}{
		{
			name:      "no truncation needed",
			input:     "IBM",
			maxLength: 10,
			expected:  "IBM",
		},
		{
			name:      "truncation with ellipsis",
			input:     "This is a very long string",
			maxLength: 10,
			expected:  "This is...",
		},
		{
			name:      "truncation with short max length",
			input:     "IBM",
			maxLength: 2,
			expected:  "IB",
		},
		{
			name:      "exact length",
			input:     "IBM",
			maxLength: 3,
			expected:  "IBM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.TruncateString(tt.input, tt.maxLength)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataUtils_Checksum(t *testing.T) {
	du := NewDataUtils()

	data := "test data"
	checksum := du.GenerateChecksum(data)

	// Verify checksum is generated
	assert.NotEqual(t, uint32(0), checksum)

	// Verify validation works
	assert.True(t, du.ValidateChecksum(data, checksum))
	assert.False(t, du.ValidateChecksum("different data", checksum))

	// Verify same data produces same checksum
	checksum2 := du.GenerateChecksum(data)
	assert.Equal(t, checksum, checksum2)
}
