package costume

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type preview3DCoverageErrorReader struct{}

func (preview3DCoverageErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func preview3DCoverageRegistry() *preview3DRegistry {
	return &preview3DRegistry{
		partRegistryVersion: 2,
		characters: []preview3DCharacterEntry{{
			Character3DID:   1,
			CharacterID:     1,
			Unit:            "light_sound",
			BodyCostume3DID: 101,
			HeadCostume3DID: 102,
			HairCostume3DID: 103,
			Status:          "available",
		}},
		parts: []preview3DPartEntry{
			{Costume3DID: 101, Costume3DGroupID: 10_001, OutfitID: 10, PartType: "body", CharacterID: 1, Unit: "light_sound", ColorID: 1, Status: "available"},
			{Costume3DID: 102, Costume3DGroupID: 10_001, AccessoryID: 20, PartType: "head", CharacterID: 1, Unit: "light_sound", ColorID: 1, PackagePath: "parts/head/102", Status: "available"},
			{Costume3DID: 103, Costume3DGroupID: 10_001, PartType: "hair", CharacterID: 1, Unit: "light_sound", ColorID: 1, Status: "available"},
		},
	}
}

func cachedPreview3DServiceForCoverage(t *testing.T, registry *preview3DRegistry, baseURL string) (*Preview3DService, preview3DEndpoint) {
	t.Helper()
	service := NewPreview3DService(Preview3DConfig{
		Enabled:             true,
		EngineBaseURL:       baseURL,
		RegistryCacheTTL:    time.Hour,
		CaptureExistsTTL:    -1,
		CaptureCacheVersion: "coverage",
	})
	endpoint, err := service.endpointForRegion("jp")
	if err != nil {
		t.Fatalf("endpointForRegion: %v", err)
	}
	service.cached[endpoint.key()] = registry
	service.cachedAt[endpoint.key()] = time.Now()
	enEndpoint, err := service.endpointForRegion("en")
	if err != nil {
		t.Fatalf("en endpointForRegion: %v", err)
	}
	service.cached[enEndpoint.key()] = registry
	service.cachedAt[enEndpoint.key()] = time.Now()
	return service, endpoint
}

func preview3DPublicCalls() []struct {
	name string
	call func(*Preview3DService) error
} {
	return []struct {
		name string
		call func(*Preview3DService) error
	}{
		{name: "resolve", call: func(s *Preview3DService) error {
			_, err := s.ResolveQueryPreviewPath(context.Background(), "en", 101, Query{})
			return err
		}},
		{name: "ensure", call: func(s *Preview3DService) error {
			return s.EnsureQueryPreviewCapture(context.Background(), "en", 101, Query{})
		}},
		{name: "combo", call: func(s *Preview3DService) error {
			_, err := s.CaptureTemporaryCombo(context.Background(), "en", ComboQuery{Character3DID: 1})
			return err
		}},
		{name: "hair ids", call: func(s *Preview3DService) error { _, err := s.HairIDsForRole(context.Background(), "en", 1); return err }},
		{name: "accessory ids", call: func(s *Preview3DService) error {
			_, err := s.AccessoryIDsForRole(context.Background(), "en", 1)
			return err
		}},
		{name: "accessory costume", call: func(s *Preview3DService) error {
			_, err := s.AccessoryCostume3DIDForRole(context.Background(), "en", 20, 1, 1)
			return err
		}},
		{name: "outfit ids", call: func(s *Preview3DService) error {
			_, err := s.OutfitIDsForRole(context.Background(), "en", 1)
			return err
		}},
		{name: "outfit costume", call: func(s *Preview3DService) error {
			_, err := s.OutfitCostume3DIDForRole(context.Background(), "en", 10, 1, 1)
			return err
		}},
		{name: "hair costume", call: func(s *Preview3DService) error {
			_, err := s.HairCostume3DIDForRole(context.Background(), "en", 1, 1)
			return err
		}},
		{name: "catalog", call: func(s *Preview3DService) error {
			_, err := s.AccessoryCatalog(context.Background(), "en", 1)
			return err
		}},
	}
}

func TestPreview3DPublicMethodsHandleDisabledAndInvalidInput(t *testing.T) {
	var disabled *Preview3DService
	if path, err := disabled.ResolvePreviewPath(context.Background(), "jp", 101); err != nil || path != "" {
		t.Fatalf("disabled ResolvePreviewPath = %q, %v", path, err)
	}
	if err := disabled.EnsurePreviewCapture(context.Background(), "jp", 101); err != nil {
		t.Fatalf("disabled EnsurePreviewCapture: %v", err)
	}

	for _, call := range preview3DPublicCalls()[2:] {
		t.Run(call.name, func(t *testing.T) {
			if err := call.call(disabled); err == nil {
				t.Fatal("disabled service unexpectedly succeeded")
			}
		})
	}

	service := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: "http://preview.invalid"})
	if path, err := service.ResolveQueryPreviewPath(context.Background(), "jp", 0, Query{}); err != nil || path != "" {
		t.Fatalf("invalid ResolveQueryPreviewPath = %q, %v", path, err)
	}
	if err := service.EnsureQueryPreviewCapture(context.Background(), "jp", 0, Query{}); err != nil {
		t.Fatalf("invalid EnsureQueryPreviewCapture: %v", err)
	}
}

