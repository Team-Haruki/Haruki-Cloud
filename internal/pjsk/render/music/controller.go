package music

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
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

var boardDefaultDifficulties = []string{"easy", "normal", "hard", "expert", "master", "append"}

type Controller struct {
	sources               *regionsource.Registry[DataSource]
	drawing               *drawing.HarukiDrawingClient
	assets                *assets.AssetHelper
	banCharacterNicknames map[string]int
	aliases               musicAliasResolver
	snapshot              *userdata.Service
	metaLoader            *meta.Loader
}

type musicAliasResolver interface {
	TryResolveMusicID(ctx context.Context, token string) (int, bool, error)
}

func NewController(defaultSource DataSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot *userdata.Service, metaLoader *meta.Loader) *Controller {
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

	if c != nil && c.aliases != nil {
		musicID, ok, err := c.aliases.TryResolveMusicID(context.Background(), query)
		if err != nil {
			return nil, err
		}
		if ok {
			return source.GetMusicByID(musicID)
		}
	}

	return source.SearchMusic(query)
}

func (c *Controller) resolveMusicListKeywordFilter(keyword string) (*int, string, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, "", nil
	}

	if c != nil && c.aliases != nil {
		musicID, ok, err := c.aliases.TryResolveMusicID(context.Background(), keyword)
		if err != nil {
			return nil, "", err
		}
		if ok {
			return &musicID, "", nil
		}
	}

	return nil, strings.ToLower(keyword), nil
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

	filterMusicID, keyword, err := c.resolveMusicListKeywordFilter(query.Keyword)
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

func (c *Controller) profileCardWithMessage(override *drawing.ProfileCardRequest, region renderregion.Value, message *string) drawing.ProfileCardRequest {
	card := c.resolveProfileCard(override, region)
	if message == nil {
		return card
	}
	copy := *message
	card.ErrorMessage = &copy
	return card
}

