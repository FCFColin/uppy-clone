package metrics //nolint:revive // intentional package name

import (
	"testing"
)

// TestSLOBuckets verifies the SLO bucket configuration.
func TestSLOBuckets(t *testing.T) {
	if len(SLOBuckets) == 0 {
		t.Fatal("SLOBuckets should not be empty")
	}
	// Buckets must be sorted ascending for Prometheus histograms.
	for i := 1; i < len(SLOBuckets); i++ {
		if SLOBuckets[i] <= SLOBuckets[i-1] {
			t.Fatalf("SLOBuckets not strictly ascending at index %d: %v <= %v",
				i, SLOBuckets[i], SLOBuckets[i-1])
		}
	}
	// The 10s bucket should be absent (per enterprise rationale comment).
	for _, b := range SLOBuckets {
		if b >= 10.0 {
			t.Fatalf("SLOBuckets should not contain 10s bucket, got %v", b)
		}
	}
}