func TestPreview3DPublicMethodsPropagateEndpointAndRegistryErrors(t *testing.T) {
	endpointMissing := NewPreview3DService(Preview3DConfig{
		Enabled:        true,
		EngineBaseURLs: map[string]string{"cn": "http://preview.invalid"},
	})
	for _, call := range preview3DPublicCalls() {
		t.Run("endpoint/"+call.name, func(t *testing.T) {
			if err := call.call(endpointMissing); err == nil || !strings.Contains(err.Error(), "region en") {
				t.Fatalf("endpoint error = %v", err)
			}
		})
	}

	registryFailure := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: "http://preview.invalid"})
	registryFailure.client.Transport = preview3DRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("registry offline")
	})
	for _, call := range preview3DPublicCalls() {
		t.Run("registry/"+call.name, func(t *testing.T) {
			if err := call.call(registryFailure); err == nil || !strings.Contains(err.Error(), "registry offline") {
				t.Fatalf("registry error = %v", err)
			}
		})
	}
}

func TestPreview3DPublicMethodsPropagateResolutionErrors(t *testing.T) {
	service, _ := cachedPreview3DServiceForCoverage(t, &preview3DRegistry{}, "http://preview.invalid")
	for _, call := range preview3DPublicCalls() {
		t.Run(call.name, func(t *testing.T) {
			if err := call.call(service); err == nil {
				t.Fatal("empty registry unexpectedly resolved request")
			}
		})
	}

	service, _ = cachedPreview3DServiceForCoverage(t, preview3DCoverageRegistry(), "http://preview.invalid")
	if _, err := service.AccessoryCostume3DIDForRole(context.Background(), "jp", 999, 1, 1); err == nil {
		t.Fatal("unknown accessory unexpectedly resolved")
	}
	if _, err := service.OutfitCostume3DIDForRole(context.Background(), "jp", 999, 1, 1); err == nil {
		t.Fatal("unknown outfit unexpectedly resolved")
	}
	if _, err := service.HairCostume3DIDForRole(context.Background(), "jp", 999, 1); err == nil {
		t.Fatal("unknown hair unexpectedly resolved")
	}
}

