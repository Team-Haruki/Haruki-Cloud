package music

import (
	"context"
	"fmt"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/meta"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
	sekai "haruki-cloud/utils/sekai"
)

var hiddenMusicIDs = map[int]struct{}{
	241: {},
	290: {},
}

type Controller struct {
	sources               *regionsource.Registry[DataSource]
	drawing               *drawing.HarukiDrawingClient
	assets                *assets.AssetHelper
	banCharacterNicknames map[string]int
	aliases               musicAliasResolver
	snapshot              userdata.Snapshot
	metaLoader            *meta.Loader
	requestCtx            context.Context
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.requestCtx = ctx
	clone.sources = regionsource.NewRegistry[DataSource](c.sources.ResolveRegion(renderregion.Unknown))
	for _, source := range c.sources.OrderedSources() {
		if contextual, ok := any(source).(contextualDataSource); ok {
			clone.sources.RegisterSource(contextual.WithContext(ctx))
			continue
		}
		clone.sources.RegisterSource(source)
	}
	return &clone
}

func (c *Controller) WithSnapshot(snapshot userdata.Snapshot) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.snapshot = snapshot
	return &clone
}

type musicAliasResolver interface {
	TryResolveMusicID(ctx context.Context, token string) (int, bool, error)
	TryResolveMusicTitleOrAliasID(ctx context.Context, token string) (int, bool, error)
}

func NewController(defaultSource DataSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot userdata.Snapshot, metaLoader *meta.Loader) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	controller := &Controller{
		sources:               regionsource.NewRegistry[DataSource](renderregion.JP),
		drawing:               drawingClient,
		assets:                assetHelper,
		banCharacterNicknames: cloneNicknames(defaultBanCharacterNicknames),
		snapshot:              snapshot,
		metaLoader:            metaLoader,
	}
	controller.RegisterSource(defaultSource)
	return controller
}

func (c *Controller) RegisterSource(source DataSource) {
	c.sources.RegisterSource(source)
}

func (c *Controller) SetAliasResolver(resolver musicAliasResolver) {
	if c == nil {
		return
	}
	c.aliases = resolver
}

func (c *Controller) contextOrBackground() context.Context {
	if c != nil && c.requestCtx != nil {
		return c.requestCtx
	}
	return context.Background()
}

func (c *Controller) newSearchService(source DataSource) *SearchService {
	return NewSearchService(source, NewParser(c.banCharacterNicknames)).WithTitleResolver(func(query string) (*masterdata.Music, error) {
		return c.resolveMusicTitleQuery(source, query)
	})
}

func (c *Controller) resolveMusicTitleQuery(source DataSource, query string) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("music query is empty")
	}

	if musicID, ok := ParseExplicitMusicID(query); ok {
		return source.GetMusicByID(musicID)
	}

	if c != nil && c.aliases != nil {
		musicID, ok, err := c.aliases.TryResolveMusicTitleOrAliasID(c.contextOrBackground(), query)
		if err != nil {
			return nil, err
		}
		if ok {
			return source.GetMusicByID(musicID)
		}
	}

	return resolveUniqueMusicQuery(source, query)
}

func (c *Controller) resolveMusicListKeywordFilter(source DataSource, keyword string) (*int, string, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, "", nil
	}

	if musicID, ok := ParseExplicitMusicID(keyword); ok {
		if source == nil {
			return nil, "", fmt.Errorf("music data source is not configured")
		}
		if _, err := source.GetMusicByID(musicID); err != nil {
			return nil, "", err
		}
		return &musicID, "", nil
	}

	if musicID, ok := ParseImplicitMusicID(keyword); ok {
		if source != nil {
			if _, err := source.GetMusicByID(musicID); err == nil {
				return &musicID, "", nil
			}
		}
	}

	if c != nil && c.aliases != nil {
		musicID, ok, err := c.aliases.TryResolveMusicTitleOrAliasID(c.contextOrBackground(), keyword)
		if err != nil {
			return nil, "", err
		}
		if ok {
			return &musicID, "", nil
		}
	}

	return nil, strings.ToLower(keyword), nil
}

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
	musicInfo, err := searcher.Search(query.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to search music: %w", err)
	}
	req, err := builder.BuildMusicDetailRequest(musicInfo, region)
	if err != nil {
		return nil, err
	}
	c.enrichMusicDetailRequest(req, region, source, builder, musicInfo)
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

		list = append(list, map[string]interface{}{
			"id":         musicInfo.ID,
			"difficulty": level,
			"release_at": musicInfo.PublishedAt,
		})
		jackets[musicInfo.ID] = builder.BuildMusicJacketPath(musicInfo.AssetBundleName, region)
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

func (c *Controller) BuildMusicChartRequest(query ChartQuery) (*drawing.GenerateMusicChartRequest, error) {
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	searcher := c.newSearchService(source)
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
		if query.UserResults != nil {
			counts = c.buildProgressCountsFromResults(source, builder, diff, query.UserResults)
		} else if userCounts := c.buildUserProgressCounts(source, builder, diff); len(userCounts) > 0 {
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

func (c *Controller) BuildMusicProgressRequestFromSnapshot(query ProgressQuery, snapshot userdata.Snapshot, fallbackProfile *drawing.ProfileCardRequest) (*drawing.PlayProgressRequest, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	controller := c
	if snapshot != nil {
		controller = c.WithSnapshot(snapshot)
		if query.Profile == nil {
			region := controller.resolveRegion(query.Region)
			if profile := snapshot.ProfileCard(region); profile != nil {
				query.Profile = profile
			}
		}
	}
	if query.Profile == nil {
		query.Profile = fallbackProfile
	}
	return controller.BuildMusicProgressRequest(query)
}

func (c *Controller) RenderMusicProgressFromSnapshot(query ProgressQuery, snapshot userdata.Snapshot, fallbackProfile *drawing.ProfileCardRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicProgressRequestFromSnapshot(query, snapshot, fallbackProfile)
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

func (c *Controller) RenderMusicRewardsDetailFromAchievements(query RewardsDetailQuery, achievementsJSON []byte) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicRewardsDetailRequestFromAchievements(query, achievementsJSON)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateDetailMusicRewards(payload)
}

func (c *Controller) RenderMusicRewardsDetailFromSnapshot(query RewardsDetailQuery, snapshot userdata.Snapshot) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicRewardsDetailRequestFromSnapshot(query, snapshot)
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

func (c *Controller) RenderMusicRewardsBasicEstimate(query RewardsBasicQuery, clearCounts []sekai.AnotherUserMusicDifficultyClearCount, reason string) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicRewardsBasicEstimateRequest(query, clearCounts, reason)
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
