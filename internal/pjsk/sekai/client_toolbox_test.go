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

func TestToolboxConditionalFetchAnswers304WithSingleRequest(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RawQuery)
		if got := r.URL.Query().Get("known_upload_time"); got != "1710000000" {
			t.Fatalf("known_upload_time = %q, want 1710000000", got)
		}
		w.Header().Set("X-Upload-Time", "1710000000")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL, ConditionalFetch: true})
	ctx, trace := commandtrace.WithTrace(context.Background())
	data, notModified, err := client.GetSuiteDataConditionalContext(ctx, "jp", 123456789, "qq", "10001", 1710000000)
	if err != nil {
		t.Fatalf("GetSuiteDataConditionalContext() error = %v", err)
	}
	if !notModified || data != nil {
		t.Fatalf("expected not-modified with no payload, got notModified=%v data=%q", notModified, data)
	}
	if len(requests) != 1 {
		t.Fatalf("conditional fetch must be a single request, got %d", len(requests))
	}
	found := false
	for _, op := range trace.Snapshot().Operations {
		if op.Name == "toolbox.conditional_not_modified" && op.Count == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a toolbox.conditional_not_modified operation")
	}
}

func TestToolboxConditionalFetchReturnsChangedPayload(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get("known_upload_time"); got != "1710000000" {
			t.Fatalf("known_upload_time = %q, want 1710000000", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upload_time":1710000500}`))
	}))
	defer server.Close()

	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL, ConditionalFetch: true})
	ctx, trace := commandtrace.WithTrace(context.Background())
	data, notModified, err := client.GetSuiteDataConditionalContext(ctx, "jp", 123456789, "qq", "10001", 1710000000)
	if err != nil {
		t.Fatalf("GetSuiteDataConditionalContext() error = %v", err)
	}
	if notModified {
		t.Fatal("changed payload must not report not-modified")
	}
	if string(data) != `{"upload_time":1710000500}` {
		t.Fatalf("unexpected payload: %s", data)
	}
	if requests != 1 {
		t.Fatalf("changed conditional fetch must be a single request, got %d", requests)
	}
	found := false
	for _, op := range trace.Snapshot().Operations {
		if op.Name == "toolbox.conditional_changed" && op.Count == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a toolbox.conditional_changed operation")
	}
}

func TestToolboxConditionalFetchEmulatesProbeWhenDisabled(t *testing.T) {
	uploadTime := "1710000000"
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RawQuery)
		if got := r.URL.Query().Get("known_upload_time"); got != "" {
			t.Fatalf("disabled conditional fetch must not send known_upload_time, got %q", got)
		}
		if r.URL.Query().Get("key") == "upload_time" {
			_, _ = w.Write([]byte(uploadTime))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upload_time":1710000500}`))
	}))
	defer server.Close()

	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL})

	// Unchanged: one probe answers the validation, no payload transfer.
	data, notModified, err := client.GetSuiteDataConditionalContext(context.Background(), "jp", 123456789, "qq", "10001", 1710000000)
	if err != nil {
		t.Fatalf("unchanged emulation error = %v", err)
	}
	if !notModified || data != nil {
		t.Fatalf("unchanged emulation: notModified=%v data=%q", notModified, data)
	}
	if len(requests) != 1 {
		t.Fatalf("unchanged emulation must issue exactly the probe, got %d requests", len(requests))
	}

	// Changed: probe then full fetch.
	uploadTime = "1710000500"
	data, notModified, err = client.GetSuiteDataConditionalContext(context.Background(), "jp", 123456789, "qq", "10001", 1710000000)
	if err != nil {
		t.Fatalf("changed emulation error = %v", err)
	}
	if notModified || string(data) != `{"upload_time":1710000500}` {
		t.Fatalf("changed emulation: notModified=%v data=%q", notModified, data)
	}
	if len(requests) != 3 {
		t.Fatalf("changed emulation must issue probe+fetch, got %d total requests", len(requests))
	}
}

