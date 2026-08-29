package parser

import (
	"strings"
	"testing"
)

func TestCommandParserRecognizedForms(t *testing.T) {
	parser := NewCommandParser()
	tests := []struct {
		input      string
		wantType   CommandType
		wantTarget string
		wantParam1 int
		wantParam2 int
		wantMulti  int
	}{
		{"", CmdTypeEventQuerySelf, "", 0, 0, 0},
		{"bind 1234567890", CmdTypeBind, "1234567890", 0, 0, 0},
		{"unbind", CmdTypeUnbind, "", 0, 0, 0},
		{"@123456789", CmdTypeEventQueryAt, "123456789", 0, 0, 0},
		{"10-20", CmdTypeEventQueryRankRange, "", 10, 20, 0},
		{"1 3 5", CmdTypeEventQueryMultiRank, "", 0, 0, 3},
		{"100", CmdTypeEventQueryRank, "", 100, 0, 0},
		{"12345678901234", CmdTypeEventQueryUID, "12345678901234", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parser.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.input, err)
			}
			if got.Type != tt.wantType || got.TargetID != tt.wantTarget || got.Param1 != tt.wantParam1 || got.Param2 != tt.wantParam2 || len(got.MultiArgs) != tt.wantMulti {
				t.Fatalf("Parse(%q) = %#v", tt.input, got)
			}
			if got.Original != strings.TrimSpace(tt.input) {
				t.Fatalf("original = %q", got.Original)
			}
		})
	}
}

func TestCommandParserRejectsInvalidForms(t *testing.T) {
	parser := NewCommandParser()
	for _, input := range []string{
		"bind short",
		"bind 123456789x",
		"@not-numeric",
		"20-10",
		"one two",
		"10 x",
		"unknown",
		"1-2-3",
	} {
		if got, err := parser.Parse(input); err == nil || got != nil {
			t.Fatalf("Parse(%q) = %#v, %v", input, got, err)
		}
	}
}
