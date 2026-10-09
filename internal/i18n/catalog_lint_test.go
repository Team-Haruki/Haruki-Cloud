package i18n

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Catalog style lint. Every rule below comes from the copy spec in AGENTS.md
// ("用户文案规范"). Catalog messages must pass with zero findings; help
// documents are checked in ratchet mode against testdata/helpdoc_style.baseline
// until the help-doc migration has cleaned them up.

var (
	idPattern          = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	templateAction     = regexp.MustCompile(`\{\{.*?\}\}`)
	hanColon           = regexp.MustCompile(`\p{Han}:`)
	hanParen           = regexp.MustCompile(`\p{Han}[()]|[()]\p{Han}|\([^()]*\p{Han}[^()]*\)`)
	hanLatinAdjacent   = regexp.MustCompile(`\p{Han}[A-Za-z0-9]|[A-Za-z0-9]\p{Han}`)
	commandToken       = regexp.MustCompile(`/[^\s“”"'，。、；：）（]+`)
	positionalVerb     = regexp.MustCompile(`%[-+# 0]*\d*(\.\d+)?[sdvqfgxXtwT]`)
	reservedIDSegments = []string{"id", "description", "hash", "leftdelim", "rightdelim", "zero", "one", "two", "few", "many", "other", "translation"}
)

// adjacencyTokens are tokens users type verbatim that legitimately put Han
// next to Latin letters or digits (decision D exceptions). Commands such as
// /jp查曲 are exempt through commandToken.
var adjacencyTokens = []string{"u序号", "ms材料", "10火"}

// bannedTerms are words that must not appear in user copy, with the reason
// shown in the lint failure.
var bannedTerms = []struct{ term, why string }{
	{"您", "称呼用户用“你”或省略主语"},
	{"请稍后重试", "统一写“请稍后再试”"},
	{"未就绪", "写“暂时不可用”"},
	{"命令", "写“指令”"},
	{"Toolbox", "写“工具箱”（帮助里首次写“Haruki 工具箱”）"},
	{"toolbox", "写“工具箱”"},
	{"SekaiAPI", "不对用户提内部组件；写“游戏数据服务”或“获取游戏数据失败”"},
	{"Sekai API", "不对用户提内部组件"},
	{"Tracker", "不对用户提内部组件；写“查榜服务”"},
	{"tracker", "不对用户提内部组件；写“查榜服务”"},
	{"masterdata", "不对用户提内部组件；写“游戏数据”"},
	{"Cloud", "不对用户提内部组件"},
	{"suite", "写“抓包数据（Suite）”或“抓包数据”"},
	{"Mysekai", "写“烤森（MySekai）”或“烤森”"},
	{"mysekai", "写“烤森（MySekai）”或“烤森”"},
	{"User Data", "写“抓包数据”"},
	{"套装", "suite 的误译；写“抓包数据”"},
	{"档线", "写“榜线”"},
	{"分数线", "写“榜线”"},
	{"（状态", "不向用户展示状态码"},
	{"\"", "关键词和用户输入用“”，游戏专有名词用「」"},
	{"...", "省略号写“……”"},
	{"✅", "文字回复不用 emoji"},
	{"❌", "文字回复不用 emoji"},
}

type lintFinding struct {
	rule   string
	detail string
}

// lintCopyText applies the style rules shared by catalog messages and help
// documents. text must already have template actions and code spans removed.
func lintCopyText(text string) []lintFinding {
	var findings []lintFinding
	add := func(rule, detail string) { findings = append(findings, lintFinding{rule, detail}) }
	for _, banned := range bannedTerms {
		if strings.Contains(text, banned.term) {
			add("banned_term", fmt.Sprintf("%q: %s", banned.term, banned.why))
		}
	}
	if m := hanColon.FindString(text); m != "" {
		add("halfwidth_colon", fmt.Sprintf("%q: 汉字后用全角“：”", m))
	}
	if m := hanParen.FindString(stripRegionLabels(text)); m != "" {
		add("halfwidth_paren", fmt.Sprintf("%q: 中文里用全角“（）”", m))
	}
	stripped := commandToken.ReplaceAllString(text, " ")
	for _, token := range adjacencyTokens {
		stripped = strings.ReplaceAll(stripped, token, " ")
	}
	if m := hanLatinAdjacent.FindString(stripped); m != "" {
		add("han_latin_space", fmt.Sprintf("%q: 中文与拉丁字母/数字之间加一个半角空格", m))
	}
	if m := positionalVerb.FindString(text); m != "" {
		add("positional_placeholder", fmt.Sprintf("%q: 用命名占位符 {{.Name}}", m))
	}
	return findings
}