func (c *Controller) buildPlaceholderProfile(region renderregion.Value) drawing.DetailedProfileCardRequest {
	mode := "service"
	leaderPath := assets.ResolveRegionAssetPath(
		c.assets, renderregion.WithDefault(region).String(),
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

func (c *Controller) buildPlayResultIconMap(_ renderregion.Value) map[string]string {
	return map[string]string{
		"not_clear": assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_not_clear.png"),
		"clear":     assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_clear.png"),
		"fc":        assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_fc.png"),
		"ap":        assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_ap.png"),
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

	path := filepath.ToSlash(filepath.Join(assets.StaticImagesDir, filename))
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

// ResolveCustomRoomMusicList returns song candidates grouped by event rate.
func (c *Controller) ResolveCustomRoomMusicList(region string, eventRates []int, numPerRate int) (map[int][]map[string]interface{}, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	if len(eventRates) == 0 {
		return nil, fmt.Errorf("no event rates provided")
	}
	if numPerRate <= 0 {
		numPerRate = 3
	}

	resolved, source, builder, err := c.resolveBuilder(region)
	if err != nil {
		return nil, err
	}
	payload := c.loadMetaPayload(resolved)
	if len(payload) == 0 {
		return nil, fmt.Errorf("music meta data unavailable for region %s", resolved)
	}

	wantRates := make(map[int]struct{}, len(eventRates))
	for _, rate := range eventRates {
		if rate > 0 {
			wantRates[rate] = struct{}{}
		}
	}
	if len(wantRates) == 0 {
		return nil, fmt.Errorf("no valid event rates provided")
	}

	result := make(map[int][]map[string]interface{}, len(wantRates))
	seenMusic := make(map[int]map[int]struct{}, len(wantRates))
	musics := append([]*masterdata.Music(nil), source.GetMusics()...)
	sort.SliceStable(musics, func(i, j int) bool {
		if musics[i] == nil || musics[j] == nil {
			return musics[i] != nil
		}
		if musics[i].PublishedAt == musics[j].PublishedAt {
			return musics[i].ID < musics[j].ID
		}
		return musics[i].PublishedAt < musics[j].PublishedAt
	})

	for _, musicInfo := range musics {
		if musicInfo == nil {
			continue
		}
		metas := findAllMusicMetas(payload, musicInfo.ID)
		if len(metas) == 0 {
			continue
		}
		for _, meta := range metas {
			rate := int(floatValue(meta["event_rate"]))
			if _, ok := wantRates[rate]; !ok {
				continue
			}
			if len(result[rate]) >= numPerRate {
				continue
			}

			rateSeen := seenMusic[rate]
			if rateSeen == nil {
				rateSeen = make(map[int]struct{})
				seenMusic[rate] = rateSeen
			}
			if _, exists := rateSeen[musicInfo.ID]; exists {
				continue
			}
			rateSeen[musicInfo.ID] = struct{}{}

			result[rate] = append(result[rate], map[string]interface{}{
				"music_id":    musicInfo.ID,
				"music_title": builder.buildDisplayMusicTitle(musicInfo, resolved),
				"music_cover": builder.BuildMusicJacketPath(musicInfo.AssetBundleName, resolved),
			})
		}
	}

	return result, nil
}

// ResolveMusicMetaRequests resolves music queries into metadata payloads for score commands.
func (c *Controller) ResolveMusicMetaRequests(region string, queries []string) ([]drawing.MusicMetaRequest, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	if len(queries) == 0 {
		return nil, fmt.Errorf("no music queries provided")
	}

	resolved, source, builder, err := c.resolveBuilder(region)
	if err != nil {
		return nil, err
	}
	payload := c.loadMetaPayload(resolved)
	if len(payload) == 0 {
		return nil, fmt.Errorf("music meta data unavailable for region %s", resolved)
	}

	searcher := NewSearchService(source, NewParser(c.banCharacterNicknames))
	results := make([]drawing.MusicMetaRequest, 0, len(queries))
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		musicInfo, searchErr := searcher.Search(query)
		if searchErr != nil || musicInfo == nil {
			continue
		}
		results = append(results, drawing.MusicMetaRequest{
			MusicID:        musicInfo.ID,
			MusicTitle:     builder.buildDisplayMusicTitle(musicInfo, resolved),
			MusicCoverPath: builder.BuildMusicJacketPath(musicInfo.AssetBundleName, resolved),
			Metas:          musicMetaInfosFromPayload(payload, musicInfo.ID),
		})
	}
	return results, nil
}

// ResolveMusicBoardRequest builds a score board payload from local meta and master data.
func (c *Controller) ResolveMusicBoardRequest(region string, query BoardQuery) (*drawing.MusicBoardRequest, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}

	resolved, source, builder, err := c.resolveBuilder(region)
	if err != nil {
		return nil, err
	}
	payload := c.loadMetaPayload(resolved)
	if len(payload) == 0 {
		return nil, fmt.Errorf("music meta data unavailable for region %s", resolved)
	}

	liveType := normalizeBoardLiveType(query.LiveType)
	target := normalizeBoardTarget(query.Target)
	diffs := normalizeBoardDiffFilter(query.DiffFilter)
	page := query.Page
	if page <= 0 {
		page = 1
	}

	items := make([]drawing.MusicBoardItem, 0, 128)
	now := time.Now().UnixMilli()
	for _, musicInfo := range source.GetMusics() {
		if musicInfo == nil || musicInfo.PublishedAt > now {
			continue
		}
		if _, hidden := hiddenMusicIDs[musicInfo.ID]; hidden {
			continue
		}
		for _, diff := range diffs {
			level := builder.GetDifficultyLevel(musicInfo.ID, diff)
			if level == 0 || !boardLevelMatched(level, query.LevelFilter) {
				continue
			}

			meta := findMusicMeta(payload, musicInfo.ID, diff)
			if meta == nil {
				continue
			}

			eventRate := floatValue(meta["event_rate"])
			musicTime := floatValue(meta["music_time"])
			tapCount := floatValue(meta["tap_count"])
			baseScore := floatValue(meta["base_score"])
			baseScoreAuto := floatValue(meta["base_score_auto"])
			feverScore := floatValue(meta["fever_score"])

			score, skillAccount := resolveBoardLiveScore(
				liveType,
				baseScore,
				baseScoreAuto,
				floatSliceValue(meta["skill_score_solo"]),
				floatSliceValue(meta["skill_score_auto"]),
				floatSliceValue(meta["skill_score_multi"]),
				feverScore,
			)

			realScore := score
			if query.Power > 0 {
				realScore = score * float64(query.Power)
			}
			if query.DeckBonus > 0 {
				realScore = realScore * (100 + query.DeckBonus) / 100
			}

			pt := eventRate * realScore / 100
			interval := query.PlayInterval
			if interval < 0 {
				interval = 0
			}
			playCountPerHour := 0.0
			if musicTime+interval > 0 {
				playCountPerHour = 3600 / (musicTime + interval)
			}
			ptPerHour := pt * playCountPerHour

			item := drawing.MusicBoardItem{
				MusicID:              musicInfo.ID,
				Difficulty:           diff,
				Level:                level,
				MusicTitle:           builder.buildDisplayMusicTitle(musicInfo, resolved),
				MusicCoverPath:       builder.BuildMusicJacketPath(musicInfo.AssetBundleName, resolved),
				LiveTypePt:           floatPtr(pt),
				LiveTypeRealScore:    floatPtr(realScore),
				LiveTypeScore:        floatPtr(score),
				LiveTypeSkillAccount: floatPtr(skillAccount),
				LiveTypePtPerHour:    floatPtr(ptPerHour),
				PlayCountPerHour:     floatPtr(playCountPerHour),
				EventRate:            eventRate,
				MusicTime:            musicTime,
				Tps:                  boardTPS(tapCount, musicTime),
			}
			items = append(items, item)
		}
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("music board request has no items")
	}

	sortMusicBoardItems(items, target, query.Ascend)

	const pageSize = 50
	totalPage := (len(items) + pageSize - 1) / pageSize
	if page > totalPage {
		page = totalPage
	}
	if page <= 0 {
		page = 1
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	pageItems := append([]drawing.MusicBoardItem(nil), items[start:end]...)
	for i := range pageItems {
		pageItems[i].Rank = start + i + 1
	}

	return &drawing.MusicBoardRequest{
		LiveType:     liveType,
		Target:       target,
		Ascend:       query.Ascend,
		Page:         page,
		TotalPage:    totalPage,
		TitleText:    buildMusicBoardTitle(liveType, target, query),
		Items:        pageItems,
		SpecMidDiffs: c.resolveBoardSpecMidDiffs(source, builder, query.SpecQueries),
		Description:  buildMusicBoardDescription(query),
	}, nil
}

func (c *Controller) loadMetaPayload(region renderregion.Value) []byte {
	if c != nil && c.metaLoader != nil {
		if payload := c.metaLoader.Get(region.String()); len(payload) > 0 {
			return payload
		}
	}
	if snapshot := c.currentSnapshot(); snapshot != nil {
		if payload := snapshot.MusicMetaBytes(); len(payload) > 0 {
			return payload
		}
	}
	return nil
}

func (c *Controller) resolveBoardSpecMidDiffs(source DataSource, builder *Builder, queries []string) [][]interface{} {
	if len(queries) == 0 {
		return nil
	}
	searcher := NewSearchService(source, NewParser(c.banCharacterNicknames))
	seen := make(map[string]struct{})
	result := make([][]interface{}, 0, len(queries))
	for _, raw := range queries {
		query := strings.TrimSpace(raw)
		if query == "" {
			continue
		}
		musicInfo, err := searcher.Search(query)
		if err != nil || musicInfo == nil {
			continue
		}
		for _, diff := range boardDefaultDifficulties {
			if builder.GetDifficultyLevel(musicInfo.ID, diff) == 0 {
				continue
			}
			key := strconv.Itoa(musicInfo.ID) + ":" + diff
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, []interface{}{musicInfo.ID, diff})
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		leftID := result[i][0].(int)
		rightID := result[j][0].(int)
		if leftID == rightID {
			return difficultyOrder(result[i][1].(string)) < difficultyOrder(result[j][1].(string))
		}
		return leftID < rightID
	})
	return result
}

func normalizeBoardLiveType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "multi":
		return "multi"
	case "auto":
		return "auto"
	default:
		return "solo"
	}
}

