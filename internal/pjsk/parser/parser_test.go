package parser

import (
	"testing"
)

func TestExtractRegionPrefix(t *testing.T) {
	e := NewExtractor(nil)

	tests := []struct {
		name      string
		input     string
		expected  string
		remaining string
		found     bool
	}{
		{"Valid JP Prefix", "/jp/event-list", "jp", "/event-list", true},
		{"Valid EN Prefix Space", "/en event-list", "en", "/event-list", true},
		{"Valid CN Prefix Without Slash", "/cn card", "cn", "/card", true},
		{"Valid KR Prefix Mixed Case", "/kR/music", "kr", "/music", true},
		{"Valid TW Prefix Extra Slashes", "/tw//music", "tw", "/music", true},
		{"Valid JP Prefix With Spaces Before Slash", "/jp  /music", "jp", "/music", true},
		{"No Prefix", "/event-list", "", "/event-list", false},
		{"False Positive Match", "/jpop", "", "/jpop", false},
		{"Empty String", "", "", "", false},
		{"Not Starting With Slash", "jp/event", "", "jp/event", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := e.ExtractRegionPrefix(tt.input)
			if res.Found != tt.found {
				t.Errorf("expected found: %v, got: %v", tt.found, res.Found)
			}
			if res.Value != tt.expected {
				t.Errorf("expected value: %s, got: %s", tt.expected, res.Value)
			}
			if res.Remaining != tt.remaining {
				t.Errorf("expected remaining: %s, got: %s", tt.remaining, res.Remaining)
			}
		})
	}
}

func TestExtractPreview(t *testing.T) {
	e := NewExtractor(nil)

	tests := []struct {
		name      string
		input     string
		remaining string
		found     bool
	}{
		{"With -p", "/sk-player-trace -p", "/sk-player-trace", true},
		{"With --preview", "/card-list --preview", "/card-list", true},
		{"Misleading word", "/sk-player-trace", "/sk-player-trace", false},
		{"Inside word", "this-pat is cool", "this-pat is cool", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := e.ExtractPreview(tt.input)
			if res.Found != tt.found {
				t.Errorf("expected found: %v, got: %v", tt.found, res.Found)
			}
			if res.Remaining != tt.remaining {
				t.Errorf("expected remaining: %s, got: %s", tt.remaining, res.Remaining)
			}
		})
	}
}

func TestExtractUid(t *testing.T) {
	e := NewExtractor(nil)

	tests := []struct {
		name      string
		input     string
		expected  string
		remaining string
		found     bool
	}{
		{"Index Selector", "u12 目标", "u12", "目标", true},
		{"Index Selector Ignores mu Prefix", "mu12 目标", "", "mu12 目标", false},
		{"Game UID", "12345678901234 查档", "12345678901234", "查档", true},
		{"QQ Mention", "@123456789 查询", "@123456789", "查询", true},
		{"Sequential Override", "u2 12345678901234 @123456789", "@123456789", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := e.ExtractUid(tt.input)
			if res.Found != tt.found {
				t.Fatalf("expected found=%v, got %v", tt.found, res.Found)
			}
			if res.Value != tt.expected {
				t.Fatalf("expected value=%q, got %q", tt.expected, res.Value)
			}
			if res.Remaining != tt.remaining {
				t.Fatalf("expected remaining=%q, got %q", tt.remaining, res.Remaining)
			}
		})
	}
}
