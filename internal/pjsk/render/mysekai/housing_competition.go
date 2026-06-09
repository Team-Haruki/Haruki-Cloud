package mysekai

import (
	"context"
	"encoding/base64"
	stdjson "encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

const (
	DefaultHousingCompetitionSampleCount     = 1
	MaxHousingCompetitionSampleCount         = 10
	DefaultHousingCompetitionRefreshInterval = 10 * time.Second
	MaxHousingCompetitionRankCount           = 5
	HousingCompetitionNotice                 = "基于统计得出结果并不一定精确，仅供参考"
	housingCompetitionIdleCheckInterval      = time.Hour
)

type HousingCompetitionListClient interface {
	GetMySekaiHousingCompetitionList(server string, housingID int, isLottery bool) (stdjson.RawMessage, error)
}

type HousingCompetitionThumbnailClient interface {
	GetMySekaiHousingThumbnail(server, imagePath string) ([]byte, error)
}

type HousingCompetitionLineQuery struct {
	Region               string    `json:"region,omitempty"`
	HousingID            int       `json:"housing_id,omitempty"`
	Ranks                []int     `json:"ranks,omitempty"`
	SampleCount          int       `json:"sample_count,omitempty"`
	SampleIntervalMillis int       `json:"sample_interval_ms,omitempty"`
	Full                 bool      `json:"full,omitempty"`
	Now                  time.Time `json:"-"`
}

type HousingCompetitionInfo struct {
	ID                                 int
	Name                               string
	Description                        string
	SubmitStartAt                      int64
	ReviewStartAt                      int64
	SubmitEndAt                        int64
	AggregateAt                        int64
	BackgroundImageAssetbundleFileName string
	BannerImgPath                      string
	BackNumberAccentColorCode          string
}

type HousingCompetitionEntry struct {
	CacheKey      string
	Rank          int
	CompetitionID int
	OwnerUserID   int64
	OwnerUserName string
	EntryName     string
	EntryWord     string
	ThumbnailPath string
	SubmittedAt   int64
	ReviewCount   int
	TabType       string
	LastSeenAt    int64
}

type HousingCompetitionLineResult struct {
	Competition HousingCompetitionInfo
	Region      string
	Entries     []HousingCompetitionEntry
	AllEntries  []HousingCompetitionEntry
	Request     drawing.MysekaiHousingCompetitionRequest
	SampleCount int
	UniqueCount int
	SampledAt   time.Time
}

type housingCompetitionRefreshTarget struct {
	Competition HousingCompetitionInfo
	Active      bool
	NextStartAt int64
}

func (c *Controller) BuildHousingCompetitionLine(ctx context.Context, api HousingCompetitionListClient, query HousingCompetitionLineQuery) (*HousingCompetitionLineResult, error) {
	if c == nil {
		return nil, fmt.Errorf("mysekai controller is not initialized")
	}
	if api == nil {
		return nil, fmt.Errorf("sekai api client is not configured")
	}
	if ctx == nil {
		ctx = context.TODO()
	}

	region := renderregion.WithDefault(renderregion.Normalize(query.Region))
	controller := c.withRegion(region.String())
	if err := controller.ensureMasterdata(); err != nil {
		return nil, err
	}
	controller.syncHousingCompetitionBannersFromMasterdata()

	competition, err := controller.resolveHousingCompetition(query)
	if err != nil {
		return nil, err
	}
	ranks, err := NormalizeHousingCompetitionRanks(query.Ranks)
	if err != nil {
		return nil, err
	}
	sampleCount := normalizeHousingCompetitionSampleCount(query.SampleCount)
	allEntries, sampledAt, refreshedCount, err := controller.loadHousingCompetitionStats(ctx, api, region.String(), competition.ID, sampleCount, query.SampleIntervalMillis)
	if err != nil {
		return nil, err
	}
	if len(allEntries) == 0 {
		return nil, fmt.Errorf("没有采样到可用的百景投稿")
	}

	sortHousingCompetitionEntries(allEntries)
	for i := range allEntries {
		allEntries[i].Rank = i + 1
	}

	selected := make([]HousingCompetitionEntry, 0, len(ranks))
	requestEntries := make([]drawing.MysekaiHousingCompetitionEntry, 0, len(ranks))
	for _, rank := range ranks {
		if rank > len(allEntries) {
			return nil, fmt.Errorf("只采样到%d个唯一百景投稿，无法查询第%d名", len(allEntries), rank)
		}
		entry := allEntries[rank-1]
		selected = append(selected, entry)
		requestEntries = append(requestEntries, c.housingCompetitionDrawingEntry(api, region.String(), allEntries, rank-1))
	}

	request := drawing.MysekaiHousingCompetitionRequest{
		CompetitionID:     competition.ID,
		Region:            region.String(),
		Name:              strings.TrimSpace("烤森百景 " + competition.Name),
		Description:       drawing.StringPtr(HousingCompetitionNotice),
		BannerImagePath:   stringPtrIfNotEmpty(competition.BannerImgPath),
		BannerImageBase64: controller.housingCompetitionBannerBase64(competition),
		SampleCount:       refreshedCount,
		UniqueCount:       len(allEntries),
		SampledAt:         sampledAt.UnixMilli(),
		Entries:           requestEntries,
	}

	return &HousingCompetitionLineResult{
		Competition: competition,
		Region:      region.String(),
		Entries:     selected,
		AllEntries:  allEntries,
		Request:     request,
		SampleCount: refreshedCount,
		UniqueCount: len(allEntries),
		SampledAt:   sampledAt,
	}, nil
}

func (c *Controller) loadHousingCompetitionStats(ctx context.Context, api HousingCompetitionListClient, region string, housingID, sampleCount, sampleIntervalMillis int) ([]HousingCompetitionEntry, time.Time, int, error) {
	if c.housingCompetitionStats != nil {
		return c.housingCompetitionStats.GetOrRefresh(ctx, api, region, housingID, sampleCount)
	}
	return fetchHousingCompetitionSamples(ctx, api, region, housingID, sampleCount, sampleIntervalMillis)
}

func (c *Controller) RefreshHousingCompetitionStats(ctx context.Context, api HousingCompetitionListClient, query HousingCompetitionLineQuery) error {
	if c == nil || api == nil || c.housingCompetitionStats == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.TODO()
	}
	region := renderregion.WithDefault(renderregion.Normalize(query.Region))
	controller := c.withRegion(region.String())
	if err := controller.ensureMasterdata(); err != nil {
		return err
	}
	controller.syncHousingCompetitionBannersFromMasterdata()
	competition, err := controller.resolveHousingCompetition(query)
	if err != nil {
		return err
	}
	_, _, _, err = controller.housingCompetitionStats.Refresh(ctx, api, region.String(), competition.ID, 1)
	return err
}

