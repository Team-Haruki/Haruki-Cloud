package sekai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"
	"haruki-cloud/utils/usererror"

	"github.com/go-resty/resty/v2"
)

var restyLogger = logger.NewLoggerFromGlobal("SekaiRESTY")

const (
	maxUpstreamResponseBytes = 64 << 20
	slowHTTPLogThreshold     = 2 * time.Second
)

// sanitizeNetworkError strips embedded URLs from HTTP client errors so that
// internal service hostnames are never exposed in user-facing messages.
// The original error type is intentionally discarded; only the sanitized
// message text is preserved.
func sanitizeNetworkError(err error) error {
	if err == nil {
		return nil
	}
	if usererror.MessageContainsSensitiveURL(err.Error()) {
		return errors.New("network request failed")
	}
	return errors.New(err.Error())
}

// newRestyClient returns a resty.Client pre-configured with common retry
// logic shared by all Sekai HTTP clients. Each client may further configure
// timeout, headers, etc. on the returned instance.
func newRestyClient() *resty.Client {
	return resty.New().
		SetTransport(upstream.NewTunedTransport(upstream.TunedTransportConfig{})).
		SetLogger(restyLogger).
		SetResponseBodyLimit(maxUpstreamResponseBytes).
		SetRetryCount(maxRetries).
		SetRetryWaitTime(retryWaitTime).
		AddRetryHook(logRestyRetry).
		OnAfterResponse(logRestyResponse).
		OnError(logRestyError).
		AddRetryCondition(isRetryable)
}

func logRestyRetry(resp *resty.Response, err error) {
	if resp == nil || resp.Request == nil {
		restyLogger.WarnContext(context.Background(), "resty request retry",
			"error_type", fmt.Sprintf("%T", err),
		)
		return
	}
	ctx := resp.Request.Context()
	method, attempt := restyRequestDebug(resp.Request)
	if err != nil {
		restyLogger.WarnContext(ctx, "resty request retry",
			"http_method", method,
			"upstream", "pjsk_external",
			"attempt", attempt,
			"error_type", fmt.Sprintf("%T", err),
		)
		return
	}
	restyLogger.WarnContext(ctx, "resty response retry",
		"http_method", method,
		"upstream", "pjsk_external",
		"status_code", resp.StatusCode(),
		"duration_ms", commandtrace.Milliseconds(resp.Time()),
		"attempt", attempt,
		"response_bytes", resp.Size(),
	)
}

func logRestyResponse(_ *resty.Client, resp *resty.Response) error {
	if resp == nil || resp.Request == nil {
		return nil
	}
	elapsed := resp.Time()
	status := resp.StatusCode()
	if status < 500 && elapsed < slowHTTPLogThreshold {
		return nil
	}
	method, attempt := restyRequestDebug(resp.Request)
	if status >= 500 {
		restyLogger.WarnContext(resp.Request.Context(), "resty upstream response failed",
			"http_method", method,
			"upstream", "pjsk_external",
			"status_code", status,
			"duration_ms", commandtrace.Milliseconds(elapsed),
			"attempt", attempt,
			"response_bytes", resp.Size(),
		)
		return nil
	}
	restyLogger.InfoContext(resp.Request.Context(), "resty upstream response slow",
		"http_method", method,
		"upstream", "pjsk_external",
		"status_code", status,
		"duration_ms", commandtrace.Milliseconds(elapsed),
		"attempt", attempt,
		"response_bytes", resp.Size(),
	)
	return nil
}

func logRestyError(req *resty.Request, err error) {
	method, attempt := restyRequestDebug(req)
	ctx := context.Background()
	if req != nil {
		ctx = req.Context()
	}
	restyLogger.ErrorContext(ctx, "resty request failed",
		"http_method", method,
		"upstream", "pjsk_external",
		"attempt", attempt,
		"error_type", fmt.Sprintf("%T", err),
	)
}

func restyRequestDebug(req *resty.Request) (string, int) {
	if req == nil {
		return "", 0
	}
	method := strings.TrimSpace(req.Method)
	if req.RawRequest != nil {
		if method == "" {
			method = req.RawRequest.Method
		}
	}
	return method, req.Attempt
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
