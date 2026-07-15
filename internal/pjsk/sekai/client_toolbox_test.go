package sekai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"

	"github.com/go-resty/resty/v2"
	"github.com/klauspost/compress/zstd"
)

func TestToolboxClientLimitsCompressedResponseBody(t *testing.T) {
	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: "http://toolbox.invalid"})
	if client.http.ResponseBodyLimit != toolboxMaxResponseBytes {
		t.Fatalf("response body limit = %d, want %d", client.http.ResponseBodyLimit, toolboxMaxResponseBytes)
	}
}

func TestToolboxDecompressionRejectsOversizedPayload(t *testing.T) {
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("zstd.NewWriter() error = %v", err)
	}
	defer encoder.Close()
	compressed := encoder.EncodeAll([]byte("payload larger than test limit"), nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "zstd")
		_, _ = w.Write(compressed)
	}))
	defer server.Close()
	resp, err := resty.New().R().Get(server.URL)
	if err != nil {
		t.Fatalf("GET compressed response: %v", err)
	}

	if _, err := decompressContextLimit(context.Background(), resp, 8); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized decompression error = %v", err)
	}
}

func TestToolboxPrivateDataValuesAcceptsAndDecodesZstd(t *testing.T) {
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("zstd.NewWriter() error = %v", err)
	}
	defer encoder.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/private/game-data/jp/suite/123456789" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "toolbox-secret" {
			t.Fatalf("unexpected authorization header: %q", auth)
		}
		if accept := r.Header.Get("Accept-Encoding"); !strings.Contains(accept, "zstd") {
			t.Fatalf("expected zstd accept encoding, got %q", accept)
		}
		if got := r.URL.Query().Get("key"); got != "upload_time,version" {
			t.Fatalf("unexpected key query: %q", got)
		}

		w.Header().Set("Content-Encoding", "zstd")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encoder.EncodeAll([]byte(`{"upload_time":1774339266,"version":"1"}`), nil))
	}))
	defer server.Close()

	client := NewToolboxClient(&config.ToolboxConfig{
		BaseURL:  server.URL,
		APIToken: "toolbox-secret",
	})

	data, err := client.GetPrivateDataValues("jp", ToolboxDataTypeSuite, 123456789, "qq", "10001", "upload_time", "version")
	if err != nil {
		t.Fatalf("GetPrivateDataValues() error = %v", err)
	}
	if got := string(data); got != `{"upload_time":1774339266,"version":"1"}` {
		t.Fatalf("unexpected decoded body: %s", got)
	}
}

func TestToolboxPrivateDataPreservesServiceUnavailableMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"user store unavailable"}`))
	}))
	defer server.Close()

	client := NewToolboxClient(&config.ToolboxConfig{
		BaseURL:  server.URL,
		APIToken: "toolbox-secret",
	})

	_, err := client.GetPrivateData("jp", ToolboxDataTypeSuite, 123456789, "qq", "10001")
	var toolboxErr *ToolboxAPIError
	if !errors.As(err, &toolboxErr) {
		t.Fatalf("expected ToolboxAPIError, got %T (%v)", err, err)
	}
	if toolboxErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status: %d", toolboxErr.StatusCode)
	}
	if toolboxErr.Message != "user store unavailable" {
		t.Fatalf("unexpected message: %q", toolboxErr.Message)
	}
}

func TestToolboxPrivateDataContextSupportsCancellation(t *testing.T) {
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.GetSuiteDataContext(ctx, "jp", 123456789, "qq", "10001")
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("toolbox request did not start")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("GetSuiteDataContext() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("toolbox request did not stop after context cancellation")
	}
}

func TestToolboxPrivateDataContextRecordsHTTPAndDecompression(t *testing.T) {
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("zstd.NewWriter() error = %v", err)
	}
	defer encoder.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "zstd")
		_, _ = w.Write(encoder.EncodeAll([]byte(`{"ok":true}`), nil))
	}))
	defer server.Close()

	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL})
	ctx, trace := commandtrace.WithTrace(context.Background())
	if _, err := client.GetSuiteDataContext(ctx, "jp", 123456789, "qq", "10001"); err != nil {
		t.Fatalf("GetSuiteDataContext() error = %v", err)
	}

	operations := trace.Snapshot().Operations
	for _, name := range []string{"toolbox.http", "toolbox.decompress"} {
		found := false
		for _, operation := range operations {
			if operation.Name == name && operation.Count == 1 {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected one %s operation, got %+v", name, operations)
		}
	}
}
