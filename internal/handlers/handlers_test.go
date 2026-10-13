package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/espebra/pastebin/internal/config"
	"github.com/espebra/pastebin/internal/csrf"
	"github.com/espebra/pastebin/internal/metrics"
	"github.com/espebra/pastebin/internal/requestid"
	"github.com/espebra/pastebin/internal/storage"
)

// mockTemplateFS creates a minimal template filesystem for testing
func mockTemplateFS() fstest.MapFS {
	return fstest.MapFS{
		"templates/index.html": &fstest.MapFile{
			Data: []byte(`<!DOCTYPE html><html><body>Index {{.CSRFToken}}</body></html>`),
		},
		"templates/paste.html": &fstest.MapFile{
			Data: []byte(`<!DOCTYPE html><html><body>{{if .Error}}Error: {{.Error}}{{else}}Paste: {{.Checksum}}{{end}}</body></html>`),
		},
	}
}

// addCSRFToken adds a valid CSRF token to the request (cookie and form value)
func addCSRFToken(req *http.Request, form url.Values) {
	token, _ := csrf.GenerateToken()
	req.AddCookie(&http.Cookie{
		Name:  "csrf_token",
		Value: token,
	})
	if form != nil {
		form.Set("csrf_token", token)
	}
}

func TestNew(t *testing.T) {
	cfg := &config.Config{
		DefaultTTL: 24 * time.Hour,
	}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h == nil {
		t.Fatal("expected handler to be non-nil")
	}

	if h.templates == nil {
		t.Fatal("expected templates to be parsed")
	}
}

func TestNew_InvalidTemplate(t *testing.T) {
	cfg := &config.Config{}

	invalidFS := fstest.MapFS{
		"templates/index.html": &fstest.MapFile{
			Data: []byte(`{{invalid template`),
		},
	}

	_, err := New(cfg, nil, invalidFS)
	if err == nil {
		t.Error("expected error for invalid template")
	}
}

func TestHandleIndex(t *testing.T) {
	cfg := &config.Config{
		DefaultTTL: 24 * time.Hour,
	}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.handleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Index") {
		t.Error("expected index template to be rendered")
	}
}

func TestHandleCreate_EmptyContent(t *testing.T) {
	cfg := &config.Config{
		MaxPasteSize: 1024,
		DefaultTTL:   24 * time.Hour,
	}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	form := url.Values{}
	form.Add("content", "")

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	addCSRFToken(req, form)
	req.Body = io.NopCloser(strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.handleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty content, got %d", rec.Code)
	}
}

func TestHandleCreate_ContentTooLarge(t *testing.T) {
	cfg := &config.Config{
		MaxPasteSize: 10, // Very small limit
		DefaultTTL:   24 * time.Hour,
	}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	form := url.Values{}
	form.Add("content", "This content is way too large for the limit")

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	addCSRFToken(req, form)
	req.Body = io.NopCloser(strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.handleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for oversized content, got %d", rec.Code)
	}

	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "exceeds maximum size") {
		t.Error("expected error message about size limit")
	}
}

