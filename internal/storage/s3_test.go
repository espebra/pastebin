package storage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/espebra/pastebin/internal/paste"
)

func TestPrefixConstants(t *testing.T) {
	if pastePrefix != "pastes/" {
		t.Errorf("expected pastePrefix to be 'pastes/', got %q", pastePrefix)
	}

	if metaPrefix != "meta/" {
		t.Errorf("expected metaPrefix to be 'meta/', got %q", metaPrefix)
	}
}

func TestDelete_SendsCorrectRequests(t *testing.T) {
	checksum := "abc123def456"
	var deletedPaths []string

	// Create mock S3 server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletedPaths = append(deletedPaths, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	// Extract host:port from server URL
	endpoint := strings.TrimPrefix(server.URL, "http://")

	ctx := context.Background()
	storage, err := New(ctx, endpoint, "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	// Call Delete
	err = storage.Delete(ctx, checksum)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify both objects were deleted
	expectedPaths := []string{
		"/test-bucket/pastes/" + checksum,
		"/test-bucket/meta/" + checksum + ".json",
	}

	if len(deletedPaths) != 2 {
		t.Fatalf("expected 2 delete requests, got %d: %v", len(deletedPaths), deletedPaths)
	}

	for _, expected := range expectedPaths {
		found := false
		for _, path := range deletedPaths {
			if path == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected delete request for %q, got: %v", expected, deletedPaths)
		}
	}
}

func TestDelete_DeletesPasteObject(t *testing.T) {
	checksum := "testchecksum789"
	pasteDeleted := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/test-bucket/pastes/"+checksum {
			pasteDeleted = true
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	ctx := context.Background()

	storage, err := New(ctx, endpoint, "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	if err := storage.Delete(ctx, checksum); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if !pasteDeleted {
		t.Error("paste object was not deleted")
	}
}

func TestDelete_DeletesMetadataObject(t *testing.T) {
	checksum := "testchecksum789"
	metaDeleted := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/test-bucket/meta/"+checksum+".json" {
			metaDeleted = true
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	ctx := context.Background()

	storage, err := New(ctx, endpoint, "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	if err := storage.Delete(ctx, checksum); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if !metaDeleted {
		t.Error("metadata object was not deleted")
	}
}

func TestGet_VerifiesChecksum(t *testing.T) {
	content := "Hello, World!"
	checksum := paste.ComputeChecksum(content)
	meta := paste.Meta{
		Checksum:  checksum,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
		Size:      int64(len(content)),
	}
	metaJSON, _ := json.Marshal(meta)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pastes/") {
			_, _ = w.Write([]byte(content))
		} else if strings.Contains(r.URL.Path, "/meta/") {
			_, _ = w.Write(metaJSON)
		}
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	ctx := context.Background()

	storage, err := New(ctx, endpoint, "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	p, m, err := storage.Get(ctx, checksum)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if p.Content != content {
		t.Errorf("expected content %q, got %q", content, p.Content)
	}

	if p.Checksum != checksum {
		t.Errorf("expected checksum %q, got %q", checksum, p.Checksum)
	}

	if m.Checksum != checksum {
		t.Errorf("expected meta checksum %q, got %q", checksum, m.Checksum)
	}
}

func TestGet_DetectsCorruption(t *testing.T) {
	content := "Hello, World!"
	checksum := paste.ComputeChecksum(content)
	corruptedContent := "Corrupted content!"
	meta := paste.Meta{
		Checksum:  checksum,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
		Size:      int64(len(content)),
	}
	metaJSON, _ := json.Marshal(meta)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pastes/") {
			// Return corrupted content
			_, _ = w.Write([]byte(corruptedContent))
		} else if strings.Contains(r.URL.Path, "/meta/") {
			_, _ = w.Write(metaJSON)
		}
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	ctx := context.Background()

	storage, err := New(ctx, endpoint, "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	_, _, err = storage.Get(ctx, checksum)
	if err == nil {
		t.Fatal("expected error for corrupted content")
	}

	if !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("expected ErrChecksumMismatch, got: %v", err)
	}
}

func TestForEachMeta_IteratesAllItems(t *testing.T) {
	metas := []paste.Meta{
		{Checksum: "checksum1", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour), Size: 100},
		{Checksum: "checksum2", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour), Size: 200},
		{Checksum: "checksum3", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour), Size: 300},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test-bucket" && r.URL.Query().Get("list-type") == "2" {
			// List objects response
			response := `<?xml version="1.0" encoding="UTF-8"?>
				<ListBucketResult>
					<Contents><Key>meta/checksum1.json</Key></Contents>
					<Contents><Key>meta/checksum2.json</Key></Contents>
					<Contents><Key>meta/checksum3.json</Key></Contents>
				</ListBucketResult>`
			_, _ = w.Write([]byte(response))
		} else if strings.Contains(r.URL.Path, "/meta/checksum1.json") {
			_ = json.NewEncoder(w).Encode(metas[0])
		} else if strings.Contains(r.URL.Path, "/meta/checksum2.json") {
			_ = json.NewEncoder(w).Encode(metas[1])
		} else if strings.Contains(r.URL.Path, "/meta/checksum3.json") {
			_ = json.NewEncoder(w).Encode(metas[2])
		}
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	ctx := context.Background()

	storage, err := New(ctx, endpoint, "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	var visited []string
	err = storage.ForEachMeta(ctx, func(meta *paste.Meta) error {
		visited = append(visited, meta.Checksum)
		return nil
	})

	if err != nil {
		t.Fatalf("ForEachMeta failed: %v", err)
	}

	if len(visited) != 3 {
		t.Errorf("expected 3 items, got %d: %v", len(visited), visited)
	}
}

func TestForEachMeta_StopsOnCallbackError(t *testing.T) {
	metas := []paste.Meta{
		{Checksum: "checksum1", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour), Size: 100},
		{Checksum: "checksum2", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour), Size: 200},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test-bucket" && r.URL.Query().Get("list-type") == "2" {
			response := `<?xml version="1.0" encoding="UTF-8"?>
				<ListBucketResult>
					<Contents><Key>meta/checksum1.json</Key></Contents>
					<Contents><Key>meta/checksum2.json</Key></Contents>
				</ListBucketResult>`
			_, _ = w.Write([]byte(response))
		} else if strings.Contains(r.URL.Path, "/meta/checksum1.json") {
			_ = json.NewEncoder(w).Encode(metas[0])
		} else if strings.Contains(r.URL.Path, "/meta/checksum2.json") {
			_ = json.NewEncoder(w).Encode(metas[1])
		}
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	ctx := context.Background()

	storage, err := New(ctx, endpoint, "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	expectedErr := errors.New("stop iteration")
	var visited int
	err = storage.ForEachMeta(ctx, func(meta *paste.Meta) error {
		visited++
		return expectedErr // Stop after first item
	})

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected callback error, got: %v", err)
	}

	if visited != 1 {
		t.Errorf("expected 1 visit before stopping, got %d", visited)
	}
}
