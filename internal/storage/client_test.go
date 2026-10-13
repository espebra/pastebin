package storage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/espebra/pastebin/internal/metrics"
)

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("failed to read counter: %v", err)
	}
	return m.GetCounter().GetValue()
}

func TestInstrumentedClient_RecordsS3Metrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	storage, err := New(context.Background(), strings.TrimPrefix(server.URL, "http://"), "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	counter := metrics.S3Requests.WithLabelValues("DeleteObject", "204")
	before := counterValue(t, counter)

	if err := storage.Delete(context.Background(), "abc123"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if got := counterValue(t, counter) - before; got != 2 {
		t.Errorf("expected 2 DeleteObject requests to be counted, got %v", got)
	}
}

type failingDoer struct{}

func (failingDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection refused")
}

func TestInstrumentedClient_CountsTransportErrors(t *testing.T) {
	counter := metrics.S3Requests.WithLabelValues("unknown", "error")
	before := counterValue(t, counter)

	req := httptest.NewRequest(http.MethodGet, "http://s3.example/bucket/key", nil)
	if _, err := (instrumentedClient{next: failingDoer{}}).Do(req); err == nil {
		t.Fatal("expected error to be passed through")
	}

	if got := counterValue(t, counter) - before; got != 1 {
		t.Errorf("expected 1 failed request to be counted, got %v", got)
	}
}
