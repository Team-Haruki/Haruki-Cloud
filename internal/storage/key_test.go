package storage

import (
	"errors"
	"strings"
	"testing"
)

func TestCleanKey(t *testing.T) {
	long := strings.Repeat("a", MaxKeyBytes)
	cases := []struct {
		raw  string
		want Key
		ok   bool
	}{
		{"a", "a", true},
		{"a/b/c.png", "a/b/c.png", true},
		{"./a", "a", true},
		{"/a/b", "a/b", true},
		{"//a", "a", true},
		{"././a", "a", true},
		{"./.hidden", ".hidden", true},
		{"a/./b", "a/b", true},
		{"a/.", "a", true},
		{`a\b`, "a/b", true},
		{`\a\b`, "a/b", true},
		{"日本/画像.png", "日本/画像.png", true},
		{"with space/+plus", "with space/+plus", true},
		{"a..b/c", "a..b/c", true},
		{long, Key(long), true},
		{"asset/jp-assets/ondemand/x.png", "asset/jp-assets/ondemand/x.png", true},
		{"", "", false},
		{"/", "", false},
		{"./", "", false},
		{".", "", false},
		{"..", "", false},
		{"a//b", "", false},
		{"a/", "", false},
		{"../x", "", false},
		{"a/..", "", false},
		{"a/../b", "", false},
		{"a/../../b", "", false},
		{`..\x`, "", false},
		{long + "a", "", false},
		{"a\x00b", "", false},
		{"a\nb", "", false},
		{"a\tb", "", false},
		{"\x1f", "", false},
	}
	for _, tc := range cases {
		got, err := CleanKey(tc.raw)
		if tc.ok {
			if err != nil || got != tc.want {
				t.Errorf("CleanKey(%q) = %q, %v; want %q", tc.raw, got, err, tc.want)
			}
			continue
		}
		if !errors.Is(err, ErrInvalidKey) || got != "" {
			t.Errorf("CleanKey(%q) = %q, %v; want ErrInvalidKey", tc.raw, got, err)
		}
	}
}

func TestInvalidKeyErrorTruncatesLongKeys(t *testing.T) {
	_, err := CleanKey(strings.Repeat("x", 2000))
	if err == nil || len(err.Error()) > 200 || !strings.Contains(err.Error(), "...") {
		t.Fatalf("error = %v", err)
	}
}

func TestJoin(t *testing.T) {
	got, err := Join("profile_bg", "jp", "123.png")
	if err != nil || got != "profile_bg/jp/123.png" {
		t.Fatalf("Join = %q, %v", got, err)
	}
	if _, err := Join("a", "", "b"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("Join with empty part error = %v", err)
	}
	if _, err := Join("a", "..", "b"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("Join with parent part error = %v", err)
	}
	if _, err := Join(); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("empty Join error = %v", err)
	}
}

func TestCleanPrefix(t *testing.T) {
	cases := []struct {
		raw  string
		want Key
		ok   bool
	}{
		{"", "", true},
		{"/", "", true},
		{"./", "", true},
		{"a", "a", true},
		{"a/", "a/", true},
		{`a\b\`, "a/b/", true},
		{"/a/b", "a/b", true},
		{"../", "", false},
		{"a//", "", false},
		{"a//b/", "", false},
	}
	for _, tc := range cases {
		got, err := CleanPrefix(tc.raw)
		if tc.ok != (err == nil) || got != tc.want {
			t.Errorf("CleanPrefix(%q) = %q, %v; want %q ok=%v", tc.raw, got, err, tc.want, tc.ok)
		}
	}
}
