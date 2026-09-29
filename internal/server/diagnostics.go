package server

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"net/netip"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strings"
	"time"

	harukiConfig "haruki-cloud/config"
	json "haruki-cloud/internal/jsonutil"
	harukiLogger "haruki-cloud/utils/logger"
	"haruki-cloud/version"
)

const (
	diagnosticsReadHeaderTimeout = 5 * time.Second
	diagnosticsShutdownTimeout   = 5 * time.Second

	metricCPUTotal     = "/cpu/classes/total:cpu-seconds"
	metricCPUGCTotal   = "/cpu/classes/gc/total:cpu-seconds"
	metricGOGC         = "/gc/gogc:percent"
	metricLiveHeap     = "/gc/heap/live:bytes"
	metricGoroutines   = "/sched/goroutines:goroutines"
	metricTotalMemory  = "/memory/classes/total:bytes"
	metricGCCycleCount = "/gc/cycles/total:gc-cycles"
)

var errDiagnosticsNotLoopback = errors.New("diagnostics listen address is not loopback")

// runtimeSnapshot is the /debug/runtime payload. Byte counts come from
// runtime.MemStats; the CPU classes, GOGC and live heap come from
// runtime/metrics. gc_cpu_seconds and total_cpu_seconds are cumulative since
// process start, so sample twice and take the delta for a window.
type runtimeSnapshot struct {
	Timestamp         time.Time `json:"timestamp"`
	Version           string    `json:"version"`
	GoVersion         string    `json:"go_version"`
	NumCPU            int       `json:"num_cpu"`
	GOMAXPROCS        int       `json:"gomaxprocs"`
	Goroutines        int       `json:"goroutines"`
	GOGC              int       `json:"gogc"`
	MemoryLimitBytes  int64     `json:"memory_limit_bytes"`
	HeapAllocBytes    uint64    `json:"heap_alloc_bytes"`
	HeapLiveBytes     uint64    `json:"heap_live_bytes"`
	HeapInuseBytes    uint64    `json:"heap_inuse_bytes"`
	HeapIdleBytes     uint64    `json:"heap_idle_bytes"`
	HeapReleasedBytes uint64    `json:"heap_released_bytes"`
	HeapSysBytes      uint64    `json:"heap_sys_bytes"`
	StackInuseBytes   uint64    `json:"stack_inuse_bytes"`
	SysBytes          uint64    `json:"sys_bytes"`
	RuntimeTotalBytes uint64    `json:"runtime_total_bytes"`
	NextGCBytes       uint64    `json:"next_gc_bytes"`
	NumGC             uint32    `json:"num_gc"`
	NumForcedGC       uint32    `json:"num_forced_gc"`
	LastGC            time.Time `json:"last_gc"`
	PauseTotalSeconds float64   `json:"pause_total_seconds"`
	GCCPUFraction     float64   `json:"gc_cpu_fraction"`
	GCCPUSeconds      float64   `json:"gc_cpu_seconds"`
	TotalCPUSeconds   float64   `json:"total_cpu_seconds"`
}

