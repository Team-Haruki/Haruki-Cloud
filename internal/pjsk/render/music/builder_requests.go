package music

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"haruki-cloud/internal/pjsk/chartstyle"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
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
			MVInfo:       slices.Clone(categories),
			Categories:   categories,
			ReleaseAt:    music.PublishedAt,
			IsFullLength: music.IsFullLength,
		},
		Difficulty:      *diffInfo,
		Vocal:           *vocalInfo,
		MusicJacketPath: b.BuildMusicJacketPath(music.AssetBundleName, region),
		Alias:           []string{},
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

	for _, query := range items {
		item, diff, ok := b.buildMusicBriefListItem(query, region)
		if !ok {
			continue
		}
		list = append(list, item)
		if diff == "" {
			continue
		}
		if requiredDiff == "" {
			requiredDiff = diff
		} else if requiredDiff != diff {
			sameDifficulty = false
		}
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

func (b *Builder) buildMusicBriefListItem(query BriefListItemQuery, region renderregion.Value) (drawing.MusicBriefList, string, bool) {
	if query.MusicID <= 0 {
		return drawing.MusicBriefList{}, "", false
	}
	musicInfo, err := b.source.GetMusicByID(query.MusicID)
	if err != nil || musicInfo == nil {
		return drawing.MusicBriefList{}, "", false
	}
	difficulty, diff, level, ok := b.buildBriefListDifficulty(musicInfo.ID, query.Difficulty)
	if !ok {
		return drawing.MusicBriefList{}, "", false
	}
	return drawing.MusicBriefList{
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
		Difficulty: difficulty,
	}, diff, true
}

func (b *Builder) buildBriefListDifficulty(musicID int, raw string) (drawing.DifficultyInfo, string, int, bool) {
	diff := strings.TrimSpace(raw)
	if diff == "" {
		info, err := b.buildDifficultyInfo(musicID)
		if err != nil {
			return drawing.DifficultyInfo{}, "", 0, false
		}
		return *info, "", maxMusicDifficultyLevel(info.Level), true
	}
	diff = normalizeDifficulty(diff)
	level := b.GetDifficultyLevel(musicID, diff)
	if level == 0 {
		return drawing.DifficultyInfo{}, "", 0, false
	}
	info := drawing.DifficultyInfo{
		Level:     []int{level},
		NoteCount: []int{0},
		HasAppend: strings.EqualFold(diff, "append"),
		Order:     []string{diff},
	}
	return info, diff, level, true
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

	stylePath := chartstyle.CSSPath(query.Style)

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
