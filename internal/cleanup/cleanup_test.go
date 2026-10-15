package cleanup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/espebra/pastebin/internal/metrics"
	"github.com/espebra/pastebin/internal/paste"
	"github.com/espebra/pastebin/internal/storage"
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

func pastesAvailable(t *testing.T) float64 {
	t.Helper()
	var m dto.Metric
	if err := metrics.PastesAvailable.Write(&m); err != nil {
		t.Fatalf("failed to read gauge: %v", err)
	}
	return m.GetGauge().GetValue()
}

func TestCleanup_SetsPastesAvailable(t *testing.T) {
	now := time.Now()
	metas := map[string]paste.Meta{
		"live":    {Checksum: "live", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		"expired": {Checksum: "expired", CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)},
	}
	listFails := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/test-bucket" && r.URL.Query().Get("list-type") == "2":
			if listFails {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
				<ListBucketResult>
					<Contents><Key>meta/live.json</Key></Contents>
					<Contents><Key>meta/expired.json</Key></Contents>
				</ListBucketResult>`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(r.URL.Path, "/test-bucket/meta/"):
			name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/test-bucket/meta/"), ".json")
			_ = json.NewEncoder(w).Encode(metas[name])
		}
	}))
	defer server.Close()

	ctx := context.Background()
	store, err := storage.New(ctx, strings.TrimPrefix(server.URL, "http://"), "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	c := New(store, time.Hour)

	c.cleanup(ctx)
	if got := pastesAvailable(t); got != 1 {
		t.Errorf("expected 1 available paste, got %v", got)
	}

	// A failed run must leave the previous count in place
	listFails = true
	c.cleanup(ctx)
	if got := pastesAvailable(t); got != 1 {
		t.Errorf("expected failed run to keep 1 available paste, got %v", got)
	}
}

// Note: Full cleanup tests require an actual S3Storage instance
// These would typically be integration tests

// TestCleaner_Start_Integration would be an integration test that:
// 1. Creates pastes with short TTLs
// 2. Starts the cleanup routine
// 3. Verifies expired pastes are deleted
// 4. Verifies non-expired pastes remain
