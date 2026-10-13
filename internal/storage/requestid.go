package storage

import (
	"context"
	"sync"
)

// requestIDHeader is the response header S3 uses to identify a request
const requestIDHeader = "X-Amz-Request-Id"

type requestIDsKey struct{}

// requestIDs collects the S3 request IDs seen while serving one request
type requestIDs struct {
	mu  sync.Mutex
	ids []string
}

// WithRequestIDs returns a copy of ctx that collects the request ID of every
// S3 request made with it, including retries. Use RequestIDs to read them.
func WithRequestIDs(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestIDsKey{}, &requestIDs{})
}

// RequestIDs returns the S3 request IDs collected in ctx, in the order the
// responses were received
func RequestIDs(ctx context.Context) []string {
	c, ok := ctx.Value(requestIDsKey{}).(*requestIDs)
	if !ok {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.ids...)
}

// addRequestID records id in the collector carried by ctx, if any
func addRequestID(ctx context.Context, id string) {
	c, ok := ctx.Value(requestIDsKey{}).(*requestIDs)
	if !ok {
		return
	}
	c.mu.Lock()
	c.ids = append(c.ids, id)
	c.mu.Unlock()
}