func TestPreview3DPublicCapturePathsPropagateEngineFailures(t *testing.T) {
	var captureStatus int
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			http.NotFound(w, r)
		case http.MethodPost:
			http.Error(w, "capture failed", captureStatus)
		case http.MethodGet:
			http.Error(w, "fetch failed", http.StatusBadGateway)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer engine.Close()

	service, _ := cachedPreview3DServiceForCoverage(t, preview3DCoverageRegistry(), engine.URL)
	captureStatus = http.StatusServiceUnavailable
	if err := service.EnsureQueryPreviewCapture(context.Background(), "jp", 101, Query{}); err == nil || !strings.Contains(err.Error(), "capture failed") {
		t.Fatalf("EnsureQueryPreviewCapture error = %v", err)
	}
	if _, err := service.CaptureTemporaryCombo(context.Background(), "jp", ComboQuery{Character3DID: 1}); err == nil || !strings.Contains(err.Error(), "capture failed") {
		t.Fatalf("CaptureTemporaryCombo capture error = %v", err)
	}

	// A successful existence probe skips capture and reaches the fetch error.
	service.cfg.CaptureExistsTTL = time.Minute
	service.markCaptureExists(preview3DEndpoint{region: "jp", baseURL: engine.URL}, "unused")
	service.client.Transport = preview3DRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		status := http.StatusOK
		body := ""
		if req.Method == http.MethodGet {
			status = http.StatusBadGateway
			body = "fetch failed"
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	if _, err := service.CaptureTemporaryCombo(context.Background(), "jp", ComboQuery{Character3DID: 1}); err == nil || !strings.Contains(err.Error(), "fetch failed") {
		t.Fatalf("CaptureTemporaryCombo fetch error = %v", err)
	}
}

func TestEnsureStaticCaptureFileFastPathsAndFetchFailure(t *testing.T) {
	service, endpoint := cachedPreview3DServiceForCoverage(t, preview3DCoverageRegistry(), "http://preview.invalid")
	if err := service.ensureStaticCaptureFile(context.Background(), endpoint, ""); err != nil {
		t.Fatalf("empty static output: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.ensureStaticCaptureFile(canceled, endpoint, "image"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled static write error = %v", err)
	}

	service.cfg.StaticOutputDir = t.TempDir()
	target := filepath.Join(service.cfg.StaticOutputDir, "existing.png")
	if err := os.WriteFile(target, []byte("png"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := service.ensureStaticCaptureFile(context.Background(), endpoint, "existing"); err != nil {
		t.Fatalf("existing target: %v", err)
	}

	service.client.Transport = preview3DRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("fetch failed")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	if err := service.ensureStaticCaptureFile(context.Background(), endpoint, "missing"); err == nil || !strings.Contains(err.Error(), "fetch failed") {
		t.Fatalf("static fetch error = %v", err)
	}
}

func TestPreview3DRegistryProtocolValidationBranches(t *testing.T) {
	service, endpoint := cachedPreview3DServiceForCoverage(t, preview3DCoverageRegistry(), "http://preview.invalid")
	service.cfg.RegistryCacheTTL = time.Second
	service.cachedAt[endpoint.key()] = time.Now().Add(-2 * time.Second)
	if cached := service.validCachedRegistryLocked(endpoint, time.Now()); cached != nil {
		t.Fatal("expired registry was returned")
	}
	if err := validatePreview3DRoleCatalog(preview3DRoleCatalog{}); err == nil {
		t.Fatal("empty role catalog was accepted")
	}
	if _, _, ok := preview3DExpectedRoleIdentity(0); ok {
		t.Fatal("role zero was accepted")
	}

	service.client.Transport = preview3DRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})
	if _, err := service.getPartRegistry(context.Background(), endpoint); err == nil {
		t.Fatal("part registry transport error was ignored")
	}
	if _, err := service.getCompatibilityRegistry(context.Background(), endpoint); err == nil {
		t.Fatal("compatibility registry transport error was ignored")
	}

	partPayload := compactRegistryBytes(t, preview3DCompactPartRegistry{SchemaVersion: 99})
	compatibilityPayload := compactRegistryBytes(t, preview3DCompactCompatibilityRegistry{SchemaVersion: 99})
	invalidSchema := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/runtime/parts/part-registry-compact.msgpack.br":
			_, _ = w.Write(partPayload)
		case "/runtime/parts/head-hair-compatibility-compact.msgpack.br":
			_, _ = w.Write(compatibilityPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer invalidSchema.Close()
	schemaService := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: invalidSchema.URL})
	schemaEndpoint, _ := schemaService.endpointForRegion("jp")
	if _, err := schemaService.getPartRegistry(context.Background(), schemaEndpoint); err == nil || !strings.Contains(err.Error(), "schema 99") {
		t.Fatalf("part schema error = %v", err)
	}
	if _, err := schemaService.getCompatibilityRegistry(context.Background(), schemaEndpoint); err == nil || !strings.Contains(err.Error(), "schema 99") {
		t.Fatalf("compatibility schema error = %v", err)
	}

	if _, err := service.getRegistryResponse(context.Background(), preview3DEndpoint{baseURL: ":"}, "/registry"); err == nil {
		t.Fatal("invalid registry URL was accepted")
	}
	service.client.Transport = preview3DRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTeapot, Body: io.NopCloser(strings.NewReader("no")), Header: make(http.Header), Request: req}, nil
	})
	if _, err := service.getRegistryResponse(context.Background(), endpoint, "/registry"); err == nil || !strings.Contains(err.Error(), "HTTP 418") {
		t.Fatalf("registry status error = %v", err)
	}

	invalidMessagePack := compressedRegistryBytes(t, []byte("not messagepack"))
	service.client.Transport = preview3DRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(invalidMessagePack)), ContentLength: int64(len(invalidMessagePack)), Header: make(http.Header), Request: req}, nil
	})
	var decoded preview3DRoleCatalog
	if err := service.getMessagePackRegistry(context.Background(), endpoint, "/registry", &decoded, false); err == nil || !strings.Contains(err.Error(), "decode failed") {
		t.Fatalf("messagepack decode error = %v", err)
	}
}

