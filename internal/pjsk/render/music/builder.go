package music

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

type Builder struct {
	source   DataSource
	fallback DataSource
	assets   *assets.AssetHelper
}

func NewBuilder(source DataSource, fallback DataSource, assetHelper *assets.AssetHelper) *Builder {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	return &Builder{
		source:   source,
		fallback: fallback,
		assets:   assetHelper,
	}
}

func (b *Builder) BuildMusicDetailRequest(music *masterdata.Music, region renderregion.Value) (*drawing.MusicDetailRequest, error) {
	if music == nil {
		return nil, fmt.Errorf("music is required")
	}
	region = renderregion.WithDefault(region)
	regionCode := strings.ToUpper(region.String())

	diffInfo, err := b.buildDifficultyInfo(music.ID)
	if err != nil {
		return nil, err
	}
	vocalInfo, err := b.buildVocalInfo(music.ID, region)
	if err != nil {
		return nil, err
	}

	req := &drawing.MusicDetailRequest{
		Region: regionCode,
		MusicInfo: drawing.MusicMD{
			ID:           music.ID,
			Title:        b.buildDisplayMusicTitle(music, region),
			Composer:     music.Composer,
			Lyricist:     music.Lyricist,
			Arranger:     music.Arranger,
			Categories:   b.buildCategories(music.ID),
			ReleaseAt:    music.PublishedAt,
			IsFullLength: music.IsFullLength,
		},
		Difficulty:      *diffInfo,
		Vocal:           *vocalInfo,
		MusicJacketPath: b.BuildMusicJacketPath(music.AssetBundleName, region),
		Alias:           b.buildMusicAliases(music),
	}

	if eventInfo, err := b.source.GetPrimaryEventByMusicID(music.ID); err == nil && eventInfo != nil {
		req.EventID = &eventInfo.ID
		if bannerPath := b.buildEventBannerPath(eventInfo.AssetBundleName, region); bannerPath != "" {
			req.EventBannerPath = &bannerPath
		}
	}
	if limited := b.buildLimitedTimes(music.ID, region); len(limited) > 0 {
		req.LimitedTimes = limited
	}
	return req, nil
}

