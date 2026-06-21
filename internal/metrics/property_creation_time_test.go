package metrics

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/segmentio/kafka-go"
)

// **Validates: Requirements 8.3, 8.4, 8.5, 12.5**

// headerFormatMillis formats a time as Unix epoch milliseconds string.
func headerFormatMillis(t time.Time) string {
	return fmt.Sprintf("%d", t.UnixMilli())
}

// headerFormatDecimalSecs formats a time as Unix epoch decimal seconds string.
// Uses a fractional representation with 3 decimal places (millisecond precision).
func headerFormatDecimalSecs(t time.Time) string {
	millis := t.UnixMilli()
	sec := millis / 1000
	frac := millis % 1000
	return fmt.Sprintf("%d.%03d", sec, frac)
}

// headerFormatRFC3339 formats a time as RFC 3339 string.
func headerFormatRFC3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// sourcePresence encodes whether a timestamp source is valid, invalid, or absent.
type sourcePresence int

const (
	sourceAbsent  sourcePresence = 0
	sourceValid   sourcePresence = 1
	sourceInvalid sourcePresence = 2
)

// headerFormat encodes valid header format variants.
type headerFormat int

const (
	formatMillis      headerFormat = 0
	formatDecimalSecs headerFormat = 1
	formatRFC3339     headerFormat = 2
)

// payloadFormat encodes valid payload format variants.
type payloadFormat int

const (
	payloadFormatMillisNumeric payloadFormat = 0
	payloadFormatRFC3339String payloadFormat = 1
)

// payloadFieldName encodes payload field name variants.
type payloadFieldName int

const (
	fieldCreatedAt      payloadFieldName = 0 // "createdAt"
	fieldCreatedAtSnake payloadFieldName = 1 // "created_at"
)

func TestPropertyCreationTimePriorityResolution(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.MaxSize = 50

	properties := gopter.NewProperties(parameters)

	// Generator for sourcePresence
	genSourcePresence := gen.IntRange(0, 2).Map(func(i int) sourcePresence {
		return sourcePresence(i)
	})

	// Generator for headerFormat
	genHeaderFormat := gen.IntRange(0, 2).Map(func(i int) headerFormat {
		return headerFormat(i)
	})

	// Generator for payloadFormat
	genPayloadFormat := gen.IntRange(0, 1).Map(func(i int) payloadFormat {
		return payloadFormat(i)
	})

	// Generator for payloadFieldName
	genPayloadFieldName := gen.IntRange(0, 1).Map(func(i int) payloadFieldName {
		return payloadFieldName(i)
	})

	// Property: When header has a valid timestamp, it is always returned regardless of payload/msgTime
	properties.Property("valid header always takes priority over payload and msgTime", prop.ForAll(
		func(headerFmt headerFormat, payloadPresence sourcePresence, payloadFmt payloadFormat,
			payloadField payloadFieldName, msgTimePresence sourcePresence,
			headerEpoch int64, headerNsec int64, payloadEpoch int64, msgTimeEpoch int64) bool {

			headerTime := time.Unix(headerEpoch, headerNsec%1000000000)

			// Build header value
			var headerValue string
			switch headerFmt {
			case formatMillis:
				headerValue = headerFormatMillis(headerTime)
			case formatDecimalSecs:
				headerValue = headerFormatDecimalSecs(headerTime)
			case formatRFC3339:
				headerValue = headerFormatRFC3339(headerTime)
			}

			msg := kafka.Message{
				Headers: []kafka.Header{
					{Key: "created_at", Value: []byte(headerValue)},
				},
			}

			// Build payload (may or may not have valid timestamps)
			var payload []byte
			if payloadPresence == sourceValid {
				payloadTime := time.Unix(payloadEpoch, 0)
				payload = buildPayload(payloadFmt, payloadField, payloadTime)
			} else if payloadPresence == sourceInvalid {
				payload = buildInvalidPayload(payloadField)
			}

			// Set msgTime
			if msgTimePresence == sourceValid {
				msg.Time = time.Unix(msgTimeEpoch, 0)
			}

			result, ok := ResolveMessageCreationTime(msg, payload)
			if !ok {
				return false // header was valid, should have returned ok
			}

			// The result should match the header time
			return timesMatchForFormat(result, headerTime, headerFmt)
		},
		genHeaderFormat.WithLabel("headerFmt"),
		genSourcePresence.WithLabel("payloadPresence"),
		genPayloadFormat.WithLabel("payloadFmt"),
		genPayloadFieldName.WithLabel("payloadField"),
		genSourcePresence.WithLabel("msgTimePresence"),
		gen.Int64Range(946684800, 1893456000).WithLabel("headerEpoch"),
		gen.Int64Range(0, 999999999).WithLabel("headerNsec"),
		gen.Int64Range(946684800, 1893456000).WithLabel("payloadEpoch"),
		gen.Int64Range(946684800, 1893456000).WithLabel("msgTimeEpoch"),
	))

	// Property: When header is absent/invalid and payload has valid timestamp, payload is returned
	properties.Property("valid payload takes priority over msgTime when header is absent or invalid", prop.ForAll(
		func(headerPresence sourcePresence, payloadFmt payloadFormat,
			payloadField payloadFieldName, msgTimePresence sourcePresence,
			payloadEpoch int64, msgTimeEpoch int64) bool {

			// Header is absent or invalid
			if headerPresence == sourceValid {
				return true // skip — covered by first property
			}

			msg := kafka.Message{}

			if headerPresence == sourceInvalid {
				msg.Headers = []kafka.Header{
					{Key: "created_at", Value: []byte("not-a-valid-timestamp-xyz")},
				}
			}

			// Build valid payload
			payloadTime := time.Unix(payloadEpoch, 0)
			payload := buildPayload(payloadFmt, payloadField, payloadTime)

			// Set msgTime
			if msgTimePresence == sourceValid {
				msg.Time = time.Unix(msgTimeEpoch, 0)
			}

			result, ok := ResolveMessageCreationTime(msg, payload)
			if !ok {
				return false // payload was valid, should succeed
			}

			// Result should match the payload time
			return timesMatchForPayloadFormat(result, payloadTime, payloadFmt)
		},
		genSourcePresence.WithLabel("headerPresence"),
		genPayloadFormat.WithLabel("payloadFmt"),
		genPayloadFieldName.WithLabel("payloadField"),
		genSourcePresence.WithLabel("msgTimePresence"),
		gen.Int64Range(946684800, 1893456000).WithLabel("payloadEpoch"),
		gen.Int64Range(946684800, 1893456000).WithLabel("msgTimeEpoch"),
	))

	// Property: When header and payload are absent/invalid, msgTime is returned if valid
	properties.Property("msgTime is used as fallback when header and payload are absent or invalid", prop.ForAll(
		func(headerPresence sourcePresence, payloadPresence sourcePresence, msgTimeEpoch int64) bool {

			if headerPresence == sourceValid || payloadPresence == sourceValid {
				return true // skip — covered by other properties
			}

			msg := kafka.Message{}

			if headerPresence == sourceInvalid {
				msg.Headers = []kafka.Header{
					{Key: "created_at", Value: []byte("garbage-data")},
				}
			}

			var payload []byte
			if payloadPresence == sourceInvalid {
				payload = []byte(`{"createdAt": "not-valid-rfc3339"}`)
			}

			msgTime := time.Unix(msgTimeEpoch, 0)
			msg.Time = msgTime

			result, ok := ResolveMessageCreationTime(msg, payload)
			if !ok {
				return false // msgTime was valid, should succeed
			}

			return result.Unix() == msgTime.Unix()
		},
		genSourcePresence.WithLabel("headerPresence"),
		genSourcePresence.WithLabel("payloadPresence"),
		gen.Int64Range(946684800, 1893456000).WithLabel("msgTimeEpoch"),
	))

	// Property: When all sources are absent or invalid, no time is returned
	properties.Property("returns false when all sources are absent or invalid", prop.ForAll(
		func(headerPresence sourcePresence, payloadPresence sourcePresence) bool {

			if headerPresence == sourceValid || payloadPresence == sourceValid {
				return true // skip — covered by other properties
			}

			msg := kafka.Message{} // msg.Time is zero by default

			if headerPresence == sourceInvalid {
				msg.Headers = []kafka.Header{
					{Key: "created_at", Value: []byte("xyz-invalid")},
				}
			}

			var payload []byte
			if payloadPresence == sourceInvalid {
				payload = []byte(`{"createdAt": "also-invalid"}`)
			}

			_, ok := ResolveMessageCreationTime(msg, payload)
			return !ok
		},
		genSourcePresence.WithLabel("headerPresence"),
		genSourcePresence.WithLabel("payloadPresence"),
	))

	properties.TestingRun(t)
}

