package i18n

import (
	"io/fs"
	"strings"
	"testing"
)

// stripRegionLabels removes the region display names (日服(JP) …), whose
// half-width parentheses are the spec (decision B), before the punctuation
// lint looks for half-width parentheses in Chinese text.
func stripRegionLabels(text string) string {
	for _, code := range RegionCodes {
		text = strings.ReplaceAll(text, RegionLabel(code).String(), "\u2063")
	}
	return text
}

// helpDocBannedTerms are glossary terms (AGENTS.md 12.5) that help documents
// must not use outside code spans, in addition to the shared lint rules.
var helpDocBannedTerms = []struct{ term, why string }{
	{"个人资料", "写“个人信息”"},
	{"资料卡", "写“自定义个人信息”"},
	{"模块化资料", "写“模块化个人信息”"},
	{"自定义档案", "写“自定义个人信息”"},
	{"Suite数据", "写“抓包数据（Suite）”或“抓包数据”"},
	{"Suite 数据", "写“抓包数据（Suite）”或“抓包数据”"},
	{"MySekai数据", "写“烤森（MySekai）数据”或“烤森数据”"},
	{"MySekai 数据", "写“烤森（MySekai）数据”或“烤森数据”"},
	{"用户数据", "写“抓包数据”"},
	{"SK 线", "写“榜线”"},
	{"PJSK", "写“游戏账号”或省略"},
	{"协力", "写“多人 Live”"},
	{"虚拟Live", "写“虚拟 Live”"},
	{"虚拟live", "写“虚拟 Live”"},
	{"多人LIVE", "写“多人 Live”"},
	{"默认账号", "写“默认绑定”"},
	{"主账号", "写“默认绑定”"},
	{"游戏ID", "写“游戏 UID”"},
	{"账号ID", "写“游戏 UID”"},
	{"QQ号", "写“QQ 号”"},
	{"Haruki工具箱", "写“Haruki 工具箱”"},
	{"服务器", "区服写“区服”"},
	{"使用方式", "写“用法”"},
	{"参数解析失败", "写“参数格式不正确”"},
	{"推荐队伍", "写“卡组”"},
	{"曲目", "写“歌曲”"},
}

// TestHelpDocTerms keeps the help documents on the glossary. Code spans and
// fenced blocks are skipped: they hold what users type.
func TestHelpDocTerms(t *testing.T) {
	for _, locale := range Locales() {
		if locale != ZhCN {
			continue
		}
		for _, key := range HelpDocKeys(locale) {
			data, err := fs.ReadFile(localeFS, HelpDocFile(locale, key))
			if err != nil {
				t.Fatal(err)
			}
			text := codeSpan.ReplaceAllString(fencedBlock.ReplaceAllString(string(data), "\n"), "\u2063")
			for _, banned := range helpDocBannedTerms {
				if strings.Contains(text, banned.term) {
					t.Errorf("%s: %q: %s", HelpDocFile(locale, key), banned.term, banned.why)
				}
			}
		}
	}
}

func TestStripRegionLabels(t *testing.T) {
	text := "国服(CN)不支持烤森功能"
	if got := hanParen.FindString(text); got == "" {
		t.Fatal("hanParen should flag a region label before stripping")
	}
	if got := hanParen.FindString(stripRegionLabels(text)); got != "" {
		t.Fatalf("region label still flagged: %q", got)
	}
	if got := hanParen.FindString(stripRegionLabels("国服(cn)")); got == "" {
		t.Fatal("only the exact region labels are exempt")
	}
}
