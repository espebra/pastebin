package handlers

import (
	"context"
	"errors"
	"html"
	"io"
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/espebra/pastebin/internal/config"
	"github.com/espebra/pastebin/internal/paste"
	"github.com/espebra/pastebin/web"
)

var errFakeStorage = errors.New("fake storage failure")

// memStorage is an in-memory Storage. The err fields make the corresponding
// method fail, to exercise the error paths of the handlers.
type memStorage struct {
	mu        sync.Mutex
	pastes    map[string]*paste.Paste
	metas     map[string]*paste.Meta
	storeErr  error
	getErr    error
	deleteErr error
	deletes   []string
}

func newMemStorage() *memStorage {
	return &memStorage{
		pastes: make(map[string]*paste.Paste),
		metas:  make(map[string]*paste.Meta),
	}
}

func (m *memStorage) Store(_ context.Context, p *paste.Paste, meta *paste.Meta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.storeErr != nil {
		return m.storeErr
	}
	m.pastes[p.Checksum] = p
	m.metas[p.Checksum] = meta
	return nil
}

func (m *memStorage) Get(_ context.Context, checksum string) (*paste.Paste, *paste.Meta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, nil, m.getErr
	}
	p, ok := m.pastes[checksum]
	if !ok {
		return nil, nil, errors.New("not found")
	}
	return p, m.metas[checksum], nil
}

func (m *memStorage) Delete(_ context.Context, checksum string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deletes = append(m.deletes, checksum)
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.pastes, checksum)
	delete(m.metas, checksum)
	return nil
}

func (m *memStorage) has(checksum string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.pastes[checksum]
	return ok
}

func (m *memStorage) meta(checksum string) *paste.Meta {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.metas[checksum]
}

func (m *memStorage) deleteCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.deletes...)
}

// put adds a paste that expires after ttl, which may be negative
func (m *memStorage) put(content string, ttl time.Duration) string {
	p := paste.NewPaste(content)
	meta := paste.NewMeta(p.Checksum, int64(len(content)), time.Hour)
	meta.ExpiresAt = time.Now().Add(ttl)
	m.pastes[p.Checksum] = p
	m.metas[p.Checksum] = meta
	return p.Checksum
}

