package handlers

import (
	"context"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/espebra/pastebin/internal/config"
	"github.com/espebra/pastebin/internal/csrf"
	"github.com/espebra/pastebin/internal/metrics"
	"github.com/espebra/pastebin/internal/paste"
	"github.com/espebra/pastebin/internal/requestid"
	"github.com/espebra/pastebin/internal/storage"
	"github.com/espebra/pastebin/internal/version"
)

const (
	checksumLength = 64 // SHA256 produces 32 bytes = 64 hex characters
	// maxRequestBodySize is the maximum size for request bodies (form data)
	// This is separate from MaxPasteSize and includes form overhead
	maxRequestBodySize = 10 * 1024 * 1024 // 10MB
)

// isValidChecksum validates that a checksum is a valid SHA256 hex string
func isValidChecksum(checksum string) bool {
	if len(checksum) != checksumLength {
		return false
	}
	_, err := hex.DecodeString(checksum)
	return err == nil
}

// Handler holds dependencies for HTTP handlers
type Handler struct {
	cfg       *config.Config
	storage   *storage.S3Storage
	templates *template.Template
	build     BuildInfo
}

// BuildInfo identifies the running build for display in the page header.
// It is embedded in every template data struct, since the header is shown
// on every page.
type BuildInfo struct {
	Version string
	Commit  string
}

// IndexData is the data passed to the index template
type IndexData struct {
	BuildInfo
	TTLOptions []paste.TTLOption
	CSRFToken  string
}

// PasteData is the data passed to the paste view template
type PasteData struct {
	BuildInfo
	Checksum   string
	Content    string
	CreatedAt  string
	ExpiresAt  string
	Size       int64
	Error      string
	TTLOptions []paste.TTLOption
	CSRFToken  string
}

// New creates a new Handler
func New(cfg *config.Config, storage *storage.S3Storage, templateFS fs.FS) (*Handler, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	build := version.Get()

	return &Handler{
		cfg:       cfg,
		storage:   storage,
		templates: tmpl,
		build: BuildInfo{
			Version: build.Version,
			Commit:  build.Commitish(),
		},
	}, nil
}

// RegisterRoutes registers all HTTP routes using Go 1.22+ ServeMux patterns
// and returns a handler wrapped with security headers middleware
func (h *Handler) RegisterRoutes(mux *http.ServeMux, staticFS fs.FS) http.Handler {
	// Static files
	staticHandler := http.StripPrefix("/static/", http.FileServerFS(staticFS))
	mux.HandleFunc("GET /static/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		staticHandler.ServeHTTP(w, r)
	})

	// Health endpoint
	mux.HandleFunc("GET /health", h.handleHealth)

	// robots.txt
	mux.HandleFunc("GET /robots.txt", h.handleRobots)

	// favicon.ico, served from the static files
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFileFS(w, r, staticFS, "favicon.ico")
	})

	// Application routes
	mux.HandleFunc("GET /{$}", h.handleIndex)
	mux.HandleFunc("POST /{$}", h.handleCreate)
	mux.HandleFunc("GET /raw/{checksum}", h.handleRaw)
	mux.HandleFunc("POST /delete/{checksum}", h.handleDelete)
	mux.HandleFunc("GET /{checksum}", h.handleView)

	// Wrap with security headers and access log middleware
	return accessLog(h.cfg.TrustedProxies, securityHeaders(mux))
}

// responseWriter records the status code and number of bytes written
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	wroteHeader  bool
	bytesWritten int64
}

