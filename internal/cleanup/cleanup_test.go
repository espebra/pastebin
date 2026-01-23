package cleanup

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	interval := 30 * time.Minute

	c := New(nil, interval)

	if c == nil {
		t.Fatal("expected Cleaner to be non-nil")
	}

	if c.interval != interval {
		t.Errorf("expected interval %v, got %v", interval, c.interval)
	}
}

func TestNew_DifferentIntervals(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
	}{
		{"1 minute", time.Minute},
		{"1 hour", time.Hour},
		{"24 hours", 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(nil, tt.interval)
			if c.interval != tt.interval {
				t.Errorf("expected interval %v, got %v", tt.interval, c.interval)
			}
		})
	}
}

// Note: Full cleanup tests require an actual S3Storage instance
// These would typically be integration tests

// TestCleaner_Start_Integration would be an integration test that:
// 1. Creates pastes with short TTLs
// 2. Starts the cleanup routine
// 3. Verifies expired pastes are deleted
// 4. Verifies non-expired pastes remain
