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

func TestGlobalResolver_Help(t *testing.T) {
	resolver := NewGlobalCommandResolver(nil)

	tests := []struct {
		name  string
		input string
	}{
		{"Standard help flag", "/card -h"},
		{"Standard help command", "/help"},
		{"Standard help command with slash", "/帮助"},
		{"Help with word", "帮助"},
		{"Empty string defaults to help", ""},
		{"Only spaces defaults to help", "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := resolver.Resolve(tt.input)
			if err != nil && err.Error() != "无法识别指令格式，请发送 /help 查看说明" {
				t.Errorf("unexpected error: %v", err)
			}
			if err == nil {
				if res.Module != ModuleHelp {
					t.Errorf("expected module Help, got: %v", res.Module)
				}
				if !res.IsHelp {
					t.Errorf("expected IsHelp to be true")
				}
			}
		})
	}
}

func TestGlobalResolver_Resolution(t *testing.T) {
	resolver := NewGlobalCommandResolver(nil)

	tests := []struct {
		name         string
		input        string
		expectedMode string
		expectErr    bool
	}{
		{"Player Trace No Flag", "/sk-player-trace", "sk-player-trace", false},
		{"Player Trace With Extraneous Text", "/sk玩家轨迹 a b c", "sk-player-trace", false},
		{"Line", "/sk线", "sk-line", false},
		{"Event List with JP region", "/jp/event-list", "event-list", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := resolver.Resolve(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if res.Mode != tt.expectedMode {
					t.Errorf("expected mode: %s, got: %s", tt.expectedMode, res.Mode)
				}
			}
		})
	}
}
