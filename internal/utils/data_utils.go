package utils

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// DataUtils provides utility functions for data transformations and calculations
type DataUtils struct{}

// NewDataUtils creates a new DataUtils instance
func NewDataUtils() *DataUtils {
	return &DataUtils{}
}

// CalculatePercentageChange calculates the percentage change between two values
func (du *DataUtils) CalculatePercentageChange(oldValue, newValue float64) float64 {
	if oldValue == 0 {
		if newValue == 0 {
			return 0
		}
		return 100 // 100% change from zero
	}

	return ((newValue - oldValue) / oldValue) * 100
}

// CalculateTotalAmount calculates the total amount from quantity and average price
func (du *DataUtils) CalculateTotalAmount(quantity int64, averagePrice float64) float64 {
	return float64(quantity) * averagePrice
}

// ValidateTotalAmount validates that the total amount matches the calculated value within tolerance
func (du *DataUtils) ValidateTotalAmount(quantity int64, averagePrice, totalAmount, tolerance float64) bool {
	expectedTotal := du.CalculateTotalAmount(quantity, averagePrice)
	diff := math.Abs(totalAmount - expectedTotal)
	return diff <= tolerance
}

// CalculateAveragePrice calculates the average price from total amount and quantity
func (du *DataUtils) CalculateAveragePrice(totalAmount float64, quantity int64) float64 {
	if quantity == 0 {
		return 0
	}
	return totalAmount / float64(quantity)
}

// RoundToDecimalPlaces rounds a float64 to the specified number of decimal places
func (du *DataUtils) RoundToDecimalPlaces(value float64, places int) float64 {
	multiplier := math.Pow(10, float64(places))
	return math.Round(value*multiplier) / multiplier
}

// FormatCurrency formats a float64 as currency with the specified number of decimal places
func (du *DataUtils) FormatCurrency(amount float64, decimalPlaces int) string {
	format := fmt.Sprintf("%%.%df", decimalPlaces)
	return fmt.Sprintf("$"+format, amount)
}

// ParseCurrency parses a currency string and returns the float64 value
func (du *DataUtils) ParseCurrency(currencyStr string) (float64, error) {
	// Remove currency symbols and whitespace
	cleaned := strings.TrimSpace(currencyStr)
	cleaned = strings.ReplaceAll(cleaned, "$", "")
	cleaned = strings.ReplaceAll(cleaned, ",", "")

	return strconv.ParseFloat(cleaned, 64)
}

// ValidateTickerFormat validates that a ticker symbol follows the expected format
func (du *DataUtils) ValidateTickerFormat(ticker string) bool {
	// Ticker should be 1-5 uppercase letters
	matched, _ := regexp.MatchString(`^[A-Z]{1,5}$`, ticker)
	return matched
}

// ValidateSecurityIDFormat validates that a security ID follows the expected format
func (du *DataUtils) ValidateSecurityIDFormat(securityID string) bool {
	// Security ID should be alphanumeric
	matched, _ := regexp.MatchString(`^[A-Za-z0-9]+$`, securityID)
	return matched
}

// ValidateDestinationFormat validates that a destination follows the expected format
func (du *DataUtils) ValidateDestinationFormat(destination string) bool {
	// Destination should be 2-4 uppercase letters
	matched, _ := regexp.MatchString(`^[A-Z]{2,4}$`, destination)
	return matched
}

