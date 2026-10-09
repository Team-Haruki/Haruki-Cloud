package handler

import (
	"context"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	corehandler "haruki-cloud/internal/handler"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/utils/usererror"
)

// helpDocTriggers returns the code spans of a help document that start with
// "/", i.e. every command the document shows.
func helpDocTriggers(markdown string) []string {
	var triggers []string
	for _, span := range helpDocCodeSpans(markdown) {
		if !strings.HasPrefix(span, "/") || span == "/" {
			continue
		}
		if !slices.Contains(triggers, span) {
			triggers = append(triggers, span)
		}
	}
	return triggers
}

// helpDocTriggerResolves reports whether a documented command span reaches a
// registered command: a registered spelling (region-prefixed forms are
// registered too), optionally followed by arguments. Arguments written
// directly after the command must not start with a Chinese character, since
// "/歌曲表" would otherwise pass as "/歌曲" plus "表".
func helpDocTriggerResolves(span string) bool {
	return helpDocSpanCommand(span) != ""
}

// requireNoHelpDocFindings fails for every finding.
func requireNoHelpDocFindings(t *testing.T, findings []string, explain string) {
	t.Helper()
	sort.Strings(findings)
	for _, finding := range findings {
		t.Errorf("%s (%s)", strings.ReplaceAll(finding, "\t", " "), explain)
	}
}

func helpDocMarkdown(t *testing.T, key string) string {
	t.Helper()
	markdown, ok, err := i18n.HelpDoc(i18n.DefaultLocale, key)
	if err != nil || !ok {
		t.Fatalf("HelpDoc(%q) = %v, %v", key, ok, err)
	}
	return markdown
}

// TestHelpDocTriggersAreRegistered checks that every command a help document
// shows in backticks can actually be sent. Fix the document, or register the
// documented spelling as an alias when that is unambiguous.
func TestHelpDocTriggersAreRegistered(t *testing.T) {
	EnsureCommandHandlersRegistered()
	var findings []string
	for _, key := range i18n.HelpDocKeys(i18n.DefaultLocale) {
		for _, trigger := range helpDocTriggers(helpDocMarkdown(t, key)) {
			if !helpDocTriggerResolves(trigger) {
				findings = append(findings, key+".md\t"+strings.Fields(trigger)[0])
			}
		}
	}
	findings = slices.Compact(sortedStrings(findings))
	requireNoHelpDocFindings(t, findings, "help document shows a command that is not registered")
}

func sortedStrings(values []string) []string {
	sort.Strings(values)
	return values
}

// TestEveryRouteHasHelpDoc checks that each registered bot route has its own
// help document (locales/<locale>/help/<route path with / as _>.md).
func TestEveryRouteHasHelpDoc(t *testing.T) {
	EnsureCommandHandlersRegistered()
	var findings []string
	for _, route := range corehandler.ListBotRoutes() {
		if _, ok, _ := i18n.HelpDoc(i18n.DefaultLocale, commandHelpDocKey(route.Path)); !ok {
			findings = append(findings, route.Path)
		}
	}
	requireNoHelpDocFindings(t, findings, "route has no help document")
}

// helpDocRouteKeys maps every help document a user can reach to the route it
// describes ("" for the generic document).
func helpDocRouteKeys() map[string]string {
	keys := map[string]string{
		commandHelpGenericKey:                       "",
		commandHelpDocKey(mysekaiBlueprintHelpPath): commandHelpRoutePath(mysekaiBlueprintHelpPath),
	}
	for _, route := range corehandler.ListBotRoutes() {
		keys[commandHelpDocKey(route.Path)] = strings.Trim(route.Path, "/")
	}
	return keys
}

// TestEveryHelpDocIsReachable rejects help documents that no route shows, so
// no copy is kept that users can never see.
func TestEveryHelpDocIsReachable(t *testing.T) {
	EnsureCommandHandlersRegistered()
	reachable := helpDocRouteKeys()
	for _, locale := range i18n.Locales() {
		for _, key := range i18n.HelpDocKeys(locale) {
			if _, ok := reachable[key]; !ok {
				t.Errorf("%s: no route shows this help document; merge it into a route document or delete it", i18n.HelpDocFile(locale, key))
			}
		}
	}
}

