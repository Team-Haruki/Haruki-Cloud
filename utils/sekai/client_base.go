package sekai

import (
	"errors"
	"net"
	"strings"

	"github.com/go-resty/resty/v2"
)

// newRestyClient returns a resty.Client pre-configured with common retry
// logic shared by all Sekai HTTP clients. Each client may further configure
// timeout, headers, etc. on the returned instance.
func newRestyClient() *resty.Client {
	return resty.New().
		SetRetryCount(maxRetries).
		SetRetryWaitTime(retryWaitTime).
		AddRetryCondition(isRetryable)
}

// isRetryable returns true for transient errors that warrant an automatic
// retry: network-level failures and 5xx server errors.
func isRetryable(r *resty.Response, err error) bool {
	if err != nil {
		if _, ok := errors.AsType[net.Error](err); ok {
			return true
		}
		msg := err.Error()
		return strings.Contains(msg, "connection refused") ||
			strings.Contains(msg, "no such host") ||
			strings.Contains(msg, "i/o timeout") ||
			strings.Contains(msg, "EOF")
	}
	return r.StatusCode() >= 500
}
