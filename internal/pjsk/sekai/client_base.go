package sekai

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode"

	"haruki-cloud/utils/logger"
	"haruki-cloud/utils/usererror"

	"github.com/go-resty/resty/v2"
)

var restyLogger = logger.NewLoggerFromGlobal("SekaiRESTY")

const slowHTTPLogThreshold = 2 * time.Second

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
		SetLogger(restyLogger).
		SetRetryCount(maxRetries).
		SetRetryWaitTime(retryWaitTime).
		AddRetryHook(logRestyRetry).
		OnAfterResponse(logRestyResponse).
		OnError(logRestyError).
		AddRetryCondition(isRetryable)
}

func logRestyRetry(resp *resty.Response, err error) {
	if resp == nil || resp.Request == nil {
		restyLogger.Warnf("resty retry: err=%v", sanitizeNetworkError(err))
		return
	}
	method, requestURL, attempt := restyRequestDebug(resp.Request)
	if err != nil {
		restyLogger.Warnf("resty retry: method=%s url=%s attempt=%d err=%v", method, requestURL, attempt, sanitizeNetworkError(err))
		return
	}
	restyLogger.Warnf("resty retry: method=%s url=%s status=%d elapsed=%dms attempt=%d bytes=%d",
		method, requestURL, resp.StatusCode(), resp.Time().Milliseconds(), attempt, resp.Size())
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
	method, requestURL, attempt := restyRequestDebug(resp.Request)
	if status >= 500 {
		restyLogger.Warnf("resty response: method=%s url=%s status=%d elapsed=%dms attempt=%d bytes=%d",
			method, requestURL, status, elapsed.Milliseconds(), attempt, resp.Size())
		return nil
	}
	restyLogger.Infof("resty slow response: method=%s url=%s status=%d elapsed=%dms attempt=%d bytes=%d",
		method, requestURL, status, elapsed.Milliseconds(), attempt, resp.Size())
	return nil
}

func logRestyError(req *resty.Request, err error) {
	method, requestURL, attempt := restyRequestDebug(req)
	restyLogger.Errorf("resty request failed: method=%s url=%s attempt=%d err=%v",
		method, requestURL, attempt, sanitizeNetworkError(err))
}

func restyRequestDebug(req *resty.Request) (string, string, int) {
	if req == nil {
		return "", "", 0
	}
	method := strings.TrimSpace(req.Method)
	requestURL := strings.TrimSpace(req.URL)
	if req.RawRequest != nil {
		if method == "" {
			method = req.RawRequest.Method
		}
		if req.RawRequest.URL != nil {
			requestURL = req.RawRequest.URL.String()
		}
	}
	return method, sanitizeDebugURL(requestURL), req.Attempt
}

func sanitizeDebugURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return maskLongNumericPath(strings.Split(raw, "?")[0])
	}
	parsed.User = nil
	parsed.Path = maskLongNumericPath(parsed.Path)
	parsed.RawPath = ""
	query := parsed.Query()
	for key, values := range query {
		if shouldMaskQueryValue(key) {
			for i, value := range values {
				values[i] = maskDebugValue(value)
			}
			query[key] = values
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func shouldMaskQueryValue(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "user") ||
		strings.Contains(key, "uid") ||
		strings.Contains(key, "token") ||
		strings.Contains(key, "authorization")
}

func maskLongNumericPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if len(part) >= 7 && isNumericString(part) {
			parts[i] = maskDebugValue(part)
		}
	}
	return strings.Join(parts, "/")
}

func isNumericString(value string) bool {
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return value != ""
}

func maskDebugValue(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= 6 {
		return "***"
	}
	return string(runes[:3]) + "***" + string(runes[len(runes)-3:])
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