func normalizeBoardTarget(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pt", "pt/time", "tps", "time":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "score"
	}
}

func normalizeBoardDiffFilter(raw []string) []string {
	if len(raw) == 0 {
		return append([]string(nil), boardDefaultDifficulties...)
	}
	seen := make(map[string]struct{}, len(raw))
	result := make([]string, 0, len(raw))
	for _, diff := range raw {
		normalized := normalizeDifficulty(diff)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return append([]string(nil), boardDefaultDifficulties...)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return difficultyOrder(result[i]) < difficultyOrder(result[j])
	})
	return result
}

func boardLevelMatched(level int, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}

	operator := ""
	switch {
	case strings.HasPrefix(filter, "<="), strings.HasPrefix(filter, ">="), strings.HasPrefix(filter, "=="):
		operator = filter[:2]
		filter = strings.TrimSpace(filter[2:])
	case strings.HasPrefix(filter, "<"), strings.HasPrefix(filter, ">"), strings.HasPrefix(filter, "="):
		operator = filter[:1]
		filter = strings.TrimSpace(filter[1:])
	default:
		return true
	}

	value, err := strconv.Atoi(filter)
	if err != nil {
		return true
	}

	switch operator {
	case "<":
		return level < value
	case "<=":
		return level <= value
	case ">":
		return level > value
	case ">=":
		return level >= value
	case "=", "==":
		return level == value
	default:
		return true
	}
}