func TestPreview3DRegistryWaitHonorsCallerCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	service := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: "http://preview.invalid", Timeout: time.Second})
	service.client.Transport = preview3DRoundTripFunc(func(*http.Request) (*http.Response, error) {
		close(started)
		<-release
		return nil, errors.New("released")
	})
	endpoint, _ := service.endpointForRegion("jp")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := service.registry(ctx, endpoint)
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("registry cancellation error = %v", err)
	}
	close(release)
}

func TestPreview3DCaptureCacheAndRequestFailures(t *testing.T) {
	service := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: "http://preview.invalid", CaptureExistsTTL: time.Minute})
	endpoint, _ := service.endpointForRegion("jp")
	service.markCaptureExists(endpoint, "cached")
	service.client.Transport = preview3DRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("cached capture performed an HTTP request")
		return nil, nil
	})
	if !service.captureExists(context.Background(), endpoint, "cached") {
		t.Fatal("cached capture was not found")
	}

	service.captures[endpoint.captureKey("expired")] = time.Now().Add(-time.Second)
	service.captureNextSweep = time.Now().Add(time.Hour)
	if service.cachedCaptureExists(endpoint, "expired") {
		t.Fatal("expired capture was returned")
	}
	empty := &Preview3DService{}
	empty.sweepCaptureCacheLocked(time.Now())
	if empty.captures == nil {
		t.Fatal("capture cache was not initialized")
	}

	service.client.Transport = preview3DRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("head offline")
	})
	if service.captureExists(context.Background(), endpoint, "offline") {
		t.Fatal("transport failure reported an existing capture")
	}
	if service.captureExists(context.Background(), preview3DEndpoint{baseURL: ":"}, "invalid") {
		t.Fatal("invalid capture URL reported an existing capture")
	}
}

