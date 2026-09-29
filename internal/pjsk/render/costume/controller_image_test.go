package costume

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/imagecache"
)

type controllerImageIndex struct{}

func (controllerImageIndex) LookupRender(_ context.Context, key string) (imagecache.RenderIndexEntry, bool, error) {
	return imagecache.RenderIndexEntry{
		RequestKey: key, ContentHash: strings.Repeat("a", 64),
		Entry: imagecache.ImageEntry{CDNPath: "pjsk/costume/cached.png", StorageBackend: imagecache.BackendGarage, MediaType: "image/png"},
	}, true, nil
}

func (controllerImageIndex) TouchRender(context.Context, []string) (int64, error)  { return 0, nil }
func (controllerImageIndex) DeleteRender(context.Context, []string) (int64, error) { return 0, nil }

func newImageResultController(t *testing.T) (*Controller, *storagetest.Memory) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("cache hit must not request Drawing")
		http.Error(w, "unexpected render", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	objects := storagetest.NewMemory()
	objects.Seed(map[string][]byte{"pjsk/costume/cached.png": []byte("cached-image")})
	client := drawing.NewHarukiDrawingClient(server.URL, drawing.WithRetryCount(0))
	cache := drawing.NewRenderCacheClient(drawing.RenderCacheConfig{TTL: time.Hour, Index: controllerImageIndex{}, Artifacts: objects})
	client.SetRenderCache(cache)
	t.Cleanup(func() { _ = client.Close() })
	item := controllerCoverageCostume(33001, "body")
	source := &controllerCoverageSource{
		costumes:   map[int]*masterdata.Costume3d{item.ID: item},
		characters: map[int]*masterdata.Character{20: {ID: 20, GivenName: "Kanade", Unit: "school_refusal", Gender: "female"}},
	}
	return NewController(source, client, nil).WithContext(t.Context()), objects
}

func TestCostumeListImagePreservesRequestAndArtifact(t *testing.T) {
	controller, objects := newImageResultController(t)
	query := ListQuery{PartType: "body"}
	image, payload, err := controller.RenderCostumeListWithRequestImage(query)
	if err != nil || image.Ref() == nil || payload == nil || payload.Total != 1 || len(objects.Calls()) != 0 {
		t.Fatalf("image ref = %+v, payload = %+v, calls = %+v, err = %v", image.Ref(), payload, objects.Calls(), err)
	}
	image, err = controller.RenderCostumeListImage(query)
	if err != nil || image.Ref() == nil || len(objects.Calls()) != 0 {
		t.Fatalf("image ref = %+v, calls = %+v, err = %v", image.Ref(), objects.Calls(), err)
	}
	data, payload, err := controller.RenderCostumeListWithRequest(query)
	if err != nil || string(data) != "cached-image" || payload == nil || payload.Total != 1 || len(objects.Calls()) != 1 {
		t.Fatalf("legacy bytes = %q, payload = %+v, calls = %+v, err = %v", data, payload, objects.Calls(), err)
	}
}

func TestCostumeListRetainsRequestWhenArtifactReadFails(t *testing.T) {
	controller, objects := newImageResultController(t)
	if err := objects.Delete(t.Context(), "pjsk/costume/cached.png"); err != nil {
		t.Fatal(err)
	}
	data, payload, err := controller.RenderCostumeListWithRequest(ListQuery{PartType: "body"})
	if !errors.Is(err, drawing.ErrArtifactBytesUnavailable) || data != nil || payload == nil || payload.Total != 1 {
		t.Fatalf("legacy bytes = %q, payload = %+v, err = %v", data, payload, err)
	}
}

func TestCostumeDetailImageCacheHitSkipsPreviewAndArtifactRead(t *testing.T) {
	controller, objects := newImageResultController(t)
	var previewRequests atomic.Int32
	preview := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		previewRequests.Add(1)
		http.Error(w, "unexpected preview", http.StatusInternalServerError)
	}))
	defer preview.Close()
	controller.Set3DPreviewConfig(Preview3DConfig{Enabled: true, EngineBaseURL: preview.URL})
	image, err := controller.RenderCostumeDetailImage(Query{ID: 33001})
	if err != nil || image.Ref() == nil || len(objects.Calls()) != 0 || previewRequests.Load() != 0 {
		t.Fatalf("image ref = %+v, object calls = %+v, preview calls = %d, err = %v", image.Ref(), objects.Calls(), previewRequests.Load(), err)
	}
}
