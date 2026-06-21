package metrics

import (
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
)

func TestResolveMessageCreationTime_HeaderMillis(t *testing.T) {
	msg := kafka.Message{
		Headers: []kafka.Header{
			{Key: "created_at", Value: []byte("1700000000000")},
		},
	}

	got, ok := ResolveMessageCreationTime(msg, nil)
	assert.True(t, ok)
	assert.Equal(t, int64(1700000000), got.Unix())
}

func TestResolveMessageCreationTime_HeaderDecimalSeconds(t *testing.T) {
	msg := kafka.Message{
		Headers: []kafka.Header{
			{Key: "created_at", Value: []byte("1700000000.123")},
		},
	}

	got, ok := ResolveMessageCreationTime(msg, nil)
	assert.True(t, ok)
	assert.Equal(t, int64(1700000000), got.Unix())
}

func TestResolveMessageCreationTime_HeaderRFC3339(t *testing.T) {
	msg := kafka.Message{
		Headers: []kafka.Header{
			{Key: "created_at", Value: []byte("2024-01-15T10:30:00Z")},
		},
	}

	expected, _ := time.Parse(time.RFC3339, "2024-01-15T10:30:00Z")
	got, ok := ResolveMessageCreationTime(msg, nil)
	assert.True(t, ok)
	assert.Equal(t, expected.Unix(), got.Unix())
}

func TestResolveMessageCreationTime_PayloadCreatedAt(t *testing.T) {
	msg := kafka.Message{}
	payload := []byte(`{"createdAt": 1700000000000}`)

	got, ok := ResolveMessageCreationTime(msg, payload)
	assert.True(t, ok)
	assert.Equal(t, int64(1700000000), got.Unix())
}

func TestResolveMessageCreationTime_PayloadCreatedAtSnakeCase(t *testing.T) {
	msg := kafka.Message{}
	payload := []byte(`{"created_at": "2024-01-15T10:30:00Z"}`)

	expected, _ := time.Parse(time.RFC3339, "2024-01-15T10:30:00Z")
	got, ok := ResolveMessageCreationTime(msg, payload)
	assert.True(t, ok)
	assert.Equal(t, expected.Unix(), got.Unix())
}

func TestResolveMessageCreationTime_FallbackToMsgTime(t *testing.T) {
	msgTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	msg := kafka.Message{
		Time: msgTime,
	}

	got, ok := ResolveMessageCreationTime(msg, nil)
	assert.True(t, ok)
	assert.Equal(t, msgTime.Unix(), got.Unix())
}

func TestResolveMessageCreationTime_NoValidSource(t *testing.T) {
	msg := kafka.Message{}

	_, ok := ResolveMessageCreationTime(msg, nil)
	assert.False(t, ok)
}

func TestResolveMessageCreationTime_HeaderTakesPriorityOverPayload(t *testing.T) {
	headerTime := int64(1700000001000)
	payloadTime := int64(1700000002000)

	msg := kafka.Message{
		Headers: []kafka.Header{
			{Key: "created_at", Value: []byte("1700000001000")},
		},
	}
	payload := []byte(`{"createdAt": 1700000002000}`)
	_ = payloadTime

	got, ok := ResolveMessageCreationTime(msg, payload)
	assert.True(t, ok)
	assert.Equal(t, headerTime/1000, got.Unix())
}

func TestResolveMessageCreationTime_PayloadTakesPriorityOverMsgTime(t *testing.T) {
	msgTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	msg := kafka.Message{
		Time: msgTime,
	}
	payload := []byte(`{"createdAt": 1700000002000}`)

	got, ok := ResolveMessageCreationTime(msg, payload)
	assert.True(t, ok)
	assert.Equal(t, int64(1700000002), got.Unix())
}

func TestResolveMessageCreationTime_InvalidHeaderFallsToPayload(t *testing.T) {
	msg := kafka.Message{
		Headers: []kafka.Header{
			{Key: "created_at", Value: []byte("not-a-timestamp")},
		},
	}
	payload := []byte(`{"createdAt": 1700000000000}`)

	got, ok := ResolveMessageCreationTime(msg, payload)
	assert.True(t, ok)
	assert.Equal(t, int64(1700000000), got.Unix())
}

func TestResolveMessageCreationTime_EmptyHeaderFallsToPayload(t *testing.T) {
	msg := kafka.Message{
		Headers: []kafka.Header{
			{Key: "created_at", Value: []byte("")},
		},
	}
	payload := []byte(`{"createdAt": 1700000000000}`)

	got, ok := ResolveMessageCreationTime(msg, payload)
	assert.True(t, ok)
	assert.Equal(t, int64(1700000000), got.Unix())
}

func TestResolveMessageCreationTime_ZeroEpochSkipped(t *testing.T) {
	msg := kafka.Message{
		Headers: []kafka.Header{
			{Key: "created_at", Value: []byte("0")},
		},
	}

	_, ok := ResolveMessageCreationTime(msg, nil)
	assert.False(t, ok)
}

func TestResolveMessageCreationTime_NegativeEpochSkipped(t *testing.T) {
	msg := kafka.Message{
		Headers: []kafka.Header{
			{Key: "created_at", Value: []byte("-1000")},
		},
	}

	_, ok := ResolveMessageCreationTime(msg, nil)
	assert.False(t, ok)
}

func TestCalculateLatency_Positive(t *testing.T) {
	creation := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	completion := time.Date(2024, 1, 15, 10, 0, 5, 0, time.UTC)

	latency, ok := CalculateLatency(creation, completion)
	assert.True(t, ok)
	assert.InDelta(t, 5.0, latency, 0.001)
}

func TestCalculateLatency_Zero(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	latency, ok := CalculateLatency(now, now)
	assert.True(t, ok)
	assert.Equal(t, 0.0, latency)
}

func TestCalculateLatency_SlightlyNegativeClamped(t *testing.T) {
	creation := time.Date(2024, 1, 15, 10, 0, 0, 500000000, time.UTC) // 0.5s in the future
	completion := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	latency, ok := CalculateLatency(creation, completion)
	assert.True(t, ok)
	assert.Equal(t, 0.0, latency)
}

func TestCalculateLatency_NegativeExactlyOneSecondSkipped(t *testing.T) {
	creation := time.Date(2024, 1, 15, 10, 0, 1, 0, time.UTC)
	completion := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	_, ok := CalculateLatency(creation, completion)
	assert.False(t, ok)
}

func TestCalculateLatency_NegativeMoreThanOneSecondSkipped(t *testing.T) {
	creation := time.Date(2024, 1, 15, 10, 0, 5, 0, time.UTC)
	completion := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	_, ok := CalculateLatency(creation, completion)
	assert.False(t, ok)
}
