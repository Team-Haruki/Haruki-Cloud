package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	harukiConfig "haruki-cloud/config"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/testutil"
	harukiLogger "haruki-cloud/utils/logger"
)

func TestDiagnosticsAddrIsLoopback(t *testing.T) {
	tests := []struct {
		addr     string
		loopback bool
		wantErr  bool
	}{
		{"127.0.0.1:6060", true, false},
		{"127.0.0.2:6060", true, false},
		{"[::1]:6060", true, false},
		{"[::1%lo0]:6060", true, false},
		{"localhost:6060", true, false},
		{"LOCALHOST:6060", true, false},
		{"0.0.0.0:6060", false, false},
		{"[::]:6060", false, false},
		{":6060", false, false},
		{"10.0.0.5:6060", false, false},
		{"cloud.internal:6060", false, false},
		{"6060", false, true},
		{"", false, true},
	}
	for _, tt := range tests {
		got, err := diagnosticsAddrIsLoopback(tt.addr)
		testutil.Check(t, (err != nil) == tt.wantErr, "diagnosticsAddrIsLoopback(%q) error = %v, wantErr %v", tt.addr, err, tt.wantErr)
		testutil.Check(t, got == tt.loopback, "diagnosticsAddrIsLoopback(%q) = %v, want %v", tt.addr, got, tt.loopback)
	}
}

func TestValidateDiagnosticsAddrEnforcesLoopback(t *testing.T) {
	err := validateDiagnosticsAddr(harukiConfig.DiagnosticsConfig{ListenAddr: "0.0.0.0:6060"})
	testutil.Require(t, err == errDiagnosticsNotLoopback, "non-loopback without allow flag: err = %v, want errDiagnosticsNotLoopback", err)

	err = validateDiagnosticsAddr(harukiConfig.DiagnosticsConfig{ListenAddr: "0.0.0.0:6060", AllowNonLoopback: true})
	testutil.Require(t, err == nil, "non-loopback with allow flag: err = %v, want nil", err)

	err = validateDiagnosticsAddr(harukiConfig.DiagnosticsConfig{ListenAddr: "127.0.0.1:6060"})
	testutil.Require(t, err == nil, "loopback: err = %v, want nil", err)

	err = validateDiagnosticsAddr(harukiConfig.DiagnosticsConfig{ListenAddr: "not an address"})
	testutil.Require(t, err != nil && err != errDiagnosticsNotLoopback, "malformed address: err = %v, want parse error", err)
}

func TestStartDiagnosticsServerRefusesNonLoopbackAndKeepsRunning(t *testing.T) {
	var logs bytes.Buffer
	mainLogger := harukiLogger.NewLogger("Main", "DEBUG", &logs)
	stop := startDiagnosticsServer(mainLogger, harukiConfig.DiagnosticsConfig{ListenAddr: "0.0.0.0:0"})
	testutil.Require(t, stop != nil, "stop function must be returned even when refused")
	stop()
	testutil.Require(t, strings.Contains(logs.String(), "diagnostics server refused to start"), "expected refusal log, got: %s", logs.String())
}

func TestStartDiagnosticsServerDisabledWhenEmpty(t *testing.T) {
	var logs bytes.Buffer
	mainLogger := harukiLogger.NewLogger("Main", "DEBUG", &logs)
	stop := startDiagnosticsServer(mainLogger, harukiConfig.DiagnosticsConfig{})
	stop()
	testutil.Require(t, !strings.Contains(logs.String(), "diagnostics server"), "empty listen_addr must not log a start or refusal, got: %s", logs.String())
}

func TestDiagnosticsRuntimeHandlerKeys(t *testing.T) {
	mux := newDiagnosticsMux()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/runtime", nil))
	testutil.Require(t, rec.Code == http.StatusOK, "/debug/runtime status = %d", rec.Code)
	testutil.Require(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json"), "content type = %q", rec.Header().Get("Content-Type"))

	var payload map[string]any
	testutil.Require(t, json.Unmarshal(rec.Body.Bytes(), &payload) == nil, "decode /debug/runtime: %s", rec.Body.String())
	for _, key := range []string{
		"timestamp", "version", "go_version", "num_cpu", "gomaxprocs", "goroutines", "gogc",
		"memory_limit_bytes", "heap_alloc_bytes", "heap_live_bytes", "heap_inuse_bytes", "heap_sys_bytes",
		"next_gc_bytes", "num_gc", "gc_cpu_fraction", "gc_cpu_seconds", "total_cpu_seconds",
	} {
		_, ok := payload[key]
		testutil.Check(t, ok, "/debug/runtime missing key %q", key)
	}
	testutil.Check(t, payload["goroutines"].(float64) >= 1, "goroutines = %v", payload["goroutines"])
	testutil.Check(t, payload["heap_alloc_bytes"].(float64) > 0, "heap_alloc_bytes = %v", payload["heap_alloc_bytes"])
	testutil.Check(t, payload["gogc"].(float64) > 0, "gogc = %v", payload["gogc"])
}

func TestDiagnosticsMuxServesPprofAndExpvar(t *testing.T) {
	mux := newDiagnosticsMux()
	for _, path := range []string{"/debug/pprof/", "/debug/pprof/heap?debug=1", "/debug/vars"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		testutil.Check(t, rec.Code == http.StatusOK, "%s status = %d", path, rec.Code)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/vars", nil))
	testutil.Check(t, strings.Contains(rec.Body.String(), "\"memstats\""), "/debug/vars missing memstats")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	testutil.Check(t, rec.Code == http.StatusNotFound, "unrelated path status = %d, want 404", rec.Code)
}

func TestStartDiagnosticsServerServesAndStops(t *testing.T) {
	var logs bytes.Buffer
	mainLogger := harukiLogger.NewLogger("Main", "DEBUG", &logs)
	server, listener, err := newDiagnosticsServer(harukiConfig.DiagnosticsConfig{ListenAddr: "127.0.0.1:0"})
	testutil.Require(t, err == nil, "newDiagnosticsServer: %v", err)
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + listener.Addr().String() + "/debug/runtime")
	testutil.Require(t, err == nil, "GET /debug/runtime: %v", err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	testutil.Require(t, resp.StatusCode == http.StatusOK, "status = %d body = %s", resp.StatusCode, body)
	testutil.Require(t, strings.Contains(string(body), "\"next_gc_bytes\""), "body = %s", body)

	stop := startDiagnosticsServer(mainLogger, harukiConfig.DiagnosticsConfig{ListenAddr: "127.0.0.1:0"})
	testutil.Require(t, strings.Contains(logs.String(), "diagnostics server starting"), "expected start log, got: %s", logs.String())
	stop()
	testutil.Require(t, !strings.Contains(logs.String(), "shutdown incomplete"), "unexpected shutdown warning: %s", logs.String())
}
