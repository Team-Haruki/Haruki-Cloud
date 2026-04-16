package common

import "strings"

// DefaultNicknames maps user-typed character aliases (pinyin abbreviations,
// Japanese kana/kanji, simplified/traditional Chinese) to game character IDs.
// Shared across render sub-packages that need nickname lookup. Treat as
// read-only — clone before mutating.
var DefaultNicknames = map[string]int{
	"ick": 1, "ichika": 1, "星乃一歌": 1, "星乃": 1, "一歌": 1,
	"saki": 2, "咲希": 2, "天马咲希": 2, "天馬咲希": 2,
	"hnm": 3, "honami": 3, "穗波": 3, "望月穗波": 3, "望月穂波": 3, "望月": 3,
	"shiho": 4, "志步": 4, "志歩": 4, "日野森志步": 4, "日野森志歩": 4,
	"mnr": 5, "minori": 5, "实乃理": 5, "实乃里": 5, "花里みのり": 5, "花里实乃理": 5, "花里实乃里": 5, "花里": 5, "みのり": 5,
	"hrk": 6, "haruka": 6, "遥": 6, "桐谷遥": 6, "桐谷": 6,
	"airi": 7, "爱莉": 7, "愛莉": 7, "桃井爱莉": 7, "桃井愛莉": 7, "桃井": 7,
	"szk": 8, "shizuku": 8, "雫": 8, "日野森雫": 8,
	"khn": 9, "kohane": 9, "小豆泽心羽": 9, "小豆沢心羽": 9, "小豆泽": 9, "小豆沢": 9, "心羽": 9, "こはね": 9, "小豆沢こはね": 9,
	"an": 10, "杏": 10, "白石杏": 10, "白石": 10,
	"akt": 11, "akito": 11, "彰人": 11, "青柳彰人": 11, "东云彰人": 11, "東雲彰人": 11,
	"toya": 12, "冬弥": 12, "青柳冬弥": 12, "青柳": 12,
	"tsks": 13, "tks": 13, "tms": 13, "tsukasa": 13, "司": 13, "天马司": 13, "天馬司": 13,
	"emu": 14, "笑梦": 14, "笑夢": 14, "凤笑梦": 14, "鳳えむ": 14, "凤": 14,
	"nene": 15, "宁宁": 15, "寧々": 15, "草薙宁宁": 15, "草薙寧々": 15, "草薙": 15,
	"rui": 16, "sdl": 16, "类": 16, "類": 16, "神代类": 16, "神代類": 16, "神代": 16,
	"knd": 17, "kanade": 17, "奏": 17, "宵崎奏": 17, "宵崎": 17,
	"mfy": 18, "mafuyu": 18, "真冬": 18, "朝比奈真冬": 18, "朝比奈まふゆ": 18, "朝比奈": 18,
	"ena": 19, "enana": 19, "绘名": 19, "絵名": 19, "东云绘名": 19, "東雲絵名": 19, "えな": 19,
	"mzk": 20, "mizuki": 20, "瑞希": 20, "晓山瑞希": 20, "暁山瑞希": 20, "晓山": 20, "暁山": 20,
	"miku": 21, "初音": 21, "初音未来": 21, "初音ミク": 21,
	"rin": 22, "镜音铃": 22, "鏡音リン": 22,
	"len": 23, "镜音连": 23, "鏡音レン": 23,
	"luka": 24, "巡音流歌": 24, "巡音ルカ": 24,
	"meiko": 25, "mei": 25,
	"kaito": 26, "kai": 26,
}

// CloneNicknames returns a shallow copy of the nickname map so callers can
// mutate it without affecting the shared default.
func CloneNicknames(src map[string]int) map[string]int {
	result := make(map[string]int, len(src))
	for key, value := range src {
		result[key] = value
	}
	return result
}

// NormalizeNicknameQuery lowercases the query and collapses internal
// whitespace, producing the canonical key used for nickname lookups.
func NormalizeNicknameQuery(query string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(query)), ""))
}
