package drawing

import (
	"fmt"
	"testing"
)

// The in-memory transport returns the same stored artifact for both APIs.
// This measures Cloud's client boundary, not rendering or real network latency.
func BenchmarkImageResultPipeline(b *testing.B) {
	for _, size := range []int{1 << 20, 4 << 20} {
		for _, warm := range []bool{false, true} {
			cacheState := "ColdRef"
			if warm {
				cacheState = "WarmIndex"
			}
			for _, imageResult := range []bool{false, true} {
				api := "Bytes"
				if imageResult {
					api = "ImageResult"
				}
				b.Run(fmt.Sprintf("%dMiB/%s/%s", size>>20, cacheState, api), func(b *testing.B) {
					client, objects, index, renders := newImagePipelineClient(b, size, warm)
					request := &MysekaiShopRequest{Title: "shop"}
					b.ReportAllocs()
					for b.Loop() {
						if imageResult {
							image, err := client.GenerateMysekaiShopImage(request)
							if err != nil || image.Ref() == nil {
								b.Fatalf("image ref=%v err=%v", image.Ref(), err)
							}
						} else {
							data, err := client.GenerateMysekaiShop(request)
							if err != nil || len(data) != size {
								b.Fatalf("image len=%d err=%v", len(data), err)
							}
						}
					}
					b.ReportMetric(float64(objects.gets.Load())/float64(b.N), "Get/op")
					b.ReportMetric(float64(objects.readBytes.Load())/float64(b.N), "read_B/op")
					b.ReportMetric(float64(index.lookups.Load())/float64(b.N), "index/op")
					b.ReportMetric(float64(renders.Load())/float64(b.N), "render/op")
				})
			}
		}
	}
}
