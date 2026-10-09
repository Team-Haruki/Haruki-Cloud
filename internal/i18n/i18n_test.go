package i18n

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// updateGolden rewrites testdata files instead of comparing against them:
// HARUKI_UPDATE_GOLDEN=1 go test ./internal/i18n/...
var updateGolden = os.Getenv("HARUKI_UPDATE_GOLDEN") == "1"

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (regenerate with HARUKI_UPDATE_GOLDEN=1)", path, err)
	}
	if string(want) != got {
		t.Fatalf("%s is out of date; review the diff and regenerate with HARUKI_UPDATE_GOLDEN=1\n--- got ---\n%s", path, got)
	}
}

// TestHelperGolden locks the zh-CN output of every shared helper. A wording
// change in format.toml or common.toml shows up here as a reviewable diff.
func TestHelperGolden(t *testing.T) {
	tokyo := time.FixedZone("JST", 9*3600)
	india := time.FixedZone("IST", 5*3600+30*60)
	newfoundland := time.FixedZone("NST", -(3*3600 + 30*60))
	at := time.Date(2026, 10, 9, 6, 5, 9, 0, time.UTC)
	rows := []struct{ name, got string }{
		{"RegionLabel(jp)", RegionLabel("jp").String()},
		{"RegionLabel(CN)", RegionLabel("CN").String()},
		{"RegionLabel(tw)", RegionLabel("tw").String()},
		{"RegionLabel(kr)", RegionLabel("kr").String()},
		{"RegionLabel(en)", RegionLabel("en").String()},
		{"RegionLabel(xx)", RegionLabel("xx").String()},
		{"AccountLabel(jp, hidden)", AccountLabel("jp", "7487590788965145370", false).String()},
		{"AccountLabel(cn, visible)", AccountLabel("cn", "7487590788965145370", true).String()},
		{"MaskUID(short)", MaskUID("123456", false)},
		{"FormatUserTime(nil loc)", FormatUserTime(at, nil).String()},
		{"FormatUserTime(UTC+9)", FormatUserTime(at, tokyo).String()},
		{"FormatUserTime(UTC+5:30)", FormatUserTime(at, india).String()},
		{"FormatUserTime(UTC-3:30)", FormatUserTime(at, newfoundland).String()},
		{"FormatUserTime(UTC)", FormatUserTime(at, time.UTC).String()},
		{"FormatUserTime(zero)", FormatUserTime(time.Time{}, nil).String()},
		{"FormatDuration(45s)", FormatDuration(45 * time.Second).String()},
		{"FormatDuration(2m3s)", FormatDuration(2*time.Minute + 3*time.Second).String()},
		{"FormatDuration(1h2m3s)", FormatDuration(time.Hour + 2*time.Minute + 3*time.Second).String()},
		{"FormatDuration(-1s)", FormatDuration(-time.Second).String()},
		{"Thousands(1234)", Thousands(1234)},
		{"Thousands(12345)", Thousands(12345)},
		{"Thousands(-1234567)", Thousands(-1234567)},
		{"Wan(9999)", Wan(9999).String()},
		{"Wan(123456)", Wan(123456).String()},
		{"Wan(3000000)", Wan(3000000).String()},
		{"Percent(12.34)", Percent(12.34)},
		{"Percent(50)", Percent(50)},
		{"PercentN(12.3456, 2)", PercentN(12.3456, 2)},
		{"PageLabel(1, 3)", PageLabel(1, 3).String()},
		{"DifficultyLabel(expert)", DifficultyLabel("expert").String()},
		{"DifficultyLabel(append)", DifficultyLabel("APPEND").String()},
		{"DifficultyLabel(unknown)", DifficultyLabel("special").String()},
		{"LiveTypeLabel(solo)", LiveTypeLabel("solo").String()},
		{"LiveTypeLabel(multi)", LiveTypeLabel("multi").String()},
		{"LiveTypeLabel(auto)", LiveTypeLabel("auto").String()},
		{"LiveTypeLabel(challenge)", LiveTypeLabel("challenge").String()},
		{"LiveTypeLabel(unknown)", LiveTypeLabel("mystery").String()},
		{"RequestFailed", RequestFailed().String()},
		{"Misconfigured", Misconfigured().String()},
		{"ReadOnly", ReadOnly().String()},
		{"Unavailable(ranking)", Unavailable(FeatureRanking).String()},
		{"Unavailable(game data)", Unavailable(FeatureGameData).String()},
		{"Timeout(render)", Timeout(FeatureRender).String()},
		{"Timeout(toolbox)", Timeout(FeatureToolbox).String()},
		{"Unavailable(deck)", Unavailable(FeatureDeck).String()},
		{"Unavailable(account)", Unavailable(FeatureAccount).String()},
		{"NotFound", NotFound(Verbatim("歌曲"), "tell your world").String()},
		{"NotFound(long query)", NotFound(Verbatim("歌曲"), strings.Repeat("长", 40)).String()},
		{"Ambiguous", Ambiguous(Verbatim("角色"), "mk").String()},
		{"OutOfRange", OutOfRange(Verbatim("活动序号"), 1, 20).String()},
		{"BadParam", BadParam("abc", M("moderation.qq_invalid")).String()},
		{"Usage(/查曲)", Usage("/查曲").String()},
		{"Usage(kill)", Usage("kill").String()},
		{"WithUsage", WithUsage(M("moderation.back.usage_reason"), "/back").String()},
	}
	var b strings.Builder
	for _, row := range rows {
		b.WriteString("== " + row.name + "\n" + row.got + "\n")
	}
	compareGolden(t, "helpers.zh-CN.golden", b.String())
}