func TestPreview3DCapturePermitAndFlightErrorBranches(t *testing.T) {
	var disabled *Preview3DService
	release, err := disabled.acquireCapturePermit(context.Background())
	if err != nil {
		t.Fatalf("nil service permit: %v", err)
	}
	release()

	busy := &Preview3DService{
		cfg:        Preview3DConfig{CaptureAcquireTimeout: time.Millisecond},
		captureSem: make(chan struct{}, 1),
	}
	busy.captureSem <- struct{}{}
	if release, err := busy.acquireCapturePermit(context.Background()); err == nil || release != nil {
		t.Fatalf("busy permit unexpectedly returned a release function or nil error: err=%v", err)
	}

	response := func(req *http.Request, status int) *http.Response {
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: req}
	}
	selection := preview3DSelection{ImageID: "flight", RoleID: "1:light_sound", BodyCostume3DID: 101, HeadCostume3DID: 102, HairCostume3DID: 103}
	endpoint := preview3DEndpoint{region: "jp", baseURL: "http://preview.invalid"}

	busyService := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: endpoint.baseURL, CaptureAcquireTimeout: time.Millisecond, CaptureExistsTTL: -1})
	busyService.captureSem <- struct{}{}
	busyService.client.Transport = preview3DRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(req, http.StatusNotFound), nil
	})
	if err := busyService.ensureCapture(context.Background(), endpoint, selection, "persistent"); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("busy ensureCapture error = %v", err)
	}

	var heads int
	alreadyCreated := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: endpoint.baseURL, CaptureExistsTTL: -1})
	alreadyCreated.client.Transport = preview3DRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodHead {
			heads++
			if heads == 1 {
				return response(req, http.StatusNotFound), nil
			}
			return response(req, http.StatusOK), nil
		}
		t.Fatalf("unexpected request after capture appeared: %s", req.Method)
		return nil, nil
	})
	if err := alreadyCreated.ensureCapture(context.Background(), endpoint, selection, "persistent"); err != nil {
		t.Fatalf("ensureCapture after external creation: %v", err)
	}

	fallbackTimeout := &Preview3DService{
		cfg:        Preview3DConfig{Timeout: 0, CaptureAcquireTimeout: 0, CaptureExistsTTL: -1},
		captureSem: nil,
		captures:   make(map[string]time.Time),
		client: &http.Client{Transport: preview3DRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodHead {
				return response(req, http.StatusNotFound), nil
			}
			return nil, errors.New("capture offline")
		})},
	}
	if err := fallbackTimeout.ensureCapture(context.Background(), endpoint, selection, "persistent"); err == nil || !strings.Contains(err.Error(), "capture offline") {
		t.Fatalf("fallback-timeout capture error = %v", err)
	}
}

func TestPreview3DCaptureFlightHonorsCallerCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	heads := 0
	var startedOnce sync.Once
	service := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: "http://preview.invalid", Timeout: time.Second, CaptureExistsTTL: -1})
	service.client.Transport = preview3DRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodHead {
			heads++
			if heads == 1 {
				return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: req}, nil
			}
			startedOnce.Do(func() { close(started) })
			<-release
		}
		return nil, errors.New("released")
	})
	endpoint, _ := service.endpointForRegion("jp")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- service.ensureCapture(ctx, endpoint, preview3DSelection{ImageID: "cancel"}, "persistent")
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("capture cancellation error = %v", err)
	}
	close(release)
}

func TestPreview3DCaptureProtocolBranches(t *testing.T) {
	var payload map[string]any
	service := NewPreview3DService(Preview3DConfig{
		Enabled:             true,
		EngineBaseURL:       "http://preview.invalid",
		Width:               640,
		Height:              960,
		Scale:               1.5,
		TemporaryCaptureTTL: time.Second + time.Nanosecond,
	})
	service.client.Transport = preview3DRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode capture payload: %v", err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header), Request: req}, nil
	})
	optionalID := 104
	selection := preview3DSelection{
		ImageID:                 "protocol",
		RoleID:                  "1:light_sound",
		HeadPackagePath:         "parts/head/102",
		HeadOptionalCostume3DID: &optionalID,
	}
	endpoint := preview3DEndpoint{region: "jp", baseURL: "http://preview.invalid"}
	if err := service.captureSelection(context.Background(), endpoint, selection, ""); err != nil {
		t.Fatalf("captureSelection: %v", err)
	}
	for _, key := range []string{"headPackagePath", "headOptionalCostume3dId", "width", "height", "scale"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("capture payload missing %s: %+v", key, payload)
		}
	}
	if err := service.captureSelection(context.Background(), preview3DEndpoint{baseURL: ":"}, selection, "persistent"); err == nil {
		t.Fatal("invalid capture URL was accepted")
	}
	service.client.Transport = preview3DRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("post offline")
	})
	if err := service.captureSelection(context.Background(), endpoint, selection, "persistent"); err == nil || !strings.Contains(err.Error(), "post offline") {
		t.Fatalf("capture transport error = %v", err)
	}
	if _, err := service.getCapture(context.Background(), preview3DEndpoint{baseURL: ":"}, "image"); err == nil {
		t.Fatal("invalid fetch URL was accepted")
	}
	if _, err := service.getCapture(context.Background(), endpoint, "image"); err == nil || !strings.Contains(err.Error(), "post offline") {
		t.Fatalf("fetch transport error = %v", err)
	}
}

