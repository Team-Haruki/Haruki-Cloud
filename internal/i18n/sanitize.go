package i18n

import (
	"strings"
	"sync"
	"unicode"
)

// linePattern is one line of a catalog message with its placeholders as
// wildcards: the literal fragments must appear in order, the first one at
// the start of the line unless the template line starts with a placeholder,
// the last one at the end unless it ends with one.
type linePattern struct {
	fragments     []string
	anchoredStart bool
	anchoredEnd   bool
}

var (
	linePatternsOnce sync.Once
	linePatterns     []linePattern
)

func catalogLinePatterns() []linePattern {
	linePatternsOnce.Do(func() {
		c := mustLoad()
		seen := map[string]bool{}
		for _, locale := range c.locales {
			for _, entry := range c.entries[locale] {
				for _, line := range strings.Split(entry.Text, "\n") {
					line = strings.TrimSpace(line)
					if line == "" || seen[line] {
						continue
					}
					seen[line] = true
					if pattern, ok := newLinePattern(line); ok {
						linePatterns = append(linePatterns, pattern)
					}
				}
			}
		}
	})
	return linePatterns
}

// newLinePattern compiles a template line. A line made only of placeholders
// (and punctuation) gives no pattern: its content is a nested message (checked on its own) or
// a value, and a value alone on a line is not catalog text.
func newLinePattern(line string) (linePattern, bool) {
	locs := placeholderPattern.FindAllStringIndex(line, -1)
	pattern := linePattern{anchoredStart: true, anchoredEnd: true}
	last := 0
	for _, loc := range locs {
		if loc[0] == 0 {
			pattern.anchoredStart = false
		}
		if loc[1] == len(line) {
			pattern.anchoredEnd = false
		}
		if fragment := strings.TrimSpace(line[last:loc[0]]); fragment != "" {
			pattern.fragments = append(pattern.fragments, fragment)
		}
		last = loc[1]
	}
	if fragment := strings.TrimSpace(line[last:]); fragment != "" {
		pattern.fragments = append(pattern.fragments, fragment)
	}
	// Fragments without a letter (a lone "：" between two placeholders) would
	// match almost any line, so such a line is treated like a placeholder-only
	// line.
	return pattern, strings.IndexFunc(strings.Join(pattern.fragments, ""), unicode.IsLetter) >= 0
}

func (p linePattern) match(line string) bool {
	body := line
	fragments := p.fragments
	if p.anchoredStart {
		if !strings.HasPrefix(body, fragments[0]) {
			return false
		}
		body = body[len(fragments[0]):]
		fragments = fragments[1:]
		if len(fragments) == 0 {
			return !p.anchoredEnd || strings.TrimSpace(body) == ""
		}
	}
	if p.anchoredEnd {
		last := fragments[len(fragments)-1]
		if !strings.HasSuffix(body, last) {
			return false
		}
		body = body[:len(body)-len(last)]
		fragments = fragments[:len(fragments)-1]
	}
	for _, fragment := range fragments {
		idx := strings.Index(body, fragment)
		if idx < 0 {
			return false
		}
		body = body[idx+len(fragment):]
	}
	return true
}

// IsCatalogLine reports whether line could have been rendered from a line of
// a catalog message (in any locale).
func IsCatalogLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	for _, pattern := range catalogLinePatterns() {
		if pattern.match(line) {
			return true
		}
	}
	return false
}

// SanitizeLines keeps the lines of text that come from catalog messages and
// returns the dropped ones separately (for logging). It is the last line of
// defence on reply paths that show error text: a line that no catalog message
// can produce (a raw upstream error, an internal cause) never reaches a user.
func SanitizeLines(text string) (string, []string) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	kept := lines[:0:0]
	var dropped []string
	for _, line := range lines {
		if IsCatalogLine(line) {
			kept = append(kept, line)
			continue
		}
		dropped = append(dropped, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n")), dropped
}