func (rw *responseWriter) WriteHeader(code int) {
	// Only the first call takes effect, matching net/http
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.wroteHeader = true
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// Unwrap allows http.ResponseController to reach the underlying writer
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// countingBody records the number of request body bytes read
type countingBody struct {
	io.ReadCloser
	bytesRead int64
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.bytesRead += int64(n)
	return n, err
}

// accessLog assigns a request ID to each request and logs it using
// structured logging when it completes
func accessLog(trustedProxies []netip.Prefix, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		id := r.Header.Get(requestid.Header)
		if !requestid.Valid(id) {
			id = requestid.New()
		}
		w.Header().Set(requestid.Header, id)
		r = r.WithContext(storage.WithRequestIDs(requestid.WithID(r.Context(), id)))

		body := &countingBody{ReadCloser: r.Body}
		if r.Body != nil && r.Body != http.NoBody {
			r.Body = body
		}

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		defer func() {
			duration := time.Since(start)

			// Log health checks at debug level to reduce noise
			level := slog.LevelInfo
			if r.URL.Path == "/health" {
				level = slog.LevelDebug
			}

			// A panic in the handler is logged before re-panicking so that
			// net/http still aborts the connection as usual
			p := recover()
			if p != nil {
				level = slog.LevelError
				if !rw.wroteHeader {
					rw.statusCode = http.StatusInternalServerError
				}
			}

			attrs := []slog.Attr{
				slog.String("client_ip", clientIP(r, trustedProxies)),
				slog.String("method", r.Method),
				slog.String("path", r.URL.RequestURI()),
				slog.String("proto", r.Proto),
				slog.Int("status", rw.statusCode),
				slog.Int64("bytes_in", body.bytesRead),
				slog.Int64("bytes_out", rw.bytesWritten),
				slog.Float64("duration_ms", float64(duration.Microseconds())/1000),
				slog.String("user_agent", r.UserAgent()),
				slog.String("referer", r.Referer()),
			}
			if ids := storage.RequestIDs(r.Context()); len(ids) > 0 {
				attrs = append(attrs, slog.Any("s3_request_ids", ids))
			}
			if p != nil {
				attrs = append(attrs, slog.Any("panic", p))
			}
			slog.LogAttrs(r.Context(), level, "request", attrs...)

			route := routeLabel(r)
			metrics.HTTPRequests.WithLabelValues(r.Method, route, strconv.Itoa(rw.statusCode)).Inc()
			metrics.HTTPRequestDuration.WithLabelValues(r.Method, route).Observe(duration.Seconds())

			if p != nil {
				panic(p)
			}
		}()

		next.ServeHTTP(rw, r)
	})
}

// routeLabel returns the route pattern that matched the request, such as
// "/{checksum}", so that metrics are not labelled with unbounded paths.
// ServeMux records the pattern on the request it dispatches. Requests that
// matched no route, such as 405 responses, are labelled "other".
func routeLabel(r *http.Request) string {
	if r.Pattern == "" {
		return "other"
	}
	// Strip the method and host from patterns like "GET /{checksum}"
	route := r.Pattern
	if i := strings.Index(route, "/"); i >= 0 {
		route = route[i:]
	}
	return route
}

// clientIP returns the IP address of the client. The X-Forwarded-For header
// is only consulted when the request comes from a trusted proxy, and is then
// walked from right to left until the first untrusted address is found.
func clientIP(r *http.Request, trustedProxies []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	addr, err := netip.ParseAddr(host)
	if err != nil || !isTrusted(addr, trustedProxies) {
		return host
	}

	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(v, ",")...)
	}

	client := host
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			// Unparseable entry, so stop at the last known good address
			break
		}
		client = hop.Unmap().String()
		if !isTrusted(hop, trustedProxies) {
			break
		}
	}
	return client
}

func isTrusted(addr netip.Addr, trustedProxies []netip.Prefix) bool {
	addr = addr.Unmap()
	for _, p := range trustedProxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// securityHeaders adds security headers to all responses
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive, nosnippet")
		next.ServeHTTP(w, r)
	})
}