// buildPayload constructs a valid JSON payload with a timestamp field.
func buildPayload(fmt payloadFormat, field payloadFieldName, t time.Time) []byte {
	fieldName := "createdAt"
	if field == fieldCreatedAtSnake {
		fieldName = "created_at"
	}

	var value interface{}
	switch fmt {
	case payloadFormatMillisNumeric:
		value = t.UnixMilli()
	case payloadFormatRFC3339String:
		value = t.UTC().Format(time.RFC3339)
	}

	data := map[string]interface{}{
		fieldName: value,
	}
	b, _ := json.Marshal(data)
	return b
}

// buildInvalidPayload constructs a payload with an invalid timestamp value.
func buildInvalidPayload(field payloadFieldName) []byte {
	fieldName := "createdAt"
	if field == fieldCreatedAtSnake {
		fieldName = "created_at"
	}

	data := map[string]interface{}{
		fieldName: "not-a-valid-format",
	}
	b, _ := json.Marshal(data)
	return b
}

// timesMatchForFormat compares result with expected time accounting for format precision loss.
func timesMatchForFormat(result, expected time.Time, fmt headerFormat) bool {
	switch fmt {
	case formatMillis:
		// Millis precision: truncate to milliseconds
		return result.UnixMilli() == expected.UnixMilli()
	case formatDecimalSecs:
		// Decimal seconds parsed via float64 may have floating-point precision issues.
		// Re-parse the formatted string through the same code path to get expected result.
		formatted := headerFormatDecimalSecs(expected)
		reparsed, ok := parseHeaderValue(formatted)
		if !ok {
			return false
		}
		return result.Unix() == reparsed.Unix() && abs64(result.UnixNano()-reparsed.UnixNano()) <= 1
	case formatRFC3339:
		// RFC3339 has second precision
		return result.Unix() == expected.Unix()
	}
	return false
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// timesMatchForPayloadFormat compares result with expected time for payload format precision.
func timesMatchForPayloadFormat(result, expected time.Time, fmt payloadFormat) bool {
	switch fmt {
	case payloadFormatMillisNumeric:
		// Millis precision
		return result.UnixMilli() == expected.UnixMilli()
	case payloadFormatRFC3339String:
		// RFC3339 has second precision
		return result.Unix() == expected.Unix()
	}
	return false
}
