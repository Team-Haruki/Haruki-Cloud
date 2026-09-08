package drawing

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"haruki-cloud/utils/imagecache"
)

// Both paths exclude the common metadata HTTP lookup and real rendering.
// The byte path includes the existing singleflight ownership copy and warm store.
func BenchmarkCachedImageDelivery(b *testing.B) {
	for _, size := range []int{512 << 10, 2 << 20, 8 << 20} {
		b.Run(fmt.Sprintf("bytes_%d", size), func(b *testing.B) {
			root := b.TempDir()
			file := filepath.Join(root, "cached.png")
			data := make([]byte, size)
			if err := os.WriteFile(file, data, 0600); err != nil {
				b.Fatal(err)
			}
			cache := &RenderCacheClient{storageDir: root}
			store := imagecache.New("https://images.test", root)
			if _, err := store.StoreAndGetURL(b.Context(), data, "pjsk"); err != nil {
				b.Fatal(err)
			}
			b.Run("bytes_to_store", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					image, err := cache.readCacheFile(file)
					if err != nil {
						b.Fatal(err)
					}
					owned := cloneRenderBytes(image)
					if _, err := store.StoreAndGetURL(b.Context(), owned, "pjsk"); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("artifact_url", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					image, err := cache.cachedFile(file)
					if err != nil {
						b.Fatal(err)
					}
					if _, ok := store.URLForFile(b.Context(), image.FilePath()); !ok {
						b.Fatal("no URL")
					}
				}
			})
		})
	}
}
