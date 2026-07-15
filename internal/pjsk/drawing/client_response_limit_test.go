package drawing

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestDrawingClientLimitsResponseBodies(t *testing.T) {
	client := NewHarukiDrawingClient("http://drawing.invalid")
	if got := client.client.ResponseBodyLimit; got != drawingMaxResponseBytes {
		t.Fatalf("response body limit = %d, want %d", got, drawingMaxResponseBytes)
	}
}

func TestDrawingClientRejectsOversizedResponseWithoutLeakingBody(t *testing.T) {
	const (
		testLimit = 32
		secret    = "drawing-secret-that-must-not-appear"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat(secret, 2)))
	}))
	defer server.Close()

	client := NewHarukiDrawingClient(server.URL)
	client.client.SetResponseBodyLimit(testLimit)
	_, err := client.postPrepared("/render", map[string]any{"value": 1})
	if err == nil {
		t.Fatal("expected oversized response error")
	}
	if !errors.Is(err, resty.ErrResponseBodyTooLarge) {
		t.Fatalf("error = %v, want resty.ErrResponseBodyTooLarge", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked upstream response body: %v", err)
	}
}

func TestDrawingClientClassifiesInsufficientDataWithoutLeakingBody(t *testing.T) {
	const upstreamDetail = "single positional indexer is out-of-bounds"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"detail":"` + upstreamDetail + `"}`))
	}))
	defer server.Close()

	client := NewHarukiDrawingClient(server.URL)
	_, err := client.postPrepared("/render", map[string]any{"value": 1})
	if !errors.Is(err, ErrDrawingDataInsufficient) {
		t.Fatalf("error = %v, want ErrDrawingDataInsufficient", err)
	}
	if strings.Contains(err.Error(), upstreamDetail) {
		t.Fatalf("error leaked upstream response detail: %v", err)
	}
}
