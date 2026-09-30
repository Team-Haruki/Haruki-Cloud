package stamp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/imagecache"
)

type stampImageIndex struct {
	mu      sync.Mutex
	entries map[string]imagecache.RenderIndexEntry
}

func (index *stampImageIndex) LookupRender(_ context.Context, key string) (imagecache.RenderIndexEntry, bool, error) {
	index.mu.Lock()
	defer index.mu.Unlock()
	if entry, ok := index.entries[key]; ok {
		return entry, true, nil
	}
	entry := imagecache.RenderIndexEntry{
		RequestKey: key, ContentHash: strings.Repeat("a", 64),
		Entry: imagecache.ImageEntry{
			CDNPath:        fmt.Sprintf("stamp/page%d.png", len(index.entries)+1),
			StorageBackend: imagecache.BackendGarage, MediaType: "image/png",
		},
	}
	index.entries[key] = entry
	return entry, true, nil
}

func (*stampImageIndex) TouchRender(context.Context, []string) (int64, error) { return 0, nil }
func (*stampImageIndex) DeleteExpiredRender(context.Context, []string, time.Time) (int64, error) {
	return 0, nil
}

func TestStampImageResultsKeepPageOrderAndDeferArtifactReads(t *testing.T) {
	objects := storagetest.NewMemory()
	objects.Seed(map[string][]byte{
		"stamp/page1.png": []byte("first page"),
		"stamp/page2.png": []byte("second page"),
	})
	cache := drawing.NewRenderCacheClient(drawing.RenderCacheConfig{
		TTL: time.Hour, Index: &stampImageIndex{entries: make(map[string]imagecache.RenderIndexEntry)}, Artifacts: objects,
	})
	t.Cleanup(func() { _ = cache.Close() })
	client := drawing.NewHarukiDrawingClient("http://drawing.invalid", drawing.WithArtifactConfig(drawing.ArtifactConfig{
		Endpoints: []string{"*"}, Objects: objects,
	}))
	client.SetRenderCache(cache)
	source := newTestStampSource(renderregion.JP)
	for id := 1; id <= 26; id++ {
		source.stamps = append(source.stamps, masterdata.Stamp{ID: id, AssetBundleName: fmt.Sprintf("stamp_%d", id)})
	}
	controller := NewController(source, client, assets.NewAssetHelper("", nil)).WithContext(t.Context())
	query := ListQuery{Region: renderregion.JP, All: true}

	images, err := controller.RenderStampListPagesImage(query)
	if err != nil {
		t.Fatalf("RenderStampListPagesImage: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("got %d pages, want 2", len(images))
	}
	for page, image := range images {
		want := fmt.Sprintf("stamp/page%d.png", page+1)
		if ref := image.Ref(); ref == nil || ref.CDNPath != want {
			t.Fatalf("page %d ref = %+v, want %s", page+1, ref, want)
		}
	}
	if calls := objects.Calls(); len(calls) != 0 {
		t.Fatalf("image results fetched artifacts: %+v", calls)
	}

	pages, err := controller.RenderStampListPages(query)
	if err != nil {
		t.Fatalf("RenderStampListPages: %v", err)
	}
	if len(pages) != 2 || string(pages[0]) != "first page" || string(pages[1]) != "second page" {
		t.Fatalf("byte wrapper pages = %q", pages)
	}
	if calls := objects.Calls(); len(calls) != 2 || calls[0].Method != "Get" || calls[1].Method != "Get" {
		t.Fatalf("byte wrapper artifact reads = %+v", calls)
	}

	image, err := controller.RenderStampListImage(query)
	if err != nil || image.Ref() == nil || image.Ref().CDNPath != "stamp/page1.png" {
		t.Fatalf("single image = %+v, err = %v", image.Ref(), err)
	}
	if calls := objects.Calls(); len(calls) != 2 {
		t.Fatalf("single image fetched its artifact: %+v", calls)
	}
	data, err := controller.RenderStampList(query)
	if err != nil || string(data) != "first page" {
		t.Fatalf("single byte wrapper = %q, err = %v", data, err)
	}
}
