package deck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/imagecache"
)

type controllerImageIndex struct{}

func (controllerImageIndex) LookupRender(_ context.Context, key string) (imagecache.RenderIndexEntry, bool, error) {
	return imagecache.RenderIndexEntry{
		RequestKey: key, ContentHash: strings.Repeat("a", 64),
		Entry: imagecache.ImageEntry{CDNPath: "pjsk/deck/cached.png", StorageBackend: imagecache.BackendGarage, MediaType: "image/png"},
	}, true, nil
}

func (controllerImageIndex) TouchRender(context.Context, []string) (int64, error) { return 0, nil }
func (controllerImageIndex) DeleteExpiredRender(context.Context, []string, time.Time) (int64, error) {
	return 0, nil
}

func TestRecommendationImagePreservesArtifactAndLegacyBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("cache hit must not request Drawing")
		http.Error(w, "unexpected render", http.StatusInternalServerError)
	}))
	defer server.Close()
	objects := storagetest.NewMemory()
	objects.Seed(map[string][]byte{"pjsk/deck/cached.png": []byte("cached-image")})
	client := drawing.NewHarukiDrawingClient(server.URL, drawing.WithRetryCount(0))
	cache := drawing.NewRenderCacheClient(drawing.RenderCacheConfig{TTL: time.Hour, Index: controllerImageIndex{}, Artifacts: objects})
	client.SetRenderCache(cache)
	defer client.Close()
	controller := (&Controller{drawing: client}).WithContext(t.Context())
	req := drawing.DeckRequest{Region: "jp", DeckData: []drawing.DeckData{{}}}
	image, err := controller.RenderRecommendImage(req)
	if err != nil || image.Ref() == nil || len(objects.Calls()) != 0 {
		t.Fatalf("image ref = %+v, calls = %+v, err = %v", image.Ref(), objects.Calls(), err)
	}
	data, err := controller.RenderRecommend(req)
	if err != nil || string(data) != "cached-image" || len(objects.Calls()) != 1 {
		t.Fatalf("legacy bytes = %q, calls = %+v, err = %v", data, objects.Calls(), err)
	}
}
