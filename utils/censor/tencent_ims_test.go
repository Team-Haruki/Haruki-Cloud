package censor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"haruki-cloud/internal/observability/commandtrace"
)

func TestTencentIMSContextAndTrace(t *testing.T) {
	type contextKey string
	const key contextKey = "request"
	const value = "image-moderation"

	client := NewTencentIMSClient("secret-id", "secret-key", "ap-guangzhou", "")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Context().Value(key); got != value {
			return nil, fmt.Errorf("request context value = %v, want %q", got, value)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"Response":{"Suggestion":"Pass"}}`)),
			Request:    req,
		}, nil
	})}

	base := context.WithValue(context.Background(), key, value)
	ctx, trace := commandtrace.WithTrace(base)
	suggestion, err := client.ImageModerationURL(ctx, "https://example.test/image.png")
	if err != nil {
		t.Fatalf("ImageModerationURL() error = %v", err)
	}
	if suggestion != IMSSuggestionPass {
		t.Fatalf("suggestion = %q, want %q", suggestion, IMSSuggestionPass)
	}
	snapshot := trace.Snapshot()
	if got := operationCount(snapshot, "censor.http"); got != 1 {
		t.Fatalf("censor.http count = %d, want 1", got)
	}
	if got := operationCount(snapshot, "censor.decode"); got != 1 {
		t.Fatalf("censor.decode count = %d, want 1", got)
	}
}

func TestTencentIMSRejectsOversizedResponse(t *testing.T) {
	client := NewTencentIMSClient("secret-id", "secret-key", "ap-guangzhou", "")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", imsMaxResponseBytes+1))),
			Request:    req,
		}, nil
	})}

	_, err := client.ImageModerationURL(context.Background(), "https://example.test/image.png")
	if !errors.Is(err, errTencentIMSResponseTooLarge) {
		t.Fatalf("ImageModerationURL() error = %v, want response size error", err)
	}
}

func TestTencentIMSErrorsDoNotLeakResponseOrRequestData(t *testing.T) {
	t.Run("API response", func(t *testing.T) {
		const sensitiveMarker = "private-upstream-error-message"
		client := NewTencentIMSClient("secret-id", "secret-key", "ap-guangzhou", "")
		client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"Response":{"Error":{"Code":"InternalError","Message":"` + sensitiveMarker + `"}}}`,
				)),
				Request: req,
			}, nil
		})}

		_, err := client.ImageModerationURL(context.Background(), "https://example.test/image.png")
		if !errors.Is(err, errTencentIMSAPIRejected) {
			t.Fatalf("ImageModerationURL() error = %v, want API rejection", err)
		}
		if strings.Contains(err.Error(), sensitiveMarker) {
			t.Fatalf("error leaked API response: %v", err)
		}
	})

	t.Run("transport error", func(t *testing.T) {
		const sensitiveMarker = "private-image-url-token"
		client := NewTencentIMSClient("secret-id", "secret-key", "ap-guangzhou", "")
		client.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("failed request containing %s", sensitiveMarker)
		})}

		_, err := client.ImageModerationURL(context.Background(), "https://example.test/"+sensitiveMarker)
		if !errors.Is(err, errTencentIMSRequestFailed) {
			t.Fatalf("ImageModerationURL() error = %v, want sanitized request error", err)
		}
		if strings.Contains(err.Error(), sensitiveMarker) {
			t.Fatalf("error leaked request data: %v", err)
		}
	})
}
