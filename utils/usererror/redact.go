package usererror

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DefaultLogMessageLimit bounds a redacted error message in a log record.
const DefaultLogMessageLimit = 300

var logRedactions = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{urlPattern, "<url>"},
	{regexp.MustCompile(`(?i)\bbearer\s+\S+`), "bearer <redacted>"},
	{regexp.MustCompile(`(?i)\b(access_token|api[_-]?key|apikey|authorization|jwt|passw(?:or)?d|secret|session(?:_id)?|sid|token)\b(\s*[=:]\s*)\S+`), "$1$2<redacted>"},
	{regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`), "<email>"},
	{regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`), "<uuid>"},
	{regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}(?::\d+)?\b`), "<ip>"},
	// Opaque credentials, digests and encoded blobs.
	{regexp.MustCompile(`[A-Za-z0-9_\-+/=]{24,}`), "<token>"},
	// Account, game and platform user IDs and millisecond timestamps. Short
	// numbers (event, card, music IDs, status codes) stay readable.
	{regexp.MustCompile(`\d{5,}`), "<n>"},
}

// RedactForLog turns an error message into a single bounded line that is safe
// to log: URLs, IP addresses, e-mail addresses, credentials, long opaque
// tokens and IDs of five or more digits are replaced by placeholders, control
// characters and line breaks become spaces, and the result is cut to limit
// runes (DefaultLogMessageLimit when limit <= 0).
func RedactForLog(message string, limit int) string {
	if limit <= 0 {
		limit = DefaultLogMessageLimit
	}
	message = strings.ToValidUTF8(message, "?")
	for _, redaction := range logRedactions {
		message = redaction.pattern.ReplaceAllString(message, redaction.replacement)
	}
	message = strings.Join(strings.FieldsFunc(message, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}), " ")
	if utf8.RuneCountInString(message) <= limit {
		return message
	}
	runes := []rune(message)
	return string(runes[:limit]) + "…"
}
