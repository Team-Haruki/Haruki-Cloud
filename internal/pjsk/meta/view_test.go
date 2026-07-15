package meta

import (
	"fmt"
	"testing"
)

func TestPrepareBuildsImmutableIndexedView(t *testing.T) {
	payload := []byte(`[
		{"music_id":1,"difficulty":"master","tap_count":10,"skill_score_solo":[1,2]},
		{"music_id":2,"difficulty":"expert","tap_count":20}
	]`)
	processed, view, err := Prepare(payload)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(processed) == 0 || view == nil {
		t.Fatal("Prepare() returned an empty generation")
	}

	entry, ok := view.Find(1, " MASTER ")
	if !ok || entry.Int("tap_count") != 10 {
		t.Fatalf("unexpected indexed entry: ok=%t entry=%+v", ok, entry.Map())
	}
	row := entry.Map()
	row["tap_count"] = float64(999)
	row["skill_score_solo"].([]any)[0] = float64(999)
	again, _ := view.Find(1, "master")
	if again.Int("tap_count") != 10 || again.FloatSlice("skill_score_solo")[0] != 1 {
		t.Fatal("caller mutation escaped into immutable view")
	}
	if _, ok := view.Find(10000, "master"); !ok {
		t.Fatal("synthetic omakase entry was not indexed")
	}
}

func BenchmarkMusicMetaViewFind(b *testing.B) {
	rows := make([]byte, 0, 1<<20)
	rows = append(rows, '[')
	for musicID := 1; musicID <= 1000; musicID++ {
		if musicID > 1 {
			rows = append(rows, ',')
		}
		rows = append(rows, fmt.Sprintf(`{"music_id":%d,"difficulty":"master","tap_count":%d}`, musicID, musicID)...)
	}
	rows = append(rows, ']')
	view, err := Parse(rows)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry, ok := view.Find(i%1000+1, "master")
		if !ok || entry.Int("tap_count") == 0 {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkMusicMetaParseAndFind(b *testing.B) {
	payload := []byte(`[{"music_id":1,"difficulty":"master","tap_count":10}]`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		view, err := Parse(payload)
		if err != nil {
			b.Fatal(err)
		}
		if _, ok := view.Find(1, "master"); !ok {
			b.Fatal("lookup failed")
		}
	}
}
