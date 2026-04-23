package sekai

import (
	"errors"
	"net"
	"regexp"
	"strings"

	"github.com/go-resty/resty/v2"
)

// quotedURL matches any quoted http/https URL in an error string, e.g.
// the fragment produced by Go's net/http: Get "https://host/path": ...
var quotedURL = regexp.MustCompile(`"https?://[^"]+"`)

// sanitizeNetworkError strips embedded URLs from HTTP client errors so that
// internal service hostnames are never exposed in user-facing messages.
// The original error type is intentionally discarded; only the sanitized
// message text is preserved.
func sanitizeNetworkError(err error) error {
	if err == nil {
		return nil
	}
	cleaned := quotedURL.ReplaceAllString(err.Error(), `"<url>"`)
	return errors.New(cleaned)
}

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
