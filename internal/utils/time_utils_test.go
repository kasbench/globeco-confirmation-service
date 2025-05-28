package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTimeUtils(t *testing.T) {
	tu := NewTimeUtils()
	assert.NotNil(t, tu)
}

func TestTimeUtils_UnixFloatToTime(t *testing.T) {
	tu := NewTimeUtils()

	tests := []struct {
		name      string
		timestamp float64
		expected  time.Time
	}{
		{
			name:      "valid timestamp with fractional seconds",
			timestamp: 1748354367.509362,
			expected:  time.Unix(1748354367, 509362000),
		},
		{
			name:      "whole number timestamp",
			timestamp: 1748354367.0,
			expected:  time.Unix(1748354367, 0),
		},
		{
			name:      "zero timestamp",
			timestamp: 0,
			expected:  time.Time{},
		},
		{
			name:      "negative timestamp",
			timestamp: -1,
			expected:  time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tu.UnixFloatToTime(tt.timestamp)
			if tt.timestamp <= 0 {
				assert.True(t, result.IsZero())
			} else {
				assert.Equal(t, tt.expected.Unix(), result.Unix())
				// Check nanoseconds are approximately correct (within 1ms tolerance)
				assert.InDelta(t, tt.expected.Nanosecond(), result.Nanosecond(), 1000000)
			}
		})
	}
}

func TestTimeUtils_TimeToUnixFloat(t *testing.T) {
	tu := NewTimeUtils()

	tests := []struct {
		name     string
		time     time.Time
		expected float64
	}{
		{
			name:     "valid time",
			time:     time.Unix(1748354367, 509362000),
			expected: 1748354367.509362,
		},
		{
			name:     "whole second time",
			time:     time.Unix(1748354367, 0),
			expected: 1748354367.0,
		},
		{
			name:     "zero time",
			time:     time.Time{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tu.TimeToUnixFloat(tt.time)
			if tt.time.IsZero() {
				assert.Equal(t, 0.0, result)
			} else {
				assert.InDelta(t, tt.expected, result, 0.000001)
			}
		})
	}
}

func TestTimeUtils_CalculateProcessingTime(t *testing.T) {
	tu := NewTimeUtils()

	tests := []struct {
		name         string
		startTime    float64
		endTime      float64
		expectedDur  time.Duration
		shouldBeZero bool
	}{
		{
			name:        "valid time range",
			startTime:   1748354367.509362,
			endTime:     1748354367.512467,
			expectedDur: time.Duration(3105000), // ~3.1ms in nanoseconds
		},
		{
			name:         "zero start time",
			startTime:    0,
			endTime:      1748354367.512467,
			shouldBeZero: true,
		},
		{
			name:         "zero end time",
			startTime:    1748354367.509362,
			endTime:      0,
			shouldBeZero: true,
		},
		{
			name:         "end before start",
			startTime:    1748354367.512467,
			endTime:      1748354367.509362,
			shouldBeZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tu.CalculateProcessingTime(tt.startTime, tt.endTime)
			if tt.shouldBeZero {
				assert.Equal(t, time.Duration(0), result)
			} else {
				assert.InDelta(t, tt.expectedDur.Nanoseconds(), result.Nanoseconds(), 1000000)
			}
		})
	}
}

