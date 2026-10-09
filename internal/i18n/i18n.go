// Package i18n holds every user-facing message of Haruki-Cloud.
//
// Messages live in TOML catalogs under locales/<locale>/<domain>.toml (one
// file per domain) and help documents under locales/<locale>/help/*.md. Both
// are embedded into the binary. Call sites never hold user copy themselves:
// they build a Message from a stable, namespaced ID and named placeholders
// (M or T), or use one of the typed helpers in this package.
//
// Only zh-CN is written today. Every lookup takes a Locale (or uses
// DefaultLocale), and a message missing from a locale falls back to
// DefaultLocale, so another locale can be added by dropping a new directory
// next to zh-CN. See docs/i18n.md.
package i18n

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/nicksnyder/go-i18n/v2/i18n/template"
	"golang.org/x/text/language"
)

// Locale is a BCP 47 language tag of a catalog directory, e.g. "zh-CN".
type Locale string

const (
	// ZhCN is Simplified Chinese, the only locale written today.
	ZhCN Locale = "zh-CN"
	// DefaultLocale is used when no locale is given, and is the fallback for
	// messages missing from another locale.
	DefaultLocale = ZhCN
)

// Data holds the named placeholders of a message. Keys are the placeholder
// names used in the catalog ({{.Name}}). A value that is itself a Message is
// rendered in the same locale before it is substituted.
type Data map[string]any

// Message is a catalog message ID plus its placeholder values. It is
// rendered lazily, so the same value can be shown in any locale.
type Message struct {
	ID   string
	Data Data
}

// M builds a Message. Pass the ID as a string literal and the placeholders as
// an i18n.Data literal so the catalog integrity test can check both.
func M(id string, data ...Data) Message {
	return Message{ID: id, Data: mergeData(data)}
}

// T renders a message in DefaultLocale. It is shorthand for M(id, data...).String().
func T(id string, data ...Data) string {
	return M(id, data...).String()
}

// IsZero reports whether the message has no ID.
func (m Message) IsZero() bool { return m.ID == "" }

// String renders the message in DefaultLocale.
func (m Message) String() string { return m.In(DefaultLocale) }

// In renders the message in locale, falling back to DefaultLocale for a
// message the locale does not have.
func (m Message) In(locale Locale) string {
	return render(locale, m.ID, m.Data)
}

func mergeData(data []Data) Data {
	switch len(data) {
	case 0:
		return nil
	case 1:
		return data[0]
	}
	merged := Data{}
	for _, d := range data {
		for k, v := range d {
			merged[k] = v
		}
	}
	return merged
}

type localeKey struct{}

// WithLocale returns a context carrying the requester's locale.
func WithLocale(ctx context.Context, locale Locale) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, localeKey{}, locale)
}

// LocaleFromContext returns the locale stored by WithLocale, or DefaultLocale.
func LocaleFromContext(ctx context.Context) Locale {
	if ctx != nil {
		if locale, ok := ctx.Value(localeKey{}).(Locale); ok && locale != "" {
			return locale
		}
	}
	return DefaultLocale
}

//go:embed locales
var localeFS embed.FS

// CatalogEntry is one message of a loaded catalog, exposed for the catalog
// lint and integrity tests and for review tooling.
type CatalogEntry struct {
	Locale       Locale
	File         string // path inside the embedded tree, e.g. locales/zh-CN/common.toml
	ID           string
	Description  string
	Text         string
	Placeholders []string
}

type catalog struct {
	bundle     *goi18n.Bundle
	localizers map[Locale]*goi18n.Localizer
	entries    map[Locale]map[string]CatalogEntry
	locales    []Locale
}

