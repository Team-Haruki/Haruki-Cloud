package sekai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haruki-cloud/config"

	"github.com/klauspost/compress/zstd"
)

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
