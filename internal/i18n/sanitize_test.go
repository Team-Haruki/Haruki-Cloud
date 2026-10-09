package i18n

import (
	"slices"
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