func TestToolboxConditionalFetchZeroKnownFetchesDirectly(t *testing.T) {
	for _, conditional := range []bool{true, false} {
		var requests []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r.URL.RawQuery)
			if got := r.URL.Query().Get("known_upload_time"); got != "" {
				t.Fatalf("known=0 must not send known_upload_time, got %q", got)
			}
			if got := r.URL.Query().Get("key"); got != "" {
				t.Fatalf("known=0 must not probe, got key=%q", got)
			}
			_, _ = w.Write([]byte(`{"upload_time":1710000000}`))
		}))

		client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL, ConditionalFetch: conditional})
		data, notModified, err := client.GetSuiteDataConditionalContext(context.Background(), "jp", 123456789, "qq", "10001", 0)
		if err != nil {
			t.Fatalf("conditional=%v error = %v", conditional, err)
		}
		if notModified || string(data) != `{"upload_time":1710000000}` {
			t.Fatalf("conditional=%v: notModified=%v data=%q", conditional, notModified, data)
		}
		if len(requests) != 1 {
			t.Fatalf("conditional=%v: requests = %d, want 1", conditional, len(requests))
		}
		server.Close()
	}
}

func TestToolboxConditionalFetchEmulationPropagatesProbeErrors(t *testing.T) {
	t.Run("authorization error", func(t *testing.T) {
		var requests int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			if r.URL.Query().Get("key") != "upload_time" {
				t.Fatal("a failing probe must not be followed by a full fetch")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"invalid platform or platform_user_id"}`))
		}))
		defer server.Close()

		client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL})
		data, notModified, err := client.GetSuiteDataConditionalContext(context.Background(), "jp", 123456789, "qq", "10001", 1710000000)
		if !errors.Is(err, ErrInvalidPlatformUser) {
			t.Fatalf("probe error must propagate, got %v", err)
		}
		if notModified || data != nil {
			t.Fatalf("probe error must not report data (notModified=%v data=%q)", notModified, data)
		}
		if requests != 1 {
			t.Fatalf("failing probe must issue exactly one request, got %d", requests)
		}
	})

	t.Run("unparseable upload_time", func(t *testing.T) {
		var requests int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			if r.URL.Query().Get("key") != "upload_time" {
				t.Fatal("an unparseable probe must not be followed by a full fetch")
			}
			_, _ = w.Write([]byte("not-a-number"))
		}))
		defer server.Close()

		client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL})
		data, notModified, err := client.GetSuiteDataConditionalContext(context.Background(), "jp", 123456789, "qq", "10001", 1710000000)
		if err == nil || !strings.Contains(err.Error(), "invalid upload_time") {
			t.Fatalf("parse error must propagate, got %v", err)
		}
		if notModified || data != nil {
			t.Fatalf("parse error must not report data (notModified=%v data=%q)", notModified, data)
		}
		if requests != 1 {
			t.Fatalf("unparseable probe must issue exactly one request, got %d", requests)
		}
	})
}

func TestToolboxConditionalFetchMapsErrorStatusWithoutServingNotModified(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get("known_upload_time"); got != "1710000000" {
			t.Fatalf("known_upload_time = %q, want 1710000000", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"invalid platform or platform_user_id"}`))
	}))
	defer server.Close()

	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL, ConditionalFetch: true})
	data, notModified, err := client.GetSuiteDataConditionalContext(context.Background(), "jp", 123456789, "qq", "10001", 1710000000)
	if !errors.Is(err, ErrInvalidPlatformUser) {
		t.Fatalf("authorization failure must propagate, got %v", err)
	}
	if notModified {
		t.Fatal("an error status must never be reported as not-modified")
	}
	if data != nil {
		t.Fatalf("an error status must not carry data, got %q", data)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestToolboxUnexpected304WithoutKnownUploadTimeIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL})
	data, err := client.GetSuiteDataContext(context.Background(), "jp", 123456789, "qq", "10001")
	var toolboxErr *ToolboxAPIError
	if !errors.As(err, &toolboxErr) || toolboxErr.StatusCode != http.StatusNotModified {
		t.Fatalf("a 304 without known_upload_time must surface as a typed error, got data=%q err=%v", data, err)
	}
}
