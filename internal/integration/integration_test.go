//go:build integration

package integration

import (
	"context"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/espebra/pastebin/internal/cleanup"
	"github.com/espebra/pastebin/internal/config"
	"github.com/espebra/pastebin/internal/handlers"
	"github.com/espebra/pastebin/internal/paste"
	"github.com/espebra/pastebin/internal/storage"
	"github.com/espebra/pastebin/web"
)

// newStorage connects to the S3 server given by the PASTEBIN_TEST_S3_*
// environment variables, using a new bucket so that tests do not see each
// other's pastes. The bucket is created by storage.New.
func newStorage(t *testing.T) *storage.S3Storage {
	t.Helper()
	endpoint := os.Getenv("PASTEBIN_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Fatal("PASTEBIN_TEST_S3_ENDPOINT is not set, see the test-integration target in the Makefile")
	}
	bucket := fmt.Sprintf("pastebin-test-%d", time.Now().UnixNano())

	s, err := storage.New(context.Background(), endpoint, "us-east-1", bucket,
		os.Getenv("PASTEBIN_TEST_S3_ACCESS_KEY"), os.Getenv("PASTEBIN_TEST_S3_SECRET_KEY"), false)
	if err != nil {
		t.Fatalf("failed to connect to S3 at %s: %v", endpoint, err)
	}
	return s
}

func newServer(t *testing.T, s *storage.S3Storage) *httptest.Server {
	t.Helper()
	cfg := &config.Config{MaxPasteSize: 1024 * 1024, DefaultTTL: 24 * time.Hour}
	h, err := handlers.New(cfg, s, web.Templates)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		t.Fatalf("failed to get static fs: %v", err)
	}
	srv := httptest.NewServer(h.RegisterRoutes(http.NewServeMux(), staticFS))
	t.Cleanup(srv.Close)
	return srv
}

type client struct {
	t    *testing.T
	http *http.Client
	base string
}

func newClient(t *testing.T, base string) *client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("failed to create cookie jar: %v", err)
	}
	return &client{t: t, base: base, http: &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (c *client) get(path string) (int, http.Header, string) {
	c.t.Helper()
	resp, err := c.http.Get(c.base + path)
	if err != nil {
		c.t.Fatalf("GET %s: %v", path, err)
	}
	return c.read(resp)
}

// post submits a form the way a browser does: the front page sets the CSRF
// cookie, and the form carries the same token
func (c *client) post(path string, form url.Values) (int, http.Header, string) {
	c.t.Helper()
	c.get("/")
	u, _ := url.Parse(c.base)
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == "csrf_token" {
			form.Set("csrf_token", ck.Value)
		}
	}
	resp, err := c.http.PostForm(c.base+path, form)
	if err != nil {
		c.t.Fatalf("POST %s: %v", path, err)
	}
	return c.read(resp)
}

func (c *client) read(resp *http.Response) (int, http.Header, string) {
	c.t.Helper()
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("failed to read response body: %v", err)
	}
	return resp.StatusCode, resp.Header, string(b)
}

func TestPasteLifecycle(t *testing.T) {
	srv := newServer(t, newStorage(t))
	c := newClient(t, srv.URL)

	content := "#!/bin/sh\necho \"<hello & goodbye>\"\n\n\ttabbed ✓\n"
	checksum := paste.ComputeChecksum(content)

	code, hdr, _ := c.post("/", url.Values{"content": {content}, "ttl": {"1h"}})
	if code != http.StatusSeeOther {
		t.Fatalf("create: expected 303, got %d", code)
	}
	if loc := hdr.Get("Location"); loc != "/"+checksum {
		t.Fatalf("create: expected redirect to /%s, got %q", checksum, loc)
	}

	code, _, body := c.get("/" + checksum)
	if code != http.StatusOK {
		t.Fatalf("view: expected 200, got %d", code)
	}
	if !strings.Contains(body, html.EscapeString(content)) {
		t.Error("view: page does not contain the paste content")
	}

	code, _, body = c.get("/raw/" + checksum)
	if code != http.StatusOK {
		t.Fatalf("raw: expected 200, got %d", code)
	}
	if body != content {
		t.Errorf("raw: content changed in storage, expected %q, got %q", content, body)
	}

	// Saving the same content again is idempotent
	if code, _, _ := c.post("/", url.Values{"content": {content}}); code != http.StatusSeeOther {
		t.Fatalf("create again: expected 303, got %d", code)
	}

	code, hdr, _ = c.post("/delete/"+checksum, url.Values{})
	if code != http.StatusSeeOther || hdr.Get("Location") != "/" {
		t.Fatalf("delete: expected 303 to /, got %d to %q", code, hdr.Get("Location"))
	}

	if code, _, _ := c.get("/" + checksum); code != http.StatusNotFound {
		t.Errorf("view after delete: expected 404, got %d", code)
	}
	if code, _, _ := c.get("/raw/" + checksum); code != http.StatusNotFound {
		t.Errorf("raw after delete: expected 404, got %d", code)
	}
}

func TestExpiredPasteOnAccess(t *testing.T) {
	s := newStorage(t)
	srv := newServer(t, s)
	c := newClient(t, srv.URL)

	p := paste.NewPaste("expired on access")
	meta := paste.NewMeta(p.Checksum, int64(len(p.Content)), time.Hour)
	meta.ExpiresAt = time.Now().Add(-time.Minute)
	if err := s.Store(context.Background(), p, meta); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if code, _, _ := c.get("/" + p.Checksum); code != http.StatusGone {
		t.Errorf("expected 410, got %d", code)
	}
	if _, _, err := s.Get(context.Background(), p.Checksum); err == nil {
		t.Error("expired paste was not deleted from S3")
	}
}

func TestCleanupRemovesExpiredPastes(t *testing.T) {
	s := newStorage(t)
	ctx := context.Background()

	store := func(content string, ttl time.Duration) string {
		p := paste.NewPaste(content)
		meta := paste.NewMeta(p.Checksum, int64(len(content)), time.Hour)
		meta.ExpiresAt = time.Now().Add(ttl)
		if err := s.Store(ctx, p, meta); err != nil {
			t.Fatalf("Store failed: %v", err)
		}
		return p.Checksum
	}
	live := store("still here", time.Hour)
	var expired []string
	for i := range 3 {
		expired = append(expired, store(fmt.Sprintf("expired %d", i), -time.Minute))
	}

	// The cleaner runs once at start, then on every tick
	cleanupCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		cleanup.New(s, time.Hour).Start(cleanupCtx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	deadline := time.Now().Add(10 * time.Second)
	for _, checksum := range expired {
		for {
			if _, _, err := s.Get(ctx, checksum); err != nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("expired paste %s was not removed by the cleaner", checksum)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	if _, _, err := s.Get(ctx, live); err != nil {
		t.Errorf("live paste was removed by the cleaner: %v", err)
	}
}