func TestHandleView_EmptyChecksum(t *testing.T) {
	cfg := &config.Config{}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("checksum", "")
	rec := httptest.NewRecorder()

	h.handleView(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleDelete_EmptyChecksum(t *testing.T) {
	cfg := &config.Config{}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	form := url.Values{}
	req := httptest.NewRequest(http.MethodPost, "/delete/", nil)
	addCSRFToken(req, form)
	req.Body = io.NopCloser(strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("checksum", "")
	rec := httptest.NewRecorder()

	h.handleDelete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleRaw_EmptyChecksum(t *testing.T) {
	cfg := &config.Config{}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/raw/", nil)
	req.SetPathValue("checksum", "")
	rec := httptest.NewRecorder()

	h.handleRaw(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleCreate_InvalidCSRFToken(t *testing.T) {
	cfg := &config.Config{
		MaxPasteSize: 1024,
		DefaultTTL:   24 * time.Hour,
	}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	form := url.Values{}
	form.Add("content", "test content")
	form.Add("csrf_token", "invalid-token")

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{
		Name:  "csrf_token",
		Value: "different-token",
	})
	rec := httptest.NewRecorder()

	h.handleCreate(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403 for invalid CSRF token, got %d", rec.Code)
	}
}

func TestHandleDelete_InvalidCSRFToken(t *testing.T) {
	cfg := &config.Config{}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	form := url.Values{}
	form.Add("csrf_token", "invalid-token")

	req := httptest.NewRequest(http.MethodPost, "/delete/abc123", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{
		Name:  "csrf_token",
		Value: "different-token",
	})
	req.SetPathValue("checksum", "abc123")
	rec := httptest.NewRecorder()

	h.handleDelete(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403 for invalid CSRF token, got %d", rec.Code)
	}
}

func TestIsValidChecksum(t *testing.T) {
	tests := []struct {
		name     string
		checksum string
		valid    bool
	}{
		{
			name:     "valid SHA256",
			checksum: "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f",
			valid:    true,
		},
		{
			name:     "valid SHA256 uppercase",
			checksum: "DFFD6021BB2BD5B0AF676290809EC3A53191DD81C7F70A4B28688A362182986F",
			valid:    true,
		},
		{
			name:     "too short",
			checksum: "abc123",
			valid:    false,
		},
		{
			name:     "too long",
			checksum: "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f00",
			valid:    false,
		},
		{
			name:     "invalid characters",
			checksum: "zzzz6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f",
			valid:    false,
		},
		{
			name:     "empty string",
			checksum: "",
			valid:    false,
		},
		{
			name:     "spaces",
			checksum: "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986 ",
			valid:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidChecksum(tt.checksum); got != tt.valid {
				t.Errorf("isValidChecksum(%q) = %v, want %v", tt.checksum, got, tt.valid)
			}
		})
	}
}

func TestHandleView_InvalidChecksum(t *testing.T) {
	cfg := &config.Config{}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	invalidChecksums := []string{
		"abc123",                 // too short
		"not-a-valid-hex-string", // invalid characters
		"dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f00", // too long
	}

	for _, checksum := range invalidChecksums {
		t.Run(checksum, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/"+checksum, nil)
			req.SetPathValue("checksum", checksum)
			rec := httptest.NewRecorder()

			h.handleView(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Errorf("expected status 404 for invalid checksum %q, got %d", checksum, rec.Code)
			}
		})
	}
}

func TestHandleRaw_InvalidChecksum(t *testing.T) {
	cfg := &config.Config{}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/raw/invalid", nil)
	req.SetPathValue("checksum", "invalid-checksum-format")
	rec := httptest.NewRecorder()

	h.handleRaw(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for invalid checksum, got %d", rec.Code)
	}
}

func TestHandleDelete_InvalidChecksum(t *testing.T) {
	cfg := &config.Config{}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	form := url.Values{}
	req := httptest.NewRequest(http.MethodPost, "/delete/invalid", nil)
	addCSRFToken(req, form)
	req.Body = io.NopCloser(strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("checksum", "invalid-checksum-format")
	rec := httptest.NewRecorder()

	h.handleDelete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for invalid checksum, got %d", rec.Code)
	}
}

func TestHandleHealth(t *testing.T) {
	cfg := &config.Config{}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body != "OK" {
		t.Errorf("expected body 'OK', got %q", body)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("expected Content-Type 'text/plain; charset=utf-8', got %q", contentType)
	}
}

func TestHandleRobots(t *testing.T) {
	cfg := &config.Config{}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()

	h.handleRobots(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body != "User-agent: *\nDisallow: /\n" {
		t.Errorf("unexpected robots.txt body: %q", body)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("expected Content-Type 'text/plain; charset=utf-8', got %q", contentType)
	}
}

func TestFavicon(t *testing.T) {
	cfg := &config.Config{}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	icon := []byte("\x00\x00\x01\x00fake-icon")
	staticFS := fstest.MapFS{
		"favicon.ico": &fstest.MapFile{Data: icon},
	}

	mux := http.NewServeMux()
	wrapped := h.RegisterRoutes(mux, staticFS)

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if rec.Body.String() != string(icon) {
		t.Errorf("unexpected favicon body: %q", rec.Body.String())
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "image/vnd.microsoft.icon" && contentType != "image/x-icon" {
		t.Errorf("expected icon Content-Type, got %q", contentType)
	}

	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=86400" {
		t.Errorf("expected Cache-Control 'public, max-age=86400', got %q", cc)
	}
}

func TestSecurityHeaders(t *testing.T) {
	// Create a simple handler to wrap
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := securityHeaders(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	expectedHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"X-Robots-Tag":           "noindex, nofollow, noarchive, nosnippet",
	}

	for header, expected := range expectedHeaders {
		actual := rec.Header().Get(header)
		if actual != expected {
			t.Errorf("expected %s header to be %q, got %q", header, expected, actual)
		}
	}
}

// captureLogs redirects the default logger to a buffer for the duration of
// the test, using the same request ID log handler as the application
func captureLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := slog.Default()
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(requestid.NewLogHandler(handler)))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return &buf
}

func parseLogLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("failed to parse log entry %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func TestAccessLog(t *testing.T) {
	buf := captureLogs(t, slog.LevelInfo)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		slog.InfoContext(r.Context(), "inner")
		w.WriteHeader(http.StatusNotFound)
		w.WriteHeader(http.StatusInternalServerError) // ignored by net/http
		_, _ = w.Write([]byte("not found"))
	})

	req := httptest.NewRequest(http.MethodPost, "/abc?x=1", strings.NewReader("content=hello"))
	req.RemoteAddr = "192.0.2.1:1234"
	req.Header.Set("User-Agent", "test-agent")
	req.Header.Set("Referer", "http://example.com/")
	rec := httptest.NewRecorder()
	accessLog(nil, inner).ServeHTTP(rec, req)

	entries := parseLogLines(t, buf)
	if len(entries) != 2 {
		t.Fatalf("expected 2 log entries, got %d: %s", len(entries), buf.String())
	}
	innerEntry, entry := entries[0], entries[1]

	id := rec.Header().Get(requestid.Header)
	if !requestid.Valid(id) {
		t.Fatalf("expected a valid request ID response header, got %q", id)
	}

	expected := map[string]any{
		"level":      "INFO",
		"msg":        "request",
		"method":     "POST",
		"path":       "/abc?x=1",
		"proto":      "HTTP/1.1",
		"status":     float64(http.StatusNotFound),
		"bytes_in":   float64(len("content=hello")),
		"bytes_out":  float64(len("not found")),
		"client_ip":  "192.0.2.1",
		"user_agent": "test-agent",
		"referer":    "http://example.com/",
		"request_id": id,
	}
	for key, want := range expected {
		if got := entry[key]; got != want {
			t.Errorf("expected %s to be %v, got %v", key, want, got)
		}
	}
	if _, ok := entry["duration_ms"]; !ok {
		t.Error("expected duration_ms field")
	}
	if innerEntry["request_id"] != id {
		t.Errorf("expected handler log entry to carry request ID %s, got %v", id, innerEntry["request_id"])
	}
}

