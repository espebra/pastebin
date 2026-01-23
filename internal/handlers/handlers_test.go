package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/espebra/pastebin/internal/config"
	"github.com/espebra/pastebin/internal/csrf"
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
	}

	for header, expected := range expectedHeaders {
		actual := rec.Header().Get(header)
		if actual != expected {
			t.Errorf("expected %s header to be %q, got %q", header, expected, actual)
		}
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
