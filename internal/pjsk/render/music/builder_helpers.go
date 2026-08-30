package music

import (
	"fmt"
	"regexp"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func normalizeDifficulty(value string) string {
	diff := strings.ToLower(strings.TrimSpace(value))
	if diff == "" {
		return "master"
	}
	return diff
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

	switch renderregion.Normalize(region) {
	case renderregion.CN, renderregion.TW:
		for _, candidate := range candidates {
			if hanPattern.MatchString(candidate) && !kanaPattern.MatchString(candidate) {
				return candidate
			}
		}
		return ""
	case renderregion.KR:
		for _, candidate := range candidates {
			if hangulPattern.MatchString(candidate) {
				return candidate
			}
		}
		return ""
	case renderregion.EN:
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
	"バーチャル・シンガーver.":               virtualSingerLabel,
	"バーチャルシンガーver.":                virtualSingerLabel,
	"アナザーボーカルver.":                 anotherVocalLabel,
	"原曲ver.":                       originalSongLabel,
	"原曲 ver.":                      originalSongLabel,
	"ストリーミングライブver.":               connectLiveLabel,
	"ストリーミングライブ ver.":              connectLiveLabel,
	"エイプリルフールver.":                 aprilFoolLabel,
	"あんさんぶるスターズ！！コラボver.":          ensembleStarsCollabLabel,
	"「劇場版プロジェクトセカイ」ver.":           "Movie",
	"sekai ver.":                   "Sekai",
	"sekai":                        "Sekai",
	"virtual singer ver.":          virtualSingerLabel,
	virtualSingerLowerLabel:        virtualSingerLabel,
	"another vocal ver.":           anotherVocalLabel,
	"another vocal":                anotherVocalLabel,
	"original song ver.":           originalSongLabel,
	"original song":                originalSongLabel,
	"streaming live ver.":          connectLiveLabel,
	"streaming live":               connectLiveLabel,
	"instrumental ver.":            "Inst.",
	"instrumental":                 "Inst.",
	"april fool 2022 ver.":         aprilFoolLabel,
	"april_fool_2022 ver.":         aprilFoolLabel,
	"april_fool_2022":              aprilFoolLabel,
	"april fool":                   aprilFoolLabel,
	"sekai version":                "Sekai",
	"virtual singer version":       virtualSingerLabel,
	"another vocal version":        anotherVocalLabel,
	"original song version":        originalSongLabel,
	"streaming live version":       connectLiveLabel,
	"instrumental version":         "Inst.",
	"april fool 2022 version":      aprilFoolLabel,
	"ensemble stars!! collab":      ensembleStarsCollabLabel,
	"ensemble stars!! collab ver.": ensembleStarsCollabLabel,
	"movie ver.":                   "Movie",
	"movie":                        "Movie",
}

var vocalTypeFallbacks = map[string]string{
	"sekai":           "Sekai",
	"virtual_singer":  virtualSingerLabel,
	"original_song":   originalSongLabel,
	"another_vocal":   anotherVocalLabel,
	"streaming_live":  connectLiveLabel,
	"instrumental":    "Inst.",
	"april_fool_2022": aprilFoolLabel,
}

var vocalLocalizationByRegion = map[renderregion.Value]map[string]string{
	renderregion.EN: {
		"sekai":                 "Sekai",
		virtualSingerLowerLabel: virtualSingerLabel,
	},
	renderregion.JP: {
		"sekai":                 "Sekai",
		virtualSingerLowerLabel: virtualSingerLabel,
	},
	renderregion.CN: {
		"sekai":                 "「世界」",
		virtualSingerLowerLabel: "虚拟歌手",
	},
	renderregion.TW: {
		"sekai":                 "「世界」",
		virtualSingerLowerLabel: "虚擬歌手",
	},
	renderregion.KR: {
		"sekai":                 "세카이",
		virtualSingerLowerLabel: "버추얼 싱어",
	},
}

func normalizeVocalCaption(raw string, vocalType string, assetBundleName string, region renderregion.Value) string {
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
	if trimmed != "" {
		return trimmed
	}
	if preferred := classifyVocalByAssetBundle(assetBundleName, region); preferred != "" {
		return preferred
	}
	if resolved, ok := vocalTypeFallbacks[strings.ToLower(strings.TrimSpace(vocalType))]; ok {
		return localizeVocalCaption(resolved, region)
	}
	if strings.EqualFold(key, virtualSingerLowerLabel) {
		return localizeVocalCaption(virtualSingerLabel, region)
	}
	return trimmed
}

func classifyVocalByAssetBundle(assetBundleName string, region renderregion.Value) string {
	name := strings.ToLower(strings.TrimSpace(assetBundleName))
	switch {
	case strings.HasPrefix(name, "se_"):
		return localizeVocalCaption("Sekai", region)
	case strings.HasPrefix(name, "vs_"):
		return localizeVocalCaption(virtualSingerLabel, region)
	case strings.HasPrefix(name, "an_"):
		return localizeVocalCaption(anotherVocalLabel, region)
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
