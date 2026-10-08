package snapshot

import (
	"context"
	"fmt"
	"sync"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
)

var (
	syntheticPayloadsOnce sync.Once
	syntheticPayloads     map[string][]byte
)

func benchmarkSyntheticPayloads() map[string][]byte {
	syntheticPayloadsOnce.Do(func() {
		syntheticPayloads = map[string][]byte{
			"suite5MiB":    syntheticSuitePayload(5<<20, 1700000000),
			"suite13MiB":   syntheticSuitePayload(13<<20, 1700000000),
			"mysekai5MiB":  syntheticMySekaiPayload(5<<20, 1700000000),
			"mysekai13MiB": syntheticMySekaiPayload(13<<20, 1700000000),
		}
	})
	return syntheticPayloads
}

// BenchmarkSuitePayloadIngest measures what a raw-cache miss pays after the
// body is decompressed: reading upload_time and taking ownership of the bytes.
func BenchmarkSuitePayloadIngest(b *testing.B) {
	payloads := benchmarkSyntheticPayloads()
	for _, name := range []string{"suite5MiB", "suite13MiB", "mysekai5MiB", "mysekai13MiB"} {
		data := payloads[name]
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				payload := newPrivateDataPayloadContext(context.Background(), data)
				if payload.uploadTime != 1700000000 {
					b.Fatalf("upload_time = %d", payload.uploadTime)
				}
			}
		})
	}
}

// BenchmarkSuiteSnapshotBuild measures factory.Build on a suite-only payload
// (normalize, typed decode, model and the retained raw JSON).
func BenchmarkSuiteSnapshotBuild(b *testing.B) {
	payloads := benchmarkSyntheticPayloads()
	factory := NewDefaultSnapshotFactory(nil, nil)
	for _, name := range []string{"suite5MiB", "suite13MiB"} {
		data := payloads[name]
		for _, immutable := range []bool{false, true} {
			b.Run(fmt.Sprintf("%s/immutable=%t", name, immutable), func(b *testing.B) {
				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				for b.Loop() {
					input := BuildInput{Region: renderregion.JP, Source: "toolbox_live", SuiteJSON: data, SuiteJSONImmutable: immutable}
					if _, err := factory.Build(context.Background(), input); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
