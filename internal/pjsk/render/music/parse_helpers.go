package music

import "strings"

type difficultyAlias struct {
	Canonical string
	Alias     string
}

var musicDifficultyAliases = []difficultyAlias{
	{Canonical: "append", Alias: "append"},
	{Canonical: "expert", Alias: "expert"},
	{Canonical: "master", Alias: "master"},
	{Canonical: "normal", Alias: "normal"},
	{Canonical: "append", Alias: "粉谱"},
	{Canonical: "expert", Alias: "红谱"},
	{Canonical: "master", Alias: "紫谱"},
	{Canonical: "normal", Alias: "蓝谱"},
	{Canonical: "easy", Alias: "easy"},
	{Canonical: "hard", Alias: "hard"},
	{Canonical: "easy", Alias: "绿谱"},
	{Canonical: "hard", Alias: "黄谱"},
	{Canonical: "append", Alias: "apd"},
	{Canonical: "append", Alias: "app"},
	{Canonical: "expert", Alias: "exp"},
	{Canonical: "master", Alias: "mas"},
	{Canonical: "normal", Alias: "nm"},
	{Canonical: "easy", Alias: "ez"},
	{Canonical: "hard", Alias: "hd"},
	{Canonical: "expert", Alias: "ex"},
	{Canonical: "master", Alias: "ma"},
}

func ExtractMusicDifficulty(text string) (string, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}
	lower := strings.ToLower(text)
	for _, item := range musicDifficultyAliases {
		alias := strings.ToLower(item.Alias)
		index := strings.Index(lower, alias)
		if index < 0 {
			continue
		}
		cleaned := strings.TrimSpace(text[:index] + text[index+len(alias):])
		return item.Canonical, strings.Join(strings.Fields(cleaned), " ")
	}
	return "", strings.Join(strings.Fields(text), " ")
}

func SplitMusicQueries(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "/", "\n")
	text = strings.ReplaceAll(text, "|", "\n")

	segments := strings.Split(text, "\n")
	result := make([]string, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg != "" {
			result = append(result, seg)
		}
	}
	return result
}

func ParseExplicitMusicID(text string) (int, bool) {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if !strings.HasPrefix(normalized, "music") {
		return 0, false
	}
	raw := strings.TrimPrefix(normalized, "music")
	if !isNumeric(raw) {
		return 0, false
	}
	id := 0
	for _, ch := range raw {
		id = id*10 + int(ch-'0')
	}
	return id, id > 0
}

func ParseImplicitMusicID(text string) (int, bool) {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return 0, false
	}
	if strings.HasPrefix(normalized, "id") {
		raw := strings.TrimPrefix(normalized, "id")
		if !isNumeric(raw) {
			return 0, false
		}
		id := 0
		for _, ch := range raw {
			id = id*10 + int(ch-'0')
		}
		return id, id > 0
	}
	if !isNumeric(normalized) {
		return 0, false
	}
	id := 0
	for _, ch := range normalized {
		id = id*10 + int(ch-'0')
	}
	return id, id > 0
}