var helpDocHeading = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)

// helpDocSections are the level-2 headings a help document may use, in this
// order. 用法, 参数 and 示例 are required for route documents; 说明 is
// optional. The 区服 and 指令别名 sections are generated, never written.
var helpDocSections = []string{"用法", "参数", "示例", "说明"}

var helpDocRequiredSections = []string{"用法", "参数", "示例"}

// TestHelpDocsFollowLayout checks the single help layout (docs/i18n.md):
//
//	# 标题
//	一句话说明（可选，可多行）
//	## 用法 / ## 参数 / ## 示例 / ## 说明（可选），按此顺序
//
// Level-3 headings may group content inside a section. Region prefixes and
// aliases are generated, so documents must not explain them by hand.
func TestHelpDocsFollowLayout(t *testing.T) {
	for _, key := range i18n.HelpDocKeys(i18n.DefaultLocale) {
		markdown := helpDocMarkdown(t, key)
		for _, problem := range helpDocLayoutProblems(markdown) {
			t.Errorf("%s.md: %s", key, problem)
		}
	}
}

func helpDocLayoutProblems(markdown string) []string {
	var problems []string
	lines := strings.Split(strings.TrimSpace(markdown), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "# ") {
		problems = append(problems, "the first line must be the title (# 标题)")
	}
	var sections []string
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.Contains(line, "区服前缀") {
			problems = append(problems, "explains the region prefix by hand; the 区服 section is generated")
		}
		match := helpDocHeading.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		level, title := len(match[1]), match[2]
		switch {
		case level == 1 && i > 0:
			problems = append(problems, "only the first line may be a level-1 heading")
		case level == 2:
			if !slices.Contains(helpDocSections, title) {
				problems = append(problems, "unknown section \"## "+title+"\"; use "+strings.Join(helpDocSections, ", ")+" (### for groups)")
				continue
			}
			sections = append(sections, title)
		case level > 3:
			problems = append(problems, "use at most level-3 headings")
		}
	}
	for _, required := range helpDocRequiredSections {
		if !slices.Contains(sections, required) {
			problems = append(problems, "missing section \"## "+required+"\"")
		}
	}
	order := make([]int, 0, len(sections))
	for _, section := range sections {
		order = append(order, slices.Index(helpDocSections, section))
	}
	if !slices.IsSorted(order) || len(slices.Compact(slices.Clone(order))) != len(order) {
		problems = append(problems, "sections must appear once each, in the order "+strings.Join(helpDocSections, ", "))
	}
	return problems
}

// helpDocExamples returns the commands listed under "## 示例": the first code
// span of each list item, and each fenced block as one multi-line message.
func helpDocExamples(markdown string) []string {
	var examples []string
	inExamples := false
	var fence []string
	inFence := false
	for _, line := range strings.Split(markdown, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inFence && inExamples && len(fence) > 0 {
				examples = append(examples, strings.Join(fence, "\n"))
			}
			inFence, fence = !inFence, nil
			continue
		}
		if inFence {
			fence = append(fence, line)
			continue
		}
		if match := helpDocHeading.FindStringSubmatch(line); match != nil && len(match[1]) <= 2 {
			inExamples = len(match[1]) == 2 && match[2] == "示例"
			continue
		}
		if !inExamples || !strings.HasPrefix(strings.TrimSpace(line), "- ") {
			continue
		}
		if spans := helpDocCodeSpans(line); len(spans) > 0 && strings.HasPrefix(spans[0], "/") {
			examples = append(examples, spans[0])
		}
	}
	return examples
}

var helpExampleMention = regexp.MustCompile(`@\S+`)

// helpExampleEvent builds the chat message an example stands for: "@名字"
// becomes a mention, and an image is attached for commands that take one.
func helpExampleEvent(example string) Event {
	mentions := len(helpExampleMention.FindAllString(example, -1))
	text := strings.TrimSpace(helpExampleMention.ReplaceAllString(example, ""))
	message := onebot11.Message{onebot11.Text(text)}
	for range mentions {
		message = append(message, onebot11.Segment{Type: onebot11.TypeAt, Data: onebot11.AtData{QQ: "10001"}})
	}
	message = append(message, onebot11.Segment{Type: onebot11.TypeImage, Data: onebot11.ImageData{Url: "https://example.com/bg.png"}})
	return Event{Platform: "qq", UserId: "10000", GroupId: "20000", Message: message}
}