// stripTemplate replaces {{...}} actions with a neutral separator so that
// placeholder names are not mistaken for Latin text.
func stripTemplate(text string) string {
	return templateAction.ReplaceAllString(text, "\u2063")
}

func lintCatalogEntry(entry CatalogEntry) []lintFinding {
	var findings []lintFinding
	add := func(rule, detail string) { findings = append(findings, lintFinding{rule, detail}) }

	if !idPattern.MatchString(entry.ID) {
		add("id_format", "ID 用小写字母、数字、下划线，按“领域.子域.名称”分段")
	}
	segments := strings.Split(entry.ID, ".")
	if domain := strings.TrimSuffix(path.Base(entry.File), ".toml"); segments[0] != domain {
		add("id_domain", fmt.Sprintf("ID 必须以文件名 %q 开头", domain+"."))
	}
	for _, segment := range segments {
		if slices.Contains(reservedIDSegments, segment) {
			add("id_reserved", fmt.Sprintf("ID 段 %q 是 go-i18n 保留字", segment))
		}
	}
	if entry.Description == "" {
		add("description", "缺少 description：写明出现位置和每个占位符的含义")
	}
	if strings.TrimSpace(entry.Text) == "" {
		add("empty", "缺少 other 文本")
	}
	for _, action := range templateAction.FindAllString(entry.Text, -1) {
		if !placeholderPattern.MatchString(action) || placeholderPattern.FindString(action) != action {
			add("template_action", fmt.Sprintf("%q: 只允许 {{.Name}} 形式的占位符", action))
		}
	}
	if len(entry.Placeholders) > 0 {
		_, documented, ok := strings.Cut(entry.Description, "占位符：")
		for _, name := range entry.Placeholders {
			if !ok || !strings.Contains(documented, name+"=") {
				add("placeholder_doc", fmt.Sprintf("description 的“占位符：”部分没有说明 %s（写成 %s=含义）", name, name))
			}
		}
	}
	text := stripTemplate(entry.Text)
	findings = append(findings, lintCopyText(text)...)
	trimmed := strings.TrimSpace(text)
	if strings.HasSuffix(trimmed, "。") && strings.Count(trimmed, "。") == 1 {
		add("trailing_period", "单句回复结尾不加“。”")
	}
	return findings
}

func TestCatalogStyle(t *testing.T) {
	for _, locale := range Locales() {
		for _, entry := range Entries(locale) {
			for _, f := range lintCatalogEntry(entry) {
				t.Errorf("%s %s [%s] %s", entry.File, entry.ID, f.rule, f.detail)
			}
		}
	}
}

func TestCatalogStyleLintCatchesViolations(t *testing.T) {
	cases := map[string]CatalogEntry{
		"banned_term":            {ID: "common.x", File: "common.toml", Description: "d", Text: "您好"},
		"halfwidth_colon":        {ID: "common.x", File: "common.toml", Description: "d", Text: "原因: x"},
		"halfwidth_paren":        {ID: "common.x", File: "common.toml", Description: "d", Text: "队长次数(EX)"},
		"han_latin_space":        {ID: "common.x", File: "common.toml", Description: "d", Text: "最多5个"},
		"positional_placeholder": {ID: "common.x", File: "common.toml", Description: "d", Text: "第 %d 名"},
		"trailing_period":        {ID: "common.x", File: "common.toml", Description: "d", Text: "已完成。"},
		"description":            {ID: "common.x", File: "common.toml", Text: "x"},
		"empty":                  {ID: "common.x", File: "common.toml", Description: "d"},
		"id_format":              {ID: "Common.X", File: "Common.toml", Description: "d", Text: "x"},
		"id_domain":              {ID: "other_domain.x", File: "common.toml", Description: "d", Text: "x"},
		"id_reserved":            {ID: "common.other", File: "common.toml", Description: "d", Text: "x"},
		"template_action":        {ID: "common.x", File: "common.toml", Description: "d", Text: "{{if .A}}x{{end}}"},
		"placeholder_doc":        {ID: "common.x", File: "common.toml", Description: "d", Text: "{{.A}}", Placeholders: []string{"A"}},
	}
	for rule, entry := range cases {
		var rules []string
		for _, f := range lintCatalogEntry(entry) {
			rules = append(rules, f.rule)
		}
		if !slices.Contains(rules, rule) {
			t.Errorf("rule %s not triggered by %+v (got %v)", rule, entry, rules)
		}
	}
	clean := CatalogEntry{
		ID: "common.x", File: "common.toml",
		Description:  "示例。占位符：QQ=QQ 号",
		Text:         "已绑定 QQ 号 {{.QQ}}，发送 /jp查曲 或 u序号 查看",
		Placeholders: []string{"QQ"},
	}
	if findings := lintCatalogEntry(clean); len(findings) != 0 {
		t.Fatalf("clean entry flagged: %+v", findings)
	}
}

