package vlive

import "testing"

func TestSoloLiveShortName(t *testing.T) {
	cases := []struct{ name, group, want string }{
		{"Solo Live（一歌）", "Solo Live", "（一歌）"},
		{"Solo Live  Miku ", "Solo Live", "Miku"},
		{"Solo Live", "Solo Live", "Solo Live"},
		{"Other Live", "Solo Live", "Other Live"},
		{"Solo Live（一歌）", "", "Solo Live（一歌）"},
	}
	for _, tc := range cases {
		if got := soloLiveShortName(tc.name, tc.group); got != tc.want {
			t.Errorf("soloLiveShortName(%q, %q) = %q, want %q", tc.name, tc.group, got, tc.want)
		}
	}
}
