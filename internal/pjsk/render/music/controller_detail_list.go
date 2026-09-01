package music

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (c *Controller) ResolveMusicCoverByTitleOrAlias(query Query) (*CoverResult, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	allowUnreleased := c.shouldAllowLookupLeaks(query.Region, query.AllowUnreleased)
	musicInfo, err := c.resolveMusicTitleQuery(source, query.Query, allowUnreleased)
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
			Categories:         slices.Clone(musicInfo.Categories),
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
	if IsCustomChartIDQuery(query.Query) {
		return c.buildCustomMusicDetailRequest(query, source, builder, region)
	}
	allowUnreleased := c.shouldAllowLookupLeaks(query.Region, query.AllowUnreleased)
	searcher := c.newSearchService(source, allowUnreleased)
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
	var difficulty string
	if info != nil {
		difficulty = info.Difficulty
	}
	c.enrichMusicDetailRequest(req, region, source, builder, musicInfo, difficulty)
	return req, nil
}

func (c *Controller) RenderMusicDetail(query Query) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), payloadBuildStage)
	payload, err := c.BuildMusicDetailRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMusicDetail(payload)
}

func (c *Controller) BuildMusicBriefListRequest(query BriefListQuery) (*drawing.MusicBriefListRequest, error) {
	region, _, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	var payload *drawing.MusicBriefListRequest
	switch {
	case len(query.Items) > 0:
		payload, err = builder.BuildMusicBriefListRequestFromItems(query.Items, region)
	case len(query.MusicIDs) > 0:
		payload, err = builder.BuildMusicBriefListRequest(query.MusicIDs, query.Difficulty, region)
	default:
		return nil, fmt.Errorf("music ids are required")
	}
	if err != nil {
		return nil, err
	}
	payload.Title = query.Title
	payload.TitleStyle = query.TitleStyle
	payload.TitleShadow = query.TitleShadow
	return payload, nil
}

