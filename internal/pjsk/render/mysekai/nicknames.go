package mysekai

var defaultNicknames = map[string]int{
	"ick": 1, "ichika": 1, "星乃一歌": 1,
	"saki": 2, "咲希": 2, "天马咲希": 2,
	"hnm": 3, "honami": 3, "穗波": 3,
	"shiho": 4, "志步": 4, "日野森志步": 4,
	"mnr": 5, "minori": 5, "实乃理": 5, "花里みのり": 5,
	"hrk": 6, "haruka": 6, "遥": 6,
	"airi": 7, "爱莉": 7, "桃井爱莉": 7,
	"szk": 8, "shizuku": 8, "雫": 8,
	"kohane": 9, "小豆泽心羽": 9,
	"an": 10, "杏": 10, "白石杏": 10,
	"akito": 11, "彰人": 11, "青柳彰人": 11,
	"toya": 12, "冬弥": 12, "天马冬弥": 12,
	"tsks": 13, "tsukasa": 13, "司": 13,
	"emu": 14, "笑梦": 14, "天马笑梦": 14,
	"nene": 15, "宁宁": 15, "楠宁宁": 15,
	"rui": 16, "类": 16, "神代类": 16,
	"knd": 17, "kanade": 17, "奏": 17,
	"mfy": 18, "mafuyu": 18, "真冬": 18,
	"ena": 19, "绘名": 19, "朝比奈绘名": 19,
	"mzk": 20, "mizuki": 20, "瑞希": 20, "晓山瑞希": 20,
	"miku": 21, "初音": 21, "初音未来": 21,
	"rin": 22, "镜音铃": 22,
	"len": 23, "镜音连": 23,
	"luka": 24, "巡音流歌": 24,
	"meiko": 25,
	"kaito": 26,
}

func cloneNicknames(items map[string]int) map[string]int {
	result := make(map[string]int, len(items))
	for key, value := range items {
		result[key] = value
	}
	return result
}
