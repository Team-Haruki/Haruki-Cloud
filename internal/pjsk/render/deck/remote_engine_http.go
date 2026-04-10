package deck

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

// isRetryableError returns true for transient errors that warrant a retry.
func isRetryableError(err error, statusCode int) bool {
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) {
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

func (r *RemoteDeckRecommender) postJSON(path string, requestBody any, responseBody any) error {
	body, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(r.retryWaitTime)
			r.logger.Debugf("retry %d/%d for POST %s", attempt, r.maxRetries, path)
		}

		req, err := http.NewRequest(http.MethodPost, r.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		start := time.Now()
		resp, err := r.client.Do(req)
		elapsed := time.Since(start)

		if err != nil {
			lastErr = err
			r.logger.Warnf("POST %s attempt %d failed after %v: %v", path, attempt, elapsed, err)
			if isRetryableError(err, 0) {
				continue
			}
			return err
		}

		payload, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			r.logger.Debugf("POST %s succeeded in %v", path, elapsed)
			if responseBody == nil || len(payload) == 0 {
				return nil
			}
			return json.Unmarshal(payload, responseBody)
		}

		lastErr = parseRemoteHTTPError(resp.StatusCode, payload)
		r.logger.Warnf("POST %s attempt %d returned HTTP %d after %v", path, attempt, resp.StatusCode, elapsed)
		if !isRetryableError(nil, resp.StatusCode) {
			return lastErr
		}
	}
	return lastErr
}

func (r *RemoteDeckRecommender) postBinary(path string, payload []byte, responseBody any) error {
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(r.retryWaitTime)
			r.logger.Debugf("retry %d/%d for POST %s (binary)", attempt, r.maxRetries, path)
		}

		req, err := http.NewRequest(http.MethodPost, r.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/octet-stream")

		start := time.Now()
		resp, err := r.client.Do(req)
		elapsed := time.Since(start)

		if err != nil {
			lastErr = err
			r.logger.Warnf("POST %s (binary) attempt %d failed after %v: %v", path, attempt, elapsed, err)
			if isRetryableError(err, 0) {
				continue
			}
			return err
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			r.logger.Debugf("POST %s (binary) succeeded in %v", path, elapsed)
			if responseBody == nil || len(body) == 0 {
				return nil
			}
			return json.Unmarshal(body, responseBody)
		}

		lastErr = parseRemoteHTTPError(resp.StatusCode, body)
		r.logger.Warnf("POST %s (binary) attempt %d returned HTTP %d after %v", path, attempt, resp.StatusCode, elapsed)
		if !isRetryableError(nil, resp.StatusCode) {
			return lastErr
		}
	}
	return lastErr
}

func parseRemoteHTTPError(statusCode int, payload []byte) error {
	var remoteErr remoteErrorResponse
	if json.Unmarshal(payload, &remoteErr) == nil && strings.TrimSpace(remoteErr.Error) != "" {
		return fmt.Errorf("%s", remoteErr.Error)
	}
	if trimmed := strings.TrimSpace(string(payload)); trimmed != "" {
		return fmt.Errorf("deck-service returned HTTP %d: %s", statusCode, trimmed)
	}
	return fmt.Errorf("deck-service returned HTTP %d", statusCode)
}

func buildMultipartPayload(segments ...[]byte) []byte {
	var raw bytes.Buffer
	for _, segment := range segments {
		if err := binary.Write(&raw, binary.BigEndian, uint32(len(segment))); err != nil {
			return nil
		}
		if _, err := raw.Write(segment); err != nil {
			return nil
		}
	}

	var compressed bytes.Buffer
	writer, err := zstd.NewWriter(&compressed)
	if err != nil {
		return nil
	}
	if _, err := writer.Write(raw.Bytes()); err != nil {
		_ = writer.Close()
		return nil
	}
	if err := writer.Close(); err != nil {
		return nil
	}
	return compressed.Bytes()
}
