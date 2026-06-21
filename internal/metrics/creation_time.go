package metrics

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// ResolveMessageCreationTime extracts the message creation timestamp using
// priority: (a) created_at header, (b) createdAt/created_at payload field, (c) Kafka record timestamp.
// Returns the resolved time and whether a valid time was found.
func ResolveMessageCreationTime(msg kafka.Message, payloadJSON []byte) (time.Time, bool) {
	// Priority (a): created_at Kafka header
	if t, ok := resolveFromHeader(msg.Headers); ok {
		return t, true
	}

	// Priority (b): createdAt or created_at payload JSON field
	if t, ok := resolveFromPayload(payloadJSON); ok {
		return t, true
	}

	// Priority (c): Kafka record timestamp (msg.Time)
	if !msg.Time.IsZero() && msg.Time.Unix() > 0 {
		return msg.Time, true
	}

	return time.Time{}, false
}

// CalculateLatency computes end-to-end latency and applies clamping rules.
// Returns the latency in seconds and whether it should be recorded.
// Rules: latency >= 0 → record as-is; -1s < latency < 0 → clamp to 0; latency <= -1s → skip.
func CalculateLatency(creationTime, completionTime time.Time) (float64, bool) {
	latency := completionTime.Sub(creationTime).Seconds()

	if latency >= 0 {
		return latency, true
	}

	// Negative latency: check if absolute value is less than 1 second
	if latency > -1.0 {
		return 0, true
	}

	// Absolute value is 1 second or greater — skip
	return 0, false
}

// resolveFromHeader attempts to parse the "created_at" header value.
// Supports: Unix epoch millis (integer string), Unix epoch seconds (decimal string), RFC 3339.
func resolveFromHeader(headers []kafka.Header) (time.Time, bool) {
	for _, h := range headers {
		if h.Key != "created_at" {
			continue
		}

		value := string(h.Value)
		if value == "" {
			return time.Time{}, false
		}

		if t, ok := parseHeaderValue(value); ok {
			return t, true
		}
		return time.Time{}, false
	}
	return time.Time{}, false
}

// parseHeaderValue tries to parse a header string as Unix epoch millis, Unix epoch seconds, or RFC 3339.
func parseHeaderValue(value string) (time.Time, bool) {
	// Try Unix epoch milliseconds (integer string, e.g., "1700000000000")
	if millis, err := strconv.ParseInt(value, 10, 64); err == nil {
		t := time.Unix(millis/1000, (millis%1000)*int64(time.Millisecond))
		if t.Unix() > 0 {
			return t, true
		}
		return time.Time{}, false
	}

	// Try Unix epoch seconds (decimal string, e.g., "1700000000.123")
	if strings.Contains(value, ".") {
		if secs, err := strconv.ParseFloat(value, 64); err == nil {
			sec := int64(math.Floor(secs))
			nsec := int64(math.Round((secs - float64(sec)) * 1e9))
			t := time.Unix(sec, nsec)
			if t.Unix() > 0 {
				return t, true
			}
			return time.Time{}, false
		}
	}

	// Try RFC 3339
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		if t.Unix() > 0 {
			return t, true
		}
		return time.Time{}, false
	}

	return time.Time{}, false
}

// resolveFromPayload attempts to extract a "createdAt" or "created_at" field from JSON payload.
// Supports: numeric value as Unix epoch millis, string value as RFC 3339.
func resolveFromPayload(payloadJSON []byte) (time.Time, bool) {
	if len(payloadJSON) == 0 {
		return time.Time{}, false
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payloadJSON, &raw); err != nil {
		return time.Time{}, false
	}

	// Try "createdAt" first, then "created_at"
	fieldNames := []string{"createdAt", "created_at"}
	for _, fieldName := range fieldNames {
		rawValue, exists := raw[fieldName]
		if !exists || len(rawValue) == 0 {
			continue
		}

		if t, ok := parsePayloadField(rawValue); ok {
			return t, true
		}
	}

	return time.Time{}, false
}

// parsePayloadField parses a JSON field value as either a numeric Unix epoch millis or RFC 3339 string.
func parsePayloadField(rawValue json.RawMessage) (time.Time, bool) {
	// Try parsing as a number (Unix epoch milliseconds)
	var numValue float64
	if err := json.Unmarshal(rawValue, &numValue); err == nil {
		millis := int64(numValue)
		t := time.Unix(millis/1000, (millis%1000)*int64(time.Millisecond))
		if t.Unix() > 0 {
			return t, true
		}
		return time.Time{}, false
	}

	// Try parsing as a string (RFC 3339)
	var strValue string
	if err := json.Unmarshal(rawValue, &strValue); err == nil {
		if strValue == "" {
			return time.Time{}, false
		}
		if t, err := time.Parse(time.RFC3339, strValue); err == nil {
			if t.Unix() > 0 {
				return t, true
			}
			return time.Time{}, false
		}
	}

	return time.Time{}, false
}
