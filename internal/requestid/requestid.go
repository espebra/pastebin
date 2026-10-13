// Package requestid assigns an identifier to each HTTP request and attaches
// it to every log record emitted with the request's context, so that all log
// lines belonging to one transaction can be correlated.
package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
)

// Header is the HTTP header used to accept and return the request ID
const Header = "X-Request-ID"

// maxLength limits the length of request IDs accepted from clients
const maxLength = 64

type contextKey struct{}

// New generates a random request ID
func New() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Valid reports whether an incoming request ID is safe to log and echo back.
// Only short IDs made of letters, digits, '-', '_' and '.' are accepted.
func Valid(id string) bool {
	if id == "" || len(id) > maxLength {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

// WithID returns a copy of ctx carrying the request ID
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext returns the request ID carried by ctx, or "" if there is none
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}

// LogHandler wraps a slog.Handler and adds a request_id attribute to every
// record whose context carries a request ID
type LogHandler struct {
	slog.Handler
}

// NewLogHandler returns a LogHandler wrapping h
func NewLogHandler(h slog.Handler) *LogHandler {
	return &LogHandler{Handler: h}
}

func (h *LogHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := FromContext(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *LogHandler) WithGroup(name string) slog.Handler {
	return &LogHandler{Handler: h.Handler.WithGroup(name)}
}
