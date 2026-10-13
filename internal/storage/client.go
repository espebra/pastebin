package storage

import (
	"net/http"
	"strconv"
	"time"

	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"

	"github.com/espebra/pastebin/internal/metrics"
)

// httpDoer is the HTTP client interface used by the AWS SDK
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// instrumentedClient wraps the HTTP client used by the AWS SDK. Every
// request to S3, including retries, is recorded in the metrics, and the S3
// request ID of each response is added to the collector carried by the
// request context, if any.
type instrumentedClient struct {
	next httpDoer
}

func (c instrumentedClient) Do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	operation := awsmiddleware.GetOperationName(ctx)
	if operation == "" {
		operation = "unknown"
	}

	start := time.Now()
	resp, err := c.next.Do(req)
	metrics.S3RequestDuration.WithLabelValues(operation).Observe(time.Since(start).Seconds())

	code := "error"
	if resp != nil {
		code = strconv.Itoa(resp.StatusCode)
		if id := resp.Header.Get(requestIDHeader); id != "" {
			addRequestID(ctx, id)
		}
	}
	metrics.S3Requests.WithLabelValues(operation, code).Inc()

	return resp, err
}
