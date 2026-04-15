package music

import (
	"fmt"
	"path/filepath"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/drawing"
)

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
			VocalInfo:   map[string]any{},
			VocalAssets: map[string]string{},
		}, nil
	}

	info := make(map[string]any, len(vocals))
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
		info[mapKey] = map[string]any{
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
	if musicInfo, err := b.source.GetMusicByID(musicID); err == nil && musicInfo != nil && len(musicInfo.Categories) > 0 {
		return append([]string(nil), musicInfo.Categories...)
	}
	tags, err := b.source.GetMusicTags(musicID)
	if err != nil {
		return nil
	}
	return append([]string(nil), tags...)
}

func (b *Builder) buildMusicAliases(music *masterdata.Music) []string {
	if music == nil {
		return nil
	}
	var aliases []string
	if localized, err := b.source.GetMusicLocalizedTitles(music.ID); err == nil {
		for _, title := range localized {
			title = strings.TrimSpace(title)
			if title == "" || common.ContainsString(aliases, title) {
				continue
			}
			aliases = append(aliases, title)
		}
	}
	if pronunciation := strings.TrimSpace(music.Pronunciation); pronunciation != "" && !common.ContainsString(aliases, pronunciation) {
		aliases = append(aliases, pronunciation)
	}
	if tags, err := b.source.GetMusicTags(music.ID); err == nil {
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" || common.ContainsString(aliases, tag) {
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