// collectRuntimeSnapshot reads MemStats (a brief stop-the-world) and the
// runtime/metrics samples; metrics the runtime does not support stay zero.
func collectRuntimeSnapshot(now time.Time) runtimeSnapshot {
	samples := []metrics.Sample{
		{Name: metricCPUTotal},
		{Name: metricCPUGCTotal},
		{Name: metricGOGC},
		{Name: metricLiveHeap},
		{Name: metricGoroutines},
		{Name: metricTotalMemory},
		{Name: metricGCCycleCount},
	}
	metrics.Read(samples)
	byName := make(map[string]metrics.Value, len(samples))
	for _, s := range samples {
		byName[s.Name] = s.Value
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	snap := runtimeSnapshot{
		Timestamp:         now,
		Version:           version.Get(),
		GoVersion:         runtime.Version(),
		NumCPU:            runtime.NumCPU(),
		GOMAXPROCS:        runtime.GOMAXPROCS(0),
		Goroutines:        runtime.NumGoroutine(),
		GOGC:              int(metricUint64(byName[metricGOGC])),
		MemoryLimitBytes:  debug.SetMemoryLimit(-1),
		HeapAllocBytes:    ms.HeapAlloc,
		HeapLiveBytes:     metricUint64(byName[metricLiveHeap]),
		HeapInuseBytes:    ms.HeapInuse,
		HeapIdleBytes:     ms.HeapIdle,
		HeapReleasedBytes: ms.HeapReleased,
		HeapSysBytes:      ms.HeapSys,
		StackInuseBytes:   ms.StackInuse,
		SysBytes:          ms.Sys,
		RuntimeTotalBytes: metricUint64(byName[metricTotalMemory]),
		NextGCBytes:       ms.NextGC,
		NumGC:             ms.NumGC,
		NumForcedGC:       ms.NumForcedGC,
		PauseTotalSeconds: float64(ms.PauseTotalNs) / float64(time.Second),
		GCCPUFraction:     ms.GCCPUFraction,
		GCCPUSeconds:      metricFloat64(byName[metricCPUGCTotal]),
		TotalCPUSeconds:   metricFloat64(byName[metricCPUTotal]),
	}
	if ms.LastGC != 0 {
		snap.LastGC = time.Unix(0, int64(ms.LastGC)).UTC()
	}
	if snap.TotalCPUSeconds > 0 {
		snap.GCCPUFraction = snap.GCCPUSeconds / snap.TotalCPUSeconds
	}
	return snap
}

func metricUint64(v metrics.Value) uint64 {
	if v.Kind() == metrics.KindUint64 {
		return v.Uint64()
	}
	return 0
}

func metricFloat64(v metrics.Value) float64 {
	if v.Kind() == metrics.KindFloat64 {
		return v.Float64()
	}
	return 0
}

func runtimeSnapshotHandler(w http.ResponseWriter, r *http.Request) {
	body, err := json.MarshalIndent(collectRuntimeSnapshot(time.Now().UTC()), "", "  ")
	if err != nil {
		http.Error(w, "failed to encode runtime snapshot", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// newDiagnosticsMux registers the pprof, expvar and runtime handlers on a
// private mux; nothing is served from http.DefaultServeMux.
func newDiagnosticsMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/vars", expvar.Handler())
	mux.HandleFunc("/debug/runtime", runtimeSnapshotHandler)
	return mux
}

// diagnosticsAddrIsLoopback reports whether addr binds only a loopback host:
// "localhost", 127.0.0.0/8 or ::1 (with or without a zone). An empty host
// binds every interface and is therefore not loopback.
func diagnosticsAddrIsLoopback(addr string) (bool, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false, err
	}
	if strings.EqualFold(host, "localhost") {
		return true, nil
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false, nil
	}
	return ip.IsLoopback(), nil
}

func validateDiagnosticsAddr(cfg harukiConfig.DiagnosticsConfig) error {
	loopback, err := diagnosticsAddrIsLoopback(cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("invalid diagnostics listen address: %w", err)
	}
	if !loopback && !cfg.AllowNonLoopback {
		return errDiagnosticsNotLoopback
	}
	return nil
}

// newDiagnosticsServer validates the address and binds it so a bad address or
// an occupied port is reported at startup rather than from the serving
// goroutine. Handlers such as /debug/pprof/profile block for the requested
// duration, so no write timeout is applied.
func newDiagnosticsServer(cfg harukiConfig.DiagnosticsConfig) (*http.Server, net.Listener, error) {
	if err := validateDiagnosticsAddr(cfg); err != nil {
		return nil, nil, err
	}
	listener, err := net.Listen("tcp", strings.TrimSpace(cfg.ListenAddr))
	if err != nil {
		return nil, nil, err
	}
	server := &http.Server{
		Handler:           newDiagnosticsMux(),
		ReadHeaderTimeout: diagnosticsReadHeaderTimeout,
	}
	return server, listener, nil
}

// startDiagnosticsServer starts the opt-in diagnostics listener and returns a
// stop function for the shutdown path. A disabled or refused listener returns
// a no-op stop; refusal is logged and never fatal, so the service keeps
// running without diagnostics.
func startDiagnosticsServer(mainLogger *harukiLogger.Logger, cfg harukiConfig.DiagnosticsConfig) func() {
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		return func() {}
	}
	server, listener, err := newDiagnosticsServer(cfg)
	if err != nil {
		attrs := []any{"listen_addr", cfg.ListenAddr, "error_type", fmt.Sprintf("%T", err)}
		if errors.Is(err, errDiagnosticsNotLoopback) {
			attrs = append(attrs, "hint", "bind 127.0.0.1 or set diagnostics.allow_non_loopback")
		}
		mainLogger.Error("diagnostics server refused to start", attrs...)
		return func() {}
	}
	mainLogger.Info("diagnostics server starting", "listen_addr", listener.Addr().String(), "loopback_only", !cfg.AllowNonLoopback)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			mainLogger.Error("diagnostics server stopped unexpectedly", "error_type", fmt.Sprintf("%T", err))
		}
	}()
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), diagnosticsShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			mainLogger.Warn("diagnostics server shutdown incomplete", "error_type", fmt.Sprintf("%T", err))
			_ = server.Close()
		}
	}
}
