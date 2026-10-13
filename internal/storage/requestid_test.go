package storage

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRequestIDs_CollectsS3RequestIDs(t *testing.T) {
	var n atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-amz-request-id", fmt.Sprintf("s3-%d", n.Add(1)))
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	storage, err := New(context.Background(), endpoint, "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	// The bucket check during New must not count towards any request
	n.Store(0)

	ctx := WithRequestIDs(context.Background())
	if err := storage.Delete(ctx, "abc123"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	want := []string{"s3-1", "s3-2"}
	if got := RequestIDs(ctx); !reflect.DeepEqual(got, want) {
		t.Errorf("expected request IDs %v, got %v", want, got)
	}
}

func TestRequestIDs_WithoutCollector(t *testing.T) {
	if ids := RequestIDs(context.Background()); ids != nil {
		t.Errorf("expected no request IDs, got %v", ids)
	}

	// Requests made without a collector must still succeed
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-amz-request-id", "s3-1")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	storage, err := New(context.Background(), endpoint, "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if err := storage.Delete(context.Background(), "abc123"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}
