package costume

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	renderassets "haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

type preview3DRecordingStore struct {
	storage.Store
	mu      sync.Mutex
	puts    []storage.PutOptions
	statErr error
	putErr  error
}

func (s *preview3DRecordingStore) Put(ctx context.Context, key storage.Key, data []byte, opts storage.PutOptions) error {
	s.mu.Lock()
	s.puts = append(s.puts, opts)
	s.mu.Unlock()
	if s.putErr != nil {
		return s.putErr
	}
	return s.Store.Put(ctx, key, data, opts)
}

func (s *preview3DRecordingStore) Stat(ctx context.Context, key storage.Key) (storage.Object, error) {
	if s.statErr != nil {
		return storage.Object{}, s.statErr
	}
	return s.Store.Stat(ctx, key)
}

func TestEnsureStaticCaptureObjectUsesStaticStore(t *testing.T) {
	const imageID = "pjsk3d_store"
	const png = "store-png"
	var requests atomic.Int32
	release := make(chan struct{})
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/captures/"+imageID+".png" {
			t.Errorf("unexpected engine request: %s %s", r.Method, r.URL.Path)
		}
		requests.Add(1)
		<-release
		_, _ = w.Write([]byte(png))
	}))
	defer engine.Close()

	memory := storagetest.NewMemory()
	store := &preview3DRecordingStore{Store: memory}
	service := NewPreview3DService(Preview3DConfig{
		Enabled:       true,
		EngineBaseURL: engine.URL,
		StaticStore:   store,
		Timeout:       time.Second,
	})
	endpoint, err := service.endpointForRegion("jp")
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}

	const callers = 4
	errs := make(chan error, callers)
	for range callers {
		go func() { errs <- service.ensureStaticCaptureObject(context.Background(), endpoint, imageID) }()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	for range callers {
		if err := <-errs; err != nil {
			t.Fatalf("ensure error = %v", err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("engine GETs = %d, want 1", got)
	}
	key := storage.Key(defaultPreview3DStaticRelativeDir + "/" + imageID + ".png")
	data, err := memory.Get(context.Background(), key)
	if err != nil || string(data) != png {
		t.Fatalf("stored object %s = %q, %v", key, data, err)
	}
	if len(store.puts) != 1 || store.puts[0].ContentType != "image/png" || store.puts[0].CacheControl != "" {
		t.Fatalf("put options = %+v", store.puts)
	}

	// A published object short-circuits on Stat: no further engine request.
	if err := service.ensureStaticCaptureObject(context.Background(), endpoint, imageID); err != nil {
		t.Fatalf("second ensure = %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("engine GETs after publish = %d, want 1", got)
	}
}

func TestEnsureStaticCaptureObjectStoreFailures(t *testing.T) {
	service, endpoint := cachedPreview3DServiceForCoverage(t, preview3DCoverageRegistry(), "http://preview.invalid")
	service.client.Transport = preview3DRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header), Request: req}, nil
	})

	statErr := errors.New("stat exploded")
	service.static = preview3DStaticTarget{store: &preview3DRecordingStore{Store: storagetest.NewMemory(), statErr: statErr}, prefix: "p"}
	if err := service.ensureStaticCaptureObject(context.Background(), endpoint, "a"); !errors.Is(err, statErr) {
		t.Fatalf("stat failure = %v", err)
	}

	putErr := errors.New("put exploded")
	service.static = preview3DStaticTarget{store: &preview3DRecordingStore{Store: storagetest.NewMemory(), putErr: putErr}, prefix: "p"}
	if err := service.ensureStaticCaptureObject(context.Background(), endpoint, "b"); !errors.Is(err, putErr) {
		t.Fatalf("put failure = %v", err)
	}

	service.static = preview3DStaticTarget{store: storagetest.NewMemory(), prefix: "../escape"}
	if err := service.ensureStaticCaptureObject(context.Background(), endpoint, "c"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("invalid key = %v", err)
	}

	service.static = preview3DStaticTarget{}
	var nilCtx context.Context
	if err := service.ensureStaticCaptureObject(nilCtx, endpoint, "d"); err != nil {
		t.Fatalf("disabled static target = %v", err)
	}
}

func TestResolvePreview3DStaticTarget(t *testing.T) {
	dir := t.TempDir()
	memory := storagetest.NewMemory()

	explicit := resolvePreview3DStaticTarget(Preview3DConfig{StaticOutputDir: dir, StaticStore: memory, StaticRelativeDir: "ignored"})
	if explicit.store == nil || explicit.store == storage.Store(memory) || explicit.prefix != "" {
		t.Fatalf("explicit dir target = %+v", explicit)
	}
	if err := explicit.store.Put(context.Background(), "x.png", []byte("x"), storage.PutOptions{}); err != nil {
		t.Fatalf("explicit put: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.png")); err != nil {
		t.Fatalf("explicit dir file: %v", err)
	}

	slot := resolvePreview3DStaticTarget(Preview3DConfig{StaticStore: memory, StaticRelativeDir: "/custom/previews/"})
	if slot.store != storage.Store(memory) || slot.prefix != "custom/previews" {
		t.Fatalf("slot target = %+v", slot)
	}
	if key, err := slot.key("id"); err != nil || key != "custom/previews/id.png" {
		t.Fatalf("slot key = %q, %v", key, err)
	}

	for name, cfg := range map[string]Preview3DConfig{
		"none":     {},
		"disabled": {StaticStore: storage.Disabled()},
	} {
		if target := resolvePreview3DStaticTarget(cfg); target.store != nil {
			t.Fatalf("%s target = %+v", name, target)
		}
	}
}

func TestSet3DPreviewConfigPrefersStaticStoreOverPrimaryDerivation(t *testing.T) {
	assetRoot := t.TempDir()
	controller := NewController(nil, nil, renderassets.NewAssetHelper(assetRoot, nil))
	memory := storagetest.NewMemory()
	controller.Set3DPreviewConfig(Preview3DConfig{Enabled: true, EngineBaseURL: "http://preview.invalid", StaticStore: memory})
	if got := controller.preview3D.cfg.StaticOutputDir; got != "" {
		t.Fatalf("static output dir derived despite a static store: %q", got)
	}
	if controller.preview3D.static.store != storage.Store(memory) || !strings.HasSuffix(controller.preview3D.static.prefix, "pjsk_3d_preview") {
		t.Fatalf("static target = %+v", controller.preview3D.static)
	}

	controller.Set3DPreviewConfig(Preview3DConfig{Enabled: true, EngineBaseURL: "http://preview.invalid", StaticStore: storage.Disabled()})
	if got, want := controller.preview3D.cfg.StaticOutputDir, filepath.Join(assetRoot, defaultPreview3DStaticRelativeDir); got != want {
		t.Fatalf("disabled store derived dir = %q, want %q", got, want)
	}
}