func TestAccessLog_IncomingRequestID(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		keep     bool
	}{
		{"valid", "abc-123", true},
		{"invalid", "abc 123\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureLogs(t, slog.LevelInfo)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(requestid.Header, tt.incoming)
			rec := httptest.NewRecorder()
			accessLog(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req)

			id := rec.Header().Get(requestid.Header)
			if tt.keep && id != tt.incoming {
				t.Errorf("expected incoming request ID %q to be kept, got %q", tt.incoming, id)
			}
			if !tt.keep && (id == tt.incoming || !requestid.Valid(id)) {
				t.Errorf("expected incoming request ID %q to be replaced, got %q", tt.incoming, id)
			}
			if got := parseLogLines(t, buf)[0]["request_id"]; got != id {
				t.Errorf("expected logged request ID %q, got %v", id, got)
			}
		})
	}
}

func TestAccessLog_Panic(t *testing.T) {
	buf := captureLogs(t, slog.LevelInfo)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	func() {
		defer func() {
			if p := recover(); p != "boom" {
				t.Errorf("expected panic to be re-raised, got %v", p)
			}
		}()
		accessLog(nil, inner).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}()

	entries := parseLogLines(t, buf)
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0]["level"] != "ERROR" || entries[0]["status"] != float64(http.StatusInternalServerError) || entries[0]["panic"] != "boom" {
		t.Errorf("unexpected log entry for panic: %v", entries[0])
	}
}

