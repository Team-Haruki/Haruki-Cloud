package i18n

import (
	"fmt"
	"strings"
	"testing"
)

// TestEveryMessageRendersWithSampleData renders every catalog message of
// every locale through the same path production uses, with a sample value for
// each placeholder: every message must render without a template error, use
// every value it is given and leave no template syntax behind.
func TestEveryMessageRendersWithSampleData(t *testing.T) {
	c := mustLoad()
	for _, locale := range Locales() {
		for _, entry := range Entries(locale) {
			data := Data{}
			for _, name := range entry.Placeholders {
				if IsUserPlaceholder(name) {
					data[name] = UserText("‹" + name + "›")
					continue
				}
				data[name] = "‹" + name + "›"
			}
			text, err := c.localize(RenderOptions{Locale: locale}, entry.ID, data)
			if err != nil {
				t.Errorf("%s %s: %v", locale, entry.ID, err)
				continue
			}
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s %s: renders empty", locale, entry.ID)
			}
			if strings.Contains(text, "{{") || strings.Contains(text, "<no value>") {
				t.Errorf("%s %s: template syntax left in %q", locale, entry.ID, text)
			}
			for name, value := range data {
				if !strings.Contains(text, fmt.Sprint(value)) {
					t.Errorf("%s %s: placeholder %s missing from %q", locale, entry.ID, name, text)
				}
			}
			// The public accessor must return the same text.
			if got := M(entry.ID, data).In(locale); got != text {
				t.Errorf("%s %s: M().In() = %q, localize = %q", locale, entry.ID, got, text)
			}
		}
	}
}