func TestReadPreview3DResponseAndAtomicWriteErrorBranches(t *testing.T) {
	if _, err := readPreview3DResponse(nil, 1, "nil"); err == nil {
		t.Fatal("nil response was accepted")
	}
	response := &http.Response{Body: io.NopCloser(preview3DCoverageErrorReader{}), ContentLength: -1}
	if _, err := readPreview3DResponse(response, 1, "broken"); err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("response read error = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := writePreview3DCaptureAtomically(canceled, filepath.Join(t.TempDir(), "image.png"), []byte("png")); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled atomic write error = %v", err)
	}
	if err := writePreview3DCaptureAtomically(context.Background(), filepath.Join(t.TempDir(), "missing", "image.png"), []byte("png")); err == nil {
		t.Fatal("atomic write into missing directory succeeded")
	}
	dir := t.TempDir()
	nonEmptyTarget := filepath.Join(dir, "target")
	if err := os.Mkdir(nonEmptyTarget, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nonEmptyTarget, "child"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed target child: %v", err)
	}
	if err := writePreview3DCaptureAtomically(context.Background(), nonEmptyTarget, []byte("png")); err == nil {
		t.Fatal("atomic rename over non-empty directory succeeded")
	}

	service := &Preview3DService{cfg: Preview3DConfig{Timeout: -31 * time.Second}}
	ctx, stop := service.staticCaptureSharedContext()
	defer stop()
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) < 40*time.Second {
		t.Fatalf("fallback static deadline = %v, %t", deadline, ok)
	}
	if got := normalizePreview3DRegion(" "); got != "jp" {
		t.Fatalf("empty region = %q", got)
	}
}

func TestPreview3DRegistryRawAccessorySelectionOrdering(t *testing.T) {
	registry := &preview3DRegistry{
		partRegistryVersion: 2,
		parts: []preview3DPartEntry{
			{Costume3DID: 500, PartType: "body", AccessoryID: 42, Status: "available"},
			{Costume3DID: 500, PartType: "head", CharacterID: 2, Unit: "b", PackagePath: "z", AccessoryID: 42, Status: "available"},
			{Costume3DID: 500, PartType: "head", CharacterID: 1, Unit: "b", PackagePath: "z", AccessoryID: 42, Status: "available"},
			{Costume3DID: 501, PartType: "head", CharacterID: 1, Unit: "b", PackagePath: "z", AccessoryID: 42, Status: "available"},
			{Costume3DID: 501, PartType: "head", CharacterID: 1, Unit: "a", PackagePath: "z", AccessoryID: 42, Status: "available"},
			{Costume3DID: 502, PartType: "head", CharacterID: 1, Unit: "a", PackagePath: "z", AccessoryID: 42, Status: "available"},
			{Costume3DID: 502, PartType: "head_optional", CharacterID: 1, Unit: "a", PackagePath: "a", AccessoryID: 42, Status: "available"},
			{Costume3DID: 503, PartType: "head", CharacterID: 1, Unit: "a", PackagePath: "missing", AccessoryID: 42, Status: "missing"},
			{Costume3DID: 600, PartType: "head", CharacterID: 1, Unit: "", PackagePath: "z", AccessoryID: 42, Status: "available"},
			{Costume3DID: 600, PartType: "head", CharacterID: 1, Unit: "a", PackagePath: "z", AccessoryID: 42, Status: "available"},
			{Costume3DID: 601, PartType: "head", CharacterID: 1, Unit: "a", PackagePath: "z", AccessoryID: 42, Status: "available"},
			{Costume3DID: 601, PartType: "head_optional", CharacterID: 1, Unit: "a", PackagePath: "a", AccessoryID: 42, Status: "available"},
			{Costume3DID: 602, PartType: "head", CharacterID: 2, Unit: "a", PackagePath: "wrong-character", AccessoryID: 42, Status: "available"},
			{Costume3DID: 603, PartType: "head", CharacterID: 1, Unit: "b", PackagePath: "wrong-unit", AccessoryID: 42, Status: "available"},
		},
	}

	for _, test := range []struct {
		rawID       int
		wantPackage string
	}{
		{rawID: 500, wantPackage: "z"},
		{rawID: 501, wantPackage: "z"},
		{rawID: 502, wantPackage: "a"},
	} {
		part, ok := registry.accessoryPartByRawID(test.rawID, 42)
		if !ok || part.PackagePath != test.wantPackage {
			t.Fatalf("raw accessory %d = %+v, %t", test.rawID, part, ok)
		}
	}
	if _, ok := registry.accessoryPartByRawID(999, 42); ok {
		t.Fatal("unknown raw accessory resolved")
	}

	role := preview3DCharacterEntry{Character3DID: 1, CharacterID: 1, Unit: "a"}
	for _, test := range []struct {
		rawID       int
		wantPackage string
	}{
		{rawID: 600, wantPackage: "z"},
		{rawID: 601, wantPackage: "a"},
	} {
		part, ok := registry.accessoryPartByRawIDForRole(test.rawID, 42, role)
		if !ok || part.PackagePath != test.wantPackage {
			t.Fatalf("role raw accessory %d = %+v, %t", test.rawID, part, ok)
		}
	}
	for _, rawID := range []int{602, 603, 999} {
		if _, ok := registry.accessoryPartByRawIDForRole(rawID, 42, role); ok {
			t.Fatalf("incompatible raw accessory %d resolved", rawID)
		}
	}
}