func (c *Controller) RenderMusicBriefList(query BriefListQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), payloadBuildStage)
	payload, err := c.BuildMusicBriefListRequest(query)
	finishBuild()
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
	resultFilter := normalizeMusicListResultFilter(query.ResultFilter)
	if query.Full {
		resultFilter = ""
	}
	diff := ""
	if strings.TrimSpace(query.Difficulty) != "" || len(query.Items) == 0 {
		diff = normalizeDifficulty(query.Difficulty)
	}

	minLevel := query.LevelMin
	maxLevel := query.LevelMax
	if query.Level > 0 {
		minLevel = query.Level
		maxLevel = query.Level
	}
	if minLevel > 0 && maxLevel > 0 && minLevel > maxLevel {
		minLevel, maxLevel = maxLevel, minLevel
	}

	var fallbackUserResults map[int]string
	var queryUserResults map[int]string
	if !query.Full {
		fallbackUserResults = c.buildUserResults(diff)
		queryUserResults = query.UserResults
	}
	userResults := buildMusicListUserResults(queryUserResults, fallbackUserResults)
	includeLeaks := query.IncludeLeaks
	if !includeLeaks && c.resolveRegion(query.Region) != renderregion.JP &&
		(query.Full || query.DetailedProfile == nil) && (query.Full || len(query.UserResults) == 0) &&
		len(fallbackUserResults) == 0 && resultFilter == "" {
		includeLeaks = true
	}
	filterMusicID, keyword, err := c.resolveMusicListKeywordFilter(source, query.Keyword, includeLeaks)
	if err != nil {
		return nil, err
	}
	list := make([]map[string]any, 0)
	jackets := make(map[int]string)
	if len(query.Items) > 0 {
		list, jackets = c.buildMusicListEntriesFromItems(source, builder, region, diff, query.Items, includeLeaks)
	} else {
		now := currentMusicVisibilityTime()
		for _, musicInfo := range source.GetMusics() {
			if musicInfo == nil {
				continue
			}
			if !includeLeaks && !isMusicVisibleAt(musicInfo, now) {
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
			if !matchesMusicListResultFilter(resultFilter, userResults[musicInfo.ID]) {
				continue
			}

			displayOrder := musicListDisplayOrder(musicInfo)
			list = append(list, map[string]any{
				"id":              musicInfo.ID,
				"difficulty":      level,
				"difficulty_type": diff,
				// The drawing service re-sorts each level bucket by release_at, so
				// we pass the desired display order here instead of the raw publish time.
				"release_at": displayOrder,
			})
			jackets[musicInfo.ID] = builder.BuildMusicJacketPath(musicInfo.AssetBundleName, region)
		}
	}

	if len(list) == 0 {
		return nil, fmt.Errorf("no music matched the current filters")
	}

	sort.Slice(list, func(i, j int) bool {
		diffI := normalizedDifficultyValue(list[i]["difficulty_type"])
		diffJ := normalizedDifficultyValue(list[j]["difficulty_type"])
		if diffI != diffJ {
			return difficultyOrder(diffI) < difficultyOrder(diffJ)
		}

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
	if diff == "" {
		diff = normalizedDifficultyValue(list[0]["difficulty_type"])
	}

	drawingUserResults := make(map[int]any, len(userResults))
	for musicID, result := range userResults {
		drawingUserResults[musicID] = result
	}

	req := &drawing.MusicListRequest{
		UserResults:          drawingUserResults,
		MusicList:            list,
		JacketsPathList:      jackets,
		RequiredDifficulties: diff,
		Profile:              c.resolveMusicListProfileForQuery(query, region),
		Title:                query.Title,
		TitleStyle:           query.TitleStyle,
		TitleShadow:          query.TitleShadow,
	}
	if len(userResults) > 0 && !query.Full {
		req.PlayResultIconPathMap = c.buildPlayResultIconMap(region)
	}
	return req, nil
}

func (c *Controller) buildMusicListEntriesFromItems(source DataSource, builder *Builder, region renderregion.Value, difficulty string, items []ListItemQuery, includeLeaks bool) ([]map[string]any, map[int]string) {
	now := currentMusicVisibilityTime()
	list := make([]map[string]any, 0, len(items))
	jackets := make(map[int]string, len(items))
	seen := make(map[string]struct{}, len(items))

	for _, item := range items {
		entry, jacket, seenKey, ok := buildMusicListItemEntry(source, builder, region, difficulty, item, includeLeaks, now)
		if !ok {
			continue
		}
		if _, duplicate := seen[seenKey]; duplicate {
			continue
		}

		seen[seenKey] = struct{}{}
		list = append(list, entry)
		jackets[item.MusicID] = jacket
	}

	return list, jackets
}

func buildMusicListItemEntry(source DataSource, builder *Builder, region renderregion.Value, defaultDifficulty string, item ListItemQuery, includeLeaks bool, now int64) (map[string]any, string, string, bool) {
	if item.MusicID <= 0 {
		return nil, "", "", false
	}
	musicInfo, err := source.GetMusicByID(item.MusicID)
	if err != nil || musicInfo == nil || (!includeLeaks && !isMusicVisibleAt(musicInfo, now)) {
		return nil, "", "", false
	}
	difficulty := listItemDifficulty(defaultDifficulty, item.Difficulty)
	level := item.Level
	if level <= 0 {
		level = builder.GetDifficultyLevel(musicInfo.ID, difficulty)
	}
	if level <= 0 {
		return nil, "", "", false
	}
	entry := map[string]any{
		"id":              musicInfo.ID,
		"difficulty":      level,
		"difficulty_type": difficulty,
		"release_at":      musicListDisplayOrder(musicInfo),
	}
	seenKey := fmt.Sprintf("%d:%s", item.MusicID, difficulty)
	jacket := builder.BuildMusicJacketPath(musicInfo.AssetBundleName, region)
	return entry, jacket, seenKey, true
}

func listItemDifficulty(defaultDifficulty string, itemDifficulty string) string {
	if strings.TrimSpace(itemDifficulty) == "" {
		return defaultDifficulty
	}
	return normalizeDifficulty(itemDifficulty)
}

func (c *Controller) RenderMusicList(query ListQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), payloadBuildStage)
	payload, err := c.BuildMusicListRequest(query)
	if err != nil {
		finishBuild()
		return nil, err
	}
	includeLeaks := query.IncludeLeaks
	if !includeLeaks && c.resolveRegion(query.Region) != renderregion.JP &&
		(query.Full || query.DetailedProfile == nil) && (query.Full || len(query.UserResults) == 0) &&
		(query.Full || len(c.buildUserResults(normalizeDifficulty(query.Difficulty))) == 0) &&
		(query.Full || strings.TrimSpace(query.ResultFilter) == "") {
		includeLeaks = true
	}
	finishBuild()
	return c.drawing.GenerateMusicList(payload, query.ShowID, includeLeaks)
}

func buildMusicListUserResults(primary map[int]string, fallback map[int]string) map[int]string {
	if len(primary) == 0 && len(fallback) == 0 {
		return nil
	}
	result := make(map[int]string, len(primary)+len(fallback))
	for musicID, value := range fallback {
		result[musicID] = normalizeMusicListResultValue(value)
	}
	for musicID, value := range primary {
		result[musicID] = normalizeMusicListResultValue(value)
	}
	return result
}

func normalizeMusicListResultFilter(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "not_ap":
		return "not_ap"
	case "not_fc":
		return "not_fc"
	case "not_clear":
		return "not_clear"
	default:
		return ""
	}
}

func normalizeMusicListResultValue(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ap":
		return "ap"
	case "fc":
		return "fc"
	case "clear":
		return "clear"
	default:
		return "not_clear"
	}
}

func matchesMusicListResultFilter(filter string, result string) bool {
	switch normalizeMusicListResultFilter(filter) {
	case "not_ap":
		return normalizeMusicListResultValue(result) != "ap"
	case "not_fc":
		switch normalizeMusicListResultValue(result) {
		case "ap", "fc":
			return false
		default:
			return true
		}
	case "not_clear":
		return normalizeMusicListResultValue(result) == "not_clear"
	default:
		return true
	}
}
