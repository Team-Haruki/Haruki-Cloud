package handler

import (
	"regexp"
	"strings"
	"testing"

	"haruki-cloud/internal/i18n"
)

// The help image renderer draws every list item that contains a colon as a
// definition row: the text before the first "：" (or, without one, the first
// ":") is the label, drawn in bold 21 px in a key column 190 px wide that
// neither wraps nor clips, and the rest starts at the column's right edge. A
// wider label is drawn on top of its value. These constants mirror the
// renderer's help layout (_append_command_help_definition_line).
const (
	helpLabelFontPx    = 21
	helpLabelColumnPx  = 190
	helpLabelMinGapPx  = 8
	helpLabelMaxPx     = helpLabelColumnPx - helpLabelMinGapPx
	helpLabelEmUnits   = 1000
	helpLabelWideGlyph = helpLabelEmUnits
)

// helpLabelASCIIAdvance holds the advance widths of U+0020..U+007E in the
// renderer's bold font (Source Han Sans SC Bold), in 1/1000 em. Every other
// rune in a label (Han, kana, full-width punctuation) is one em.
var helpLabelASCIIAdvance = [...]int{
	227, 370, 574, 590, 590, 963, 740, 325, 378, 378, 507, 590, 325, 370, 325, 387,
	590, 590, 590, 590, 590, 590, 590, 590, 590, 590, 325, 325, 590, 590, 590, 514,
	1007, 641, 681, 656, 714, 615, 585, 717, 757, 330, 568, 686, 578, 853, 749, 770,
	667, 770, 682, 624, 625, 748, 619, 915, 627, 580, 613, 378, 387, 378, 590, 567,
	626, 591, 644, 527, 644, 581, 372, 597, 640, 304, 306, 604, 315, 964, 641, 626,
	644, 644, 436, 495, 421, 637, 576, 863, 562, 574, 511, 378, 296, 378, 590,
}

var helpLabelLink = regexp.MustCompile(`\[([^\]]+)]\([^)]+\)`)

// helpLabelWidthPx estimates how wide label is drawn in the help image.
func helpLabelWidthPx(label string) float64 {
	units := 0
	for _, r := range label {
		if r >= 0x20 && r <= 0x7e {
			units += helpLabelASCIIAdvance[r-0x20]
			continue
		}
		units += helpLabelWideGlyph
	}
	return float64(units) * helpLabelFontPx / helpLabelEmUnits
}

// helpDefinitionLabels returns the labels the help renderer would draw for
// markdown, the way it parses list items.
func helpDefinitionLabels(markdown string) []string {
	var labels []string
	inFence := false
	for _, raw := range strings.Split(markdown, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence || len(line) < 3 || !strings.ContainsRune("-*+", rune(line[0])) || (line[1] != ' ' && line[1] != '\t') {
			continue
		}
		item := helpLabelLink.ReplaceAllString(strings.TrimSpace(line[2:]), "$1")
		item = strings.NewReplacer("`", "", "**", "", "__", "", `\`, "").Replace(item)
		label, value, found := strings.Cut(item, "：")
		if !found {
			label, value, found = strings.Cut(item, ":")
		}
		if found && strings.TrimSpace(label) != "" && strings.TrimSpace(value) != "" {
			labels = append(labels, strings.TrimSpace(label))
		}
	}
	return labels
}

// TestHelpDefinitionLabelsFitTheKeyColumn keeps every definition-row label of
// every help image, generated sections included, inside the renderer's key
// column. Shorten the label, move the detail after the colon, or write the
// item without a colon.
func TestHelpDefinitionLabelsFitTheKeyColumn(t *testing.T) {
	EnsureCommandHandlersRegistered()
	for _, key := range i18n.HelpDocKeys(i18n.DefaultLocale) {
		path := strings.ReplaceAll(key, "_", "/")
		if key == commandHelpGenericKey {
			path = ""
		}
		markdown, err := commandHelpMarkdownIn(i18n.DefaultLocale, &CommandRequest{CommandPath: path})
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		for _, label := range helpDefinitionLabels(markdown) {
			if width := helpLabelWidthPx(label); width > helpLabelMaxPx {
				t.Errorf("%s.md: label %q is about %.0f px wide; the help image's key column fits %d px", key, label, width, helpLabelMaxPx)
			}
		}
	}
}

func TestHelpDefinitionLabelsParseLikeTheRenderer(t *testing.T) {
	markdown := strings.Join([]string{
		"- `event123`、`活动123` 或活动 ID：指定活动",
		"- 没有冒号的说明",
		"- `/禁止别名提交 qq:123456789`",
		"```text",
		"- 代码块：不算",
		"```",
		"## 区服",
	}, "\n")
	labels := helpDefinitionLabels(markdown)
	want := []string{"event123、活动123 或活动 ID", "/禁止别名提交 qq"}
	if strings.Join(labels, "|") != strings.Join(want, "|") {
		t.Fatalf("helpDefinitionLabels() = %q, want %q", labels, want)
	}
	// Widths measured with the renderer's font: 队友综合力300000 is 179 px.
	if got := helpLabelWidthPx("队友综合力300000"); got < 178 || got > 180 {
		t.Fatalf("helpLabelWidthPx(队友综合力300000) = %.1f, want about 179", got)
	}
	if got := helpLabelWidthPx(labels[0]); got <= helpLabelMaxPx {
		t.Fatalf("helpLabelWidthPx(%q) = %.1f, want over %d", labels[0], got, helpLabelMaxPx)
	}
}
