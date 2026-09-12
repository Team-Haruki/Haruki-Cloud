package meta

import "testing"

func TestPrepareDoesNotDuplicateOmakaseRowsFromTheRegistry(t *testing.T) {
	// The registry feed already carries the omakase aggregate (music_id
	// 10000); preparing it again must keep exactly one copy per difficulty.
	raw := []byte(`[
		{"music_id":1,"difficulty":"master","music_time":100,"event_rate":110,"base_score":1.5,"base_score_auto":1.2,"fever_score":0.5,"fever_end_time":50,"tap_count":800,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1]},
		{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":110,"base_score":1.5,"base_score_auto":1.2,"fever_score":0.5,"fever_end_time":50,"tap_count":800,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1]}
	]`)
	processed, view, err := Prepare(raw)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if string(processed) != string(raw) {
		t.Fatalf("payload with omakase rows must pass through unchanged")
	}
	if view == nil || len(view.entries) != 2 {
		t.Fatalf("expected 2 entries, got %+v", view)
	}

	// The same input twice through the legacy path converges on one copy.
	first, _, err := Prepare([]byte(`[{"music_id":1,"difficulty":"master","music_time":100,"event_rate":110,"base_score":1.5,"base_score_auto":1.2,"fever_score":0.5,"fever_end_time":50,"tap_count":800,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1]}]`))
	if err != nil {
		t.Fatalf("Prepare legacy: %v", err)
	}
	second, secondView, err := Prepare(first)
	if err != nil {
		t.Fatalf("Prepare again: %v", err)
	}
	if string(second) != string(first) {
		t.Fatalf("second Prepare must be a no-op")
	}
	if len(secondView.entries) != 4 {
		t.Fatalf("expected 1 song + 3 omakase difficulties, got %d", len(secondView.entries))
	}
}

func TestResolveURLCoversLegacyAndRegistrySources(t *testing.T) {
	cases := []struct {
		source, base, region, want string
		wantErr                    bool
	}{
		{"", "", "jp", "https://sekai-data.3-3.dev/music_metas.json", false},
		{"legacy", "", "tw", "https://sekai-data.3-3.dev/music_metas-tc.json", false},
		{"legacy", "https://mirror.example/", "cn", "https://mirror.example/music_metas-cn.json", false},
		{"registry", "http://100.76.159.97:9998", "kr", "http://100.76.159.97:9998/v1/metas/kr/music_metas.json", false},
		{"registry", "http://registry/", "en", "http://registry/v1/metas/en/music_metas.json", false},
		{"registry", "", "jp", "", true},
		{"legacy", "", "xx", "", true},
		{"registry", "http://registry", "xx", "", true},
		{"bogus", "http://registry", "jp", "", true},
	}
	for _, tc := range cases {
		got, err := ResolveURL(tc.source, tc.base, tc.region)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%+v: expected error, got %s", tc, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("%+v: got %q err %v", tc, got, err)
		}
	}
}