func TestTimeUtils_ValidateTimestampOrder(t *testing.T) {
	tu := NewTimeUtils()

	tests := []struct {
		name       string
		timestamps []float64
		shouldErr  bool
	}{
		{
			name:       "valid order",
			timestamps: []float64{1748354367.509362, 1748354367.512467, 1748354504.1602714},
			shouldErr:  false,
		},
		{
			name:       "invalid order",
			timestamps: []float64{1748354367.512467, 1748354367.509362},
			shouldErr:  true,
		},
		{
			name:       "single timestamp",
			timestamps: []float64{1748354367.509362},
			shouldErr:  false,
		},
		{
			name:       "empty timestamps",
			timestamps: []float64{},
			shouldErr:  false,
		},
		{
			name:       "with zero timestamps",
			timestamps: []float64{0, 1748354367.509362, 1748354367.512467},
			shouldErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tu.ValidateTimestampOrder(tt.timestamps...)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTimeUtils_IsTimestampInFuture(t *testing.T) {
	tu := NewTimeUtils()
	now := time.Now().Unix()

	tests := []struct {
		name             string
		timestamp        float64
		toleranceSeconds int64
		expected         bool
	}{
		{
			name:             "future timestamp",
			timestamp:        float64(now + 7200), // 2 hours in future
			toleranceSeconds: 3600,                // 1 hour tolerance
			expected:         true,
		},
		{
			name:             "within tolerance",
			timestamp:        float64(now + 1800), // 30 minutes in future
			toleranceSeconds: 3600,                // 1 hour tolerance
			expected:         false,
		},
		{
			name:             "past timestamp",
			timestamp:        float64(now - 3600), // 1 hour in past
			toleranceSeconds: 3600,
			expected:         false,
		},
		{
			name:             "zero timestamp",
			timestamp:        0,
			toleranceSeconds: 3600,
			expected:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tu.IsTimestampInFuture(tt.timestamp, tt.toleranceSeconds)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTimeUtils_IsTimestampTooOld(t *testing.T) {
	tu := NewTimeUtils()

	tests := []struct {
		name      string
		timestamp float64
		maxAge    time.Duration
		expected  bool
	}{
		{
			name:      "old timestamp",
			timestamp: float64(time.Now().Add(-25 * time.Hour).Unix()),
			maxAge:    24 * time.Hour,
			expected:  true,
		},
		{
			name:      "recent timestamp",
			timestamp: float64(time.Now().Add(-1 * time.Hour).Unix()),
			maxAge:    24 * time.Hour,
			expected:  false,
		},
		{
			name:      "zero timestamp",
			timestamp: 0,
			maxAge:    24 * time.Hour,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tu.IsTimestampTooOld(tt.timestamp, tt.maxAge)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTimeUtils_FormatDuration(t *testing.T) {
	tu := NewTimeUtils()

	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "nanoseconds",
			duration: 500 * time.Nanosecond,
			expected: "500ns",
		},
		{
			name:     "microseconds",
			duration: 1500 * time.Microsecond,
			expected: "1.50ms",
		},
		{
			name:     "milliseconds",
			duration: 250 * time.Millisecond,
			expected: "250.00ms",
		},
		{
			name:     "seconds",
			duration: 5 * time.Second,
			expected: "5.00s",
		},
		{
			name:     "minutes",
			duration: 3 * time.Minute,
			expected: "3.0m",
		},
		{
			name:     "hours",
			duration: 2 * time.Hour,
			expected: "2.0h",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tu.FormatDuration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTimeUtils_CalculateLatencies(t *testing.T) {
	tu := NewTimeUtils()

	receivedTime := 1748354367.509362
	sentTime := 1748354367.512467
	lastFilledTime := 1748354504.1602714

	// Test CalculateLatency
	latency := tu.CalculateLatency(receivedTime, sentTime)
	assert.Greater(t, latency, time.Duration(0))

	// Test CalculateFillLatency
	fillLatency := tu.CalculateFillLatency(sentTime, lastFilledTime)
	assert.Greater(t, fillLatency, time.Duration(0))

	// Test CalculateTotalLatency
	totalLatency := tu.CalculateTotalLatency(receivedTime, lastFilledTime)
	assert.Greater(t, totalLatency, time.Duration(0))
	assert.Greater(t, totalLatency, latency)
	assert.Greater(t, totalLatency, fillLatency)
}

func TestTimeUtils_GetTimestampStats(t *testing.T) {
	tu := NewTimeUtils()

	tests := []struct {
		name       string
		timestamps []float64
		expected   map[string]interface{}
	}{
		{
			name:       "empty timestamps",
			timestamps: []float64{},
			expected: map[string]interface{}{
				"count": 0,
			},
		},
		{
			name:       "all zero timestamps",
			timestamps: []float64{0, 0, 0},
			expected: map[string]interface{}{
				"count":       3,
				"valid_count": 0,
			},
		},
		{
			name:       "mixed timestamps",
			timestamps: []float64{1748354367.509362, 0, 1748354367.512467, 1748354504.1602714},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tu.GetTimestampStats(tt.timestamps)

			if tt.expected != nil {
				for key, expectedValue := range tt.expected {
					assert.Equal(t, expectedValue, result[key])
				}
			} else {
				// For mixed timestamps test, just verify structure
				assert.Contains(t, result, "count")
				assert.Contains(t, result, "valid_count")
				if result["valid_count"].(int) > 0 {
					assert.Contains(t, result, "min")
					assert.Contains(t, result, "max")
					assert.Contains(t, result, "average")
					assert.Contains(t, result, "time_span")
					assert.Contains(t, result, "oldest")
					assert.Contains(t, result, "newest")
				}
			}
		})
	}
}

func TestTimeUtils_RoundDurations(t *testing.T) {
	tu := NewTimeUtils()

	duration := 1234567 * time.Nanosecond // 1.234567ms

	// Test RoundToMilliseconds
	rounded := tu.RoundToMilliseconds(duration)
	assert.Equal(t, 1*time.Millisecond, rounded)

	// Test RoundToMicroseconds
	rounded = tu.RoundToMicroseconds(duration)
	assert.Equal(t, 1235*time.Microsecond, rounded)
}

func TestTimeUtils_GetBusinessHours(t *testing.T) {
	tu := NewTimeUtils()

	// Create a timestamp for a Tuesday at 10 AM EST
	est, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	businessDay := time.Date(2025, 1, 28, 10, 0, 0, 0, est) // Tuesday 10 AM
	weekend := time.Date(2025, 1, 25, 10, 0, 0, 0, est)     // Saturday 10 AM
	afterHours := time.Date(2025, 1, 28, 18, 0, 0, 0, est)  // Tuesday 6 PM

	tests := []struct {
		name      string
		timestamp float64
		timezone  *time.Location
		expected  bool
	}{
		{
			name:      "business hours",
			timestamp: tu.TimeToUnixFloat(businessDay),
			timezone:  est,
			expected:  true,
		},
		{
			name:      "weekend",
			timestamp: tu.TimeToUnixFloat(weekend),
			timezone:  est,
			expected:  false,
		},
		{
			name:      "after hours",
			timestamp: tu.TimeToUnixFloat(afterHours),
			timezone:  est,
			expected:  false,
		},
		{
			name:      "zero timestamp",
			timestamp: 0,
			timezone:  est,
			expected:  false,
		},
		{
			name:      "nil timezone defaults to EST",
			timestamp: tu.TimeToUnixFloat(businessDay),
			timezone:  nil,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tu.GetBusinessHours(tt.timestamp, tt.timezone)
			assert.Equal(t, tt.expected, result)
		})
	}
}
