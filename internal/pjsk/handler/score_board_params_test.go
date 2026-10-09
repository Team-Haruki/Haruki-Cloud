package handler

import (
	"strings"
	"testing"

	"haruki-cloud/internal/i18n"
)

// The examples in the music board parameter errors are typed input: each
// one, as quoted in the catalog, must parse.
func TestMusicBoardParamErrorExamplesParse(t *testing.T) {
	cases := []struct {
		id      string
		example string
		parse   func(string) error
	}{
		{"score.board.power_invalid", "综合20w", func(s string) error { _, _, err := extractMusicBoardPower(s); return err }},
		{"score.board.bonus_invalid", "加成250", func(s string) error { _, _, err := extractMusicBoardDeckBonus(s); return err }},
		{"score.board.interval_invalid", "间隔30", func(s string) error { _, _, err := extractMusicBoardInterval(s); return err }},
	}
	for _, tc := range cases {
		if text := i18n.T(tc.id); !strings.Contains(text, "“"+tc.example+"”") {
			t.Errorf("%s = %q, want the example “%s”", tc.id, text, tc.example)
		}
		if err := tc.parse(tc.example); err != nil {
			t.Errorf("%s example %q does not parse: %v", tc.id, tc.example, err)
		}
	}
}
