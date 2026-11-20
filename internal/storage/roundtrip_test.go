package storage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/espebra/pastebin/internal/paste"
)

// fakeS3 is a minimal in-memory S3 server supporting the bucket and object
// operations used by S3Storage, with path-style addressing
type fakeS3 struct {
	mu      sync.Mutex
	buckets map[string]bool
	objects map[string][]byte // keyed by "bucket/key"
}

func newFakeS3(t *testing.T, buckets ...string) (*fakeS3, string) {
	t.Helper()
	f := &fakeS3{buckets: make(map[string]bool), objects: make(map[string][]byte)}
	for _, b := range buckets {
		f.buckets[b] = true
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, strings.TrimPrefix(srv.URL, "http://")
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if key == "" {
		switch r.Method {
		case http.MethodHead:
			if !f.buckets[bucket] {
				w.WriteHeader(http.StatusNotFound)
			}
		case http.MethodPut:
			f.buckets[bucket] = true
		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
		return
	}

	if !f.buckets[bucket] {
		s3Error(w, http.StatusNotFound, "NoSuchBucket")
		return
	}
	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		f.objects[bucket+"/"+key] = body
	case http.MethodGet:
		body, ok := f.objects[bucket+"/"+key]
		if !ok {
			s3Error(w, http.StatusNotFound, "NoSuchKey")
			return
		}
		_, _ = w.Write(body)
	case http.MethodDelete:
		delete(f.objects, bucket+"/"+key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func s3Error(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>`+code+`</Code></Error>`)
}

func (f *fakeS3) object(name string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[name]
	return b, ok
}

func (f *fakeS3) hasBucket(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buckets[name]
}

func TestNew_CreatesMissingBucket(t *testing.T) {
	fake, endpoint := newFakeS3(t)

	if _, err := New(context.Background(), endpoint, "us-east-1", "new-bucket", "key", "secret", false); err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if !fake.hasBucket("new-bucket") {
		t.Error("expected the bucket to be created")
	}
}

func TestNew_BucketCheckError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := New(context.Background(), strings.TrimPrefix(srv.URL, "http://"), "us-east-1", "b", "key", "secret", false)
	if err == nil {
		t.Fatal("expected an error when the bucket cannot be checked")
	}
}

func TestStore_RoundTrip(t *testing.T) {
	fake, endpoint := newFakeS3(t, "test-bucket")
	ctx := context.Background()
	s, err := New(ctx, endpoint, "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	content := "line one\nline two\n"
	p := paste.NewPaste(content)
	meta := paste.NewMeta(p.Checksum, int64(len(content)), time.Hour)
	if err := s.Store(ctx, p, meta); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// The content and metadata are stored under the SHA256 of the content
	stored, ok := fake.object("test-bucket/pastes/" + p.Checksum)
	if !ok {
		t.Fatal("paste object was not stored")
	}
	if string(stored) != content {
		t.Errorf("stored content %q, expected %q", stored, content)
	}
	rawMeta, ok := fake.object("test-bucket/meta/" + p.Checksum + ".json")
	if !ok {
		t.Fatal("metadata object was not stored")
	}
	var storedMeta paste.Meta
	if err := json.Unmarshal(rawMeta, &storedMeta); err != nil {
		t.Fatalf("stored metadata is not valid JSON: %v", err)
	}
	if storedMeta.Checksum != p.Checksum || storedMeta.Size != int64(len(content)) {
		t.Errorf("unexpected stored metadata: %+v", storedMeta)
	}

	// Get returns what was stored
	gotPaste, gotMeta, err := s.Get(ctx, p.Checksum)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if gotPaste.Content != content {
		t.Errorf("Get returned content %q, expected %q", gotPaste.Content, content)
	}
	if !gotMeta.ExpiresAt.Equal(meta.ExpiresAt) {
		t.Errorf("Get returned expiry %v, expected %v", gotMeta.ExpiresAt, meta.ExpiresAt)
	}

	// Delete removes both objects, after which Get fails
	if err := s.Delete(ctx, p.Checksum); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, _, err := s.Get(ctx, p.Checksum); err == nil {
		t.Error("expected Get to fail after Delete")
	}
}

func TestStore_Error(t *testing.T) {
	fake, endpoint := newFakeS3(t, "test-bucket")
	ctx := context.Background()
	s, err := New(ctx, endpoint, "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Removing the bucket makes every object write fail
	fake.mu.Lock()
	delete(fake.buckets, "test-bucket")
	fake.mu.Unlock()

	p := paste.NewPaste("hello")
	if err := s.Store(ctx, p, paste.NewMeta(p.Checksum, 5, time.Hour)); err == nil {
		t.Fatal("expected Store to fail")
	}
}
