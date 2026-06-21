package metrics

import (
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Validates: Requirements 8.1, 8.2, 8.6, 8.7, 8.8, 12.6**

// TestPropertyLatencyClamping validates Property 8: Latency observation recorded
// iff valid creation time and non-negative (or clampable) result.
// Rules:
//   - latency >= 0 → record as-is (ok=true, value=latency)
//   - -1s < latency < 0 → clamp to 0 and record (ok=true, value=0)
//   - latency <= -1s → skip (ok=false)
func TestPropertyLatencyClamping(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Base time for generating time pairs
	baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	// Property: Positive latency is recorded as-is
	properties.Property("positive latency recorded as-is", prop.ForAll(
		func(offsetMs int64) bool {
			// Generate a positive offset (1ms to 600s)
			creationTime := baseTime
			completionTime := baseTime.Add(time.Duration(offsetMs) * time.Millisecond)

			latency, ok := CalculateLatency(creationTime, completionTime)
			if !ok {
				return false
			}

			expectedLatency := completionTime.Sub(creationTime).Seconds()
			// The latency should be recorded as-is (equal to the computed value)
			return latency == expectedLatency && latency >= 0
		},
		gen.Int64Range(1, 600000), // 1ms to 600s in milliseconds
	))

	// Property: Zero latency is recorded as-is
	properties.Property("zero latency recorded as-is", prop.ForAll(
		func(_ int) bool {
			now := baseTime
			latency, ok := CalculateLatency(now, now)
			return ok && latency == 0.0
		},
		gen.IntRange(0, 100), // dummy generator for iteration count
	))

	// Property: Slightly negative latency (-1s < latency < 0) is clamped to 0
	properties.Property("slightly negative latency clamped to zero", prop.ForAll(
		func(offsetMs int64) bool {
			// Generate an offset between -999ms and -1ms (slightly negative)
			creationTime := baseTime
			completionTime := baseTime.Add(time.Duration(-offsetMs) * time.Millisecond)

			latency, ok := CalculateLatency(creationTime, completionTime)
			if !ok {
				return false
			}

			// Should be clamped to 0 and still recorded
			return latency == 0.0
		},
		gen.Int64Range(1, 999), // 1ms to 999ms negative offset
	))

	// Property: Significantly negative latency (latency <= -1s) is skipped
	properties.Property("significantly negative latency is skipped", prop.ForAll(
		func(offsetMs int64) bool {
			// Generate an offset >= 1000ms negative (>= 1s negative)
			creationTime := baseTime
			completionTime := baseTime.Add(time.Duration(-offsetMs) * time.Millisecond)

			latency, ok := CalculateLatency(creationTime, completionTime)

			// Should not be recorded
			return !ok && latency == 0.0
		},
		gen.Int64Range(1000, 600000), // 1s to 600s negative offset
	))

	// Property: Exactly -1s boundary is skipped (latency <= -1s means skip)
	properties.Property("exactly negative one second is skipped", prop.ForAll(
		func(_ int) bool {
			creationTime := baseTime.Add(1 * time.Second)
			completionTime := baseTime // exactly -1s

			latency, ok := CalculateLatency(creationTime, completionTime)
			return !ok && latency == 0.0
		},
		gen.IntRange(0, 100), // dummy generator for iteration count
	))

	// Property: For any arbitrary time pair, the result is deterministic and follows the rules
	properties.Property("all time pairs follow clamping rules consistently", prop.ForAll(
		func(offsetNs int64) bool {
			creationTime := baseTime
			completionTime := baseTime.Add(time.Duration(offsetNs))

			latency, ok := CalculateLatency(creationTime, completionTime)
			diff := completionTime.Sub(creationTime).Seconds()

			switch {
			case diff >= 0:
				// Should record as-is
				return ok && latency == diff
			case diff > -1.0:
				// Should clamp to 0 and record
				return ok && latency == 0.0
			default:
				// diff <= -1.0: should skip
				return !ok && latency == 0.0
			}
		},
		// Generate offsets from -10s to +10s in nanoseconds
		gen.Int64Range(-10_000_000_000, 10_000_000_000),
	))

	properties.TestingRun(t)
}
