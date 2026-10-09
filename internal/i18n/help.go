package i18n

import (
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// HelpDoc returns the Markdown help document key (a route path with "/"
// replaced by "_", e.g. "music" or "deck_event") in locale, falling back to
// DefaultLocale. ok is false when no locale has the document.
func HelpDoc(locale Locale, key string) (markdown string, ok bool, err error) {
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsAny(key, `/\`) || strings.HasPrefix(key, ".") {
		return "", false, nil
	}
	for _, candidate := range []Locale{locale, DefaultLocale} {
		data, err := fs.ReadFile(localeFS, helpDocPath(candidate, key))
		if err == nil {
			return strings.TrimSpace(string(data)), true, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", false, err
		}
	}
	return "", false, nil
}

// HelpDocKeys lists the help document keys of locale, sorted.
func HelpDocKeys(locale Locale) []string {
	files, _ := fs.Glob(localeFS, path.Join("locales", string(locale), "help", "*.md"))
	keys := make([]string, 0, len(files))
	for _, file := range files {
		keys = append(keys, strings.TrimSuffix(path.Base(file), ".md"))
	}
	slices.Sort(keys)
	return keys
}

// HelpDocFile is the path of a help document inside the embedded tree, for
// tests and review tooling.
func HelpDocFile(locale Locale, key string) string {
	return helpDocPath(locale, key)
}

func helpDocPath(locale Locale, key string) string {
	return path.Join("locales", string(locale), "help", key+".md")
}
