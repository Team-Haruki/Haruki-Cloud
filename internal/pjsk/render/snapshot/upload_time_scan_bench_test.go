package snapshot

import "testing"

func BenchmarkPayloadUploadTime(b *testing.B) {
	payloads := benchmarkSyntheticPayloads()
	for _, name := range []string{"suite13MiB", "mysekai13MiB"} {
		data := payloads[name]
		b.Run(name+"/full_decode", func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			for b.Loop() {
				if v, err := parseTopLevelUploadTime(data); err != nil || v != 1700000000 {
					b.Fatal(v, err)
				}
			}
		})
		b.Run(name+"/scan", func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			for b.Loop() {
				if v, err := payloadUploadTime(data); err != nil || v != 1700000000 {
					b.Fatal(v, err)
				}
			}
		})
	}
}
