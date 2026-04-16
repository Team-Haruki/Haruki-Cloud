package music

import (
	"fmt"
	"path/filepath"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/drawing"
)

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
	categories := b.buildCategories(music.ID)

	req := &drawing.MusicDetailRequest{
		Region: regionCode,
		MusicInfo: drawing.MusicMD{
			ID:           music.ID,
			Title:        b.buildDisplayMusicTitle(music, region),
			Composer:     music.Composer,
			Lyricist:     music.Lyricist,
			Arranger:     music.Arranger,
			MVInfo:       append([]string(nil), categories...),
			Categories:   categories,
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

func (b *Builder) BuildMusicBriefListRequestFromItems(items []BriefListItemQuery, region renderregion.Value) (*drawing.MusicBriefListRequest, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("brief list items are required")
	}
	region = renderregion.WithDefault(region)

	list := make([]drawing.MusicBriefList, 0, len(items))
	requiredDiff := ""
	sameDifficulty := true

	for _, item := range items {
		if item.MusicID <= 0 {
			continue
		}
		musicInfo, err := b.source.GetMusicByID(item.MusicID)
		if err != nil || musicInfo == nil {
			continue
		}

		diff := strings.TrimSpace(item.Difficulty)
		level := 0
		var diffInfo drawing.DifficultyInfo
		if diff == "" {
			builtInfo, err := b.buildDifficultyInfo(musicInfo.ID)
			if err != nil {
				continue
			}
			diffInfo = *builtInfo
			level = maxMusicDifficultyLevel(diffInfo.Level)
		} else {
			diff = normalizeDifficulty(diff)
			level = b.GetDifficultyLevel(musicInfo.ID, diff)
			if level == 0 {
				continue
			}
			if requiredDiff == "" {
				requiredDiff = diff
			} else if requiredDiff != diff {
				sameDifficulty = false
			}
			diffInfo = drawing.DifficultyInfo{
				Level:     []int{level},
				NoteCount: []int{0},
				HasAppend: strings.EqualFold(diff, "append"),
				Order:     []string{diff},
			}
		}

		list = append(list, drawing.MusicBriefList{
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
			Difficulty: diffInfo,
		})
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no valid music data")
	}

	req := &drawing.MusicBriefListRequest{
		MusicList: list,
		Region:    region.String(),
	}
	if sameDifficulty && requiredDiff != "" {
		req.RequiredDifficulty = requiredDiff
		req.RequiredDifficulties = requiredDiff
	}
	return req, nil
}

func maxMusicDifficultyLevel(levels []int) int {
	maxLevel := 0
	for _, level := range levels {
		if level > maxLevel {
			maxLevel = level
		}
	}
	return maxLevel
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
		MusicID:              music.ID,
		Title:                b.buildDisplayMusicTitle(music, region),
		Artist:               b.BuildChartArtist(music),
		Difficulty:           diff,
		PlayLevel:            playLevel,
		Skill:                query.Skill,
		JacketPath:           assets.MakeRelative(assetBase, jacketPath),
		SusPath:              assets.MakeRelative(assetBase, susPath),
		StylePath:            &stylePath,
		NoteHost:             assets.StaticImagesDir + "/chart_asset/notes",
		TargetSegmentSeconds: float64Ptr(6.0),
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