func (c *Controller) StartHousingCompetitionStatsRefresh(ctx context.Context, api HousingCompetitionListClient, region string) {
	if c == nil || c.housingCompetitionStats == nil || api == nil {
		return
	}
	if ctx == nil {
		ctx = context.TODO()
	}
	interval := c.housingCompetitionStats.RefreshInterval()
	if interval <= 0 {
		interval = DefaultHousingCompetitionRefreshInterval
	}
	region = renderregion.WithDefault(renderregion.Normalize(region)).String()
	go func() {
		for {
			if err := ctx.Err(); err != nil {
				return
			}

			query := HousingCompetitionLineQuery{Region: region, Now: time.Now()}
			controller := c.withRegion(region)
			if err := controller.ensureMasterdata(); err != nil {
				if waitHousingCompetitionSampleInterval(ctx, housingCompetitionIdleCheckInterval) != nil {
					return
				}
				continue
			}
			controller.syncHousingCompetitionBannersFromMasterdata()

			target, err := controller.resolveHousingCompetitionRefreshTarget(query)
			if err != nil {
				if waitHousingCompetitionSampleInterval(ctx, housingCompetitionIdleCheckInterval) != nil {
					return
				}
				continue
			}
			if !target.Active {
				wait := target.waitDuration(time.Now(), housingCompetitionIdleCheckInterval)
				if wait <= 0 {
					continue
				}
				if waitHousingCompetitionSampleInterval(ctx, wait) != nil {
					return
				}
				continue
			}

			_, _, _, _ = controller.housingCompetitionStats.Refresh(ctx, api, region, target.Competition.ID, 1)
			wait := target.activeRefreshWait(time.Now(), interval)
			if wait <= 0 {
				continue
			}
			if waitHousingCompetitionSampleInterval(ctx, wait) != nil {
				return
			}
		}
	}()
}

