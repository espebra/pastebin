package storage

import (
	"context"
	"net/http"
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

// httpDoer is the HTTP client interface used by the AWS SDK
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// requestIDClient records the S3 request ID of each response in the
// collector carried by the request context, if any
type requestIDClient struct {
	next httpDoer
}

func (c requestIDClient) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.next.Do(req)
	if resp != nil {
		if id := resp.Header.Get(requestIDHeader); id != "" {
			if col, ok := req.Context().Value(requestIDsKey{}).(*requestIDs); ok {
				col.mu.Lock()
				col.ids = append(col.ids, id)
				col.mu.Unlock()
			}
		}
	}
	return resp, err
}