// handleHealth returns a simple health check response
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (h *Handler) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	token, err := csrf.GenerateToken()
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to generate CSRF token", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	csrf.SetCookie(w, token, h.cfg.SecureCookies)

	data := IndexData{
		BuildInfo:  h.build,
		TTLOptions: paste.TTLOptions(h.cfg.DefaultTTL),
		CSRFToken:  token,
	}

	if err := h.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		slog.ErrorContext(r.Context(), "failed to execute index template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	// Limit request body size to prevent memory exhaustion
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if !csrf.Validate(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	content := r.FormValue("content")
	if content == "" {
		http.Error(w, "Content is required", http.StatusBadRequest)
		return
	}

	if int64(len(content)) > h.cfg.MaxPasteSize {
		http.Error(w, fmt.Sprintf("Content exceeds maximum size of %d bytes", h.cfg.MaxPasteSize), http.StatusBadRequest)
		return
	}

	// Parse and validate TTL
	ttlStr := r.FormValue("ttl")
	ttl := h.cfg.DefaultTTL
	if ttlStr != "" {
		if parsed, err := time.ParseDuration(ttlStr); err == nil && parsed > 0 {
			ttl = parsed
		}
	}
	// Ensure TTL is positive (in case default is invalid)
	if ttl <= 0 {
		ttl = h.cfg.DefaultTTL
		if ttl <= 0 {
			ttl = 24 * time.Hour // Fallback to 1 day
		}
	}

	// Create paste
	p := paste.NewPaste(content)
	meta := paste.NewMeta(p.Checksum, int64(len(content)), ttl)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := h.storage.Store(ctx, p, meta); err != nil {
		slog.ErrorContext(r.Context(), "failed to store paste", "error", err)
		http.Error(w, "Failed to store paste", http.StatusInternalServerError)
		return
	}

	metrics.PastesCreated.Inc()
	slog.InfoContext(r.Context(), "paste created", "checksum", p.Checksum, "size", meta.Size, "ttl", ttl.String())
	http.Redirect(w, r, "/"+p.Checksum, http.StatusSeeOther)
}

func (h *Handler) handleView(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	checksum := r.PathValue("checksum")
	if checksum == "" || !isValidChecksum(checksum) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Generate CSRF token for forms on this page
	token, err := csrf.GenerateToken()
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to generate CSRF token", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	csrf.SetCookie(w, token, h.cfg.SecureCookies)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	p, meta, err := h.storage.Get(ctx, checksum)
	if err != nil {
		slog.DebugContext(r.Context(), "paste not found", "checksum", checksum, "error", err)
		data := PasteData{
			BuildInfo: h.build,
			Checksum:  checksum,
			Error:     "Paste not found or has expired",
			CSRFToken: token,
		}
		w.WriteHeader(http.StatusNotFound)
		if err := h.templates.ExecuteTemplate(w, "paste.html", data); err != nil {
			slog.ErrorContext(r.Context(), "failed to execute paste template", "error", err)
		}
		return
	}

	// Check if expired
	if meta.IsExpired() {
		// Delete expired paste
		if err := h.storage.Delete(ctx, checksum); err == nil {
			metrics.PastesDeleted.WithLabelValues("expired").Inc()
		}
		slog.InfoContext(r.Context(), "deleted expired paste on access", "checksum", checksum)
		data := PasteData{
			BuildInfo: h.build,
			Checksum:  checksum,
			Error:     "Paste has expired",
			CSRFToken: token,
		}
		w.WriteHeader(http.StatusGone)
		if err := h.templates.ExecuteTemplate(w, "paste.html", data); err != nil {
			slog.ErrorContext(r.Context(), "failed to execute paste template", "error", err)
		}
		return
	}

	data := PasteData{
		BuildInfo:  h.build,
		Checksum:   checksum,
		Content:    p.Content,
		CreatedAt:  meta.CreatedAt.Format(time.RFC3339),
		ExpiresAt:  meta.ExpiresAt.Format(time.RFC3339),
		Size:       meta.Size,
		TTLOptions: paste.TTLOptions(h.cfg.DefaultTTL),
		CSRFToken:  token,
	}

	if err := h.templates.ExecuteTemplate(w, "paste.html", data); err != nil {
		slog.ErrorContext(r.Context(), "failed to execute paste template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) handleRaw(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	checksum := r.PathValue("checksum")
	if checksum == "" || !isValidChecksum(checksum) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	p, meta, err := h.storage.Get(ctx, checksum)
	if err != nil {
		http.Error(w, "Paste not found", http.StatusNotFound)
		return
	}

	if meta.IsExpired() {
		if err := h.storage.Delete(ctx, checksum); err == nil {
			metrics.PastesDeleted.WithLabelValues("expired").Inc()
		}
		http.Error(w, "Paste has expired", http.StatusGone)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(p.Content))
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if !csrf.Validate(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	checksum := r.PathValue("checksum")
	if checksum == "" || !isValidChecksum(checksum) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Verify paste exists
	_, _, err := h.storage.Get(ctx, checksum)
	if err != nil {
		http.Error(w, "Paste not found", http.StatusNotFound)
		return
	}

	if err := h.storage.Delete(ctx, checksum); err != nil {
		slog.ErrorContext(r.Context(), "failed to delete paste", "checksum", checksum, "error", err)
		http.Error(w, "Failed to delete paste", http.StatusInternalServerError)
		return
	}

	metrics.PastesDeleted.WithLabelValues("manual").Inc()
	slog.InfoContext(r.Context(), "paste deleted", "checksum", checksum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