func (b *Builder) BuildMusicBriefListRequest(musicIDs []int, difficulty string, region renderregion.Value) (*drawing.MusicBriefListRequest, error) {
	if len(musicIDs) == 0 {
		return nil, fmt.Errorf("music ids are required")
	}
	region = renderregion.WithDefault(region)
	diff := normalizeDifficulty(difficulty)

	items := make([]drawing.MusicBriefList, 0, len(musicIDs))
	for _, musicID := range musicIDs {
		musicInfo, err := b.source.GetMusicByID(musicID)
		if err != nil || musicInfo == nil {
			continue
		}
		level := b.GetDifficultyLevel(musicInfo.ID, diff)
		if level == 0 {
			continue
		}
		item := drawing.MusicBriefList{
			ID:              musicInfo.ID,
			Level:           level,
			MusicJacketPath: b.BuildMusicJacketPath(musicInfo.AssetBundleName, region),
			MusicInfo: drawing.MusicMD{
				ID:           musicInfo.ID,
				Title:        b.buildDisplayMusicTitle(musicInfo, region),
				Composer:     musicInfo.Composer,
				Lyricist:     musicInfo.Lyricist,
				Arranger:     musicInfo.Arranger,
				Categories:   b.buildCategories(musicInfo.ID),
				ReleaseAt:    musicInfo.PublishedAt,
				IsFullLength: musicInfo.IsFullLength,
			},
			Difficulty: drawing.DifficultyInfo{
				Level:     []int{level},
				NoteCount: []int{0},
				HasAppend: strings.EqualFold(diff, "append"),
				Order:     []string{diff},
			},
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no valid music data")
	}

	return &drawing.MusicBriefListRequest{
		MusicList:            items,
		Region:               region.String(),
		RequiredDifficulty:   diff,
		RequiredDifficulties: diff,
	}, nil
}

func (b *Builder) BuildMusicChartRequest(query ChartQuery, music *masterdata.Music, region renderregion.Value) (*drawing.GenerateMusicChartRequest, error) {
	if music == nil {
		return nil, fmt.Errorf("music is required")
	}
	region = renderregion.WithDefault(region)
	diff := normalizeDifficulty(query.Difficulty)

	playLevel := b.GetDifficultyLevel(music.ID, diff)
	if playLevel == 0 {
		return nil, fmt.Errorf("music %s does not have %s chart", music.Title, diff)
	}

	jacketPath := b.BuildMusicJacketPath(music.AssetBundleName, region)
	susRelative := filepath.Join("music", "music_score", fmt.Sprintf("%04d_01", music.ID), diff+".txt")
	susPath := assets.ResolveRegionAssetPath(b.assets, region.String(), susRelative)

	var stylePath *string
	if trimmed := strings.TrimSpace(query.Style); trimmed != "" {
		resolved := assets.ResolveAssetPath(b.assets, "", trimmed)
		stylePath = &resolved
	}

	assetBase := b.assets.Primary()
	return &drawing.GenerateMusicChartRequest{
		MusicID:    music.ID,
		Title:      b.buildDisplayMusicTitle(music, region),
		Artist:     b.BuildChartArtist(music),
		Difficulty: diff,
		PlayLevel:  playLevel,
		Skill:      query.Skill,
		JacketPath: assets.MakeRelative(assetBase, jacketPath),
		SusPath:    assets.MakeRelative(assetBase, susPath),
		StylePath:  stylePath,
		NoteHost:   assets.StaticImagesDir + "/chart_asset/notes",
	}, nil
}

func (b *Builder) BuildChartArtist(music *masterdata.Music) string {
	if music == nil {
		return ""
	}
	composer := strings.TrimSpace(music.Composer)
	arranger := strings.TrimSpace(music.Arranger)
	switch {
	case composer == arranger:
		return composer
	case composer == "-" || strings.Contains(arranger, composer):
		return arranger
	case arranger == "-" || strings.Contains(composer, arranger):
		return composer
	case composer == "" && arranger == "":
		return "Unknown"
	default:
		return fmt.Sprintf("%s / %s", composer, arranger)
	}
}

func (b *Builder) GetDifficultyLevel(musicID int, difficulty string) int {
	difficulties, err := b.source.GetMusicDifficulties(musicID)
	if err != nil {
		return 0
	}
	for _, diff := range difficulties {
		if strings.EqualFold(strings.TrimSpace(diff.MusicDifficulty), difficulty) {
			return diff.PlayLevel
		}
	}
	return 0
}

func (b *Builder) BuildMusicJacketPath(assetName string, region renderregion.Value) string {
	if strings.TrimSpace(assetName) == "" {
		return ""
	}
	return assets.ResolveRegionAssetPath(b.assets, region.String(), filepath.Join("music", "jacket", assetName, assetName+".png"))
}

func (b *Builder) BuildCharacterIconPath(characterID int, _ renderregion.Value) string {
	if nickname, ok := assets.CharacterIDToNickname[characterID]; ok {
		return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", nickname+".png"))
	}
	return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)))
}

func (b *Builder) buildDifficultyInfo(musicID int) (*drawing.DifficultyInfo, error) {
	difficulties, err := b.source.GetMusicDifficulties(musicID)
	if err != nil {
		return &drawing.DifficultyInfo{
			Level:     []int{0, 0, 0, 0, 0},
			NoteCount: []int{0, 0, 0, 0, 0},
			HasAppend: false,
			Order:     []string{"easy", "normal", "hard", "expert", "master"},
		}, nil
	}

	type stats struct {
		level int
		notes int
	}
	diffMap := make(map[string]stats, len(difficulties))
	for _, diff := range difficulties {
		key := strings.ToLower(strings.TrimSpace(diff.MusicDifficulty))
		diffMap[key] = stats{level: diff.PlayLevel, notes: diff.TotalNoteCount}
	}

	baseOrder := []string{"easy", "normal", "hard", "expert", "master"}
	levels := make([]int, 0, len(baseOrder)+1)
	notes := make([]int, 0, len(baseOrder)+1)
	order := make([]string, 0, len(baseOrder)+1)
	for _, diff := range baseOrder {
		if item, ok := diffMap[diff]; ok {
			levels = append(levels, item.level)
			notes = append(notes, item.notes)
		} else {
			levels = append(levels, 0)
			notes = append(notes, 0)
		}
		order = append(order, diff)
	}

	hasAppend := false
	if item, ok := diffMap["append"]; ok {
		hasAppend = true
		levels = append(levels, item.level)
		notes = append(notes, item.notes)
		order = append(order, "append")
	}

	return &drawing.DifficultyInfo{
		Level:     levels,
		NoteCount: notes,
		HasAppend: hasAppend,
		Order:     order,
	}, nil
}

