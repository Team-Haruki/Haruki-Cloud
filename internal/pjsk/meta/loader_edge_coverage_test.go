package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoaderConfigurationAndCacheGuards(t *testing.T) {
	loader := NewLoader(nil, nil, WithOutputDir("  "+t.TempDir()+"  "))
	if loader.outputDir == "" {
		t.Fatal("trimmed output directory was not configured")
	}
	if err := loader.load(context.Background(), "unknown"); err == nil {
		t.Fatal("unknown region unexpectedly loaded")
	}
	if got := (*Loader)(nil).Get("jp"); got != nil {
		t.Fatalf("nil loader cache = %v", got)
	}
	if got := (*Loader)(nil).View("jp"); got != nil {
		t.Fatalf("nil loader view = %v", got)
	}
	if got := loader.Get("jp"); got != nil {
		t.Fatalf("missing cache = %v", got)
	}
	if got := loader.View("jp"); got != nil {
		t.Fatalf("missing view = %v", got)
	}
}

func TestLoaderPersistenceGuardEdges(t *testing.T) {
	if err := (*Loader)(nil).persist("jp", []byte("[]")); err != nil {
		t.Fatalf("nil loader persist = %v", err)
	}
	if err := NewLoader(nil).persist("jp", []byte("[]")); err != nil {
		t.Fatalf("disabled persist = %v", err)
	}
	loader := NewLoader(nil, WithOutputDir(t.TempDir()))
	if err := loader.persist("unknown", []byte("[]")); err == nil {
		t.Fatal("unknown persist region unexpectedly succeeded")
	}
	if err := NewLoader(nil).loadPersisted("jp"); err == nil {
		t.Fatal("unconfigured persisted load unexpectedly succeeded")
	}
	if err := loader.loadPersisted("unknown"); err == nil {
		t.Fatal("unknown persisted region unexpectedly loaded")
	}
}

func TestLoaderRejectsMalformedPersistedPayload(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "music_metas.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	loader := NewLoader(nil, WithOutputDir(dir))
	if err := loader.loadPersisted("jp"); err == nil {
		t.Fatal("malformed persisted payload unexpectedly loaded")
	}
}

func TestLoaderSendsConditionalHeadersAndHandlesNotModified(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.Header().Set("ETag", "v1")
			w.Header().Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if r.Header.Get("If-None-Match") != "v1" || r.Header.Get("If-Modified-Since") == "" {
			t.Errorf("conditional headers = %#v", r.Header)
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	loader := NewLoader(nil)
	if err := loader.loadRemote(context.Background(), "jp", server.URL, nil); err != nil {
		t.Fatalf("initial remote load = %v", err)
	}
	entry := loader.cache["jp"]
	if err := loader.loadRemote(context.Background(), "jp", server.URL, entry); err != nil {
		t.Fatalf("conditional remote load = %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d", requestCount)
	}
}

func TestLoaderRemoteResponseErrors(t *testing.T) {
	badJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	defer badJSON.Close()
	loader := NewLoader(nil)
	if err := loader.loadRemote(context.Background(), "jp", badJSON.URL, nil); err == nil {
		t.Fatal("malformed remote payload unexpectedly loaded")
	}

	teapot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer teapot.Close()
	if err := loader.loadRemote(context.Background(), "jp", teapot.URL, nil); err == nil {
		t.Fatal("unexpected HTTP status was accepted")
	}
}

func TestLoaderLoadAllReportsRegionalFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	original := make(map[string]string, len(regionURLs))
	for region, value := range regionURLs {
		original[region] = value
		regionURLs[region] = server.URL
	}
	defer func() {
		for region, value := range original {
			regionURLs[region] = value
		}
	}()
	if err := NewLoader(nil).LoadAll(context.Background()); err == nil {
		t.Fatal("regional load failures were not reported")
	}
}
