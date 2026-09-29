package sekai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// toolboxBenchmarkPayload combines repeated fields with varying values. It is
// synthetic and carries no user data; compression ratio is reported separately.
func toolboxBenchmarkPayload(size int) []byte {
	row := []byte(`{"id":123456789,"rank":45,"exp":12345,"level":60,"state":"available"},`)
	out := bytes.Repeat(row, size/len(row)+1)
	out = out[:size]
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < len(out); i += 128 {
		for j := i; j < min(i+32, len(out)); j++ {
			out[j] = byte('0' + rng.IntN(10))
		}
	}
	return out
}

func BenchmarkToolboxDecoderConfiguration(b *testing.B) {
	for _, mib := range []int{1, 5, 20} {
		b.Run(fmt.Sprintf("%dMiB", mib), func(b *testing.B) {
			plain := toolboxBenchmarkPayload(mib << 20)
			encoder, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
			if err != nil {
				b.Fatal(err)
			}
			compressed := encoder.EncodeAll(plain, nil)
			encoder.Close()
			for _, name := range []string{"PreviousNewReader", "ResetDefault", "ResetSync"} {
				b.Run(name, func(b *testing.B) {
					var decoder *zstd.Decoder
					var err error
					if name != "PreviousNewReader" {
						var options []zstd.DOption
						if name == "ResetSync" {
							options = append(options, zstd.WithDecoderConcurrency(1))
						}
						decoder, err = zstd.NewReader(nil, options...)
						if err != nil {
							b.Fatal(err)
						}
						defer decoder.Close()
					}
					b.SetBytes(int64(len(plain)))
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						if name == "PreviousNewReader" {
							decoder, err = zstd.NewReader(bytes.NewReader(compressed))
						} else {
							err = decoder.Reset(bytes.NewReader(compressed))
						}
						if err != nil {
							b.Fatal(err)
						}
						out, err := io.ReadAll(io.LimitReader(decoder, toolboxMaxDecompressedResponseBytes+1))
						if err != nil || len(out) != len(plain) {
							b.Fatalf("size=%d err=%v", len(out), err)
						}
						if name == "PreviousNewReader" {
							decoder.Close()
						} else if err := decoder.Reset(nil); err != nil {
							b.Fatal(err)
						}
					}
					b.ReportMetric(float64(len(compressed))/float64(len(plain)), "compressed/raw")
				})
			}
		})
	}
}

func BenchmarkToolboxDecompression(b *testing.B) {
	for _, mib := range []int{1, 5, 20} {
		b.Run(fmt.Sprintf("%dMiB", mib), func(b *testing.B) {
			plain := toolboxBenchmarkPayload(mib << 20)
			encoder, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
			if err != nil {
				b.Fatal(err)
			}
			compressed := encoder.EncodeAll(plain, nil)
			encoder.Close()
			for _, name := range []string{"Previous", "PreviousWithContext", "ClientReuse"} {
				b.Run(name, func(b *testing.B) {
					client := NewToolboxClient(nil)
					defer client.Close()
					resp := toolboxCompressedResponse(compressed)
					b.SetBytes(int64(len(plain)))
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						var out []byte
						var err error
						if name == "Previous" {
							decoder, initErr := zstd.NewReader(bytes.NewReader(compressed))
							if initErr != nil {
								b.Fatal(initErr)
							}
							out, err = io.ReadAll(io.LimitReader(decoder, toolboxMaxDecompressedResponseBytes+1))
							decoder.Close()
						} else if name == "PreviousWithContext" {
							decoder, initErr := zstd.NewReader(nil)
							if initErr != nil {
								b.Fatal(initErr)
							}
							out, err = readToolboxZstd(context.Background(), decoder, compressed, toolboxMaxDecompressedResponseBytes)
							decoder.Close()
						} else {
							out, err = client.decompressContext(context.Background(), resp)
						}
						if err != nil || len(out) != len(plain) {
							b.Fatalf("size=%d err=%v", len(out), err)
						}
					}
					b.ReportMetric(float64(len(compressed))/float64(len(plain)), "compressed/raw")
				})
			}
		})
	}
}