var (
	loadOnce   sync.Once
	loaded     *catalog
	loadErr    error
	textParser = &template.TextParser{Option: "missingkey=error"}

	placeholderPattern = regexp.MustCompile(`\{\{\s*\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
)

func load() (*catalog, error) {
	loadOnce.Do(func() {
		loaded, loadErr = loadCatalog(localeFS)
	})
	return loaded, loadErr
}

func mustLoad() *catalog {
	c, err := load()
	if err != nil {
		panic(fmt.Sprintf("i18n: load catalogs: %v", err))
	}
	return c
}

func loadCatalog(fsys fs.FS) (*catalog, error) {
	defaultTag := language.Make(string(DefaultLocale))
	bundle := goi18n.NewBundle(defaultTag)
	c := &catalog{
		bundle:     bundle,
		localizers: map[Locale]*goi18n.Localizer{},
		entries:    map[Locale]map[string]CatalogEntry{},
	}
	dirs, err := fs.ReadDir(fsys, "locales")
	if err != nil {
		return nil, err
	}
	unmarshal := map[string]goi18n.UnmarshalFunc{"toml": toml.Unmarshal}
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		locale := Locale(dir.Name())
		tag, err := language.Parse(string(locale))
		if err != nil {
			return nil, fmt.Errorf("locale directory %q: %w", locale, err)
		}
		files, err := fs.Glob(fsys, path.Join("locales", string(locale), "*.toml"))
		if err != nil {
			return nil, err
		}
		entries := map[string]CatalogEntry{}
		for _, file := range files {
			buf, err := fs.ReadFile(fsys, file)
			if err != nil {
				return nil, err
			}
			// go-i18n derives the language from the file name; the locale is
			// the directory here, so parse under a synthetic name.
			parsed, err := goi18n.ParseMessageFileBytes(buf, string(locale)+".toml", unmarshal)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", file, err)
			}
			for _, msg := range parsed.Messages {
				if previous, dup := entries[msg.ID]; dup {
					return nil, fmt.Errorf("%s: message %q already defined in %s", file, msg.ID, previous.File)
				}
				entries[msg.ID] = CatalogEntry{
					Locale:       locale,
					File:         file,
					ID:           msg.ID,
					Description:  strings.TrimSpace(msg.Description),
					Text:         msg.Other,
					Placeholders: placeholdersOf(msg.Other),
				}
			}
			if err := bundle.AddMessages(tag, parsed.Messages...); err != nil {
				return nil, fmt.Errorf("%s: %w", file, err)
			}
		}
		c.entries[locale] = entries
		c.locales = append(c.locales, locale)
		c.localizers[locale] = goi18n.NewLocalizer(bundle, string(locale))
	}
	if _, ok := c.entries[DefaultLocale]; !ok {
		return nil, fmt.Errorf("default locale %s has no catalog", DefaultLocale)
	}
	slices.Sort(c.locales)
	return c, nil
}

func placeholdersOf(text string) []string {
	var names []string
	for _, match := range placeholderPattern.FindAllStringSubmatch(text, -1) {
		if !slices.Contains(names, match[1]) {
			names = append(names, match[1])
		}
	}
	slices.Sort(names)
	return names
}

func (c *catalog) localizer(locale Locale) *goi18n.Localizer {
	if l, ok := c.localizers[locale]; ok {
		return l
	}
	return c.localizers[DefaultLocale]
}

// fallbackID is rendered when a message cannot be rendered at all, so a
// broken catalog entry never shows an ID or template error to a user.
const fallbackID = "common.request_failed"

func render(locale Locale, id string, data Data) string {
	c := mustLoad()
	text, err := c.localize(locale, id, data)
	if err == nil {
		return text
	}
	slog.Error("i18n: render message", "id", id, "locale", string(locale), "error", err.Error())
	if id != fallbackID {
		if text, err := c.localize(locale, fallbackID, nil); err == nil {
			return text
		}
	}
	return id
}

func (c *catalog) localize(locale Locale, id string, data Data) (string, error) {
	if id == "" {
		return "", fmt.Errorf("empty message id")
	}
	var templateData map[string]any
	if len(data) > 0 {
		templateData = make(map[string]any, len(data))
		for k, v := range data {
			if nested, ok := v.(Message); ok {
				v = nested.In(locale)
			}
			templateData[k] = v
		}
	}
	text, err := c.localizer(locale).Localize(&goi18n.LocalizeConfig{
		MessageID:      id,
		TemplateData:   templateData,
		TemplateParser: textParser,
	})
	if err != nil {
		if _, missing := err.(*goi18n.MessageNotFoundErr); missing && text != "" {
			// Found in DefaultLocale only: acceptable for a partial locale.
			return text, nil
		}
		return "", err
	}
	return text, nil
}

// Has reports whether id exists in DefaultLocale.
func Has(id string) bool {
	_, ok := mustLoad().entries[DefaultLocale][id]
	return ok
}

// MustExist panics when any of ids is missing from DefaultLocale. Packages
// that keep message IDs in variables call it from init so a typo fails at
// startup and in every test run instead of at reply time.
func MustExist(ids ...string) {
	var missing []string
	for _, id := range ids {
		if !Has(id) {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		panic(fmt.Sprintf("i18n: unknown message ids: %s", strings.Join(missing, ", ")))
	}
}

// Locales lists the locales with a catalog directory.
func Locales() []Locale {
	return slices.Clone(mustLoad().locales)
}

// Entries returns every message of locale sorted by ID.
func Entries(locale Locale) []CatalogEntry {
	entries := mustLoad().entries[locale]
	out := make([]CatalogEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	slices.SortFunc(out, func(a, b CatalogEntry) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Entry returns one message of locale.
func Entry(locale Locale, id string) (CatalogEntry, bool) {
	entry, ok := mustLoad().entries[locale][id]
	return entry, ok
}

func init() {
	// Fail at startup (and in every test binary) when the embedded catalogs
	// cannot be parsed.
	mustLoad()
}
