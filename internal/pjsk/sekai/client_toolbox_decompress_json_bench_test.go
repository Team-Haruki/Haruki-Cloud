package sekai

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// toolboxJSONBenchmarkPayload is a synthetic suite-like JSON array of card
// rows (seeded values, no user data), so the compression ratio resembles a
// real suite better than the repeated-row payload.
func toolboxJSONBenchmarkPayload(size int) []byte {
	rng := rand.New(rand.NewPCG(3, uint64(size)))
	var out bytes.Buffer
	out.WriteString(`{"upload_time":1700000000,"userCards":[`)
	for i := 0; out.Len() < size; i++ {
		if i > 0 {
			out.WriteByte(',')
		}
		fmt.Fprintf(&out, `{"cardId":%d,"level":%d,"exp":%d,"skillLevel":%d,"masterRank":%d,"specialTrainingStatus":"%s","defaultImage":"%s","createdAt":%d,"episodes":[{"cardEpisodeId":%d,"scenarioStatus":"already_read","isNotSkipped":%t}]}`,
			i, 1+rng.IntN(60), rng.IntN(100000), 1+rng.IntN(4), rng.IntN(6), []string{"not_doing", "done"}[rng.IntN(2)],
			[]string{"original", "special_training"}[rng.IntN(2)], 1600000000000+rng.Int64N(1e11), i*2, rng.IntN(2) == 1)
	}
	out.WriteString(`]}`)
	return out.Bytes()
}

// BenchmarkToolboxSuiteDecompress measures decompressContext on JSON-shaped
// bodies of production suite sizes (zstd frames carry the content size, as
// EncodeAll writes them).
func BenchmarkToolboxSuiteDecompress(b *testing.B) {
	for _, mib := range []int{5, 13} {
		plain := toolboxJSONBenchmarkPayload(mib << 20)
		encoder, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
		if err != nil {
			b.Fatal(err)
		}
		compressed := encoder.EncodeAll(plain, nil)
		encoder.Close()
		b.Run(fmt.Sprintf("%dMiB", mib), func(b *testing.B) {
			client := NewToolboxClient(nil)
			defer client.Close()
			resp := toolboxCompressedResponse(compressed)
			b.SetBytes(int64(len(plain)))
			b.ReportAllocs()
			for b.Loop() {
				out, err := client.decompressContext(context.Background(), resp)
				if err != nil || len(out) != len(plain) {
					b.Fatalf("size=%d err=%v", len(out), err)
				}
			}
			b.ReportMetric(float64(len(compressed))/float64(len(plain)), "compressed/raw")
		})
	}
}