var (
	codeSpan    = regexp.MustCompile("`[^`\n]*`")
	fencedBlock = regexp.MustCompile("(?s)```.*?```")
)

// helpDocStyleCounts counts style findings per rule in one help document.
// Code spans are skipped: they hold commands and syntax users type.
func helpDocStyleCounts(markdown string) map[string]int {
	text := fencedBlock.ReplaceAllString(markdown, "\n")
	counts := map[string]int{}
	for _, line := range strings.Split(text, "\n") {
		line = codeSpan.ReplaceAllString(line, "\u2063")
		for _, f := range lintCopyText(line) {
			counts[f.rule]++
		}
	}
	return counts
}

// TestHelpDocStyleRatchet fails when a help document gains style findings
// compared to testdata/helpdoc_style.baseline. Regenerate the baseline with
// HARUKI_UPDATE_GOLDEN=1 after fixing documents, so the counts only go down.
func TestHelpDocStyleRatchet(t *testing.T) {
	current := map[string]int{}
	for _, locale := range Locales() {
		for _, key := range HelpDocKeys(locale) {
			data, err := fs.ReadFile(localeFS, HelpDocFile(locale, key))
			if err != nil {
				t.Fatal(err)
			}
			for rule, n := range helpDocStyleCounts(string(data)) {
				current[HelpDocFile(locale, key)+"\t"+rule] = n
			}
		}
	}
	checkRatchet(t, "helpdoc_style.baseline", current)
}

func TestHelpDocStyleCountsSkipCode(t *testing.T) {
	counts := helpDocStyleCounts("# 标题\n- `/kill <QQ号>`：封禁\n```\n原因: x\n```\n- 最多5个\n")
	if counts["han_latin_space"] != 1 || counts["halfwidth_colon"] != 0 {
		t.Fatalf("helpDocStyleCounts() = %v", counts)
	}
}

// checkRatchet compares per-key counts with a "key<TAB>count" baseline file.
// A key may only keep or lower its count; keys missing from the baseline
// count as zero.
func checkRatchet(t *testing.T, name string, current map[string]int) {
	t.Helper()
	keys := make([]string, 0, len(current))
	for key, n := range current {
		if n > 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Ratchet baseline: <key>\\t<count>. Counts may only go down.\n")
	b.WriteString("# Regenerate with HARUKI_UPDATE_GOLDEN=1 go test ./internal/i18n/ after fixing findings.\n")
	for _, key := range keys {
		fmt.Fprintf(&b, "%s\t%d\n", key, current[key])
	}
	if updateGolden {
		compareGolden(t, name, b.String())
		return
	}
	baseline := readRatchetBaseline(t, name)
	var increased []string
	lowered := 0
	for _, key := range keys {
		if current[key] > baseline[key] {
			increased = append(increased, fmt.Sprintf("%s: %d > baseline %d", key, current[key], baseline[key]))
		}
	}
	for key, n := range baseline {
		if current[key] < n {
			lowered++
		}
	}
	for _, line := range increased {
		t.Errorf("%s", line)
	}
	if len(increased) > 0 {
		t.Errorf("new copy findings above the %s ratchet; fix them (see AGENTS.md 用户文案规范) instead of raising the baseline", name)
	}
	if lowered > 0 {
		t.Logf("%d entries are below the %s baseline; lock the progress with HARUKI_UPDATE_GOLDEN=1", lowered, name)
	}
}

func readRatchetBaseline(t *testing.T, name string) map[string]int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v (generate with HARUKI_UPDATE_GOLDEN=1)", name, err)
	}
	baseline := map[string]int{}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.LastIndex(line, "\t")
		n, err := strconv.Atoi(line[idx+1:])
		if idx < 0 || err != nil {
			t.Fatalf("%s: malformed line %q", name, line)
		}
		baseline[line[:idx]] = n
	}
	return baseline
}