func TestPreview3DRegistryPartSelectionHelperBranches(t *testing.T) {
	role := preview3DCharacterEntry{Character3DID: 1, CharacterID: 1, Unit: "a"}
	registry := preview3DPartSelectionCoverageRegistry()
	testPreview3DPartSelections(t, registry, role)
	testPreview3DPartPreferences(t, role)
}

func preview3DPartSelectionCoverageRegistry() *preview3DRegistry {
	return &preview3DRegistry{
		parts: []preview3DPartEntry{
			{Costume3DID: 700, PartType: "body", CharacterID: 1, Unit: "", ColorID: 1, OutfitID: 7, Status: "available"},
			{Costume3DID: 701, PartType: "body", CharacterID: 1, Unit: "a", ColorID: 1, OutfitID: 7, Status: "available"},
			{Costume3DID: 703, PartType: "body", CharacterID: 1, Unit: "a", ColorID: 1, Costume3DGroupID: 9_000, Status: "available"},
			{Costume3DID: 710, PartType: "head_optional", CharacterID: 1, Unit: "", ColorID: 1, Status: "empty"},
			{Costume3DID: 709, PartType: "head_optional", CharacterID: 1, Unit: "a", ColorID: 1, Status: "empty"},
			{Costume3DID: 708, PartType: "head_optional", CharacterID: 1, Unit: "a", ColorID: 1, Status: "empty"},
			{Costume3DID: 711, PartType: "head_optional", CharacterID: 2, Unit: "a", ColorID: 1, Status: "empty"},
			{Costume3DID: 712, PartType: "head_optional", CharacterID: 1, Unit: "b", ColorID: 1, Status: "empty"},
			{Costume3DID: 720, PartType: "hair", CharacterID: 1, Unit: "", Status: "available"},
			{Costume3DID: 720, PartType: "hair", CharacterID: 1, Unit: "a", Status: "available"},
			{Costume3DID: 721, PartType: "head", CharacterID: 1, Unit: "a", Status: "available"},
			{Costume3DID: 721, PartType: "head_optional", CharacterID: 1, Unit: "a", Status: "available"},
			{Costume3DID: 722, PartType: "hair", CharacterID: 2, Unit: "a", Status: "available"},
			{Costume3DID: 723, PartType: "hair", CharacterID: 1, Unit: "b", Status: "available"},
			{Costume3DID: 724, PartType: "body", CharacterID: 1, Unit: "a", Status: "available"},
			{Costume3DID: 725, PartType: "hair", CharacterID: 1, Unit: "a", Status: "missing"},
		},
	}
}

