package deck

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	json "haruki-cloud/internal/jsonutil"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"haruki-cloud/internal/observability/commandtrace"

	"github.com/klauspost/compress/zstd"
)

const circuitBreakerHealthCheckTimeout = 3 * time.Second

const (
	maxDeckResponseBodyBytes = 32 << 20
	maxDeckErrorBodyBytes    = 16 << 10
)

// deckZstdEncoder is reused across all payload builds. zstd.Encoder.EncodeAll
// is safe for concurrent use, so a single shared encoder avoids allocating a
// fresh writer (and its buffers) on every deck request. Constructed with a nil
// destination writer, which never returns an error.
var deckZstdEncoder, _ = zstd.NewWriter(nil)

// isRetryableError returns true for transient errors that warrant a retry.
func isRetryableError(err error, statusCode int) bool {
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
	return statusCode >= 500
}

func (r *RemoteDeckRecommender) postJSON(ctx context.Context, exec *remoteExecution, path string, requestBody any, responseBody any) error {
	ctx = normalizeRecommendContext(ctx)
	finishEncode := commandtrace.MeasureOperation(ctx, "deck.encode")
	body, err := json.Marshal(requestBody)
	finishEncode()
	if err != nil {
		return err
	}
	baseURL := exec.BaseURL()
	if strings.TrimSpace(baseURL) == "" {
		return fmt.Errorf("deck-service target base_url is empty")
	}

	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt > 0 {
			if err := waitForDeckRetry(ctx, r.retryWaitTime); err != nil {
				return err
			}
			r.logger.DebugContext(ctx, "deck request retrying",
				"upstream", deckServiceName,
				"upstream_path", path,
				"attempt", attempt,
				"max_retries", r.maxRetries,
			)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		start := time.Now()
		finishHTTP := commandtrace.MeasureOperation(ctx, deckHTTPStage)
		resp, err := r.client.Do(req)

		if err != nil {
			finishHTTP()
			elapsed := time.Since(start)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			lastErr = err
			r.logger.WarnContext(ctx, "deck request failed",
				"upstream", deckServiceName,
				"upstream_path", path,
				"attempt", attempt,
				"duration_ms", commandtrace.Milliseconds(elapsed),
				"error_type", fmt.Sprintf("%T", err),
			)
			if isRetryableError(err, 0) {
				continue
			}
			return err
		}

		payload, truncated, readErr := readDeckResponseBody(resp.Body)
		resp.Body.Close()
		finishHTTP()
		elapsed := time.Since(start)
		if readErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			lastErr = readErr
			continue
		}
		if truncated {
			return fmt.Errorf("deck-service response exceeded %d bytes", maxDeckResponseBodyBytes)
		}

		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			r.logger.DebugContext(ctx, "deck request completed",
				"upstream", deckServiceName,
				"upstream_path", path,
				"status_code", resp.StatusCode,
				"duration_ms", commandtrace.Milliseconds(elapsed),
			)
			if responseBody == nil || len(payload) == 0 {
				return nil
			}
			finishDecode := commandtrace.MeasureOperation(ctx, "deck.decode")
			decodeErr := json.Unmarshal(payload, responseBody)
			finishDecode()
			return decodeErr
		}

		lastErr = parseRemoteHTTPError(resp.StatusCode, payload)
		r.logger.WarnContext(ctx, "deck request returned non-success status",
			"upstream", deckServiceName,
			"upstream_path", path,
			"attempt", attempt,
			"status_code", resp.StatusCode,
			"duration_ms", commandtrace.Milliseconds(elapsed),
			"response_bytes", len(payload),
		)
		if isMissingUserdataHashError(lastErr) {
			return lastErr
		}
		if !isRetryableError(nil, resp.StatusCode) {
			return lastErr
		}
	}
	return lastErr
}

