package music

import (
"fmt"
"regexp"
"strings"
"time"

"haruki-cloud/internal/pjsk/render/masterdata"
renderregion "haruki-cloud/internal/pjsk/render/region"
)

func normalizeDifficulty(value string) string {
	diff := strings.ToLower(strings.TrimSpace(value))
	if diff == "" {
		return "master"
	}
	return diff
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func regionToLocation(region renderregion.Value) *time.Location {
	switch region {
	case renderregion.JP, renderregion.KR:
		return time.FixedZone("UTC+9", 9*3600)
	case renderregion.CN, renderregion.TW:
		return time.FixedZone("UTC+8", 8*3600)
	case renderregion.EN:
		return time.UTC
	default:
		return time.FixedZone("UTC+9", 9*3600)
	}
}

func formatTimestamp(ts int64, location *time.Location) string {
	if location == nil {
		location = time.UTC
	}
	return time.UnixMilli(ts).In(location).Format("2006-01-02 15:04")
}

var (
	hanPattern    = regexp.MustCompile(`\p{Han}`)
	kanaPattern   = regexp.MustCompile(`[\p{Hiragana}\p{Katakana}]`)
	hangulPattern = regexp.MustCompile(`\p{Hangul}`)
	latinPattern  = regexp.MustCompile(`[A-Za-z]`)
)

func selectLocalizedTitle(base string, region string, titles []string) string {
	candidates := make([]string, 0, len(titles))
	for _, title := range titles {
		trimmed := strings.TrimSpace(title)
		if trimmed == "" || strings.EqualFold(trimmed, base) {
			continue
		}
		candidates = append(candidates, trimmed)
	}
	if len(candidates) == 0 {
		return ""
	}

	switch strings.ToLower(strings.TrimSpace(region)) {
	case "cn", "tw":
		for _, candidate := range candidates {
			if hanPattern.MatchString(candidate) && !kanaPattern.MatchString(candidate) {
				return candidate
			}
		}
		return ""
	case "kr":
		for _, candidate := range candidates {
			if hangulPattern.MatchString(candidate) {
				return candidate
			}
		}
		return ""
	case "en":
		for _, candidate := range candidates {
			if latinPattern.MatchString(candidate) {
				return candidate
			}
		}
		return ""
	default:
		return candidates[0]
	}
}

func buildJPVocalOrderKey(vocal *masterdata.MusicVocal) string {
	if vocal == nil {
		return "90_vocal"
	}
	base := strings.TrimSpace(vocal.AssetBundleName)
	if base == "" {
		base = "vocal"
	}

	priority := 90
	assetName := strings.ToLower(strings.TrimSpace(vocal.AssetBundleName))
	switch {
	case strings.HasPrefix(assetName, "vs_"):
		priority = 10
	case strings.HasPrefix(assetName, "se_"):
		priority = 20
	case strings.HasPrefix(assetName, "an_"):
		priority = 30
	}
	return fmt.Sprintf("%02d_%s", priority, base)
}

var vocalCaptionOverrides = map[string]string{
	"セカイver.":                      "Sekai",
	"セカイ ver.":                     "Sekai",
	"バーチャル・シンガーver.":               "Virtual Singer",
	"バーチャルシンガーver.":                "Virtual Singer",
	"アナザーボーカルver.":                 "Another Vocal",
	"原曲ver.":                       "Original Song",
	"原曲 ver.":                      "Original Song",
	"ストリーミングライブver.":               "Connect Live",
	"ストリーミングライブ ver.":              "Connect Live",
	"エイプリルフールver.":                 "April Fool",
	"あんさんぶるスターズ！！コラボver.":          "Ensemble Stars!! Collab",
	"「劇場版プロジェクトセカイ」ver.":           "Movie",
	"sekai ver.":                   "Sekai",
	"sekai":                        "Sekai",
	"virtual singer ver.":          "Virtual Singer",
	"virtual singer":               "Virtual Singer",
	"another vocal ver.":           "Another Vocal",
	"another vocal":                "Another Vocal",
	"original song ver.":           "Original Song",
	"original song":                "Original Song",
	"streaming live ver.":          "Connect Live",
	"streaming live":               "Connect Live",
	"instrumental ver.":            "Inst.",
	"instrumental":                 "Inst.",
	"april fool 2022 ver.":         "April Fool",
	"april_fool_2022 ver.":         "April Fool",
	"april_fool_2022":              "April Fool",
	"april fool":                   "April Fool",
	"sekai version":                "Sekai",
	"virtual singer version":       "Virtual Singer",
	"another vocal version":        "Another Vocal",
	"original song version":        "Original Song",
	"streaming live version":       "Connect Live",
	"instrumental version":         "Inst.",
	"april fool 2022 version":      "April Fool",
	"ensemble stars!! collab":      "Ensemble Stars!! Collab",
	"ensemble stars!! collab ver.": "Ensemble Stars!! Collab",
	"movie ver.":                   "Movie",
	"movie":                        "Movie",
}

var vocalTypeFallbacks = map[string]string{
	"sekai":           "Sekai",
	"virtual_singer":  "Virtual Singer",
	"original_song":   "Original Song",
	"another_vocal":   "Another Vocal",
	"streaming_live":  "Connect Live",
	"instrumental":    "Inst.",
	"april_fool_2022": "April Fool",
}

var vocalLocalizationByRegion = map[renderregion.Value]map[string]string{
	renderregion.EN: {
		"sekai":          "Sekai",
		"virtual singer": "Virtual Singer",
	},
	renderregion.JP: {
		"sekai":          "Sekai",
		"virtual singer": "Virtual Singer",
	},
	renderregion.CN: {
		"sekai":          "「世界」",
		"virtual singer": "虚拟歌手",
	},
	renderregion.TW: {
		"sekai":          "「世界」",
		"virtual singer": "虚擬歌手",
	},
	renderregion.KR: {
		"sekai":          "세카이",
		"virtual singer": "버추얼 싱어",
	},
}

func normalizeVocalCaption(raw string, vocalType string, assetBundleName string, region renderregion.Value) string {
	if preferred := classifyVocalByAssetBundle(assetBundleName, region); preferred != "" {
		return preferred
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = strings.TrimSpace(vocalType)
	}
	key := strings.ToLower(trimmed)
	key = strings.ReplaceAll(key, "　", " ")
	key = strings.ReplaceAll(key, "．", ".")
	key = strings.ReplaceAll(key, "version", "ver.")
	key = strings.ReplaceAll(key, "ver..", "ver.")
	for strings.Contains(key, "  ") {
		key = strings.ReplaceAll(key, "  ", " ")
	}
	key = strings.TrimSpace(key)
	if strings.HasSuffix(key, "ver") {
		key += "."
	}

	if resolved, ok := vocalCaptionOverrides[key]; ok {
		return localizeVocalCaption(resolved, region)
	}
	if resolved, ok := vocalTypeFallbacks[strings.ToLower(strings.TrimSpace(vocalType))]; ok {
		return localizeVocalCaption(resolved, region)
	}
	if strings.EqualFold(key, "virtual singer") {
		return localizeVocalCaption("Virtual Singer", region)
	}
	return trimmed
}

func classifyVocalByAssetBundle(assetBundleName string, region renderregion.Value) string {
	name := strings.ToLower(strings.TrimSpace(assetBundleName))
	switch {
	case strings.HasPrefix(name, "se_"):
		return localizeVocalCaption("Sekai", region)
	case strings.HasPrefix(name, "vs_"):
		return localizeVocalCaption("Virtual Singer", region)
	case strings.HasPrefix(name, "an_"):
		return localizeVocalCaption("Another Vocal", region)
	default:
		return ""
	}
}

func localizeVocalCaption(caption string, region renderregion.Value) string {
	base := strings.TrimSpace(caption)
	if base == "" {
		return caption
	}
	if items, ok := vocalLocalizationByRegion[region]; ok {
		if localized, ok := items[strings.ToLower(base)]; ok {
			return localized
		}
	}
	return base
}
