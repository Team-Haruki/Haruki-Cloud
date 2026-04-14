package music

import (
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/utils/drawing"
)

func (c *Controller) ResolveMusicCoverByTitleOrAlias(query Query) (*CoverResult, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	musicInfo, err := c.resolveMusicTitleQuery(source, query.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to search music by title or alias: %w", err)
	}

	jacketPath := builder.BuildMusicJacketPath(musicInfo.AssetBundleName, region)
	if localPath := c.resolveLocalMusicJacket(musicInfo.AssetBundleName); localPath != "" {
		jacketPath = localPath
	}
	if strings.TrimSpace(jacketPath) == "" {
		return nil, fmt.Errorf("music %d does not have jacket asset", musicInfo.ID)
	}

	return &CoverResult{
		Music: &masterdata.Music{
			ID:                 musicInfo.ID,
			Seq:                musicInfo.Seq,
			ReleaseConditionID: musicInfo.ReleaseConditionID,
			Categories:         append([]string(nil), musicInfo.Categories...),
			Title:              builder.buildDisplayMusicTitle(musicInfo, region),
			Pronunciation:      musicInfo.Pronunciation,
			Lyricist:           musicInfo.Lyricist,
			Composer:           musicInfo.Composer,
			Arranger:           musicInfo.Arranger,
			DancerCount:        musicInfo.DancerCount,
			SelfDancerCount:    musicInfo.SelfDancerCount,
			AssetBundleName:    musicInfo.AssetBundleName,
			PublishedAt:        musicInfo.PublishedAt,
			DigitizedAt:        musicInfo.DigitizedAt,
			IsFullLength:       musicInfo.IsFullLength,
		},
		JacketPath: jacketPath,
	}, nil
}

func (c *Controller) BuildMusicDetailRequest(query Query) (*drawing.MusicDetailRequest, error) {
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	searcher := c.newSearchService(source)
	info, err := searcher.parser.Parse(query.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to search music: %w", err)
	}
	if info != nil && strings.TrimSpace(info.Difficulty) == "" && strings.TrimSpace(query.Difficulty) != "" {
		info.Diff = normalizeDifficulty(query.Difficulty)
		info.Difficulty = info.Diff
	}
	musicInfo, err := searcher.SearchInfo(info)
	if err != nil {
		return nil, fmt.Errorf("failed to search music: %w", err)
	}
	req, err := builder.BuildMusicDetailRequest(musicInfo, region)
	if err != nil {
		return nil, err
	}
	c.appendApprovedMusicAliases(req, musicInfo.ID)
	c.enrichMusicDetailRequest(req, region, source, builder, musicInfo, info.Difficulty)
	return req, nil
}

func (c *Controller) RenderMusicDetail(query Query) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicDetailRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMusicDetail(payload)
}

func (c *Controller) BuildMusicBriefListRequest(query BriefListQuery) (*drawing.MusicBriefListRequest, error) {
	if len(query.MusicIDs) == 0 {
		return nil, fmt.Errorf("music ids are required")
	}
	region, _, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	return builder.BuildMusicBriefListRequest(query.MusicIDs, query.Difficulty, region)
}

func (c *Controller) RenderMusicBriefList(query BriefListQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicBriefListRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMusicBriefList(payload)
}

func (c *Controller) BuildMusicListRequest(query ListQuery) (*drawing.MusicListRequest, error) {
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	diff := normalizeDifficulty(query.Difficulty)

	minLevel := query.LevelMin
	maxLevel := query.LevelMax
	if query.Level > 0 {
		minLevel = query.Level
		maxLevel = query.Level
	}
	if minLevel > 0 && maxLevel > 0 && minLevel > maxLevel {
		minLevel, maxLevel = maxLevel, minLevel
	}

	filterMusicID, keyword, err := c.resolveMusicListKeywordFilter(source, query.Keyword)
	if err != nil {
		return nil, err
	}
	now := currentMusicVisibilityTime()
	list := make([]map[string]any, 0)
	jackets := make(map[int]string)

	for _, musicInfo := range source.GetMusics() {
		if musicInfo == nil {
			continue
		}
		if !query.IncludeLeaks && !isMusicVisibleAt(musicInfo, now) {
			continue
		}
		if filterMusicID != nil && musicInfo.ID != *filterMusicID {
			continue
		}
		if filterMusicID == nil && keyword != "" && !matchesMusicKeyword(source, musicInfo, keyword) {
			continue
		}

		level := builder.GetDifficultyLevel(musicInfo.ID, diff)
		if level == 0 {
			continue
		}
		if minLevel > 0 && level < minLevel {
			continue
		}
		if maxLevel > 0 && level > maxLevel {
			continue
		}

		displayOrder := musicListDisplayOrder(musicInfo)
		list = append(list, map[string]any{
			"id":         musicInfo.ID,
			"difficulty": level,
			// The drawing service re-sorts each level bucket by release_at, so
			// we pass the desired display order here instead of the raw publish time.
			"release_at": displayOrder,
		})
		jackets[musicInfo.ID] = builder.BuildMusicJacketPath(musicInfo.AssetBundleName, region)
	}

	if len(list) == 0 {
		return nil, fmt.Errorf("no music matched the current filters")
	}

	sort.Slice(list, func(i, j int) bool {
		levelI, _ := list[i]["difficulty"].(int)
		levelJ, _ := list[j]["difficulty"].(int)
		if levelI != levelJ {
			return levelI < levelJ
		}

		orderI, _ := list[i]["release_at"].(int64)
		orderJ, _ := list[j]["release_at"].(int64)
		if orderI != orderJ {
			return orderI < orderJ
		}

		idI, _ := list[i]["id"].(int)
		idJ, _ := list[j]["id"].(int)
		return idI < idJ
	})

	userResults := make(map[int]any)
	if query.UserResults != nil {
		for musicID, result := range query.UserResults {
			userResults[musicID] = result
		}
	} else {
		for musicID, result := range c.buildUserResults(diff) {
			userResults[musicID] = result
		}
	}

	req := &drawing.MusicListRequest{
		UserResults:          userResults,
		MusicList:            list,
		JacketsPathList:      jackets,
		RequiredDifficulties: diff,
		Profile:              c.resolveDetailedProfile(query.DetailedProfile, region),
		Title:                query.Title,
		TitleStyle:           query.TitleStyle,
		TitleShadow:          query.TitleShadow,
	}
	if len(userResults) > 0 {
		req.PlayResultIconPathMap = c.buildPlayResultIconMap(region)
	}
	return req, nil
}

func (c *Controller) RenderMusicList(query ListQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicListRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMusicList(payload, query.ShowID, query.IncludeLeaks)
}
