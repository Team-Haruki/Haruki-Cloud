package music

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/meta"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

var hiddenMusicIDs = map[int]struct{}{
	241: {},
	290: {},
}

type Controller struct {
	sources    *regionsource.Registry[DataSource]
	drawing    *drawing.HarukiDrawingClient
	assets     *assets.AssetHelper
	nicknames  map[string]int
	snapshot   *userdata.Service
	metaLoader *meta.Loader
}

func NewController(defaultSource DataSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot *userdata.Service, metaLoader *meta.Loader) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	controller := &Controller{
		sources:    regionsource.NewRegistry[DataSource](renderregion.JP),
		drawing:    drawingClient,
		assets:     assetHelper,
		nicknames:  cloneNicknames(defaultNicknames),
		snapshot:   snapshot,
		metaLoader: metaLoader,
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
	req, err := builder.BuildMusicChartRequest(query, musicInfo, region)
	if err != nil {
		return nil, err
	}
	if query.Skill {
		req.MusicMeta = c.resolveMusicChartMeta(region, musicInfo.ID, query.Difficulty)
	}
	return req, nil
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

func (c *Controller) BuildMusicProgressRequest(query ProgressQuery) (*drawing.PlayProgressRequest, error) {
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	diff := normalizeDifficulty(query.Difficulty)

	counts := append([]drawing.PlayProgressCount(nil), query.Counts...)
	if len(counts) == 0 {
		if userCounts := c.buildUserProgressCounts(source, builder, diff); len(userCounts) > 0 {
			counts = userCounts
		} else {
			counts = c.buildDefaultProgressCounts(source, builder, diff)
		}
	}

	return &drawing.PlayProgressRequest{
		Counts:     counts,
		Difficulty: diff,
		Profile:    c.resolveProfileCard(query.Profile, region),
	}, nil
}

func (c *Controller) RenderMusicProgress(query ProgressQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicProgressRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GeneratePlayProgress(payload)
}

func (c *Controller) BuildMusicRewardsDetailRequest(query RewardsDetailQuery) (*drawing.DetailMusicRewardsRequest, error) {
	region := c.resolveRegion(query.Region)
	return &drawing.DetailMusicRewardsRequest{
		RankRewards:   query.RankRewards,
		ComboRewards:  ensureDetailComboRewards(query.ComboRewards),
		Profile:       c.resolveProfileCard(query.Profile, region),
		JewelIconPath: c.resolveStaticIcon(query.JewelIconPath, "jewel.png"),
		ShardIconPath: c.resolveStaticIcon(query.ShardIconPath, "shard.png"),
	}, nil
}

func (c *Controller) RenderMusicRewardsDetail(query RewardsDetailQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicRewardsDetailRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateDetailMusicRewards(payload)
}

func (c *Controller) BuildMusicRewardsBasicRequest(query RewardsBasicQuery) (*drawing.BasicMusicRewardsRequest, error) {
	region := c.resolveRegion(query.Region)
	combo := query.ComboRewards
	if combo == nil {
		combo = map[string]string{
			"hard":   "0",
			"expert": "0",
			"master": "0",
			"append": "0",
		}
	}
	return &drawing.BasicMusicRewardsRequest{
		RankRewards:   query.RankRewards,
		ComboRewards:  combo,
		Profile:       c.resolveProfileCard(query.Profile, region),
		JewelIconPath: c.resolveStaticIcon(query.JewelIconPath, "jewel.png"),
		ShardIconPath: c.resolveStaticIcon(query.ShardIconPath, "shard.png"),
	}, nil
}

func (c *Controller) RenderMusicRewardsBasic(query RewardsBasicQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicRewardsBasicRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateBasicMusicRewards(payload)
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

func (c *Controller) resolveRegion(region string) renderregion.Value {
	if c == nil || c.sources == nil {
		return renderregion.WithDefault(renderregion.Normalize(region))
	}
	return c.sources.ResolveRegion(renderregion.Normalize(region))
}

func (c *Controller) currentSnapshot() *userdata.Service {
	if c == nil || c.snapshot == nil {
		return nil
	}
	if err := c.snapshot.Require(); err != nil {
		return nil
	}
	return c.snapshot
}

func (c *Controller) detailedProfile(region renderregion.Value) drawing.DetailedProfileCardRequest {
	if snapshot := c.currentSnapshot(); snapshot != nil {
		if profile := snapshot.DetailedProfile(region); profile != nil {
			return *profile
		}
	}
	return c.buildPlaceholderProfile(region)
}

func (c *Controller) resolveDetailedProfile(override *drawing.DetailedProfileCardRequest, region renderregion.Value) drawing.DetailedProfileCardRequest {
	if override != nil {
		return *override
	}
	return c.detailedProfile(region)
}

func (c *Controller) profileCard(region renderregion.Value) drawing.ProfileCardRequest {
	if snapshot := c.currentSnapshot(); snapshot != nil {
		if profile := snapshot.ProfileCard(region); profile != nil {
			return *profile
		}
	}
	return convertDetailedProfileToCard(c.buildPlaceholderProfile(region))
}

func (c *Controller) resolveProfileCard(override *drawing.ProfileCardRequest, region renderregion.Value) drawing.ProfileCardRequest {
	if override != nil {
		return *override
	}
	return c.profileCard(region)
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

func convertDetailedProfileToCard(detail drawing.DetailedProfileCardRequest) drawing.ProfileCardRequest {
	source := detail.Source
	if source == "" {
		source = "lunabot-service"
	}
	update := detail.UpdateTime
	if update == 0 {
		update = time.Now().Unix()
	}
	return drawing.ProfileCardRequest{
		Profile: &drawing.BasicProfile{
			ID:              detail.ID,
			Region:          detail.Region,
			Nickname:        detail.Nickname,
			IsHideUID:       detail.IsHideUID,
			LeaderImagePath: detail.LeaderImagePath,
			HasFrame:        detail.HasFrame,
			FramePath:       cloneStringPtr(detail.FramePath),
		},
		DataSources: []drawing.ProfileDataSource{
			{
				Name:       "User Data",
				Source:     &source,
				UpdateTime: &update,
				Mode:       cloneStringPtr(detail.Mode),
			},
		},
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

func (c *Controller) buildUserResults(diff string) map[int]string {
	snapshot := c.currentSnapshot()
	if snapshot == nil {
		return nil
	}
	return snapshot.MusicResults(diff)
}

func (c *Controller) buildDefaultProgressCounts(source DataSource, builder *Builder, diff string) []drawing.PlayProgressCount {
	countMap := make(map[int]*drawing.PlayProgressCount)
	for _, musicInfo := range source.GetMusics() {
		if musicInfo == nil {
			continue
		}
		level := builder.GetDifficultyLevel(musicInfo.ID, diff)
		if level == 0 {
			continue
		}
		entry := countMap[level]
		if entry == nil {
			entry = &drawing.PlayProgressCount{Level: level}
			countMap[level] = entry
		}
		entry.Total++
		entry.NotClear++
	}
	return flattenProgressCounts(countMap)
}

func (c *Controller) buildUserProgressCounts(source DataSource, builder *Builder, diff string) []drawing.PlayProgressCount {
	snapshot := c.currentSnapshot()
	if snapshot == nil {
		return nil
	}

	countMap := make(map[int]*drawing.PlayProgressCount)
	now := time.Now().UnixMilli()
	for _, musicInfo := range source.GetMusics() {
		if musicInfo == nil || musicInfo.PublishedAt > now {
			continue
		}
		level := builder.GetDifficultyLevel(musicInfo.ID, diff)
		if level == 0 {
			continue
		}
		entry := countMap[level]
		if entry == nil {
			entry = &drawing.PlayProgressCount{Level: level}
			countMap[level] = entry
		}
		entry.Total++
		switch snapshot.GetMusicResult(musicInfo.ID, diff) {
		case "ap":
			entry.Ap++
			entry.Fc++
			entry.Clear++
		case "fc":
			entry.Fc++
			entry.Clear++
		case "clear":
			entry.Clear++
		default:
			entry.NotClear++
		}
	}
	return flattenProgressCounts(countMap)
}

func flattenProgressCounts(countMap map[int]*drawing.PlayProgressCount) []drawing.PlayProgressCount {
	if len(countMap) == 0 {
		return nil
	}

	levels := make([]int, 0, len(countMap))
	for level := range countMap {
		levels = append(levels, level)
	}
	sort.Ints(levels)

	counts := make([]drawing.PlayProgressCount, 0, len(levels))
	for _, level := range levels {
		counts = append(counts, *countMap[level])
	}
	return counts
}

func ensureDetailComboRewards(combo map[string][]drawing.MusicComboReward) map[string][]drawing.MusicComboReward {
	if combo == nil {
		combo = make(map[string][]drawing.MusicComboReward)
	}
	for _, diff := range []string{"hard", "expert", "master", "append"} {
		if _, ok := combo[diff]; !ok {
			combo[diff] = []drawing.MusicComboReward{}
		}
	}
	return combo
}

func (c *Controller) resolveStaticIcon(explicit *string, filename string) *string {
	if explicit != nil {
		value := strings.TrimSpace(*explicit)
		if value != "" {
			return &value
		}
	}

	path := filepath.ToSlash(filepath.Join("lunabot_static_images", filename))
	if c != nil && c.assets != nil {
		if existing := c.assets.FirstExisting(path); existing != "" {
			path = assets.MakeRelative(c.assets.Primary(), existing)
		}
	}
	return &path
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (c *Controller) resolveMusicChartMeta(region renderregion.Value, musicID int, difficulty string) map[string]interface{} {
	diff := normalizeDifficulty(difficulty)
	if diff == "" || musicID <= 0 {
		return nil
	}

	if c != nil && c.metaLoader != nil {
		if payload := c.metaLoader.Get(region.String()); len(payload) > 0 {
			if item := findMusicMeta(payload, musicID, diff); item != nil {
				return item
			}
		}
	}

	if snapshot := c.currentSnapshot(); snapshot != nil {
		if payload := snapshot.MusicMetaBytes(); len(payload) > 0 {
			if item := findMusicMeta(payload, musicID, diff); item != nil {
				return item
			}
		}
	}

	return nil
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
