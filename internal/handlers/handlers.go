package handlers

import (
	"context"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/espebra/pastebin/internal/config"
	"github.com/espebra/pastebin/internal/csrf"
	"github.com/espebra/pastebin/internal/paste"
	"github.com/espebra/pastebin/internal/storage"
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
}

// IndexData is the data passed to the index template
type IndexData struct {
	TTLOptions []paste.TTLOption
	CSRFToken  string
}

// PasteData is the data passed to the paste view template
type PasteData struct {
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

	return &Handler{
		cfg:       cfg,
		storage:   storage,
		templates: tmpl,
	}, nil
}

// RegisterRoutes registers all HTTP routes using Go 1.22+ ServeMux patterns
// and returns a handler wrapped with security headers middleware
func (h *Handler) RegisterRoutes(mux *http.ServeMux, staticFS fs.FS) http.Handler {
	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	// Health endpoint
	mux.HandleFunc("GET /health", h.handleHealth)

	// Application routes
	mux.HandleFunc("GET /{$}", h.handleIndex)
	mux.HandleFunc("POST /{$}", h.handleCreate)
	mux.HandleFunc("GET /raw/{checksum}", h.handleRaw)
	mux.HandleFunc("POST /delete/{checksum}", h.handleDelete)
	mux.HandleFunc("GET /{checksum}", h.handleView)

	// Wrap with security headers middleware
	return securityHeaders(mux)
}

// securityHeaders adds security headers to all responses
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// handleHealth returns a simple health check response
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	token, err := csrf.GenerateToken()
	if err != nil {
		slog.Error("failed to generate CSRF token", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	csrf.SetCookie(w, token, h.cfg.SecureCookies)

	data := IndexData{
		TTLOptions: paste.TTLOptions(h.cfg.DefaultTTL),
		CSRFToken:  token,
	}

	w.Header().Set("Cache-Control", "no-cache")
	if err := h.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		slog.Error("failed to execute index template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
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
		slog.Error("failed to store paste", "error", err)
		http.Error(w, "Failed to store paste", http.StatusInternalServerError)
		return
	}

	slog.Info("paste created", "checksum", p.Checksum, "size", meta.Size, "ttl", ttl.String())
	http.Redirect(w, r, "/"+p.Checksum, http.StatusSeeOther)
}

func (h *Handler) handleView(w http.ResponseWriter, r *http.Request) {
	checksum := r.PathValue("checksum")
	if checksum == "" || !isValidChecksum(checksum) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Generate CSRF token for forms on this page
	token, err := csrf.GenerateToken()
	if err != nil {
		slog.Error("failed to generate CSRF token", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	csrf.SetCookie(w, token, h.cfg.SecureCookies)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	p, meta, err := h.storage.Get(ctx, checksum)
	if err != nil {
		slog.Debug("paste not found", "checksum", checksum, "error", err)
		data := PasteData{
			Checksum:  checksum,
			Error:     "Paste not found or has expired",
			CSRFToken: token,
		}
		w.WriteHeader(http.StatusNotFound)
		if err := h.templates.ExecuteTemplate(w, "paste.html", data); err != nil {
			slog.Error("failed to execute paste template", "error", err)
		}
		return
	}

	// Check if expired
	if meta.IsExpired() {
		// Delete expired paste
		_ = h.storage.Delete(ctx, checksum)
		slog.Info("deleted expired paste on access", "checksum", checksum)
		data := PasteData{
			Checksum:  checksum,
			Error:     "Paste has expired",
			CSRFToken: token,
		}
		w.WriteHeader(http.StatusGone)
		if err := h.templates.ExecuteTemplate(w, "paste.html", data); err != nil {
			slog.Error("failed to execute paste template", "error", err)
		}
		return
	}

	data := PasteData{
		Checksum:   checksum,
		Content:    p.Content,
		CreatedAt:  meta.CreatedAt.Format(time.RFC3339),
		ExpiresAt:  meta.ExpiresAt.Format(time.RFC3339),
		Size:       meta.Size,
		TTLOptions: paste.TTLOptions(h.cfg.DefaultTTL),
		CSRFToken:  token,
	}

	w.Header().Set("Cache-Control", "no-cache")
	if err := h.templates.ExecuteTemplate(w, "paste.html", data); err != nil {
		slog.Error("failed to execute paste template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) handleRaw(w http.ResponseWriter, r *http.Request) {
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
		_ = h.storage.Delete(ctx, checksum)
		http.Error(w, "Paste has expired", http.StatusGone)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(p.Content))
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
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
		slog.Error("failed to delete paste", "checksum", checksum, "error", err)
		http.Error(w, "Failed to delete paste", http.StatusInternalServerError)
		return
	}

	slog.Info("paste deleted", "checksum", checksum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