func (c *Controller) RenderHousingCompetitionLine(result *HousingCompetitionLineResult) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	if result == nil {
		return nil, fmt.Errorf("百景榜数据为空")
	}
	return c.drawing.GenerateMysekaiHousingCompetition(&result.Request)
}

func (c *Controller) housingCompetitionDrawingEntry(api HousingCompetitionListClient, region string, entries []HousingCompetitionEntry, index int) drawing.MysekaiHousingCompetitionEntry {
	entry := entries[index]
	out := drawing.MysekaiHousingCompetitionEntry{
		Rank:                 entry.Rank,
		ReviewCount:          entry.ReviewCount,
		OwnerUserName:        entry.OwnerUserName,
		Name:                 entry.EntryName,
		Word:                 entry.EntryWord,
		ThumbnailPath:        stringPtrIfNotEmpty(entry.ThumbnailPath),
		ThumbnailImageBase64: housingCompetitionThumbnailBase64(api, region, entry.ThumbnailPath),
		SubmittedAt:          entry.SubmittedAt,
	}
	if index > 0 {
		score := entries[index-1].ReviewCount
		delta := score - entry.ReviewCount
		out.PreviousReviewCount = drawing.IntPtr(score)
		out.PreviousDelta = drawing.IntPtr(delta)
	}
	if index+1 < len(entries) {
		score := entries[index+1].ReviewCount
		delta := entry.ReviewCount - score
		out.NextReviewCount = drawing.IntPtr(score)
		out.NextDelta = drawing.IntPtr(delta)
	}
	return out
}

