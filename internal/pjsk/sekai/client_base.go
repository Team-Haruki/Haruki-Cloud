package sekai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/core/upstreamerr"
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

// sanitizeNetworkError wraps a transport failure of service as an
// upstreamerr.TransportError whose text is "<prefix>: <cause>" with the
// cause's URLs (internal hostnames) removed. The original error stays
// reachable through Unwrap for errors.Is checks, and its kind (timeout or
// unavailable) is judged from its type.
func sanitizeNetworkError(service upstreamerr.Service, prefix string, err error) error {
	if err == nil {
		return nil
	}
	text := err.Error()
	if usererror.MessageContainsSensitiveURL(text) {
		text = "network request failed"
	}
	return upstreamerr.Transport(service, prefix+": "+text, err)
}

// requestContextError marks the error of a request's own context (deadline
// or cancellation) as a transport failure of service, so a request deadline
// is classified as a timeout of that service. The text and the unwrap chain
// stay those of ctxErr.
func requestContextError(service upstreamerr.Service, ctxErr error) error {
	return upstreamerr.Transport(service, "", ctxErr)
}

// newRestyClient returns a resty.Client with the shared logging and the
// retry policy. Each client may further configure timeout, headers, etc. on
// the returned instance.
func newRestyClient(retry config.UpstreamRetryConfig) *resty.Client {
	retry = retry.WithDefaults()
	client := resty.New().
		SetTransport(upstream.NewTunedTransport(upstream.TunedTransportConfig{})).
		SetLogger(restyLogger).
		SetResponseBodyLimit(maxUpstreamResponseBytes).
		OnAfterResponse(logRestyResponse).
		OnError(logRestyError)
	if retry.MaxRetries <= 0 {
		return client
	}
	policy := retryPolicy{budget: retry.Budget, wait: retry.Wait}
	return client.
		SetRetryCount(retry.MaxRetries).
		SetRetryWaitTime(retry.Wait).
		SetRetryMaxWaitTime(retry.MaxWait).
		OnBeforeRequest(markFirstAttempt).
		AddRetryHook(logRestyRetry).
		AddRetryCondition(policy.shouldRetry)
}

// retryPolicy decides whether a failed attempt is retried: any 5xx answer
// or transport failure, client timeouts included, as through 3.7.18. A
// cancelled or expired caller context is never retried (resty also stops
// on it). With a positive budget no retry starts once the next attempt would
// begin more than budget after the first one.
type retryPolicy struct {
	budget time.Duration
	wait   time.Duration
}

type firstAttemptKey struct{}

// markFirstAttempt stamps the request context with the first attempt's
// start; later attempts keep it.
func markFirstAttempt(_ *resty.Client, req *resty.Request) error {
	if req != nil && req.Attempt <= 1 {
		req.SetContext(context.WithValue(req.Context(), firstAttemptKey{}, time.Now()))
	}
	return nil
}

func (p retryPolicy) shouldRetry(r *resty.Response, err error) bool {
	if r == nil || r.Request == nil {
		return false
	}
	if ctxErr := r.Request.Context().Err(); ctxErr != nil {
		return false
	}
	if p.budget > 0 {
		started, ok := r.Request.Context().Value(firstAttemptKey{}).(time.Time)
		if !ok {
			started = r.Request.Time
		}
		if time.Since(started)+p.wait > p.budget {
			return false
		}
	}
	if err != nil {
		return isRetryableTransportError(err)
	}
	return r.StatusCode() >= http.StatusInternalServerError
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

// isRetryableTransportError reports a transport failure worth another
// attempt: any network error (timeouts included), a refused, reset or
// dropped connection, or an unknown host. Cancellation of the caller's own
// context is not retried.
func isRetryableTransportError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.HasSuffix(msg, "EOF")
}

func retryConfig(cfg *config.SekaiAPIConfig) config.UpstreamRetryConfig {
	if cfg == nil {
		return config.UpstreamRetryConfig{}
	}
	return cfg.Retry
}

func toolboxRetryConfig(cfg *config.ToolboxConfig) config.UpstreamRetryConfig {
	if cfg == nil {
		return config.UpstreamRetryConfig{}
	}
	return cfg.Retry
}

func trackerRetryConfig(cfg *config.TrackerConfig) config.UpstreamRetryConfig {
	if cfg == nil {
		return config.UpstreamRetryConfig{}
	}
	return cfg.Retry
}
