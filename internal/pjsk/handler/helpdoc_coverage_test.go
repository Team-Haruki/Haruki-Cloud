package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode"

	corehandler "haruki-cloud/internal/handler"
	"haruki-cloud/internal/i18n"
)

var helpDocCodeSpan = regexp.MustCompile("`([^`\n]+)`")

// helpDocTriggers returns the command token of every backticked span that
// starts with "/" (e.g. "/查曲" from "`/查曲 <歌曲名> [难度]`").
func helpDocTriggers(markdown string) []string {
	var triggers []string
	for _, match := range helpDocCodeSpan.FindAllStringSubmatch(markdown, -1) {
		span := strings.TrimSpace(match[1])
		if !strings.HasPrefix(span, "/") {
			continue
		}
		token := strings.Fields(span)[0]
		if token == "/" {
			continue
		}
		if !slices.Contains(triggers, token) {
			triggers = append(triggers, token)
		}
	}
	return triggers
}

// helpDocTriggerResolves reports whether a documented trigger reaches a
// registered command: exactly (region-prefixed forms are registered too),
// after stripping a region prefix, or as a registered command followed
// directly by a non-Chinese argument (e.g. "/sk100").
func helpDocTriggerResolves(trigger string) bool {
	for _, candidate := range []string{trigger, normalizeCommandHelpAlias(trigger)} {
		if _, ok := corehandler.LookupCommandHandler(candidate); ok {
			return true
		}
	}
	matched := corehandler.MatchCommandHandler(trigger)
	if matched.Handler == nil {
		return false
	}
	rest := strings.TrimSpace(string(matched.ArgText))
	return rest != "" && !strings.ContainsFunc(rest, func(r rune) bool { return unicode.Is(unicode.Han, r) })
}

func readHelpDocAllowlist(t *testing.T, name string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	entries := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			entries[line] = true
		}
	}
	return entries
}

// checkHelpDocAllowlist fails for findings missing from the allowlist and
// for allowlist entries that no longer fail (so the list only shrinks).
func checkHelpDocAllowlist(t *testing.T, name string, findings []string, explain string) {
	t.Helper()
	allowed := readHelpDocAllowlist(t, name)
	sort.Strings(findings)
	for _, finding := range findings {
		if !allowed[finding] {
			t.Errorf("%s (%s)", strings.ReplaceAll(finding, "\t", " "), explain)
		}
	}
	for entry := range allowed {
		if !slices.Contains(findings, entry) {
			t.Errorf("testdata/%s: %q is fixed; remove it from the list", name, entry)
		}
	}
}

// TestHelpDocTriggersAreRegistered checks that every command a help document
// shows in backticks can actually be sent. Known gaps are listed in
// testdata/helpdoc_unregistered_triggers.txt; fix the document or register
// the documented spelling as an alias, then delete the line.
func TestHelpDocTriggersAreRegistered(t *testing.T) {
	EnsureCommandHandlersRegistered()
	var findings []string
	for _, key := range i18n.HelpDocKeys(i18n.DefaultLocale) {
		markdown, _, err := i18n.HelpDoc(i18n.DefaultLocale, key)
		if err != nil {
			t.Fatal(err)
		}
		for _, trigger := range helpDocTriggers(markdown) {
			if !helpDocTriggerResolves(trigger) {
				findings = append(findings, key+".md\t"+trigger)
			}
		}
	}
	checkHelpDocAllowlist(t, "helpdoc_unregistered_triggers.txt", findings, "help document shows a command that is not registered")
}

// TestEveryRouteHasHelpDoc checks that each registered bot route has its own
// help document (locales/<locale>/help/<route path with / as _>.md). Known
// gaps are listed in testdata/helpdoc_missing_routes.txt.
func TestEveryRouteHasHelpDoc(t *testing.T) {
	EnsureCommandHandlersRegistered()
	var findings []string
	for _, route := range corehandler.ListBotRoutes() {
		if _, ok, _ := i18n.HelpDoc(i18n.DefaultLocale, commandHelpDocKey(route.Path)); !ok {
			findings = append(findings, route.Path)
		}
	}
	checkHelpDocAllowlist(t, "helpdoc_missing_routes.txt", findings, "route has no help document")
}

func TestHelpDocTriggerExtraction(t *testing.T) {
	got := helpDocTriggers("- `/查曲 <歌曲名> [难度]`\n- `/jp查曲`、`music123`、`/`\n- `/查曲 x`")
	if !slices.Equal(got, []string{"/查曲", "/jp查曲"}) {
		t.Fatalf("helpDocTriggers() = %v", got)
	}
	EnsureCommandHandlersRegistered()
	for trigger, want := range map[string]bool{
		"/查曲":        true,
		"/jp查曲":      true,
		"/kill":      true,
		"/不存在的指令xyz": false,
	} {
		if helpDocTriggerResolves(trigger) != want {
			t.Errorf("helpDocTriggerResolves(%q) = %v", trigger, !want)
		}
	}
}
