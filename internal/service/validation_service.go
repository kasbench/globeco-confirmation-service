package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kasbench/globeco-confirmation-service/internal/domain"
	"github.com/kasbench/globeco-confirmation-service/pkg/logger"
	"go.uber.org/zap"
)

// ValidationService handles comprehensive validation of fill messages
type ValidationService struct {
	logger *logger.Logger
}

// ValidationConfig represents the configuration for the validation service
type ValidationConfig struct {
	Logger *logger.Logger
}

// ValidationResult represents the result of validation
type ValidationResult struct {
	IsValid  bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewValidationService creates a new validation service
func NewValidationService(config ValidationConfig) *ValidationService {
	return &ValidationService{
		logger: config.Logger,
	}
}

// ValidateFillMessage performs comprehensive validation of a fill message
func (vs *ValidationService) ValidateFillMessage(ctx context.Context, fill *domain.Fill) *ValidationResult {
	result := &ValidationResult{
		IsValid:  true,
		Errors:   []ValidationError{},
		Warnings: []ValidationWarning{},
	}

	vs.logger.WithContext(ctx).Debug("Starting comprehensive fill message validation",
		zap.Int64("fill_id", fill.ID),
		zap.Int64("execution_service_id", fill.ExecutionServiceID),
	)

	// 1. Required Fields Validation
	vs.validateRequiredFields(fill, result)

	// 2. Data Type Validation
	vs.validateDataTypes(fill, result)

	// 3. Business Rules Validation
	vs.validateBusinessRules(ctx, fill, result)

	// 4. Schema Validation
	vs.validateSchema(fill, result)

	// 5. Range Validation
	vs.validateRanges(fill, result)

	// 6. Format Validation
	vs.validateFormats(fill, result)

	// 7. Timestamp Validation
	vs.validateTimestamps(fill, result)

	// Log validation results
	if !result.IsValid {
		vs.logger.WithContext(ctx).Warn("Fill message validation failed",
			zap.Int64("fill_id", fill.ID),
			zap.Int("error_count", len(result.Errors)),
			zap.Int("warning_count", len(result.Warnings)),
		)
	} else if len(result.Warnings) > 0 {
		vs.logger.WithContext(ctx).Info("Fill message validation passed with warnings",
			zap.Int64("fill_id", fill.ID),
			zap.Int("warning_count", len(result.Warnings)),
		)
	} else {
		vs.logger.WithContext(ctx).Debug("Fill message validation passed",
			zap.Int64("fill_id", fill.ID),
		)
	}

	return result
}

// validateRequiredFields validates that all required fields are present and non-zero
func (vs *ValidationService) validateRequiredFields(fill *domain.Fill, result *ValidationResult) {
	if fill.ExecutionServiceID <= 0 {
		result.addError("executionServiceId", "REQUIRED_FIELD", "executionServiceId must be a positive integer")
	}

	if fill.QuantityFilled < 0 {
		result.addError("quantityFilled", "REQUIRED_FIELD", "quantityFilled must be non-negative")
	}

	if fill.AveragePrice <= 0 {
		result.addError("averagePrice", "REQUIRED_FIELD", "averagePrice must be positive")
	}

	if fill.Version < 0 {
		result.addError("version", "REQUIRED_FIELD", "version must be non-negative")
	}

	if strings.TrimSpace(fill.ExecutionStatus) == "" {
		result.addError("executionStatus", "REQUIRED_FIELD", "executionStatus is required")
	}

	if strings.TrimSpace(fill.TradeType) == "" {
		result.addError("tradeType", "REQUIRED_FIELD", "tradeType is required")
	}

	if strings.TrimSpace(fill.Destination) == "" {
		result.addError("destination", "REQUIRED_FIELD", "destination is required")
	}

	if strings.TrimSpace(fill.SecurityID) == "" {
		result.addError("securityId", "REQUIRED_FIELD", "securityId is required")
	}

	if strings.TrimSpace(fill.Ticker) == "" {
		result.addError("ticker", "REQUIRED_FIELD", "ticker is required")
	}
}

// validateDataTypes validates that all fields have correct data types
func (vs *ValidationService) validateDataTypes(fill *domain.Fill, result *ValidationResult) {
	// Validate numeric fields are within reasonable ranges for their types
	if fill.ID < 0 {
		result.addError("id", "INVALID_TYPE", "id must be non-negative")
	}

	if fill.Quantity <= 0 {
		result.addError("quantity", "INVALID_TYPE", "quantity must be positive")
	}

	if fill.NumberOfFills < 0 {
		result.addError("numberOfFills", "INVALID_TYPE", "numberOfFills must be non-negative")
	}

	if fill.TotalAmount < 0 {
		result.addError("totalAmount", "INVALID_TYPE", "totalAmount must be non-negative")
	}

	// Validate timestamp fields
	if fill.ReceivedTimestamp < 0 {
		result.addError("receivedTimestamp", "INVALID_TYPE", "receivedTimestamp must be non-negative")
	}

	if fill.SentTimestamp < 0 {
		result.addError("sentTimestamp", "INVALID_TYPE", "sentTimestamp must be non-negative")
	}

	if fill.LastFilledTimestamp < 0 {
		result.addError("lastFilledTimestamp", "INVALID_TYPE", "lastFilledTimestamp must be non-negative")
	}
}

// validateBusinessRules validates business-specific rules
func (vs *ValidationService) validateBusinessRules(ctx context.Context, fill *domain.Fill, result *ValidationResult) {
	// Rule 1: Quantity filled should not exceed original quantity
	if fill.QuantityFilled > fill.Quantity {
		result.addError("quantityFilled", "BUSINESS_RULE_VIOLATION",
			fmt.Sprintf("quantityFilled (%d) cannot exceed original quantity (%d)",
				fill.QuantityFilled, fill.Quantity))
	}

	// Rule 2: Average price should be reasonable (> 0 and < 10000)
	if fill.AveragePrice <= 0 {
		result.addError("averagePrice", "BUSINESS_RULE_VIOLATION",
			fmt.Sprintf("averagePrice (%.2f) must be positive", fill.AveragePrice))
	} else if fill.AveragePrice > 10000 {
		result.addWarning("averagePrice", "HIGH_PRICE",
			fmt.Sprintf("averagePrice (%.2f) is unusually high", fill.AveragePrice))
	}

	// Rule 3: Execution status must be valid
	validStatuses := map[string]bool{
		"NEW":   true,
		"SENT":  true,
		"WORK":  true,
		"PART":  true,
		"FULL":  true,
		"HOLD":  true,
		"CNCL":  true,
		"CNCLD": true,
		"CPART": true,
		"DEL":   true,
	}
	if !validStatuses[fill.ExecutionStatus] {
		result.addError("executionStatus", "BUSINESS_RULE_VIOLATION",
			fmt.Sprintf("executionStatus '%s' is not valid. Must be one of: NEW, SENT, WORK, PART, FULL, HOLD, CNCL, CNCLD, CPART, DEL",
				fill.ExecutionStatus))
	}

	// Rule 4: Trade type must be valid
	validTradeTypes := map[string]bool{
		"BUY":  true,
		"SELL": true,
	}
	if !validTradeTypes[fill.TradeType] {
		result.addError("tradeType", "BUSINESS_RULE_VIOLATION",
			fmt.Sprintf("tradeType '%s' is not valid. Must be BUY or SELL", fill.TradeType))
	}

	// Rule 5: Total amount should match quantity filled * average price (with tolerance)
	expectedTotal := float64(fill.QuantityFilled) * fill.AveragePrice
	tolerance := expectedTotal * 0.01 // 1% tolerance
	if fill.TotalAmount > 0 && (fill.TotalAmount < expectedTotal-tolerance || fill.TotalAmount > expectedTotal+tolerance) {
		result.addWarning("totalAmount", "CALCULATION_MISMATCH",
			fmt.Sprintf("totalAmount (%.2f) does not match expected value (%.2f) based on quantity and price",
				fill.TotalAmount, expectedTotal))
	}

	// Rule 6: Number of fills should be reasonable
	if fill.NumberOfFills <= 0 && fill.QuantityFilled > 0 {
		result.addWarning("numberOfFills", "INCONSISTENT_DATA",
			"numberOfFills should be positive when quantityFilled is positive")
	}

	// Rule 7: If execution is FULL, quantity filled should equal total quantity
	if fill.ExecutionStatus == "FULL" && fill.QuantityFilled != fill.Quantity {
		result.addWarning("quantityFilled", "STATUS_QUANTITY_MISMATCH",
			fmt.Sprintf("execution status is FULL but quantityFilled (%d) does not equal total quantity (%d)",
				fill.QuantityFilled, fill.Quantity))
	}

	// Rule 8: If execution is PARTIAL, quantity filled should be less than total
	if fill.ExecutionStatus == "PARTIAL" && fill.QuantityFilled >= fill.Quantity {
		result.addWarning("quantityFilled", "STATUS_QUANTITY_MISMATCH",
			fmt.Sprintf("execution status is PARTIAL but quantityFilled (%d) is not less than total quantity (%d)",
				fill.QuantityFilled, fill.Quantity))
	}
}

// validateSchema validates the JSON schema structure
func (vs *ValidationService) validateSchema(fill *domain.Fill, result *ValidationResult) {
	// Try to marshal and unmarshal to validate JSON structure
	data, err := json.Marshal(fill)
	if err != nil {
		result.addError("schema", "INVALID_JSON", "fill message cannot be serialized to JSON")
		return
	}

	var testFill domain.Fill
	if err := json.Unmarshal(data, &testFill); err != nil {
		result.addError("schema", "INVALID_JSON", "fill message JSON structure is invalid")
		return
	}

	// Validate that all required JSON fields are present in the original data
	var rawData map[string]interface{}
	if err := json.Unmarshal(data, &rawData); err != nil {
		result.addError("schema", "INVALID_JSON", "cannot parse fill message as JSON object")
		return
	}

	requiredFields := []string{
		"id", "executionServiceId", "executionStatus", "tradeType",
		"destination", "securityId", "ticker", "quantity",
		"quantityFilled", "averagePrice", "version",
	}

	for _, field := range requiredFields {
		if _, exists := rawData[field]; !exists {
			result.addError("schema", "MISSING_FIELD", fmt.Sprintf("required field '%s' is missing", field))
		}
	}
}

// validateRanges validates that numeric values are within acceptable ranges
func (vs *ValidationService) validateRanges(fill *domain.Fill, result *ValidationResult) {
	// Validate ID ranges
	if fill.ID > 9223372036854775807 { // Max int64
		result.addError("id", "OUT_OF_RANGE", "id exceeds maximum allowed value")
	}

	if fill.ExecutionServiceID > 9223372036854775807 {
		result.addError("executionServiceId", "OUT_OF_RANGE", "executionServiceId exceeds maximum allowed value")
	}

	// Validate quantity ranges
	if fill.Quantity > 1000000000 { // 1 billion shares seems reasonable max
		result.addWarning("quantity", "HIGH_QUANTITY", "quantity is unusually high")
	}

	if fill.QuantityFilled > 1000000000 {
		result.addWarning("quantityFilled", "HIGH_QUANTITY", "quantityFilled is unusually high")
	}

	// Validate price ranges
	if fill.AveragePrice > 100000 { // $100k per share seems like a reasonable warning threshold
		result.addWarning("averagePrice", "HIGH_PRICE", "averagePrice is extremely high")
	}

	// Validate total amount
	if fill.TotalAmount > 1000000000000 { // $1 trillion
		result.addWarning("totalAmount", "HIGH_AMOUNT", "totalAmount is extremely high")
	}

	// Validate version
	if fill.Version > 1000000 { // 1 million versions seems excessive
		result.addWarning("version", "HIGH_VERSION", "version number is unusually high")
	}
}

// validateFormats validates string field formats
func (vs *ValidationService) validateFormats(fill *domain.Fill, result *ValidationResult) {
	// Validate ticker format (typically 1-5 uppercase letters)
	tickerRegex := regexp.MustCompile(`^[A-Z]{1,5}$`)
	if !tickerRegex.MatchString(fill.Ticker) {
		result.addWarning("ticker", "INVALID_FORMAT",
			fmt.Sprintf("ticker '%s' does not match expected format (1-5 uppercase letters)", fill.Ticker))
	}

	// Validate security ID format (should be alphanumeric)
	securityIDRegex := regexp.MustCompile(`^[A-Za-z0-9]+$`)
	if !securityIDRegex.MatchString(fill.SecurityID) {
		result.addWarning("securityId", "INVALID_FORMAT",
			fmt.Sprintf("securityId '%s' contains invalid characters", fill.SecurityID))
	}

	// Validate destination format (typically 2-4 uppercase letters)
	destinationRegex := regexp.MustCompile(`^[A-Z]{2,4}$`)
	if !destinationRegex.MatchString(fill.Destination) {
		result.addWarning("destination", "INVALID_FORMAT",
			fmt.Sprintf("destination '%s' does not match expected format (2-4 uppercase letters)", fill.Destination))
	}

	// Validate string lengths
	if len(fill.Ticker) > 10 {
		result.addError("ticker", "TOO_LONG", "ticker exceeds maximum length of 10 characters")
	}

	if len(fill.SecurityID) > 50 {
		result.addError("securityId", "TOO_LONG", "securityId exceeds maximum length of 50 characters")
	}

	if len(fill.Destination) > 10 {
		result.addError("destination", "TOO_LONG", "destination exceeds maximum length of 10 characters")
	}
}

// validateTimestamps validates timestamp fields and their relationships
func (vs *ValidationService) validateTimestamps(fill *domain.Fill, result *ValidationResult) {
	now := time.Now().Unix()

	// Validate timestamps are not in the future (with 1 hour tolerance for clock skew)
	futureThreshold := float64(now + 3600)

	if fill.ReceivedTimestamp > futureThreshold {
		result.addWarning("receivedTimestamp", "FUTURE_TIMESTAMP", "receivedTimestamp is in the future")
	}

	if fill.SentTimestamp > futureThreshold {
		result.addWarning("sentTimestamp", "FUTURE_TIMESTAMP", "sentTimestamp is in the future")
	}

	if fill.LastFilledTimestamp > futureThreshold {
		result.addWarning("lastFilledTimestamp", "FUTURE_TIMESTAMP", "lastFilledTimestamp is in the future")
	}

	// Validate timestamps are not too old (more than 1 year)
	oldThreshold := float64(now - 365*24*3600)

	if fill.ReceivedTimestamp > 0 && fill.ReceivedTimestamp < oldThreshold {
		result.addWarning("receivedTimestamp", "OLD_TIMESTAMP", "receivedTimestamp is more than 1 year old")
	}

	// Validate timestamp ordering
	if fill.ReceivedTimestamp > 0 && fill.SentTimestamp > 0 {
		if fill.SentTimestamp < fill.ReceivedTimestamp {
			result.addError("sentTimestamp", "INVALID_TIMESTAMP_ORDER",
				"sentTimestamp cannot be before receivedTimestamp")
		}
	}

	if fill.LastFilledTimestamp > 0 && fill.SentTimestamp > 0 {
		if fill.LastFilledTimestamp < fill.SentTimestamp {
			result.addError("lastFilledTimestamp", "INVALID_TIMESTAMP_ORDER",
				"lastFilledTimestamp cannot be before sentTimestamp")
		}
	}

	// Validate reasonable time gaps
	if fill.ReceivedTimestamp > 0 && fill.SentTimestamp > 0 {
		gap := fill.SentTimestamp - fill.ReceivedTimestamp
		if gap > 3600 { // More than 1 hour between received and sent
			result.addWarning("sentTimestamp", "LARGE_TIME_GAP",
				"unusually large time gap between received and sent timestamps")
		}
	}
}

// Helper methods for ValidationResult
func (vr *ValidationResult) addError(field, code, message string) {
	vr.IsValid = false
	vr.Errors = append(vr.Errors, ValidationError{
		Field:   field,
		Code:    code,
		Message: message,
	})
}

func (vr *ValidationResult) addWarning(field, code, message string) {
	vr.Warnings = append(vr.Warnings, ValidationWarning{
		Field:   field,
		Code:    code,
		Message: message,
	})
}

// GetErrorSummary returns a summary of validation errors
func (vr *ValidationResult) GetErrorSummary() string {
	if len(vr.Errors) == 0 {
		return "No validation errors"
	}

	var messages []string
	for _, err := range vr.Errors {
		messages = append(messages, fmt.Sprintf("%s: %s", err.Field, err.Message))
	}

	return strings.Join(messages, "; ")
}

// GetWarningSummary returns a summary of validation warnings
func (vr *ValidationResult) GetWarningSummary() string {
	if len(vr.Warnings) == 0 {
		return "No validation warnings"
	}

	var messages []string
	for _, warning := range vr.Warnings {
		messages = append(messages, fmt.Sprintf("%s: %s", warning.Field, warning.Message))
	}

	return strings.Join(messages, "; ")
}