func testPreview3DPartSelections(t *testing.T, registry *preview3DRegistry, role preview3DCharacterEntry) {
	t.Helper()
	if part, ok := registry.outfitPartForRole(7, 1, role); !ok || part.Costume3DID != 701 {
		t.Fatalf("outfit exact-unit selection = %+v, %t", part, ok)
	}
	if part, ok := registry.outfitPartForRole(9, 1, role); !ok || part.Costume3DID != 703 {
		t.Fatalf("group-derived outfit selection = %+v, %t", part, ok)
	}
	if got := registry.outfitIDsForRole(role)[703]; got != 9 {
		t.Fatalf("group-derived outfit id = %d", got)
	}
	if part, ok := registry.partForRole(720, role, "hair"); !ok || part.Unit != "a" {
		t.Fatalf("exact-unit part selection = %+v, %t", part, ok)
	}
	if part, ok := registry.partForRole(721, role, "head", "head_optional"); !ok || preview3DPartSlot(part) != "head" {
		t.Fatalf("slot ordering selection = %+v, %t", part, ok)
	}
	for _, costumeID := range []int{722, 723, 724, 725, 999} {
		if _, ok := registry.partForRole(costumeID, role, "hair"); ok {
			t.Fatalf("incompatible part %d resolved", costumeID)
		}
	}
	if part, ok := registry.defaultHeadOptionalPartForRole(role); !ok || part.Costume3DID != 708 {
		t.Fatalf("default empty head selection = %+v, %t", part, ok)
	}
}

func testPreview3DPartPreferences(t *testing.T, role preview3DCharacterEntry) {
	t.Helper()
	if !preferPreview3DHeadPart(
		preview3DPartEntry{Unit: "a", ColorID: 2},
		preview3DPartEntry{Unit: "", ColorID: 1},
		role,
	) {
		t.Fatal("exact-unit head was not preferred")
	}
	if !preferPreview3DHeadPart(
		preview3DPartEntry{Unit: "a", ColorID: 1},
		preview3DPartEntry{Unit: "a", ColorID: 2},
		role,
	) {
		t.Fatal("original-color head was not preferred")
	}

	for input, want := range map[string]string{" ... ": "unknown", " !!! ": "unknown"} {
		if got := sanitizePreview3DImagePart(input); got != want {
			t.Fatalf("sanitizePreview3DImagePart(%q) = %q", input, got)
		}
	}
}

func TestPreview3DResolveAdditionalValidationBranches(t *testing.T) {
	registry := preview3DCoverageRegistry()
	if selection, err := registry.resolve("jp", 102); err != nil || selection.AccessoryID != 20 {
		t.Fatalf("resolve accessory = %+v, %v", selection, err)
	}
	for _, partType := range []string{"body", "head"} {
		costumeID := 101
		if partType == "head" {
			costumeID = 102
		}
		if _, err := registry.resolveQuery("jp", costumeID, Query{Character3DID: 1, ExpectedPartType: partType}, "coverage"); err != nil {
			t.Fatalf("resolveQuery %s: %v", partType, err)
		}
	}

	ambiguous := &preview3DRegistry{partRegistryVersion: 2, parts: []preview3DPartEntry{
		{Costume3DID: 800, PartType: "head", AccessoryID: 1, Status: "available"},
		{Costume3DID: 800, PartType: "head_optional", AccessoryID: 2, Status: "available"},
	}}
	if _, err := ambiguous.resolve("jp", 800); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous raw accessory error = %v", err)
	}
	missingRole := &preview3DRegistry{parts: []preview3DPartEntry{{Costume3DID: 801, PartType: "body", CharacterID: 99, Status: "available"}}}
	if _, err := missingRole.resolve("jp", 801); err == nil || !strings.Contains(err.Error(), "default role") {
		t.Fatalf("missing role error = %v", err)
	}
	incomplete := &preview3DRegistry{
		characters: []preview3DCharacterEntry{{Character3DID: 1, CharacterID: 1, Unit: "a", HairCostume3DID: 3}},
		parts:      []preview3DPartEntry{{Costume3DID: 802, PartType: "body", CharacterID: 1, Unit: "a", Status: "available"}},
	}
	if _, err := incomplete.resolve("jp", 802); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("incomplete tuple error = %v", err)
	}
}
