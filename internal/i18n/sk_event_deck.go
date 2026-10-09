package i18n

import "strings"

// LinesText renders msgs in DefaultLocale, one message per line, skipping
// zero messages. It is the plain-text counterpart of passing a []Message as
// a placeholder value.
func LinesText(msgs []Message) string {
	lines := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		if msg.IsZero() {
			continue
		}
		lines = append(lines, msg.String())
	}
	return strings.Join(lines, "\n")
}

// Decimal shows a plain number with at most decimals decimals and without
// trailing zeros: Decimal(2.880, 3) -> "2.88", Decimal(5, 1) -> "5".
func Decimal(value float64, decimals int) string {
	return trimDecimals(value, decimals)
}