// TestHelpDocExamplesParse sends every example of every help document
// through the command parser: it must reach the document's own route and be
// accepted, so the help never teaches a command that fails.
func TestHelpDocExamplesParse(t *testing.T) {
	EnsureCommandHandlersRegistered()
	routes := helpDocRouteKeys()
	for _, key := range i18n.HelpDocKeys(i18n.DefaultLocale) {
		route := routes[key]
		examples := helpDocExamples(helpDocMarkdown(t, key))
		if len(examples) == 0 {
			t.Errorf("%s.md: no examples under ## 示例", key)
		}
		for _, example := range examples {
			resolved, err := dispatchForTest(context.Background(), helpExampleEvent(example))
			if err != nil {
				t.Errorf("%s.md: example %q is rejected (%s)", key, example, usererror.LogText(err))
				continue
			}
			if resolved == nil {
				t.Errorf("%s.md: example %q matches no command", key, example)
				continue
			}
			if route != "" && strings.Trim(resolved.CommandPath, "/") != route {
				t.Errorf("%s.md: example %q runs %s, not %s", key, example, resolved.CommandPath, route)
			}
		}
	}
}

func TestHelpDocTriggerExtraction(t *testing.T) {
	got := helpDocTriggers("- `/查曲 <歌曲名> [难度]`\n- `/jp查曲`、`music123`、`/`\n- `/查曲 <歌曲名> [难度]`")
	if !slices.Equal(got, []string{"/查曲 <歌曲名> [难度]", "/jp查曲"}) {
		t.Fatalf("helpDocTriggers() = %v", got)
	}
	EnsureCommandHandlersRegistered()
	for trigger, want := range map[string]bool{
		"/查曲":         true,
		"/jp查曲 tyw":   true,
		"/kill":       true,
		"/sk100":      true,
		"/查服装123":     true,
		"/pjsk vlive": true,
		"/mysekai 照片": true,
		"/歌曲表 初音天地":   false,
		"/不存在的指令xyz":  false,
		"/help cr任务":  false,
		"/jp b30 u2":  false,
		"/jpb30 u2":   true,
		"/pjsk":       false,
	} {
		if helpDocTriggerResolves(trigger) != want {
			t.Errorf("helpDocTriggerResolves(%q) = %v", trigger, !want)
		}
	}
}

func TestHelpDocLayoutProblems(t *testing.T) {
	good := "# 查曲\n\n查询歌曲。\n\n## 用法\n- `/查曲 <歌曲>`\n\n## 参数\n### 歌曲\n- x\n\n## 示例\n- `/查曲 x`\n\n## 说明\n- y"
	if problems := helpDocLayoutProblems(good); len(problems) != 0 {
		t.Fatalf("good layout problems = %v", problems)
	}
	for name, doc := range map[string]string{
		"no title":       "## 用法\n## 参数\n## 示例",
		"unknown":        "# t\n## 用法\n## 参数\n## 输出\n## 示例",
		"order":          "# t\n## 参数\n## 用法\n## 示例",
		"missing":        "# t\n## 用法\n## 示例",
		"region by hand": "# t\n## 用法\n## 参数\n- 区服前缀：jp\n## 示例",
		"duplicate":      "# t\n## 用法\n## 参数\n## 示例\n## 示例",
	} {
		if problems := helpDocLayoutProblems(doc); len(problems) == 0 {
			t.Errorf("%s: expected a layout problem", name)
		}
	}
}

func TestHelpDocExamplesExtraction(t *testing.T) {
	doc := "# t\n## 用法\n- `/a`\n```text\n/c\n```\n## 示例\n- `/查曲 tyw`：说明\n- 文字\n### 分组\n- `/查曲 x`\n```text\n/添加歌曲别名\ntyw\n```\n## 说明\n- `/b`"
	if got := helpDocExamples(doc); !slices.Equal(got, []string{"/查曲 tyw", "/查曲 x", "/添加歌曲别名\ntyw"}) {
		t.Fatalf("helpDocExamples() = %v", got)
	}
}