// NormalizeString normalizes a string by trimming whitespace and converting to uppercase
func (du *DataUtils) NormalizeString(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// CalculateSuccessRate calculates the success rate as a percentage
func (du *DataUtils) CalculateSuccessRate(successCount, totalCount int64) float64 {
	if totalCount == 0 {
		return 0
	}
	return (float64(successCount) / float64(totalCount)) * 100
}

// CalculateErrorRate calculates the error rate as a percentage
func (du *DataUtils) CalculateErrorRate(errorCount, totalCount int64) float64 {
	if totalCount == 0 {
		return 0
	}
	return (float64(errorCount) / float64(totalCount)) * 100
}

// IsWithinTolerance checks if two float64 values are within the specified tolerance
func (du *DataUtils) IsWithinTolerance(value1, value2, tolerance float64) bool {
	return math.Abs(value1-value2) <= tolerance
}

// CalculateMovingAverage calculates the moving average of a slice of float64 values
func (du *DataUtils) CalculateMovingAverage(values []float64, windowSize int) []float64 {
	if len(values) < windowSize || windowSize <= 0 {
		return nil
	}

	result := make([]float64, len(values)-windowSize+1)

	for i := 0; i <= len(values)-windowSize; i++ {
		sum := 0.0
		for j := i; j < i+windowSize; j++ {
			sum += values[j]
		}
		result[i] = sum / float64(windowSize)
	}

	return result
}

// GetStatistics calculates basic statistics for a slice of float64 values
func (du *DataUtils) GetStatistics(values []float64) map[string]float64 {
	if len(values) == 0 {
		return map[string]float64{
			"count": 0,
		}
	}

	// Calculate basic statistics
	sum := 0.0
	min := values[0]
	max := values[0]

	for _, v := range values {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	mean := sum / float64(len(values))

	// Calculate variance and standard deviation
	variance := 0.0
	for _, v := range values {
		variance += math.Pow(v-mean, 2)
	}
	variance /= float64(len(values))
	stdDev := math.Sqrt(variance)

	// Calculate median
	sortedValues := make([]float64, len(values))
	copy(sortedValues, values)

	// Simple bubble sort for median calculation
	for i := 0; i < len(sortedValues); i++ {
		for j := 0; j < len(sortedValues)-1-i; j++ {
			if sortedValues[j] > sortedValues[j+1] {
				sortedValues[j], sortedValues[j+1] = sortedValues[j+1], sortedValues[j]
			}
		}
	}

	var median float64
	n := len(sortedValues)
	if n%2 == 0 {
		median = (sortedValues[n/2-1] + sortedValues[n/2]) / 2
	} else {
		median = sortedValues[n/2]
	}

	return map[string]float64{
		"count":    float64(len(values)),
		"sum":      sum,
		"mean":     mean,
		"median":   median,
		"min":      min,
		"max":      max,
		"variance": variance,
		"std_dev":  stdDev,
		"range":    max - min,
	}
}

// CalculatePercentile calculates the specified percentile of a slice of float64 values
func (du *DataUtils) CalculatePercentile(values []float64, percentile float64) float64 {
	if len(values) == 0 || percentile < 0 || percentile > 100 {
		return 0
	}

	// Sort values
	sortedValues := make([]float64, len(values))
	copy(sortedValues, values)

	for i := 0; i < len(sortedValues); i++ {
		for j := 0; j < len(sortedValues)-1-i; j++ {
			if sortedValues[j] > sortedValues[j+1] {
				sortedValues[j], sortedValues[j+1] = sortedValues[j+1], sortedValues[j]
			}
		}
	}

	// Calculate percentile index
	index := (percentile / 100) * float64(len(sortedValues)-1)

	// If index is a whole number, return that value
	if index == float64(int(index)) {
		return sortedValues[int(index)]
	}

	// Otherwise, interpolate between the two nearest values
	lowerIndex := int(math.Floor(index))
	upperIndex := int(math.Ceil(index))

	if upperIndex >= len(sortedValues) {
		return sortedValues[len(sortedValues)-1]
	}

	weight := index - float64(lowerIndex)
	return sortedValues[lowerIndex]*(1-weight) + sortedValues[upperIndex]*weight
}

// SanitizeString removes potentially dangerous characters from a string
func (du *DataUtils) SanitizeString(s string) string {
	// Remove control characters and non-printable characters
	reg := regexp.MustCompile(`[^\x20-\x7E]`)
	sanitized := reg.ReplaceAllString(s, "")

	// Trim whitespace
	return strings.TrimSpace(sanitized)
}

// TruncateString truncates a string to the specified maximum length
func (du *DataUtils) TruncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}

	if maxLength <= 3 {
		return s[:maxLength]
	}

	return s[:maxLength-3] + "..."
}

// GenerateChecksum generates a simple checksum for data integrity validation
func (du *DataUtils) GenerateChecksum(data string) uint32 {
	var checksum uint32
	for _, char := range data {
		checksum = checksum*31 + uint32(char)
	}
	return checksum
}

// ValidateChecksum validates data against a checksum
func (du *DataUtils) ValidateChecksum(data string, expectedChecksum uint32) bool {
	return du.GenerateChecksum(data) == expectedChecksum
}
