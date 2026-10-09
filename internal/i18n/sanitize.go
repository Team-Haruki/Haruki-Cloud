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
//
// A weak pattern has too little literal text to tell catalog text from a raw
// error line ("u{{.Index}} {{.Account}}", "{{.Value}}万", "{{.Region}}活动
// {{.ID}}"). It is only accepted for a reply that renders its own message.
type linePattern struct {
	fragments     []string
	anchoredStart bool
	anchoredEnd   bool
	weak          bool
}

var (
	linePatternsOnce sync.Once
	// linePatterns are the strong patterns of every catalog line.
	linePatterns []linePattern
	// messagePatterns are all patterns (weak ones included) of each message
	// ID, over every locale.
	messagePatterns map[string][]linePattern
)

func loadLinePatterns() {
	linePatternsOnce.Do(func() {
		c := mustLoad()
		seen := map[string]bool{}
		messagePatterns = map[string][]linePattern{}
		for _, locale := range c.locales {
			for id, entry := range c.entries[locale] {
				for _, line := range strings.Split(entry.Text, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					pattern, ok := newLinePattern(line)
					if !ok {
						continue
					}
					messagePatterns[id] = append(messagePatterns[id], pattern)
					if pattern.weak || seen[line] {
						continue
					}
					seen[line] = true
					linePatterns = append(linePatterns, pattern)
				}
			}
		}
	})
}

func catalogLinePatterns() []linePattern {
	loadLinePatterns()
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
	if strings.IndexFunc(strings.Join(pattern.fragments, ""), unicode.IsLetter) < 0 {
		return pattern, false
	}
	if len(locs) > 0 {
		weight := literalWeight(pattern.fragments)
		open := !pattern.anchoredStart && !pattern.anchoredEnd
		pattern.weak = weight < minLiteralWeight || (open && weight < minOpenLiteralWeight)
	}
	return pattern, true
}

const (
	// minLiteralWeight is the literal text a line with placeholders needs to
	// be recognised in any reply.
	minLiteralWeight = 2
	// minOpenLiteralWeight is the same for a line open at both ends, whose
	// literal text may appear anywhere in a line.
	minOpenLiteralWeight = 4
)

// literalWeight measures how distinctive literal fragments are: one per Han
// character and one per letter or digit in a Latin run of at least three.
// Single letters, short codes ("u", "T", "ID", "/h") and punctuation count
// nothing, because raw error text is full of them.
func literalWeight(fragments []string) int {
	weight := 0
	for _, fragment := range fragments {
		run := 0
		flush := func() {
			if run >= 3 {
				weight += run
			}
			run = 0
		}
		for _, r := range fragment {
			switch {
			case unicode.Is(unicode.Han, r):
				flush()
				weight++
			case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
				run++
			default:
				flush()
			}
		}
		flush()
	}
	return weight
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
	return isCatalogLine(line, nil)
}

func isCatalogLine(line string, own []linePattern) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	for _, pattern := range catalogLinePatterns() {
		if pattern.match(line) {
			return true
		}
	}
	for _, pattern := range own {
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
	return sanitizeLines(text, nil)
}

// SanitizeMessage renders m in locale and sanitizes it like SanitizeLines.
// The lines of m and of the messages nested in it are also recognised by
// their weak patterns, which no other reply may use.
func SanitizeMessage(m Message, locale Locale) (string, []string) {
	loadLinePatterns()
	var own []linePattern
	walkMessages(m, func(id string) {
		for _, pattern := range messagePatterns[id] {
			if pattern.weak {
				own = append(own, pattern)
			}
		}
	})
	return sanitizeLines(m.In(locale), own)
}

// walkMessages calls visit with the ID of m and of every message nested in
// its placeholder values.
func walkMessages(m Message, visit func(id string)) {
	if m.IsZero() {
		return
	}
	visit(m.ID)
	for _, value := range m.Data {
		switch nested := value.(type) {
		case Message:
			walkMessages(nested, visit)
		case []Message:
			for _, item := range nested {
				walkMessages(item, visit)
			}
		}
	}
}

func sanitizeLines(text string, own []linePattern) (string, []string) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	kept := lines[:0:0]
	var dropped []string
	for _, line := range lines {
		if isCatalogLine(line, own) {
			kept = append(kept, line)
			continue
		}
		dropped = append(dropped, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n")), dropped
}