func housingCompetitionThumbnailBase64(api HousingCompetitionListClient, region, imagePath string) *string {
	imagePath = strings.TrimSpace(imagePath)
	if imagePath == "" {
		return nil
	}
	thumbClient, ok := api.(HousingCompetitionThumbnailClient)
	if !ok {
		return nil
	}
	raw, err := thumbClient.GetMySekaiHousingThumbnail(region, imagePath)
	if err != nil || len(raw) == 0 {
		return nil
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	return &encoded
}

func stringPtrIfNotEmpty(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func (c *Controller) resolveHousingCompetition(query HousingCompetitionLineQuery) (HousingCompetitionInfo, error) {
	items := c.masterdata.loadList("mysekaiHousingCompetitions.json")
	if len(items) == 0 {
		return HousingCompetitionInfo{}, fmt.Errorf("mysekaiHousingCompetitions masterdata is not available")
	}
	if query.HousingID > 0 {
		for _, item := range items {
			info := c.housingCompetitionInfoFromMasterdata(item)
			if info.ID == query.HousingID {
				return info, nil
			}
		}
		return HousingCompetitionInfo{}, fmt.Errorf("没有找到百景 housing_id=%d", query.HousingID)
	}

	target, err := c.resolveHousingCompetitionRefreshTarget(query)
	if err != nil {
		return HousingCompetitionInfo{}, err
	}
	if !target.Active {
		return HousingCompetitionInfo{}, fmt.Errorf("当前没有正在进行的烤森百景活动")
	}
	return target.Competition, nil
}

func (c *Controller) resolveHousingCompetitionRefreshTarget(query HousingCompetitionLineQuery) (housingCompetitionRefreshTarget, error) {
	items := c.masterdata.loadList("mysekaiHousingCompetitions.json")
	if len(items) == 0 {
		return housingCompetitionRefreshTarget{}, fmt.Errorf("mysekaiHousingCompetitions masterdata is not available")
	}

	now := query.Now
	if now.IsZero() {
		now = time.Now()
	}
	nowMs := now.UnixMilli()
	var target housingCompetitionRefreshTarget
	for _, item := range items {
		info := c.housingCompetitionInfoFromMasterdata(item)
		if info.ID == 0 {
			continue
		}
		startAt := housingCompetitionListStartAt(info)
		if startAt <= 0 || info.AggregateAt <= 0 {
			continue
		}
		if nowMs >= startAt && nowMs < info.AggregateAt {
			if target.Competition.ID == 0 || startAt > housingCompetitionListStartAt(target.Competition) || (startAt == housingCompetitionListStartAt(target.Competition) && info.ID > target.Competition.ID) {
				target.Competition = info
				target.Active = true
			}
			continue
		}
		if startAt > nowMs && (target.NextStartAt == 0 || startAt < target.NextStartAt) {
			target.NextStartAt = startAt
		}
	}
	return target, nil
}

func housingCompetitionListStartAt(info HousingCompetitionInfo) int64 {
	if info.ReviewStartAt > 0 {
		return info.ReviewStartAt
	}
	return info.SubmitStartAt
}

func (t housingCompetitionRefreshTarget) waitDuration(now time.Time, fallback time.Duration) time.Duration {
	if fallback <= 0 {
		fallback = housingCompetitionIdleCheckInterval
	}
	if t.NextStartAt <= 0 {
		return fallback
	}
	wait := time.UnixMilli(t.NextStartAt).Sub(now)
	if wait <= 0 {
		return 0
	}
	if wait > fallback {
		return fallback
	}
	return wait
}

func (t housingCompetitionRefreshTarget) activeRefreshWait(now time.Time, interval time.Duration) time.Duration {
	if interval <= 0 {
		interval = DefaultHousingCompetitionRefreshInterval
	}
	if t.Competition.AggregateAt <= 0 {
		return interval
	}
	untilEnd := time.UnixMilli(t.Competition.AggregateAt).Sub(now)
	if untilEnd <= 0 {
		return 0
	}
	if untilEnd < interval {
		return untilEnd
	}
	return interval
}

func (c *Controller) housingCompetitionInfoFromMasterdata(item map[string]any) HousingCompetitionInfo {
	info := HousingCompetitionInfo{
		ID:                                 intNumberFrom(item, 0, "id"),
		Name:                               stringValueFrom(item, "name"),
		Description:                        stringValueFrom(item, "description"),
		SubmitStartAt:                      int64Number(item["submitStartAt"], 0),
		ReviewStartAt:                      int64Number(item["reviewStartAt"], 0),
		SubmitEndAt:                        int64Number(item["submitEndAt"], 0),
		AggregateAt:                        int64Number(item["aggregateAt"], 0),
		BackgroundImageAssetbundleFileName: stringValueFrom(item, "backgroundImageAssetbundleFileName"),
		BackNumberAccentColorCode:          stringValueFrom(item, "backNumberAccentColorCode"),
	}
	if info.Name == "" {
		info.Name = fmt.Sprintf("第%d期", info.ID)
	}
	if info.BackgroundImageAssetbundleFileName != "" {
		info.BannerImgPath = c.regionPath(
			c.defaultRegion,
			fmt.Sprintf(
				"mysekai/effect/ui_anim/mysekai_housing_competition/lottery_result/%s.png",
				info.BackgroundImageAssetbundleFileName,
			),
		)
	}
	if info.BannerImgPath == "" {
		info.BannerImgPath = c.staticPath("unknown.jpg")
	}
	return info
}

func NormalizeHousingCompetitionRanks(ranks []int) ([]int, error) {
	if len(ranks) == 0 {
		return []int{1, 2, 3, 4, 5}, nil
	}
	seen := make(map[int]struct{}, len(ranks))
	out := make([]int, 0, len(ranks))
	for _, rank := range ranks {
		if rank <= 0 {
			return nil, fmt.Errorf("百景排名必须大于0")
		}
		if _, ok := seen[rank]; ok {
			continue
		}
		seen[rank] = struct{}{}
		out = append(out, rank)
	}
	sort.Ints(out)
	if len(out) > MaxHousingCompetitionRankCount {
		return nil, fmt.Errorf("一次最多查询%d个百景排名", MaxHousingCompetitionRankCount)
	}
	return out, nil
}

func parseHousingCompetitionEntries(raw stdjson.RawMessage) ([]HousingCompetitionEntry, int64, error) {
	var root any
	if err := decodeJSONUseNumber(raw, &root); err != nil {
		return nil, 0, fmt.Errorf("解析百景投稿列表失败: %w", err)
	}

	var lotteryAt int64
	var rawItems []any
	switch data := root.(type) {
	case map[string]any:
		lotteryAt = int64Number(data["lotteryAt"], 0)
		rawItems = nestedList(data, "results")
	case []any:
		rawItems = data
	default:
		return nil, 0, fmt.Errorf("百景投稿列表格式错误")
	}

	entries := make([]HousingCompetitionEntry, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		if displayable, ok := item["isDisplayable"]; ok && !boolValue(displayable) {
			continue
		}
		entry := HousingCompetitionEntry{
			CompetitionID: intNumberFrom(item, 0, "mysekaiHousingCompetitionId", "mysekai_housing_competition_id"),
			OwnerUserID:   int64Number(item["mysekaiOwnerUserId"], 0),
			OwnerUserName: stringValueFrom(item, "mysekaiOwnerUserName", "mysekai_owner_user_name"),
			EntryName:     stringValueFrom(item, "userMysekaiHousingCompetitionName", "user_mysekai_housing_competition_name"),
			EntryWord:     stringValueFrom(item, "userMysekaiHousingCompetitionWord", "user_mysekai_housing_competition_word"),
			ThumbnailPath: stringValueFrom(item, "thumbnailPath", "thumbnail_path"),
			SubmittedAt:   int64Number(item["submittedAt"], 0),
			ReviewCount:   intNumberFrom(item, 0, "reviewCount", "review_count"),
			TabType:       stringValueFrom(item, "mysekaiHousingCompetitionTabType", "mysekai_housing_competition_tab_type"),
			LastSeenAt:    lotteryAt,
		}
		entries = append(entries, entry)
	}
	return entries, lotteryAt, nil
}

func normalizeHousingCompetitionSampleCount(count int) int {
	if count <= 0 {
		return DefaultHousingCompetitionSampleCount
	}
	if count > MaxHousingCompetitionSampleCount {
		return MaxHousingCompetitionSampleCount
	}
	return count
}

func sortHousingCompetitionEntries(entries []HousingCompetitionEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].ReviewCount != entries[j].ReviewCount {
			return entries[i].ReviewCount > entries[j].ReviewCount
		}
		if entries[i].SubmittedAt != entries[j].SubmittedAt {
			return entries[i].SubmittedAt < entries[j].SubmittedAt
		}
		leftKey := entries[i].uniqueKey()
		rightKey := entries[j].uniqueKey()
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		return entries[i].EntryName < entries[j].EntryName
	})
}

func (e HousingCompetitionEntry) uniqueKey() string {
	if e.CacheKey != "" {
		return e.CacheKey
	}
	return housingCompetitionEntryCacheKey(e)
}