func (r *RemoteDeckRecommender) postBinary(ctx context.Context, exec *remoteExecution, path string, payload []byte, responseBody any) error {
	ctx = normalizeRecommendContext(ctx)
	baseURL := exec.BaseURL()
	if strings.TrimSpace(baseURL) == "" {
		return fmt.Errorf("deck-service target base_url is empty")
	}

	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt > 0 {
			if err := waitForDeckRetry(ctx, r.retryWaitTime); err != nil {
				return err
			}
			r.logger.DebugContext(ctx, "deck binary request retrying",
				"upstream", deckServiceName,
				"upstream_path", path,
				"attempt", attempt,
				"max_retries", r.maxRetries,
			)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/octet-stream")

		start := time.Now()
		finishHTTP := commandtrace.MeasureOperation(ctx, deckHTTPStage)
		resp, err := r.client.Do(req)

		if err != nil {
			finishHTTP()
			elapsed := time.Since(start)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			lastErr = err
			r.logger.WarnContext(ctx, "deck binary request failed",
				"upstream", deckServiceName,
				"upstream_path", path,
				"attempt", attempt,
				"duration_ms", commandtrace.Milliseconds(elapsed),
				"error_type", fmt.Sprintf("%T", err),
			)
			if isRetryableError(err, 0) {
				continue
			}
			return err
		}

		body, truncated, readErr := readDeckResponseBody(resp.Body)
		resp.Body.Close()
		finishHTTP()
		elapsed := time.Since(start)
		if readErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			lastErr = readErr
			continue
		}
		if truncated {
			return fmt.Errorf("deck-service response exceeded %d bytes", maxDeckResponseBodyBytes)
		}

		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			r.logger.DebugContext(ctx, "deck binary request completed",
				"upstream", deckServiceName,
				"upstream_path", path,
				"status_code", resp.StatusCode,
				"duration_ms", commandtrace.Milliseconds(elapsed),
			)
			if responseBody == nil || len(body) == 0 {
				return nil
			}
			finishDecode := commandtrace.MeasureOperation(ctx, "deck.decode")
			decodeErr := json.Unmarshal(body, responseBody)
			finishDecode()
			return decodeErr
		}

		lastErr = parseRemoteHTTPError(resp.StatusCode, body)
		r.logger.WarnContext(ctx, "deck binary request returned non-success status",
			"upstream", deckServiceName,
			"upstream_path", path,
			"attempt", attempt,
			"status_code", resp.StatusCode,
			"duration_ms", commandtrace.Milliseconds(elapsed),
			"response_bytes", len(body),
		)
		if isMissingUserdataHashError(lastErr) {
			return lastErr
		}
		if !isRetryableError(nil, resp.StatusCode) {
			return lastErr
		}
	}
	return lastErr
}

func parseRemoteHTTPError(statusCode int, payload []byte) error {
	if len(payload) > maxDeckErrorBodyBytes {
		payload = payload[:maxDeckErrorBodyBytes]
	}
	var remoteErr remoteErrorResponse
	if json.Unmarshal(payload, &remoteErr) == nil && strings.TrimSpace(remoteErr.Error) != "" {
		return fmt.Errorf("%s", remoteErr.Error)
	}
	if trimmed := strings.TrimSpace(string(payload)); trimmed != "" {
		return fmt.Errorf("deck-service returned HTTP %d: %s", statusCode, trimmed)
	}
	return fmt.Errorf("deck-service returned HTTP %d", statusCode)
}

func readDeckResponseBody(body io.Reader) ([]byte, bool, error) {
	payload, err := io.ReadAll(io.LimitReader(body, maxDeckResponseBodyBytes+1))
	if err != nil {
		return nil, false, err
	}
	if len(payload) > maxDeckResponseBodyBytes {
		return payload[:maxDeckResponseBodyBytes], true, nil
	}
	return payload, false, nil
}

func (r *RemoteDeckRecommender) healthCheck(ctx context.Context, baseURL string) bool {
	if r == nil || r.client == nil || strings.TrimSpace(baseURL) == "" {
		return false
	}

	ctx, cancel := context.WithTimeout(normalizeRecommendContext(ctx), circuitBreakerHealthCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return false
	}

	finishHTTP := commandtrace.MeasureOperation(ctx, deckHTTPStage)
	resp, err := r.client.Do(req)
	finishHTTP()
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}

func waitForDeckRetry(ctx context.Context, wait time.Duration) error {
	ctx = normalizeRecommendContext(ctx)
	finishWait := commandtrace.MeasureOperation(ctx, "deck.retry_wait")
	defer finishWait()
	if wait <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func buildMultipartPayload(ctx context.Context, segments ...[]byte) []byte {
	finishCompress := commandtrace.MeasureOperation(ctx, "deck.compress")
	defer finishCompress()
	var raw bytes.Buffer
	for _, segment := range segments {
		if err := binary.Write(&raw, binary.BigEndian, uint32(len(segment))); err != nil {
			return nil
		}
		if _, err := raw.Write(segment); err != nil {
			return nil
		}
	}

	return deckZstdEncoder.EncodeAll(raw.Bytes(), nil)
}
