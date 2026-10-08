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

	"haruki-cloud/internal/httpcoding"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/observability/upstreamcall"

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
	return r.postEncoded(ctx, exec, path, body, "application/json", "deck request", responseBody)
}

func (r *RemoteDeckRecommender) postBinary(ctx context.Context, exec *remoteExecution, path string, payload []byte, responseBody any) error {
	ctx = normalizeRecommendContext(ctx)
	return r.postEncoded(ctx, exec, path, payload, "application/octet-stream", "deck binary request", responseBody)
}

type deckPostOutcome struct {
	err  error
	done bool
}

func (r *RemoteDeckRecommender) postEncoded(ctx context.Context, exec *remoteExecution, path string, payload []byte, contentType, logLabel string, responseBody any) error {
	baseURL := exec.BaseURL()
	if strings.TrimSpace(baseURL) == "" {
		return fmt.Errorf("deck-service target base_url is empty")
	}
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if err := r.prepareDeckPostAttempt(ctx, path, logLabel, attempt); err != nil {
			return err
		}
		outcome := r.executeDeckPostAttempt(ctx, baseURL, path, payload, contentType, logLabel, attempt, responseBody)
		lastErr = outcome.err
		if outcome.done {
			return outcome.err
		}
	}
	return lastErr
}

func (r *RemoteDeckRecommender) prepareDeckPostAttempt(ctx context.Context, path, logLabel string, attempt int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if attempt == 0 {
		return nil
	}
	if err := waitForDeckRetry(ctx, r.retryWaitTime); err != nil {
		return err
	}
	r.logger.DebugContext(ctx, logLabel+" retrying",
		"upstream", deckServiceName, "upstream_path", path,
		"attempt", attempt, "max_retries", r.maxRetries)
	return nil
}

// executeDeckPostAttempt sends one POST. A JSON body goes out zstd-encoded
// once the target has advertised support; if the target then refuses the
// encoding (415) zstd is withdrawn and the same attempt resends identity.
func (r *RemoteDeckRecommender) executeDeckPostAttempt(ctx context.Context, baseURL, path string, payload []byte, contentType, logLabel string, attempt int, responseBody any) deckPostOutcome {
	body, coding := r.coding.PrepareBody(baseURL, contentType, payload)
	outcome, refused := r.sendDeckPost(ctx, baseURL, path, body, coding, contentType, logLabel, attempt, responseBody)
	if !refused {
		return outcome
	}
	r.coding.Reject(baseURL)
	r.logger.InfoContext(ctx, logLabel+" refused zstd body; resending identity",
		"upstream", deckServiceName, "upstream_path", path)
	outcome, _ = r.sendDeckPost(ctx, baseURL, path, payload, "", contentType, logLabel, attempt, responseBody)
	return outcome
}

func (r *RemoteDeckRecommender) sendDeckPost(ctx context.Context, baseURL, path string, payload []byte, coding, contentType, logLabel string, attempt int, responseBody any) (deckPostOutcome, bool) {
	httpCtx, timing := upstreamcall.Start(ctx)
	req, err := http.NewRequestWithContext(httpCtx, http.MethodPost, baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return deckPostOutcome{err: err, done: true}, false
	}
	req.Header.Set("Content-Type", contentType)
	httpcoding.SetRequestHeaders(req.Header, coding)
	start := time.Now()
	finishHTTP := commandtrace.MeasureOperation(ctx, deckHTTPStage)
	call := upstreamcall.Call{Op: deckHTTPStage, Target: r.targetNameFor(baseURL), Path: path, RequestBytes: len(payload)}
	resp, err := r.client.Do(req)
	if err != nil {
		finishHTTP()
		call.Err = err
		upstreamcall.Record(ctx, timing, time.Now(), call)
		return r.deckTransportError(ctx, path, logLabel, attempt, time.Since(start), err), false
	}
	r.coding.Observe(baseURL, resp.Header)
	body, truncated, readErr := readDeckResponseBody(resp.Body)
	resp.Body.Close()
	finishHTTP()
	call.StatusCode, call.ResponseBytes, call.Node, call.Err = resp.StatusCode, len(body), resp.Header.Get(upstreamcall.NodeHeader), readErr
	upstreamcall.Record(ctx, timing, time.Now(), call)
	if readErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return deckPostOutcome{err: ctxErr, done: true}, false
		}
		return deckPostOutcome{err: readErr}, false
	}
	if truncated {
		return deckPostOutcome{err: fmt.Errorf("deck-service response exceeded %d bytes", maxDeckResponseBodyBytes), done: true}, false
	}
	if httpcoding.RefusedEncoding(resp.StatusCode, coding) {
		return deckPostOutcome{}, true
	}
	body, err = httpcoding.DecodeBody(resp.Header.Get("Content-Encoding"), body, maxDeckResponseBodyBytes)
	if err != nil {
		return deckPostOutcome{err: fmt.Errorf("deck-service response: %w", err), done: true}, false
	}
	return r.handleDeckPostResponse(ctx, path, logLabel, attempt, time.Since(start), resp.StatusCode, body, responseBody), false
}

// targetNameFor names the configured target behind baseURL for call
// attribution; there are only a handful of targets.
func (r *RemoteDeckRecommender) targetNameFor(baseURL string) string {
	for _, state := range r.targetStates {
		if state != nil && state.target.BaseURL == baseURL {
			return state.target.Name
		}
	}
	return ""
}

func (r *RemoteDeckRecommender) deckTransportError(ctx context.Context, path, logLabel string, attempt int, elapsed time.Duration, err error) deckPostOutcome {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return deckPostOutcome{err: ctxErr, done: true}
	}
	r.logger.WarnContext(ctx, logLabel+" failed",
		"upstream", deckServiceName, "upstream_path", path, "attempt", attempt,
		"duration_ms", commandtrace.Milliseconds(elapsed), "error_type", fmt.Sprintf("%T", err))
	return deckPostOutcome{err: err, done: !isRetryableError(err, 0)}
}

func (r *RemoteDeckRecommender) handleDeckPostResponse(ctx context.Context, path, logLabel string, attempt int, elapsed time.Duration, statusCode int, body []byte, responseBody any) deckPostOutcome {
	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		r.logger.DebugContext(ctx, logLabel+" completed",
			"upstream", deckServiceName, "upstream_path", path,
			"status_code", statusCode, "duration_ms", commandtrace.Milliseconds(elapsed))
		if responseBody == nil || len(body) == 0 {
			return deckPostOutcome{done: true}
		}
		finishDecode := commandtrace.MeasureOperation(ctx, "deck.decode")
		err := json.Unmarshal(body, responseBody)
		finishDecode()
		return deckPostOutcome{err: err, done: true}
	}
	err := parseRemoteHTTPError(statusCode, body)
	r.logger.WarnContext(ctx, logLabel+" returned non-success status",
		"upstream", deckServiceName,
		"upstream_path", path,
		"attempt", attempt,
		"status_code", statusCode,
		"duration_ms", commandtrace.Milliseconds(elapsed),
		"response_bytes", len(body),
	)
	if isMissingUserdataHashError(err) || !isRetryableError(nil, statusCode) {
		return deckPostOutcome{err: err, done: true}
	}
	return deckPostOutcome{err: err}
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
	r.coding.Observe(baseURL, resp.Header)
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