func (b *Builder) buildVocalInfo(musicID int, region renderregion.Value) (*drawing.MusicVocalInfo, error) {
	vocals, err := b.source.GetMusicVocals(musicID)
	if err != nil && b.fallback != nil {
		vocals, err = b.fallback.GetMusicVocals(musicID)
	}
	if err != nil {
		return &drawing.MusicVocalInfo{
			VocalInfo:   map[string]interface{}{},
			VocalAssets: map[string]string{},
		}, nil
	}

	info := make(map[string]interface{}, len(vocals))
	assetsMap := make(map[string]string)
	for _, vocal := range vocals {
		if vocal == nil {
			continue
		}

		characters := make([]map[string]string, 0, len(vocal.Characters))
		for _, character := range vocal.Characters {
			name := b.lookupCharacterName(character.CharacterID)
			if name == "" {
				name = "VS"
			}
			characters = append(characters, map[string]string{"characterName": name})
			if character.CharacterID != 0 {
				assetsMap[name] = b.BuildCharacterIconPath(character.CharacterID, region)
			}
		}

		mapKey := vocal.AssetBundleName
		if region == renderregion.JP {
			mapKey = buildJPVocalOrderKey(vocal)
		}
		info[mapKey] = map[string]interface{}{
			"caption":    normalizeVocalCaption(vocal.Caption, vocal.MusicVocalType, vocal.AssetBundleName, region),
			"characters": characters,
		}
	}

	return &drawing.MusicVocalInfo{
		VocalInfo:   info,
		VocalAssets: assetsMap,
	}, nil
}

func (b *Builder) lookupCharacterName(characterID int) string {
	if characterID == 0 {
		return ""
	}
	if character, err := b.source.GetCharacterByID(characterID); err == nil && character != nil {
		return strings.TrimSpace(character.FirstName + character.GivenName)
	}
	if b.fallback != nil {
		if character, err := b.fallback.GetCharacterByID(characterID); err == nil && character != nil {
			return strings.TrimSpace(character.FirstName + character.GivenName)
		}
	}
	return ""
}

func (b *Builder) buildDisplayMusicTitle(music *masterdata.Music, region renderregion.Value) string {
	if music == nil {
		return ""
	}
	base := strings.TrimSpace(music.Title)
	if base == "" {
		return music.Title
	}
	if region == renderregion.JP {
		return base
	}

	titles, err := b.source.GetMusicLocalizedTitles(music.ID)
	if err != nil || len(titles) == 0 {
		return base
	}
	alt := selectLocalizedTitle(base, region.String(), titles)
	if alt == "" {
		return base
	}
	return fmt.Sprintf("%s (%s)", base, alt)
}

func (b *Builder) buildCategories(musicID int) []string {
	tags, err := b.source.GetMusicTags(musicID)
	if err != nil {
		return nil
	}
	return tags
}

func (b *Builder) buildMusicAliases(music *masterdata.Music) []string {
	if music == nil {
		return nil
	}
	var aliases []string
	if localized, err := b.source.GetMusicLocalizedTitles(music.ID); err == nil {
		for _, title := range localized {
			title = strings.TrimSpace(title)
			if title == "" || containsString(aliases, title) {
				continue
			}
			aliases = append(aliases, title)
		}
	}
	if pronunciation := strings.TrimSpace(music.Pronunciation); pronunciation != "" && !containsString(aliases, pronunciation) {
		aliases = append(aliases, pronunciation)
	}
	if tags, err := b.source.GetMusicTags(music.ID); err == nil {
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" || containsString(aliases, tag) {
				continue
			}
			aliases = append(aliases, tag)
		}
	}
	return aliases
}

func (b *Builder) buildLimitedTimes(musicID int, region renderregion.Value) [][]string {
	items := b.source.GetLimitedTimeMusics(musicID)
	if len(items) == 0 {
		return nil
	}
	location := regionToLocation(region)
	result := make([][]string, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result = append(result, []string{
			formatTimestamp(item.StartAt, location),
			formatTimestamp(item.EndAt, location),
		})
	}
	return result
}

func (b *Builder) buildEventBannerPath(assetBundleName string, region renderregion.Value) string {
	if strings.TrimSpace(assetBundleName) == "" {
		return ""
	}
	return assets.ResolveRegionAssetPath(
		b.assets, region.String(),
		filepath.Join("home", "banner", assetBundleName, assetBundleName+".png"),
		filepath.Join("event", assetBundleName, "banner.png"),
	)
}

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
