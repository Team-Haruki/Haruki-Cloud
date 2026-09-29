package provider

import "testing"

func TestNewResourceIconRelPath(t *testing.T) {
	tables := map[string]map[int]map[string]any{
		"honorBackgrounds.json": {10101: {"id": 10101, "assetbundleName": "honor_bg_style_01_01"}},
		"honorWords.json":       {10101: {"id": 10101, "assetbundleName": "honor_word_01_01"}},
		"virtualItems.json":     {222: {"id": 222, "assetbundleName": "fan_ichika"}},
	}
	rows := func(filename string) map[int]map[string]any { return tables[filename] }
	cases := []struct {
		typ  string
		id   int
		want string
	}{
		{"honor_background", 10101, "honor_background/honor_bg_style_01_01/degree_sub.png"},
		{"honor_word", 10101, "honor_word/honor_word_01_01_1.png"},
		{"virtual_item", 222, "thumbnail/virtual_live_item/fan_ichika.png"},
		{"virtual_item", 999, ""},
		{"material", 1, ""},
	}
	for _, tc := range cases {
		if got := NewResourceIconRelPath(tc.typ, tc.id, rows); got != tc.want {
			t.Errorf("NewResourceIconRelPath(%s, %d) = %q, want %q", tc.typ, tc.id, got, tc.want)
		}
	}
	// A region without the tables resolves nothing.
	empty := func(string) map[int]map[string]any { return nil }
	if got := NewResourceIconRelPath("honor_word", 10101, empty); got != "" {
		t.Errorf("old region = %q", got)
	}
}