func TestMessageRenderingAndFallbacks(t *testing.T) {
	if got := T("moderation.back.done", Data{"QQ": "42"}); got != "已解除 QQ 号 42 的全局封禁" {
		t.Fatalf("T() = %q", got)
	}
	nested := M("common.unavailable", Data{"Feature": FeatureRanking})
	if !strings.HasPrefix(nested.In(ZhCN), "查榜服务") {
		t.Fatalf("nested message = %q", nested.In(ZhCN))
	}
	// Unknown locales and IDs never show raw IDs or template errors.
	if got := nested.In("fr-FR"); got != nested.String() {
		t.Fatalf("unknown locale fallback = %q", got)
	}
	if got := T("does.not.exist"); got != RequestFailed().String() {
		t.Fatalf("missing id = %q", got)
	}
	if got := T("moderation.back.done"); got != RequestFailed().String() {
		t.Fatalf("missing placeholder = %q", got)
	}
	if got := (Message{}).String(); got != RequestFailed().String() {
		t.Fatalf("zero message = %q", got)
	}
	if !(Message{}).IsZero() || M("x").IsZero() {
		t.Fatal("IsZero")
	}
	merged := M("moderation.kill.done_permanent", Data{"QQ": "1"}, Data{"Reason": "r"})
	if got := merged.String(); !strings.Contains(got, "1") || !strings.Contains(got, "r") {
		t.Fatalf("merged data = %q", got)
	}
}

func TestCatalogAccessors(t *testing.T) {
	if !Has("common.request_failed") || Has("common.nope") {
		t.Fatal("Has")
	}
	MustExist("common.request_failed", "format.page")
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("MustExist did not panic on a missing id")
			}
		}()
		MustExist("common.nope")
	}()
	if locales := Locales(); len(locales) == 0 || locales[0] != ZhCN {
		t.Fatalf("Locales() = %v", locales)
	}
	entry, ok := Entry(ZhCN, "format.page")
	if !ok || entry.File != "locales/zh-CN/format.toml" || strings.Join(entry.Placeholders, ",") != "Page,Total" {
		t.Fatalf("Entry() = %+v, %v", entry, ok)
	}
	if entries := Entries(ZhCN); len(entries) < 10 || entries[0].ID > entries[1].ID {
		t.Fatalf("Entries() not sorted: %d", len(entries))
	}
}

func TestLocaleContext(t *testing.T) {
	var none context.Context
	if LocaleFromContext(none) != DefaultLocale || LocaleFromContext(context.Background()) != DefaultLocale {
		t.Fatal("default locale")
	}
	if LocaleFromContext(WithLocale(none, "")) != DefaultLocale {
		t.Fatal("empty locale")
	}
	ctx := WithLocale(context.Background(), "en-US")
	if LocaleFromContext(ctx) != "en-US" {
		t.Fatal("WithLocale")
	}
}

func TestHelpDocs(t *testing.T) {
	md, ok, err := HelpDoc(ZhCN, "music")
	if err != nil || !ok || !strings.HasPrefix(md, "# ") {
		t.Fatalf("HelpDoc(music) = %q, %v, %v", md, ok, err)
	}
	if _, ok, _ := HelpDoc("fr-FR", "music"); !ok {
		t.Fatal("help docs fall back to the default locale")
	}
	for _, key := range []string{"", "../common", "a/b", ".hidden", "missing"} {
		if _, ok, err := HelpDoc(ZhCN, key); ok || err != nil {
			t.Fatalf("HelpDoc(%q) = %v, %v", key, ok, err)
		}
	}
	keys := HelpDocKeys(ZhCN)
	if len(keys) < 100 || keys[0] > keys[1] {
		t.Fatalf("HelpDocKeys() = %d keys", len(keys))
	}
	if HelpDocFile(ZhCN, "music") != "locales/zh-CN/help/music.md" {
		t.Fatal("HelpDocFile")
	}
}

func TestLoadCatalogRejectsBrokenTrees(t *testing.T) {
	good := "[a.b]\ndescription = \"d\"\nother = \"x\"\n"
	cases := map[string]fstest.MapFS{
		"no locales":      {},
		"bad locale name": {"locales/not a tag!/a.toml": {Data: []byte(good)}},
		"bad toml":        {"locales/zh-CN/a.toml": {Data: []byte("[a")}},
		"duplicate id": {
			"locales/zh-CN/a.toml": {Data: []byte(good)},
			"locales/zh-CN/b.toml": {Data: []byte(good)},
		},
		"no default locale": {"locales/en-US/a.toml": {Data: []byte(good)}},
	}
	for name, fsys := range cases {
		if _, err := loadCatalog(fsys); err == nil {
			t.Errorf("%s: loadCatalog() succeeded", name)
		}
	}
	ok := fstest.MapFS{
		"locales/zh-CN/a.toml": {Data: []byte(good)},
		"locales/en-US/a.toml": {Data: []byte(good)},
		"locales/README.md":    {Data: []byte("not a locale")},
	}
	c, err := loadCatalog(ok)
	if err != nil || len(c.locales) != 2 {
		t.Fatalf("loadCatalog() = %+v, %v", c, err)
	}
	if text, err := c.localize(RenderOptions{Locale: "en-US"}, "a.b", nil); err != nil || text != "x" {
		t.Fatalf("localize() = %q, %v", text, err)
	}
	if _, err := c.localize(RenderOptions{Locale: ZhCN}, "", nil); err == nil {
		t.Fatal("empty id rendered")
	}
}
