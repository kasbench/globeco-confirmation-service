package utils

import (
	"fmt"
	"time"
)

// TimeUtils provides utility functions for time calculations and formatting
type TimeUtils struct{}

// NewTimeUtils creates a new TimeUtils instance
func NewTimeUtils() *TimeUtils {
	return &TimeUtils{}
}

// UnixFloatToTime converts a Unix timestamp with fractional seconds to time.Time
func (tu *TimeUtils) UnixFloatToTime(timestamp float64) time.Time {
	if timestamp <= 0 {
		return time.Time{}
	}

	seconds := int64(timestamp)
	nanoseconds := int64((timestamp - float64(seconds)) * 1e9)
	return time.Unix(seconds, nanoseconds)
}

// TimeToUnixFloat converts a time.Time to Unix timestamp with fractional seconds
func (tu *TimeUtils) TimeToUnixFloat(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}

	return float64(t.Unix()) + float64(t.Nanosecond())/1e9
}

// CalculateProcessingTime calculates the time difference between two Unix float timestamps
func (tu *TimeUtils) CalculateProcessingTime(startTimestamp, endTimestamp float64) time.Duration {
	if startTimestamp <= 0 || endTimestamp <= 0 || endTimestamp < startTimestamp {
		return 0
	}

	startTime := tu.UnixFloatToTime(startTimestamp)
	endTime := tu.UnixFloatToTime(endTimestamp)
	return endTime.Sub(startTime)
}

// ValidateTimestampOrder validates that timestamps are in the correct chronological order
func (tu *TimeUtils) ValidateTimestampOrder(timestamps ...float64) error {
	if len(timestamps) < 2 {
		return nil
	}

	for i := 1; i < len(timestamps); i++ {
		if timestamps[i] > 0 && timestamps[i-1] > 0 && timestamps[i] < timestamps[i-1] {
			return fmt.Errorf("timestamp at position %d (%.6f) is before timestamp at position %d (%.6f)",
				i, timestamps[i], i-1, timestamps[i-1])
		}
	}

	return nil
}

// IsTimestampInFuture checks if a timestamp is in the future (with tolerance for clock skew)
func (tu *TimeUtils) IsTimestampInFuture(timestamp float64, toleranceSeconds int64) bool {
	if timestamp <= 0 {
		return false
	}

	now := time.Now().Unix()
	return timestamp > float64(now+toleranceSeconds)
}

// IsTimestampTooOld checks if a timestamp is older than the specified duration
func (tu *TimeUtils) IsTimestampTooOld(timestamp float64, maxAge time.Duration) bool {
	if timestamp <= 0 {
		return false
	}

	timestampTime := tu.UnixFloatToTime(timestamp)
	return time.Since(timestampTime) > maxAge
}

// FormatDuration formats a duration in a human-readable way
func (tu *TimeUtils) FormatDuration(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%.0fns", float64(d.Nanoseconds()))
	} else if d < time.Millisecond {
		return fmt.Sprintf("%.2fμs", float64(d.Nanoseconds())/1000)
	} else if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Nanoseconds())/1e6)
	} else if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	} else {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
}

// CalculateLatency calculates the latency between message received and sent timestamps
func (tu *TimeUtils) CalculateLatency(receivedTimestamp, sentTimestamp float64) time.Duration {
	return tu.CalculateProcessingTime(receivedTimestamp, sentTimestamp)
}

// CalculateFillLatency calculates the latency between sent and last filled timestamps
func (tu *TimeUtils) CalculateFillLatency(sentTimestamp, lastFilledTimestamp float64) time.Duration {
	return tu.CalculateProcessingTime(sentTimestamp, lastFilledTimestamp)
}

// CalculateTotalLatency calculates the total latency from received to last filled
func (tu *TimeUtils) CalculateTotalLatency(receivedTimestamp, lastFilledTimestamp float64) time.Duration {
	return tu.CalculateProcessingTime(receivedTimestamp, lastFilledTimestamp)
}

// GetTimestampStats returns statistics about a set of timestamps
func (tu *TimeUtils) GetTimestampStats(timestamps []float64) map[string]interface{} {
	if len(timestamps) == 0 {
		return map[string]interface{}{
			"count": 0,
		}
	}

	var validTimestamps []float64
	for _, ts := range timestamps {
		if ts > 0 {
			validTimestamps = append(validTimestamps, ts)
		}
	}

	if len(validTimestamps) == 0 {
		return map[string]interface{}{
			"count":       len(timestamps),
			"valid_count": 0,
		}
	}

	// Find min and max
	min := validTimestamps[0]
	max := validTimestamps[0]
	sum := 0.0

	for _, ts := range validTimestamps {
		if ts < min {
			min = ts
		}
		if ts > max {
			max = ts
		}
		sum += ts
	}

	avg := sum / float64(len(validTimestamps))
	timeSpan := tu.CalculateProcessingTime(min, max)

	return map[string]interface{}{
		"count":       len(timestamps),
		"valid_count": len(validTimestamps),
		"min":         min,
		"max":         max,
		"average":     avg,
		"time_span":   timeSpan.String(),
		"oldest":      tu.UnixFloatToTime(min).Format(time.RFC3339),
		"newest":      tu.UnixFloatToTime(max).Format(time.RFC3339),
	}
}

// RoundToMilliseconds rounds a duration to the nearest millisecond
func (tu *TimeUtils) RoundToMilliseconds(d time.Duration) time.Duration {
	return d.Round(time.Millisecond)
}

// RoundToMicroseconds rounds a duration to the nearest microsecond
func (tu *TimeUtils) RoundToMicroseconds(d time.Duration) time.Duration {
	return d.Round(time.Microsecond)
}

// GetBusinessHours checks if a timestamp falls within business hours (9 AM - 5 PM EST)
func (tu *TimeUtils) GetBusinessHours(timestamp float64, timezone *time.Location) bool {
	if timestamp <= 0 {
		return false
	}

	if timezone == nil {
		// Default to EST
		var err error
		timezone, err = time.LoadLocation("America/New_York")
		if err != nil {
			timezone = time.UTC
		}
	}

	t := tu.UnixFloatToTime(timestamp).In(timezone)
	hour := t.Hour()

	// Business hours: 9 AM to 5 PM, Monday to Friday
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false
	}

	return hour >= 9 && hour < 17
}