func TestAccessLog_HealthAtDebug(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	buf := captureLogs(t, slog.LevelInfo)
	accessLog(nil, inner).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	if buf.Len() != 0 {
		t.Errorf("expected no log entry at info level, got %q", buf.String())
	}

	buf = captureLogs(t, slog.LevelDebug)
	accessLog(nil, inner).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	entries := parseLogLines(t, buf)
	if len(entries) != 1 || entries[0]["level"] != "DEBUG" {
		t.Errorf("expected one debug log entry, got %q", buf.String())
	}
}

func TestAccessLog_S3RequestIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-amz-request-id", "s3-"+r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	s, err := storage.New(context.Background(), strings.TrimPrefix(server.URL, "http://"), "us-east-1", "test-bucket", "key", "secret", false)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	buf := captureLogs(t, slog.LevelInfo)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = s.Delete(r.Context(), "abc123")
	})
	accessLog(nil, inner).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/delete/abc123", nil))

	entries := parseLogLines(t, buf)
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	ids, _ := entries[0]["s3_request_ids"].([]any)
	if len(ids) != 2 || ids[0] != "s3-DELETE" || ids[1] != "s3-DELETE" {
		t.Errorf("expected two S3 request IDs, got %v", entries[0]["s3_request_ids"])
	}
}

func TestAccessLog_NoS3RequestIDs(t *testing.T) {
	buf := captureLogs(t, slog.LevelInfo)
	accessLog(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	entries := parseLogLines(t, buf)
	if _, ok := entries[0]["s3_request_ids"]; ok {
		t.Errorf("expected no s3_request_ids attribute, got %v", entries[0]["s3_request_ids"])
	}
}

func TestRouteLabel(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{"", "other"},
		{"GET /{checksum}", "/{checksum}"},
		{"GET /{$}", "/{$}"},
		{"/static/", "/static/"},
		{"GET example.com/health", "/health"},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Pattern = tt.pattern
		if got := routeLabel(r); got != tt.want {
			t.Errorf("routeLabel(%q) = %q, want %q", tt.pattern, got, tt.want)
		}
	}
}

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("failed to read counter: %v", err)
	}
	return m.GetCounter().GetValue()
}

