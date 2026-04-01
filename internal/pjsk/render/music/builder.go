package music

import (
	"fmt"
	"path/filepath"
	"strings"

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

	var stylePath string
	if trimmed := strings.TrimSpace(query.Style); trimmed != "" {
		stylePath = assets.ResolveAssetPath(b.assets, "", trimmed)
	}
	if stylePath == "" {
		stylePath = assets.StaticImagesDir + "/chart_asset/css/black.css"
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
		StylePath:  &stylePath,
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
			name, useAvatar := b.lookupVocalCharacter(character)
			if name == "" {
				name = "VS"
			}
			characters = append(characters, map[string]string{"characterName": name})
			if useAvatar && character.CharacterID != 0 {
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

func (b *Builder) lookupVocalCharacter(character masterdata.MusicVocalCharacter) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(character.CharacterType)) {
	case "outside_character":
		if name, err := b.source.GetOutsideCharacterByID(character.CharacterID); err == nil && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name), false
		}
		if b.fallback != nil {
			if name, err := b.fallback.GetOutsideCharacterByID(character.CharacterID); err == nil && strings.TrimSpace(name) != "" {
				return strings.TrimSpace(name), false
			}
		}
		return "", false
	default:
		return b.lookupCharacterName(character.CharacterID), true
	}
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

