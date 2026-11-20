package handlers

import (
	"context"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/espebra/pastebin/internal/config"
	"github.com/espebra/pastebin/internal/paste"
	"github.com/espebra/pastebin/web"
)

func FuzzIsValidChecksum(f *testing.F) {
	// Add seed corpus
	f.Add("")
	f.Add("abc")
	f.Add("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855") // valid SHA256
	f.Add("E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855") // uppercase
	f.Add("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b85")  // 63 chars
	f.Add("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b8555") // 65 chars
	f.Add("zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz") // invalid hex
	f.Add("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b85g") // invalid hex char
	f.Add("../../../../../../../../../../../../../../../../../../../../../..") // path traversal

	f.Fuzz(func(t *testing.T, input string) {
		// The checksum is used in S3 object keys, so it must be exactly 64
		// hex digits
		want := len(input) == checksumLength
		for i := 0; i < len(input) && want; i++ {
			c := input[i]
			want = c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
		}
		if got := isValidChecksum(input); got != want {
			t.Errorf("isValidChecksum(%q) = %v, want %v", input, got, want)
		}
	})
}

// FuzzPasteRoundTrip saves arbitrary content through the HTTP handlers and
// checks that the raw view returns exactly the same bytes, and that the
// paste page never contains the content unescaped
func FuzzPasteRoundTrip(f *testing.F) {
	const maxSize = 64 * 1024

	// Every request writes an access log line, which would flood the
	// output and slow the fuzzer down to a halt
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.DiscardHandler))
	f.Cleanup(func() { slog.SetDefault(orig) })

	store := newMemStorage()
	h, err := New(&config.Config{MaxPasteSize: maxSize, DefaultTTL: time.Hour}, store, web.Templates)
	if err != nil {
		f.Fatalf("failed to create handler: %v", err)
	}
	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		f.Fatalf("failed to get static fs: %v", err)
	}
	routes := h.RegisterRoutes(http.NewServeMux(), staticFS)

	serve := func(r *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, r)
		return rec
	}
	view := func(checksum string) string {
		rec := serve(httptest.NewRequest(http.MethodGet, "/"+checksum, nil))
		if rec.Code != http.StatusOK {
			f.Fatalf("view: expected 200, got %d", rec.Code)
		}
		return rec.Body.String()
	}

	// Paste content is always escaped, so the page has as many tags as the
	// page of a paste without any markup
	plain := paste.NewPaste("plain")
	_ = store.Store(context.Background(), plain, paste.NewMeta(plain.Checksum, 5, time.Hour))
	wantTags := strings.Count(view(plain.Checksum), "<")
	_ = store.Delete(context.Background(), plain.Checksum)

	f.Add("hello")
	f.Add("line one\r\nline two\n")
	f.Add("</textarea><script>alert(1)</script>")
	f.Add("{{.CSRFToken}}")
	f.Add("\x00\xff\xfe invalid utf-8")
	f.Add("  leading and trailing whitespace \n\n")
	f.Add("emoji ✓ 🐹 and combining é")

	f.Fuzz(func(t *testing.T, content string) {
		const token = "fuzz-token"
		form := url.Values{"content": {content}, "ttl": {"1h"}, "csrf_token": {token}}
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
		rec := serve(r)

		if content == "" || len(content) > maxSize {
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("create: expected 400 for content of %d bytes, got %d", len(content), rec.Code)
			}
			return
		}
		checksum := paste.ComputeChecksum(content)
		defer func() { _ = store.Delete(context.Background(), checksum) }()

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("create: expected 303, got %d: %s", rec.Code, rec.Body)
		}
		if loc := rec.Header().Get("Location"); loc != "/"+checksum {
			t.Fatalf("create: expected redirect to /%s, got %q", checksum, loc)
		}

		rec = serve(httptest.NewRequest(http.MethodGet, "/raw/"+checksum, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("raw: expected 200, got %d", rec.Code)
		}
		if got := rec.Body.String(); got != content {
			t.Fatalf("raw: content changed, saved %q, got %q", content, got)
		}

		rec = serve(httptest.NewRequest(http.MethodGet, "/"+checksum, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("view: expected 200, got %d", rec.Code)
		}
		if got := strings.Count(rec.Body.String(), "<"); got != wantTags {
			t.Fatalf("view: page has %d '<', want %d, paste content was not escaped", got, wantTags)
		}
	})
}

// FuzzClientIP checks that X-Forwarded-For is only trusted from trusted
// proxies, for any remote address and header value
func FuzzClientIP(f *testing.F) {
	trusted := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("fd00::/8"),
	}

	f.Add("10.0.0.1:1234", "203.0.113.7", true)
	f.Add("10.0.0.1:1234", "203.0.113.7, 10.0.0.2", true)
	f.Add("10.0.0.1:1234", "garbage, 10.0.0.2", true)
	f.Add("203.0.113.9:1234", "10.0.0.5", true)
	f.Add("[fd00::1]:1234", "::ffff:203.0.113.7", true)
	f.Add("10.0.0.1:1234", "203.0.113.7", false)
	f.Add("10.0.0.1", "", true)
	f.Add("", ",,,", true)
	f.Add("not an address", "203.0.113.7", true)

	f.Fuzz(func(t *testing.T, remoteAddr, xff string, useTrusted bool) {
		var proxies []netip.Prefix
		if useTrusted {
			proxies = trusted
		}
		r := &http.Request{RemoteAddr: remoteAddr, Header: http.Header{"X-Forwarded-For": {xff}}}

		got := clientIP(r, proxies)

		host, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			host = remoteAddr
		}
		if got == host {
			return
		}
		// The header is only used when the request comes from a trusted
		// proxy, and then yields a parseable address
		addr, err := netip.ParseAddr(host)
		if err != nil || !isTrusted(addr, proxies) {
			t.Fatalf("clientIP = %q, but the request came directly from untrusted %q", got, remoteAddr)
		}
		if !containsAddr(xff, got) {
			t.Fatalf("clientIP = %q, which is neither the peer %q nor an address in X-Forwarded-For %q", got, remoteAddr, xff)
		}
	})
}

// containsAddr reports whether any entry of an X-Forwarded-For value parses
// to the given address
func containsAddr(xff, want string) bool {
	w, err := netip.ParseAddr(want)
	if err != nil {
		return false
	}
	for _, hop := range strings.Split(xff, ",") {
		if a, err := netip.ParseAddr(strings.TrimSpace(hop)); err == nil && a.Unmap() == w.Unmap() {
			return true
		}
	}
	return false
}