func TestAccessLog_HTTPMetrics(t *testing.T) {
	captureLogs(t, slog.LevelInfo)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{checksum}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	handler := accessLog(nil, mux)

	matched := metrics.HTTPRequests.WithLabelValues(http.MethodGet, "/{checksum}", "404")
	unmatched := metrics.HTTPRequests.WithLabelValues(http.MethodPost, "other", "405")
	beforeMatched, beforeUnmatched := counterValue(t, matched), counterValue(t, unmatched)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/abc123", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/def456", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/abc123", nil))

	if got := counterValue(t, matched) - beforeMatched; got != 2 {
		t.Errorf("expected 2 requests counted for /{checksum}, got %v", got)
	}
	if got := counterValue(t, unmatched) - beforeUnmatched; got != 1 {
		t.Errorf("expected 1 unmatched request counted, got %v", got)
	}
}

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	}

	tests := []struct {
		name       string
		remoteAddr string
		xff        []string
		want       string
	}{
		{"direct client", "192.0.2.1:1234", nil, "192.0.2.1"},
		{"direct client ignores XFF", "192.0.2.1:1234", []string{"198.51.100.1"}, "192.0.2.1"},
		{"ipv6 client", "[2001:db8::1]:1234", nil, "2001:db8::1"},
		{"trusted proxy without XFF", "10.0.0.1:1234", nil, "10.0.0.1"},
		{"trusted proxy", "10.0.0.1:1234", []string{"198.51.100.1"}, "198.51.100.1"},
		{"spoofed XFF", "10.0.0.1:1234", []string{"203.0.113.9, 198.51.100.1"}, "198.51.100.1"},
		{"proxy chain", "10.0.0.1:1234", []string{"198.51.100.1, 10.0.0.2"}, "198.51.100.1"},
		{"multiple headers", "10.0.0.1:1234", []string{"198.51.100.1", "10.0.0.2"}, "198.51.100.1"},
		{"all trusted", "10.0.0.1:1234", []string{"10.0.0.3, 10.0.0.2"}, "10.0.0.3"},
		{"garbage entry", "10.0.0.1:1234", []string{"garbage, 10.0.0.2"}, "10.0.0.2"},
		{"ipv6 trusted proxy", "[::1]:1234", []string{"198.51.100.1"}, "198.51.100.1"},
		{"no port", "192.0.2.1", nil, "192.0.2.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for _, v := range tt.xff {
				req.Header.Add("X-Forwarded-For", v)
			}
			if got := clientIP(req, trusted); got != tt.want {
				t.Errorf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestTTLValidation(t *testing.T) {
	tests := []struct {
		name        string
		ttlStr      string
		defaultTTL  time.Duration
		expectedTTL time.Duration
	}{
		{
			name:        "negative TTL falls back to default",
			ttlStr:      "-1h",
			defaultTTL:  24 * time.Hour,
			expectedTTL: 24 * time.Hour,
		},
		{
			name:        "zero TTL falls back to default",
			ttlStr:      "0s",
			defaultTTL:  24 * time.Hour,
			expectedTTL: 24 * time.Hour,
		},
		{
			name:        "valid TTL is used",
			ttlStr:      "48h",
			defaultTTL:  24 * time.Hour,
			expectedTTL: 48 * time.Hour,
		},
		{
			name:        "empty TTL uses default",
			ttlStr:      "",
			defaultTTL:  24 * time.Hour,
			expectedTTL: 24 * time.Hour,
		},
		{
			name:        "invalid TTL uses default",
			ttlStr:      "not-a-duration",
			defaultTTL:  24 * time.Hour,
			expectedTTL: 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the TTL parsing logic directly
			ttl := tt.defaultTTL
			if tt.ttlStr != "" {
				if parsed, err := time.ParseDuration(tt.ttlStr); err == nil && parsed > 0 {
					ttl = parsed
				}
			}
			if ttl <= 0 {
				ttl = tt.defaultTTL
				if ttl <= 0 {
					ttl = 24 * time.Hour
				}
			}

			if ttl != tt.expectedTTL {
				t.Errorf("expected TTL %v, got %v", tt.expectedTTL, ttl)
			}
		})
	}
}

func TestRegisterRoutes_ReturnsWrappedHandler(t *testing.T) {
	cfg := &config.Config{
		DefaultTTL: 24 * time.Hour,
	}

	h, err := New(cfg, nil, mockTemplateFS())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	staticFS := fstest.MapFS{
		"test.css": &fstest.MapFile{Data: []byte("body {}")},
	}

	mux := http.NewServeMux()
	wrapped := h.RegisterRoutes(mux, staticFS)

	// Test that the wrapped handler includes security headers
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Verify security headers are present
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected X-Content-Type-Options header to be set")
	}

	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("expected X-Frame-Options header to be set")
	}
}
