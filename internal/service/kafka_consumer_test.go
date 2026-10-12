package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWorkerIndexFor_Determinism verifies that the same execution id always maps to
// the same worker index, which is the serialization guarantee the routing relies on.
func TestWorkerIndexFor_Determinism(t *testing.T) {
	workerCounts := []int{1, 3, 5}
	ids := []int64{1, 2, 7, 42, 100, 999, 123456789}

	for _, n := range workerCounts {
		for _, id := range ids {
			first := workerIndexFor(id, n)
			// Same id must map to the same worker across many calls.
			for i := 0; i < 100; i++ {
				assert.Equal(t, first, workerIndexFor(id, n),
					"workerIndexFor(%d, %d) must be deterministic", id, n)
			}
			// Index must always be in range [0, n).
			assert.GreaterOrEqual(t, first, 0)
			assert.Less(t, first, n)
		}
	}
}

// TestWorkerIndexFor_SameIDSameWorker verifies that two messages carrying the same
// execution id resolve to the same worker (serialization), independent of other ids.
func TestWorkerIndexFor_SameIDSameWorker(t *testing.T) {
	const n = 5
	const id int64 = 20250617

	idxA := workerIndexFor(id, n)
	idxB := workerIndexFor(id, n)
	assert.Equal(t, idxA, idxB, "same execution id must route to the same worker")
}

// TestWorkerIndexFor_NegativeAndLargeIDs verifies negative and very large ids route
// in-range without panicking (exercises the uint64 conversion).
func TestWorkerIndexFor_NegativeAndLargeIDs(t *testing.T) {
	const n = 5
	ids := []int64{-1, -42, -9999999, 9223372036854775807, -9223372036854775808}

	for _, id := range ids {
		idx := workerIndexFor(id, n)
		assert.GreaterOrEqual(t, idx, 0, "id %d must route to a non-negative index", id)
		assert.Less(t, idx, n, "id %d must route in range", id)
	}
}

// TestWorkerIndexFor_ZeroWorkerCountGuard verifies the total-function guard.
func TestWorkerIndexFor_ZeroWorkerCountGuard(t *testing.T) {
	assert.Equal(t, 0, workerIndexFor(123, 0))
	assert.Equal(t, 0, workerIndexFor(123, -3))
}

// TestWorkerIndexForKey_Deterministic verifies the fallback path (used when a message
// value cannot be parsed) is deterministic and routes in range, so a malformed
// message is still delivered to a real worker rather than dropped.
func TestWorkerIndexForKey_Deterministic(t *testing.T) {
	workerCounts := []int{1, 3, 5}
	for _, n := range workerCounts {
		key := []byte("exec-123")
		first := workerIndexForKey(key, n)
		for i := 0; i < 100; i++ {
			assert.Equal(t, first, workerIndexForKey(key, n),
				"workerIndexForKey must be deterministic for n=%d", n)
		}
		assert.GreaterOrEqual(t, first, 0)
		assert.Less(t, first, n)
	}
}

// TestWorkerIndexForKey_EmptyKey verifies an unparseable/empty-key message still
// routes to a valid in-range worker (never dropped).
func TestWorkerIndexForKey_EmptyKey(t *testing.T) {
	const n = 5
	assert.Equal(t, workerIndexForKey(nil, n), workerIndexForKey([]byte{}, n),
		"nil and empty key must hash the same")

	for _, key := range [][]byte{nil, {}} {
		idx := workerIndexForKey(key, n)
		assert.GreaterOrEqual(t, idx, 0)
		assert.Less(t, idx, n)
	}
}

// TestWorkerIndexForKey_ZeroWorkerCountGuard verifies the total-function guard.
func TestWorkerIndexForKey_ZeroWorkerCountGuard(t *testing.T) {
	assert.Equal(t, 0, workerIndexForKey([]byte("exec-123"), 0))
	assert.Equal(t, 0, workerIndexForKey([]byte("exec-123"), -2))
}
