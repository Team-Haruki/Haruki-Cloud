package i18n

import (
	"slices"
	"strings"
	"testing"
)

func TestSanitizeLinesKeepsCatalogTextOnly(t *testing.T) {
	reply := WithUsage(BadParam("abc", M("common.param.uid_digits")), "/查曲").String()
	clean, dropped := SanitizeLines(reply)
	if clean != reply || len(dropped) != 0 {
		t.Fatalf("catalog reply changed: %q -> %q (dropped %q)", reply, clean, dropped)
	}

	leaky := Unavailable(FeatureRanking).String() + "\ntracker api error: status 503, message: \"upstream down\"\n" +
		"解析失败: context deadline exceeded"
	clean, dropped = SanitizeLines(leaky)
	if clean != Unavailable(FeatureRanking).String() {
		t.Fatalf("SanitizeLines kept non-catalog lines: %q", clean)
	}
	if len(dropped) != 2 || !slices.Contains(dropped, "解析失败: context deadline exceeded") {
		t.Fatalf("dropped = %q", dropped)
	}

	clean, dropped = SanitizeLines("Post \"http://internal:9000/render\": dial tcp: connection refused")
	if clean != "" || len(dropped) != 1 {
		t.Fatalf("raw error kept: %q", clean)
	}
}

func TestCatalogLinePatternAnchorsAndPlaceholders(t *testing.T) {
	for line, want := range map[string]bool{
		"请求处理失败，请稍后再试":            true,
		"请求处理失败，请稍后再试 status 500": false,
		"查榜服务暂时不可用，请稍后再试":         true,
		"参数格式不正确：“x y z”":         true,
		"发送 /查曲 -help 查看用法":       true,
		"":                        true,
		"日服(JP)":                  false,
		"connection refused":      false,
	} {
		if got := IsCatalogLine(line); got != want {
			t.Errorf("IsCatalogLine(%q) = %v, want %v", line, got, want)
		}
	}
	if _, ok := newLinePattern("{{.Region}}：{{.Reason}}"); ok {
		t.Fatal("a line of placeholders and punctuation must not become a pattern")
	}
}

// Raw error lines must not pass as catalog text just because a catalog line
// with a short literal ("u{{.Index}} …", "T{{.Tier}}", "ID {{.IDs}}",
// "{{.Region}}活动 {{.ID}}", "{{.Value}}万") happens to fit them.
func TestRawErrorLinesAreNotCatalogLines(t *testing.T) {
	for _, line := range []string{
		"unexpected EOF",
		"upstream connect error or disconnect/reset before headers",
		"TLS handshake timeout",
		"Toolbox returned status 500",
		"ID 123: pq: duplicate key value violates unique constraint",
		"获取活动信息失败: dial tcp 10.0.0.5:8080: connect: connection refused",
		"重试 3 次后放弃: 5万",
		"超时 30秒",
		"tracker 2024 年",
		"rate 12/h",
		"纯 garbage",
	} {
		if IsCatalogLine(line) {
			t.Errorf("IsCatalogLine(%q) = true, want false", line)
		}
		if clean, _ := SanitizeLines(line); clean != "" {
			t.Errorf("SanitizeLines(%q) kept %q", line, clean)
		}
	}
	for _, line := range []string{"u{{.Index}} {{.Account}}", "T{{.Tier}}", "ID {{.IDs}}", "{{.Region}}活动 {{.ID}}", "{{.Value}}万", "{{.Value}}/h", "纯 {{.Unit}}"} {
		if pattern, ok := newLinePattern(line); !ok || !pattern.weak {
			t.Errorf("newLinePattern(%q) = %+v, %v; want a weak pattern", line, pattern, ok)
		}
	}
}

// A weak line is still kept in a reply that renders its own message.
func TestSanitizeMessageKeepsItsOwnWeakLines(t *testing.T) {
	reply := M("alias.ambiguous", Data{"Kind": M("alias.kind.character"), "Candidates": []Message{
		M("alias.ambiguous_candidate", Data{"ID": 1, "Name": "x"}),
		M("alias.ambiguous_candidate", Data{"ID": 2, "Name": "y"}),
	}})
	text := reply.String()
	if !strings.Contains(text, "ID 2") {
		t.Fatalf("reply did not render its candidates: %q", text)
	}
	clean, dropped := SanitizeMessage(reply, DefaultLocale)
	if clean != text || len(dropped) != 0 {
		t.Fatalf("SanitizeMessage dropped own lines: %q -> %q (dropped %q)", text, clean, dropped)
	}
	if clean, _ := SanitizeLines("ID 1：x"); clean != "" {
		t.Fatalf("a weak line outside its own message must be dropped, kept %q", clean)
	}
}

// Every line of every message that has literal text survives the sanitizer
// when the message itself is the reply.
func TestSanitizeMessageKeepsEveryCatalogLine(t *testing.T) {
	for _, locale := range Locales() {
		for _, entry := range Entries(locale) {
			data := Data{}
			for _, name := range entry.Placeholders {
				data[name] = "‹" + name + "›"
			}
			_, dropped := SanitizeMessage(M(entry.ID, data), locale)
			for _, line := range dropped {
				if !placeholderOnlyLine(entry.Text, line, data) {
					t.Errorf("%s %s: line %q dropped", locale, entry.ID, line)
				}
			}
		}
	}
}

// placeholderOnlyLine reports whether rendered is a line of template that
// holds only placeholders (and punctuation), which is never a pattern.
func placeholderOnlyLine(template, rendered string, data Data) bool {
	for _, line := range strings.Split(template, "\n") {
		if _, ok := newLinePattern(strings.TrimSpace(line)); ok {
			continue
		}
		filled := line
		for name, value := range data {
			filled = strings.ReplaceAll(filled, "{{."+name+"}}", value.(string))
		}
		if strings.TrimSpace(filled) == strings.TrimSpace(rendered) {
			return true
		}
	}
	return false
}

func TestMessageListPlaceholderRendersLines(t *testing.T) {
	text := M("music.ambiguous", Data{"Candidates": []Message{
		M("music.ambiguous_candidate", Data{"ID": 1, "Title": "Tell Your World"}),
		M("music.ambiguous_candidate", Data{"ID": 2, "Title": "ロキ"}),
	}}).String()
	clean, dropped := SanitizeLines(text)
	if clean != text || len(dropped) != 0 {
		t.Fatalf("candidate lines must be catalog lines: %q (dropped %q)", text, dropped)
	}
}
