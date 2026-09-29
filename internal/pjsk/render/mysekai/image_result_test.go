package mysekai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/imagecache"
)

const shopImageResultPath = "pjsk/api/pjsk/mysekai/shop/result.png"

type shopImageResultIndex struct{}

func (shopImageResultIndex) LookupRender(_ context.Context, key string) (imagecache.RenderIndexEntry, bool, error) {
	return imagecache.RenderIndexEntry{
		RequestKey: key, ContentHash: strings.Repeat("a", 64),
		Entry: imagecache.ImageEntry{CDNPath: shopImageResultPath, StorageBackend: imagecache.BackendGarage, MediaType: "image/png"},
	}, true, nil
}

func (shopImageResultIndex) TouchRender(context.Context, []string) (int64, error) {
	return 0, nil
}

func (shopImageResultIndex) DeleteRender(context.Context, []string) (int64, error) {
	return 0, nil
}

func TestShopImageResultPreservesReferenceUntilBytesRequested(t *testing.T) {
	var downloads atomic.Int32
	objects := storagetest.NewMemory()
	objects.Seed(map[string][]byte{shopImageResultPath: []byte("stored-image")})
	objects.FailGet = func(storage.Key) error {
		downloads.Add(1)
		return nil
	}
	cache := drawing.NewRenderCacheClient(drawing.RenderCacheConfig{TTL: time.Hour, Index: shopImageResultIndex{}, Artifacts: objects})
	t.Cleanup(func() { _ = cache.Close() })
	client := drawing.NewHarukiDrawingClient("http://drawing.invalid")
	client.SetRenderCache(cache)
	controller, query, data := playerShopFixture(t)
	controller = shopWithData(t, controller, data)
	controller.drawing = client.WithContext(t.Context())
	controller.requestCtx = t.Context()

	image, err := controller.RenderShopImage(query)
	if err != nil || image.Ref() == nil || image.Ref().CDNPath != shopImageResultPath {
		t.Fatalf("shop image reference = %+v, %v", image.Ref(), err)
	}
	if got := downloads.Load(); got != 0 {
		t.Fatalf("reference rendering downloaded image %d times", got)
	}

	legacy, err := controller.RenderShop(query)
	if err != nil || string(legacy) != "stored-image" {
		t.Fatalf("legacy shop bytes = %q, %v", legacy, err)
	}
	if got := downloads.Load(); got != 1 {
		t.Fatalf("legacy bytes downloaded image %d times, want 1", got)
	}
}

func TestShopImageResultSupportsDrawingByteResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("rendered-image"))
	}))
	t.Cleanup(server.Close)
	controller := &Controller{drawing: drawing.NewHarukiDrawingClient(server.URL).WithContext(t.Context()), requestCtx: t.Context()}
	image, err := controller.RenderShopRequestImage(&drawing.MysekaiShopRequest{})
	if err != nil || image.Ref() != nil {
		t.Fatalf("byte response = %+v, %v", image.Ref(), err)
	}
	data, err := image.Bytes(t.Context())
	if err != nil || string(data) != "rendered-image" {
		t.Fatalf("rendered bytes = %q, %v", data, err)
	}
}