func resolveBoardLiveScore(liveType string, baseScore, baseScoreAuto float64, skillSolo, skillAuto, skillMulti []float64, feverScore float64) (score float64, skillAccount float64) {
	switch liveType {
	case "auto":
		skillTotal := sumFloat64(skillAuto)
		score = baseScoreAuto + skillTotal
		if score > 0 {
			skillAccount = skillTotal / score
		}
		return score, skillAccount
	case "multi":
		skillTotal := sumFloat64(skillMulti) * 1.8
		score = baseScore + skillTotal + feverScore*0.5 + 0.01875
		if score > 0 {
			skillAccount = skillTotal / score
		}
		return score, skillAccount
	default:
		skillTotal := sumFloat64(skillSolo)
		score = baseScore + skillTotal
		if score > 0 {
			skillAccount = skillTotal / score
		}
		return score, skillAccount
	}
}

func boardTPS(tapCount, musicTime float64) float64 {
	if tapCount <= 0 || musicTime <= 0 {
		return 0
	}
	return tapCount / musicTime
}

func sortMusicBoardItems(items []drawing.MusicBoardItem, target string, ascend bool) {
	valueFor := func(item drawing.MusicBoardItem) float64 {
		switch target {
		case "pt":
			if item.LiveTypePt != nil {
				return *item.LiveTypePt
			}
			return 0
		case "pt/time":
			if item.LiveTypePtPerHour != nil {
				return *item.LiveTypePtPerHour
			}
			return 0
		case "tps":
			return item.Tps
		case "time":
			return item.MusicTime
		case "score":
			fallthrough
		default:
			if item.LiveTypeScore != nil {
				return *item.LiveTypeScore
			}
			return 0
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		left := valueFor(items[i])
		right := valueFor(items[j])
		if left == right {
			if items[i].MusicID == items[j].MusicID {
				return difficultyOrder(items[i].Difficulty) < difficultyOrder(items[j].Difficulty)
			}
			return items[i].MusicID < items[j].MusicID
		}
		if ascend {
			return left < right
		}
		return left > right
	})
}

func buildMusicBoardTitle(liveType, target string, query BoardQuery) string {
	liveTypeText := map[string]string{
		"solo":  "单人",
		"multi": "多人",
		"auto":  "自动",
	}[liveType]
	targetText := map[string]string{
		"score":   "分数",
		"pt":      "PT",
		"pt/time": "时速",
		"tps":     "每秒点击",
		"time":    "时长",
	}[target]
	orderText := "降序"
	if query.Ascend {
		orderText = "升序"
	}
	return fmt.Sprintf("歌曲排行 | %s | %s | %s", liveTypeText, targetText, orderText)
}

func buildMusicBoardDescription(query BoardQuery) string {
	parts := make([]string, 0, 4)
	if query.Power > 0 {
		parts = append(parts, fmt.Sprintf("综合力: %d", query.Power))
	}
	if query.DeckBonus > 0 {
		parts = append(parts, fmt.Sprintf("活动加成: %.1f%%", query.DeckBonus))
	}
	if query.PlayInterval > 0 {
		parts = append(parts, fmt.Sprintf("周回间隔: %.1fs", query.PlayInterval))
	}
	if strategy := strings.TrimSpace(query.SkillStrategy); strategy != "" {
		parts = append(parts, fmt.Sprintf("技能策略: %s", strategy))
	}
	return strings.Join(parts, " | ")
}

func sumFloat64(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total
}

func floatPtr(value float64) *float64 {
	v := value
	return &v
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