// newTestServer serves the full route table, with the real templates and
// static files, backed by the given storage
func newTestServer(t *testing.T, store Storage) *httptest.Server {
	t.Helper()
	cfg := &config.Config{
		MaxPasteSize: 1024,
		DefaultTTL:   24 * time.Hour,
	}
	h, err := New(cfg, store, web.Templates)
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

// newClient returns a client that keeps cookies and does not follow
// redirects, so that tests can check the redirect target
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("failed to create cookie jar: %v", err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// get performs a GET request and returns the response and its body
func get(t *testing.T, c *http.Client, u string) (*http.Response, string) {
	t.Helper()
	resp, err := c.Get(u)
	if err != nil {
		t.Fatalf("GET %s: %v", u, err)
	}
	return resp, readBody(t, resp)
}

// postForm fetches a CSRF token from the front page, then posts the form
// with it
func postForm(t *testing.T, c *http.Client, srvURL, path string, form url.Values) (*http.Response, string) {
	t.Helper()
	get(t, c, srvURL+"/")
	base, _ := url.Parse(srvURL)
	for _, ck := range c.Jar.Cookies(base) {
		if ck.Name == "csrf_token" {
			form.Set("csrf_token", ck.Value)
		}
	}
	if form.Get("csrf_token") == "" {
		t.Fatal("front page did not set a CSRF cookie")
	}
	resp, err := c.PostForm(srvURL+path, form)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp, readBody(t, resp)
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	return string(b)
}

func TestPasteLifecycle(t *testing.T) {
	store := newMemStorage()
	srv := newTestServer(t, store)
	c := newClient(t)

	content := "package main\n\nfunc main() {}\n"
	checksum := paste.ComputeChecksum(content)

	// Create
	resp, _ := postForm(t, c, srv.URL, "/", url.Values{"content": {content}, "ttl": {"1h"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create: expected 303, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/"+checksum {
		t.Fatalf("create: expected redirect to /%s, got %q", checksum, loc)
	}
	if !store.has(checksum) {
		t.Fatal("create: paste was not stored")
	}

	// View
	resp, body := get(t, c, srv.URL+"/"+checksum)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("view: expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, html.EscapeString(content)) {
		t.Error("view: page does not contain the paste content")
	}
	if !strings.Contains(body, `action="/delete/`+checksum+`"`) {
		t.Error("view: page does not contain the delete form")
	}

	// Raw
	resp, body = get(t, c, srv.URL+"/raw/"+checksum)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("raw: expected 200, got %d", resp.StatusCode)
	}
	if body != content {
		t.Errorf("raw: expected %q, got %q", content, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("raw: expected text/plain content type, got %q", ct)
	}

	// Delete
	resp, _ = postForm(t, c, srv.URL, "/delete/"+checksum, url.Values{})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete: expected 303, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/" {
		t.Errorf("delete: expected redirect to /, got %q", loc)
	}
	if store.has(checksum) {
		t.Error("delete: paste is still stored")
	}

	// Gone
	resp, _ = get(t, c, srv.URL+"/"+checksum)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("view after delete: expected 404, got %d", resp.StatusCode)
	}
}

func TestHandleCreate_TTL(t *testing.T) {
	tests := []struct {
		name string
		ttl  string
		want time.Duration
	}{
		{"valid", "1h", time.Hour},
		{"empty uses default", "", 24 * time.Hour},
		{"invalid uses default", "forever", 24 * time.Hour},
		{"negative uses default", "-1h", 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemStorage()
			srv := newTestServer(t, store)
			c := newClient(t)

			before := time.Now()
			resp, _ := postForm(t, c, srv.URL, "/", url.Values{"content": {"hello"}, "ttl": {tt.ttl}})
			if resp.StatusCode != http.StatusSeeOther {
				t.Fatalf("expected 303, got %d", resp.StatusCode)
			}

			meta := store.meta(paste.ComputeChecksum("hello"))
			if meta == nil {
				t.Fatal("paste was not stored")
			}
			if meta.Size != 5 {
				t.Errorf("expected size 5, got %d", meta.Size)
			}
			got := meta.ExpiresAt.Sub(before)
			if got < tt.want || got > tt.want+time.Minute {
				t.Errorf("expected expiry about %s from now, got %s", tt.want, got)
			}
		})
	}
}

func TestHandleCreate_StorageError(t *testing.T) {
	store := newMemStorage()
	store.storeErr = errFakeStorage
	srv := newTestServer(t, store)

	resp, _ := postForm(t, newClient(t), srv.URL, "/", url.Values{"content": {"hello"}})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestHandleView_NotFound(t *testing.T) {
	srv := newTestServer(t, newMemStorage())

	resp, body := get(t, newClient(t), srv.URL+"/"+paste.ComputeChecksum("missing"))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Paste not found or has expired") {
		t.Error("expected not found message")
	}
}

func TestHandleView_Expired(t *testing.T) {
	store := newMemStorage()
	checksum := store.put("old news", -time.Minute)
	srv := newTestServer(t, store)

	resp, body := get(t, newClient(t), srv.URL+"/"+checksum)
	if resp.StatusCode != http.StatusGone {
		t.Errorf("expected 410, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Paste has expired") {
		t.Error("expected expired message")
	}
	if strings.Contains(body, "old news") {
		t.Error("expired paste content was shown")
	}
	if store.has(checksum) {
		t.Error("expired paste was not deleted")
	}
}

func TestHandleView_EscapesContent(t *testing.T) {
	store := newMemStorage()
	checksum := store.put("</textarea><script>alert(1)</script>", time.Hour)
	srv := newTestServer(t, store)

	_, body := get(t, newClient(t), srv.URL+"/"+checksum)
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("paste content was not HTML escaped")
	}
	if !strings.Contains(body, "&lt;/textarea&gt;&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Error("expected escaped paste content in the page")
	}
}

func TestHandleRaw_NotFound(t *testing.T) {
	srv := newTestServer(t, newMemStorage())

	resp, _ := get(t, newClient(t), srv.URL+"/raw/"+paste.ComputeChecksum("missing"))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandleRaw_Expired(t *testing.T) {
	store := newMemStorage()
	checksum := store.put("old news", -time.Minute)
	srv := newTestServer(t, store)

	resp, body := get(t, newClient(t), srv.URL+"/raw/"+checksum)
	if resp.StatusCode != http.StatusGone {
		t.Errorf("expected 410, got %d", resp.StatusCode)
	}
	if strings.Contains(body, "old news") {
		t.Error("expired paste content was returned")
	}
	if store.has(checksum) {
		t.Error("expired paste was not deleted")
	}
}

func TestHandleDelete_NotFound(t *testing.T) {
	store := newMemStorage()
	srv := newTestServer(t, store)

	resp, _ := postForm(t, newClient(t), srv.URL, "/delete/"+paste.ComputeChecksum("missing"), url.Values{})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	if calls := store.deleteCalls(); len(calls) != 0 {
		t.Errorf("expected no delete calls, got %v", calls)
	}
}

func TestHandleDelete_StorageError(t *testing.T) {
	store := newMemStorage()
	checksum := store.put("hello", time.Hour)
	store.deleteErr = errFakeStorage
	srv := newTestServer(t, store)

	resp, _ := postForm(t, newClient(t), srv.URL, "/delete/"+checksum, url.Values{})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestHandleDelete_WithoutCSRFCookie(t *testing.T) {
	store := newMemStorage()
	checksum := store.put("hello", time.Hour)
	srv := newTestServer(t, store)

	// A cross-site form post carries no CSRF cookie
	resp, err := newClient(t).PostForm(srv.URL+"/delete/"+checksum, url.Values{"csrf_token": {"forged"}})
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
	if !store.has(checksum) {
		t.Error("paste was deleted without a valid CSRF token")
	}
}

// TestRealTemplates renders every page with the embedded templates, so that
// a template referring to a missing field fails here rather than in
// production
func TestRealTemplates(t *testing.T) {
	store := newMemStorage()
	checksum := store.put("hello", time.Hour)
	expired := store.put("expired", -time.Minute)
	srv := newTestServer(t, store)
	c := newClient(t)

	pages := []struct {
		path string
		code int
		want string
	}{
		{"/", http.StatusOK, `name="content"`},
		{"/" + checksum, http.StatusOK, checksum[:12]},
		{"/" + paste.ComputeChecksum("missing"), http.StatusNotFound, "Paste not found or has expired"},
		{"/" + expired, http.StatusGone, "Paste has expired"},
	}
	for _, p := range pages {
		resp, body := get(t, c, srv.URL+p.path)
		if resp.StatusCode != p.code {
			t.Errorf("%s: expected %d, got %d", p.path, p.code, resp.StatusCode)
		}
		if !strings.Contains(body, p.want) {
			t.Errorf("%s: expected page to contain %q", p.path, p.want)
		}
		if !strings.HasSuffix(strings.TrimSpace(body), "</html>") {
			t.Errorf("%s: page was cut short, the template likely failed", p.path)
		}
	}
}
