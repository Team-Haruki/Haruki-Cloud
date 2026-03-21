package music

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/utils/drawing"
)

var hiddenMusicIDs = map[int]struct{}{
	241: {},
	290: {},
}

type Controller struct {
	sources   *regionsource.Registry[DataSource]
	drawing   *drawing.HarukiDrawingClient
	assets    *assets.AssetHelper
	nicknames map[string]int
}

func NewController(defaultSource DataSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	controller := &Controller{
		sources:   regionsource.NewRegistry[DataSource](renderregion.JP),
		drawing:   drawingClient,
		assets:    assetHelper,
		nicknames: cloneNicknames(defaultNicknames),
	}
	controller.RegisterSource(defaultSource)
	return controller
}

func (c *Controller) RegisterSource(source DataSource) {
	c.sources.RegisterSource(source)
}

func (c *Controller) BuildMusicDetailRequest(query Query) (*drawing.MusicDetailRequest, error) {
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	searcher := NewSearchService(source, NewParser(c.nicknames))
	musicInfo, err := searcher.Search(query.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to search music: %w", err)
	}
	return builder.BuildMusicDetailRequest(musicInfo, region)
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

	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	now := time.Now().UnixMilli()
	list := make([]map[string]interface{}, 0)
	jackets := make(map[int]string)

	for _, musicInfo := range source.GetMusics() {
		if musicInfo == nil {
			continue
		}
		if _, blocked := hiddenMusicIDs[musicInfo.ID]; blocked {
			continue
		}
		if !query.IncludeLeaks && musicInfo.PublishedAt > now {
			continue
		}
		if keyword != "" && !matchesMusicKeyword(source, musicInfo, keyword) {
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

		list = append(list, map[string]interface{}{
			"id":         musicInfo.ID,
			"difficulty": level,
			"release_at": musicInfo.PublishedAt,
		})
		jackets[musicInfo.ID] = builder.BuildMusicJacketPath(musicInfo.AssetBundleName)
	}

	if len(list) == 0 {
		return nil, fmt.Errorf("no music matched the current filters")
	}

	userResults := make(map[int]interface{})
	for musicID, result := range query.UserResults {
		userResults[musicID] = result
	}

	req := &drawing.MusicListRequest{
		UserResults:          userResults,
		MusicList:            list,
		JacketsPathList:      jackets,
		RequiredDifficulties: diff,
		Profile:              c.buildPlaceholderProfile(region),
		Title:                query.Title,
		TitleStyle:           query.TitleStyle,
		TitleShadow:          query.TitleShadow,
	}
	if len(userResults) > 0 {
		req.PlayResultIconPathMap = c.buildPlayResultIconMap()
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

func (c *Controller) BuildMusicChartRequest(query ChartQuery) (*drawing.GenerateMusicChartRequest, error) {
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	searcher := NewSearchService(source, NewParser(c.nicknames))
	info, musicInfo, err := searcher.SearchChart(query.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to search music chart: %w", err)
	}
	if strings.TrimSpace(query.Difficulty) == "" && info != nil {
		query.Difficulty = info.Difficulty
	}
	return builder.BuildMusicChartRequest(query, musicInfo, region)
}

func (c *Controller) RenderMusicChart(query ChartQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicChartRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMusicChart(payload)
}

func (c *Controller) resolveBuilder(region string) (renderregion.Value, DataSource, *Builder, error) {
	resolved := c.sources.ResolveRegion(renderregion.Normalize(region))
	source, ok := c.sources.SourceForRegion(resolved)
	if !ok {
		return resolved, nil, nil, fmt.Errorf("no music data source for region %s", resolved)
	}
	return resolved, source, NewBuilder(source, c.fallbackSource(resolved), c.assets), nil
}

func (c *Controller) fallbackSource(region renderregion.Value) DataSource {
	if region == renderregion.JP {
		return nil
	}
	if source, ok := c.sources.SourceForRegion(renderregion.JP); ok {
		return source
	}
	return nil
}

func (c *Controller) buildPlaceholderProfile(region renderregion.Value) drawing.DetailedProfileCardRequest {
	mode := "service"
	leaderPath := assets.ResolveAssetPath(
		c.assets,
		"",
		filepath.Join("user", "leader.png"),
		filepath.Join("chara_icon", "miku.png"),
	)
	return drawing.DetailedProfileCardRequest{
		ID:              "service",
		Region:          strings.ToUpper(renderregion.WithDefault(region).String()),
		Nickname:        "Lunabot",
		Source:          "lunabot-service",
		UpdateTime:      time.Now().Unix(),
		Mode:            &mode,
		IsHideUID:       true,
		LeaderImagePath: leaderPath,
		HasFrame:        false,
		UserCards:       []interface{}{},
	}
}

func (c *Controller) buildPlayResultIconMap() map[string]string {
	return map[string]string{
		"not_clear": assets.ResolveAssetPath(c.assets, "", "icon_not_clear.png"),
		"clear":     assets.ResolveAssetPath(c.assets, "", "icon_clear.png"),
		"fc":        assets.ResolveAssetPath(c.assets, "", "icon_fc.png"),
		"ap":        assets.ResolveAssetPath(c.assets, "", "icon_ap.png"),
	}
}

func matchesMusicKeyword(source DataSource, musicInfo *masterdata.Music, keyword string) bool {
	if musicInfo == nil {
		return false
	}
	if strings.Contains(strings.ToLower(musicInfo.Title), keyword) {
		return true
	}
	if pronunciation := strings.ToLower(strings.TrimSpace(musicInfo.Pronunciation)); pronunciation != "" && strings.Contains(pronunciation, keyword) {
		return true
	}
	if tags, err := source.GetMusicTags(musicInfo.ID); err == nil {
		for _, tag := range tags {
			if strings.Contains(strings.ToLower(tag), keyword) {
				return true
			}
		}
	}
	if titles, err := source.GetMusicLocalizedTitles(musicInfo.ID); err == nil {
		for _, title := range titles {
			if strings.Contains(strings.ToLower(strings.TrimSpace(title)), keyword) {
				return true
			}
		}
	}
	return false
}
