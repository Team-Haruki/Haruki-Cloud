package handler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	eventdb "haruki-cloud/database/sekai/event"
	"haruki-cloud/database/sekai/resourceboxe"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderprovider "haruki-cloud/internal/pjsk/render/provider"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

type eventMasterEntry = map[string]any

// buildEventRecordFromSnapshot constructs an EventRecordRequest from live
// Toolbox suite data, cross-referencing with master data for event metadata.
// Regular events come from userEvents; world bloom events come from userWorldBlooms.
func buildEventRecordFromSnapshot(rc *RequestContext, region renderregion.Value) (*drawing.EventRecordRequest, error) {
	builder := eventRecordBuilder{rc: rc, region: region}
	return builder.build()
}

type eventRecordBuilder struct {
	rc                     *RequestContext
	region                 renderregion.Value
	assetHelper            *assets.AssetHelper
	snapshot               rendersnapshot.Snapshot
	rawData                *rendersnapshot.RawUserData
	rankByEvent            map[int]int
	eventMaster            map[int]eventMasterEntry
	eventRankBoundaries    map[int]map[int]struct{}
	eventHonorTiers        map[int]map[int]int
	worldBloomEventIDs     map[int]struct{}
	rankDisplayByEvent     map[int]eventRecordHonorRankDisplay
	wlRankDisplayByChapter map[eventRecordWorldBloomChapterKey]eventRecordHonorRankDisplay
	regionString           string
	eventInfo              []drawing.EventHistory
	eventInfoByID          map[int]int
	wlEventInfo            []drawing.EventHistory
	wlEventInfoByKey       map[eventRecordWorldBloomChapterKey]int
}

func (b *eventRecordBuilder) build() (*drawing.EventRecordRequest, error) {
	if b.rc == nil || b.rc.App == nil {
		return nil, fmt.Errorf("event record runtime is unavailable")
	}
	b.assetHelper = b.rc.App.Assets.WithContext(b.rc.Ctx)
	if err := b.loadSnapshot(); err != nil {
		return nil, err
	}
	if err := b.loadMasterData(); err != nil {
		return nil, err
	}
	b.prepareRankDisplays()
	b.buildRegularEntries()
	b.buildWorldBloomEntries()
	if len(b.eventInfo) == 0 && len(b.wlEventInfo) == 0 {
		return nil, fmt.Errorf("event record requires at least one history entry")
	}
	return b.finishRequest()
}

func (b *eventRecordBuilder) loadSnapshot() error {
	rc := b.rc
	if rc.App.Bindings != nil && strings.TrimSpace(rc.Platform) != "" && strings.TrimSpace(rc.PlatformUserID) != "" {
		binding, snapshot, err := rc.requireVisibleSuiteSnapshot()
		if err != nil {
			return err
		}
		if snapshot == nil {
			return newSuiteDataNotFoundReplayErrorForBinding(binding)
		}
		b.snapshot = snapshot
	} else {
		b.snapshot = rc.ResolveSnapshot(false)
		if b.snapshot == nil {
			return fmt.Errorf("event record requires user data (suite snapshot unavailable)")
		}
	}
	b.rawData = b.snapshot.RawData()
	if b.rawData == nil || !hasEventRecordHistorySource(b.rawData) {
		return fmt.Errorf("event record requires at least one history entry")
	}
	b.rankByEvent = eventRecordRanks(b.rawData)
	return nil
}

func eventRecordRanks(rawData *rendersnapshot.RawUserData) map[int]int {
	ranks := make(map[int]int, len(rawData.UserEventResults))
	for _, result := range rawData.UserEventResults {
		ranks[result.EventID] = result.Rank
	}
	for _, userEvent := range rawData.UserEvents {
		if userEvent.EventID <= 0 || userEvent.Rank <= 0 {
			continue
		}
		if _, exists := ranks[userEvent.EventID]; !exists {
			ranks[userEvent.EventID] = userEvent.Rank
		}
	}
	return ranks
}

func (b *eventRecordBuilder) loadMasterData() error {
	eventEntities, err := b.rc.App.Sekai.Event.Query().
		Where(eventdb.ServerRegionEQ(b.region.String())).
		All(b.rc.Ctx)
	if err != nil || len(eventEntities) == 0 {
		return fmt.Errorf("event master data not available")
	}
	b.eventMaster = make(map[int]eventMasterEntry, len(eventEntities))
	b.eventRankBoundaries = make(map[int]map[int]struct{}, len(eventEntities))
	b.eventHonorTiers = make(map[int]map[int]int, len(eventEntities))
	b.worldBloomEventIDs = make(map[int]struct{})
	resourceBoxHonorIDs := eventRecordResourceBoxHonorIDs(b.rc, b.region)
	for _, eventInfo := range eventEntities {
		b.addMasterEvent(eventInfo.GameID, eventInfo.EventType, eventInfo.AssetbundleName, eventInfo.Name, eventInfo.StartAt, eventInfo.AggregateAt, eventInfo.RankingAnnounceAt, eventInfo.ClosedAt, eventInfo.EventRankingRewardRanges, resourceBoxHonorIDs)
	}
	return nil
}

func (b *eventRecordBuilder) addMasterEvent(gameID int64, eventType string, assetbundleName string, name string, startAt int64, aggregateAt int64, rankingAnnounceAt int64, closedAt int64, rewardRanges json.RawMessage, resourceBoxHonorIDs map[int][]int) {
	id := int(gameID)
	b.eventMaster[id] = eventMasterEntry{
		"id": id, "eventType": eventType, "assetbundleName": assetbundleName, "name": name,
		"startAt": float64(startAt), "aggregateAt": float64(aggregateAt),
		"rankingAnnounceAt": float64(rankingAnnounceAt), "closedAt": float64(closedAt),
	}
	if boundaries := eventRecordRankBoundaries(rewardRanges); len(boundaries) > 0 {
		b.eventRankBoundaries[id] = boundaries
	}
	if tiers := eventRecordHonorTiers(rewardRanges, resourceBoxHonorIDs); len(tiers) > 0 {
		b.eventHonorTiers[id] = tiers
	}
	if eventType == "world_bloom" {
		b.worldBloomEventIDs[id] = struct{}{}
	}
}

func (b *eventRecordBuilder) prepareRankDisplays() {
	dropUntrustedEventRecordSnapshotRanks(b.region, b.rankByEvent, b.eventRankBoundaries)
	b.rankDisplayByEvent = buildEventRecordHonorRankDisplays(b.rawData, b.eventMaster, b.eventHonorTiers)
	b.wlRankDisplayByChapter = buildEventRecordWorldBloomChapterHonorRankDisplays(b.rc, b.region, b.rawData, b.eventMaster)
	b.regionString = regionWithDefault(b.region.String())
}

func (b *eventRecordBuilder) buildRegularEntries() {
	b.eventInfo = make([]drawing.EventHistory, 0)
	b.eventInfoByID = make(map[int]int)
	b.appendUserEventEntries()
	b.appendHonorDisplayEntries()
	b.appendRankOnlyEntries()
}

func (b *eventRecordBuilder) appendUserEventEntries() {
	for _, userEvent := range b.rawData.UserEvents {
		history := b.newEventHistory(userEvent.EventID, intPtr(userEvent.EventPoint))
		if history == nil {
			continue
		}
		b.applyEventDisplayAndRank(history, userEvent.EventID)
		b.appendEventHistory(userEvent.EventID, history)
	}
}

func (b *eventRecordBuilder) appendHonorDisplayEntries() {
	for eventID := range b.rankDisplayByEvent {
		if _, exists := b.eventInfoByID[eventID]; exists {
			continue
		}
		history := b.newEventHistory(eventID, nil)
		if history == nil {
			continue
		}
		b.applyEventDisplayAndRank(history, eventID)
		b.appendEventHistory(eventID, history)
	}
}

func (b *eventRecordBuilder) appendRankOnlyEntries() {
	for eventID, rank := range b.rankByEvent {
		if _, exists := b.eventInfoByID[eventID]; exists {
			continue
		}
		history := b.newEventHistory(eventID, nil)
		if history == nil {
			continue
		}
		history.Rank = intPtr(rank)
		b.appendEventHistory(eventID, history)
	}
}

func (b *eventRecordBuilder) newEventHistory(eventID int, points *int) *drawing.EventHistory {
	history := buildEventHistoryFromMaster(b.eventMaster[eventID], eventID, points, b.assetHelper, b.regionString)
	if history != nil {
		_, history.IsWlEvent = b.worldBloomEventIDs[eventID]
	}
	return history
}

func (b *eventRecordBuilder) applyEventDisplayAndRank(history *drawing.EventHistory, eventID int) {
	if display, ok := b.rankDisplayByEvent[eventID]; ok {
		history.RankDisplay = stringPtr(display.text)
		history.RankTier = intPtr(display.tier)
	}
	if rank, ok := b.rankByEvent[eventID]; ok {
		history.Rank = intPtr(rank)
		history.RankDisplay = nil
		history.RankTier = nil
	}
}

func (b *eventRecordBuilder) appendEventHistory(eventID int, history *drawing.EventHistory) {
	b.eventInfo = append(b.eventInfo, *history)
	b.eventInfoByID[eventID] = len(b.eventInfo) - 1
}

func (b *eventRecordBuilder) buildWorldBloomEntries() {
	b.wlEventInfo = make([]drawing.EventHistory, 0, len(b.rawData.UserWorldBlooms))
	b.wlEventInfoByKey = make(map[eventRecordWorldBloomChapterKey]int)
	b.appendUserWorldBloomEntries()
	b.appendWorldBloomDisplayEntries()
}

func (b *eventRecordBuilder) appendUserWorldBloomEntries() {
	for _, worldBloom := range b.rawData.UserWorldBlooms {
		history := b.newWorldBloomHistory(worldBloom.EventID, worldBloom.GameCharacterID, intPtr(worldBloom.WorldBloomChapterPoint))
		if history == nil {
			continue
		}
		key := eventRecordWorldBloomChapterKey{eventID: worldBloom.EventID, gameCharacterID: worldBloom.GameCharacterID}
		b.applyWorldBloomDisplay(history, key)
		if worldBloom.Rank > 0 {
			history.Rank = intPtr(worldBloom.Rank)
			history.RankDisplay = nil
			history.RankTier = nil
		}
		b.appendWorldBloomHistory(key, history)
	}
}

func (b *eventRecordBuilder) appendWorldBloomDisplayEntries() {
	for key := range b.wlRankDisplayByChapter {
		if _, exists := b.wlEventInfoByKey[key]; exists {
			continue
		}
		history := b.newWorldBloomHistory(key.eventID, key.gameCharacterID, nil)
		if history == nil {
			continue
		}
		b.applyWorldBloomDisplay(history, key)
		b.appendWorldBloomHistory(key, history)
	}
}

func (b *eventRecordBuilder) newWorldBloomHistory(eventID int, gameCharacterID int, points *int) *drawing.EventHistory {
	history := buildEventHistoryFromMaster(b.eventMaster[eventID], eventID, points, b.assetHelper, b.regionString)
	if history == nil {
		return nil
	}
	history.IsWlEvent = true
	if gameCharacterID > 0 {
		history.WlCharaIconPath = stringPtr(assets.ResolveRegionAssetPath(
			b.assetHelper, b.regionString, fmt.Sprintf("character/character_sd_l/chr_sp_%d.png", gameCharacterID),
		))
	}
	return history
}

func (b *eventRecordBuilder) applyWorldBloomDisplay(history *drawing.EventHistory, key eventRecordWorldBloomChapterKey) {
	if display, ok := b.wlRankDisplayByChapter[key]; ok {
		history.RankDisplay = stringPtr(display.text)
		history.RankTier = intPtr(display.tier)
	}
}

func (b *eventRecordBuilder) appendWorldBloomHistory(key eventRecordWorldBloomChapterKey, history *drawing.EventHistory) {
	b.wlEventInfo = append(b.wlEventInfo, *history)
	b.wlEventInfoByKey[key] = len(b.wlEventInfo) - 1
}

func (b *eventRecordBuilder) finishRequest() (*drawing.EventRecordRequest, error) {
	sortEventHistory(b.eventInfo)
	sortEventHistory(b.wlEventInfo)
	profile, _ := resolveCommandDisplayProfiles(b.rc, b.snapshot)
	if profile == nil {
		return nil, fmt.Errorf("event record requires user profile data")
	}
	return &drawing.EventRecordRequest{
		EventInfo: b.eventInfo, WlEventInfo: b.wlEventInfo, UserInfo: *profile, RankNote: eventRecordRankNote(b.region),
	}, nil
}

func hasEventRecordHistorySource(rawData *rendersnapshot.RawUserData) bool {
	if rawData == nil {
		return false
	}
	return len(rawData.UserEvents) > 0 ||
		len(rawData.UserWorldBlooms) > 0 ||
		len(rawData.UserEventResults) > 0 ||
		len(rawData.UserHonors) > 0 ||
		len(rawData.UserProfileHonors) > 0
}

type eventRecordRankingRewardRange struct {
	FromRank                   int                              `json:"fromRank"`
	ToRank                     int                              `json:"toRank"`
	EventRankingRewards        []eventRecordRankingReward       `json:"eventRankingRewards"`
	EventRankingRewards2       []eventRecordRankingReward       `json:"event_ranking_rewards"`
	EventRankingRewardDetails  []eventRecordRankingRewardDetail `json:"eventRankingRewardDetails"`
	EventRankingRewardDetails2 []eventRecordRankingRewardDetail `json:"event_ranking_reward_details"`
}

type eventRecordRankingReward struct {
	ResourceBoxID  int `json:"resourceBoxId"`
	ResourceBoxID2 int `json:"resource_box_id"`
}

type eventRecordRankingRewardDetail struct {
	ResourceType  string `json:"resourceType"`
	ResourceType2 string `json:"resource_type"`
	ResourceID    int    `json:"resourceId"`
	ResourceID2   int    `json:"resource_id"`
}

func eventRecordRankBoundaries(raw json.RawMessage) map[int]struct{} {
	if len(raw) == 0 {
		return nil
	}
	var ranges []eventRecordRankingRewardRange
	if err := json.Unmarshal(raw, &ranges); err != nil {
		return nil
	}
	boundaries := make(map[int]struct{}, len(ranges)*2)
	for _, item := range ranges {
		if item.FromRank > 0 {
			boundaries[item.FromRank] = struct{}{}
			if item.FromRank > 1 {
				boundaries[item.FromRank-1] = struct{}{}
			}
		}
		if item.ToRank > 0 {
			boundaries[item.ToRank] = struct{}{}
		}
	}
	return boundaries
}

type eventRecordHonorRankDisplay struct {
	text string
	tier int
}

func eventRecordHonorTiers(raw json.RawMessage, resourceBoxHonorIDs map[int][]int) map[int]int {
	if len(raw) == 0 {
		return nil
	}
	var ranges []eventRecordRankingRewardRange
	if err := json.Unmarshal(raw, &ranges); err != nil {
		return nil
	}
	tiers := make(map[int]int)
	for _, item := range ranges {
		if item.ToRank <= 0 {
			continue
		}
		for _, honorID := range eventRecordHonorIDsFromRewardRange(item, resourceBoxHonorIDs) {
			if honorID <= 0 {
				continue
			}
			if current, ok := tiers[honorID]; !ok || item.ToRank < current {
				tiers[honorID] = item.ToRank
			}
		}
	}
	return tiers
}

func eventRecordHonorIDsFromRewardRange(item eventRecordRankingRewardRange, resourceBoxHonorIDs map[int][]int) []int {
	var ids []int
	for _, detail := range append(item.EventRankingRewardDetails, item.EventRankingRewardDetails2...) {
		if eventRecordDetailResourceType(detail) == "honor" && eventRecordDetailResourceID(detail) > 0 {
			ids = append(ids, eventRecordDetailResourceID(detail))
		}
	}
	for _, reward := range append(item.EventRankingRewards, item.EventRankingRewards2...) {
		boxID := reward.ResourceBoxID
		if boxID <= 0 {
			boxID = reward.ResourceBoxID2
		}
		if boxID <= 0 {
			continue
		}
		ids = append(ids, resourceBoxHonorIDs[boxID]...)
	}
	return ids
}

func eventRecordResourceBoxHonorIDs(rc *RequestContext, region renderregion.Value) map[int][]int {
	return eventRecordResourceBoxHonorIDsForPurpose(rc, region, "event_ranking_reward")
}

func eventRecordWorldBloomResourceBoxHonorIDs(rc *RequestContext, region renderregion.Value) map[int][]int {
	return eventRecordResourceBoxHonorIDsForPurpose(rc, region, "world_bloom_chapter_ranking_reward")
}

func eventRecordResourceBoxHonorIDsForPurpose(rc *RequestContext, region renderregion.Value, purpose string) map[int][]int {
	if rc == nil || rc.App == nil {
		return nil
	}
	if ids := eventRecordResourceBoxHonorIDsFromProvider(rc, region, purpose); len(ids) > 0 {
		return ids
	}
	if rc.App.Sekai == nil || rc.Ctx == nil {
		return nil
	}

	items, err := rc.App.Sekai.Resourceboxe.Query().
		Where(
			resourceboxe.ServerRegionEQ(region.String()),
			resourceboxe.ResourceBoxPurposeEQ(purpose),
		).
		All(rc.Ctx)
	if err != nil {
		return nil
	}

	out := make(map[int][]int)
	for _, item := range items {
		if item == nil || item.GameID <= 0 || len(item.Details) == 0 {
			continue
		}
		var details []eventRecordRankingRewardDetail
		if err := json.Unmarshal(item.Details, &details); err != nil {
			continue
		}
		eventRecordAddHonorDetails(out, int(item.GameID), details)
	}
	return out
}

func eventRecordResourceBoxHonorIDsFromProvider(rc *RequestContext, region renderregion.Value, purpose string) map[int][]int {
	source := rc.App.ProviderForRegion(region)
	if source == nil || rc.Ctx == nil {
		return nil
	}
	education := source.Education()
	if education == nil {
		return nil
	}
	boxes := education.GetResourceBoxesByPurpose(rc.Ctx, purpose)
	out := make(map[int][]int)
	for _, box := range boxes {
		if box == nil || box.ID <= 0 {
			continue
		}
		details := make([]eventRecordRankingRewardDetail, 0, len(box.Details))
		for _, detail := range box.Details {
			details = append(details, eventRecordRankingRewardDetail{
				ResourceType: detail.ResourceType,
				ResourceID:   detail.ResourceID,
			})
		}
		eventRecordAddHonorDetails(out, box.ID, details)
	}
	return out
}

type eventRecordWorldBloomChapterKey struct {
	eventID         int
	gameCharacterID int
}

func buildEventRecordWorldBloomChapterHonorRankDisplays(rc *RequestContext, region renderregion.Value, rawData *rendersnapshot.RawUserData, eventMaster map[int]eventMasterEntry) map[eventRecordWorldBloomChapterKey]eventRecordHonorRankDisplay {
	if rc == nil || rc.App == nil || rc.Ctx == nil || rawData == nil || len(eventMaster) == 0 {
		return nil
	}
	userHonorIDs := collectEventRecordUserHonorIDs(rawData)
	if len(userHonorIDs) == 0 {
		return nil
	}

	source := rc.App.ProviderForRegion(region)
	if source == nil || source.Events() == nil {
		return nil
	}
	resourceBoxHonorIDs := eventRecordWorldBloomResourceBoxHonorIDs(rc, region)
	if len(resourceBoxHonorIDs) == 0 {
		return nil
	}
	now := eventRecordSnapshotNow(rawData)
	out := make(map[eventRecordWorldBloomChapterKey]eventRecordHonorRankDisplay)
	for eventID, master := range eventMaster {
		if stringVal(master, "eventType") != "world_bloom" {
			continue
		}
		appendEventRecordWorldBloomDisplays(out, rc, source.Events(), eventID, now, userHonorIDs, resourceBoxHonorIDs)
	}
	return out
}

func eventRecordSnapshotNow(rawData *rendersnapshot.RawUserData) int64 {
	if rawData.Now > 0 {
		return rawData.Now
	}
	return time.Now().UnixMilli()
}

func appendEventRecordWorldBloomDisplays(out map[eventRecordWorldBloomChapterKey]eventRecordHonorRankDisplay, rc *RequestContext, events renderprovider.EventProvider, eventID int, now int64, userHonorIDs map[int]struct{}, resourceBoxHonorIDs map[int][]int) {
	for _, chapter := range events.GetWorldBloomChapters(rc.Ctx, eventID) {
		key, display, ok := eventRecordWorldBloomChapterDisplay(rc, events, eventID, chapter, now, userHonorIDs, resourceBoxHonorIDs)
		if ok {
			out[key] = display
		}
	}
}

func eventRecordWorldBloomChapterDisplay(rc *RequestContext, events renderprovider.EventProvider, eventID int, chapter *masterdata.WorldBloom, now int64, userHonorIDs map[int]struct{}, resourceBoxHonorIDs map[int][]int) (eventRecordWorldBloomChapterKey, eventRecordHonorRankDisplay, bool) {
	if chapter == nil || chapter.GameCharacterID == nil || *chapter.GameCharacterID <= 0 || !eventRecordWorldBloomChapterClosed(chapter, now) {
		return eventRecordWorldBloomChapterKey{}, eventRecordHonorRankDisplay{}, false
	}
	gameCharacterID := *chapter.GameCharacterID
	ranges, err := events.GetWorldBloomChapterRankingRewardRanges(rc.Ctx, eventID, gameCharacterID)
	if err != nil || len(ranges) == 0 {
		return eventRecordWorldBloomChapterKey{}, eventRecordHonorRankDisplay{}, false
	}
	bestTier := eventRecordWorldBloomBestHonorTier(ranges, userHonorIDs, resourceBoxHonorIDs)
	if bestTier <= 0 {
		return eventRecordWorldBloomChapterKey{}, eventRecordHonorRankDisplay{}, false
	}
	key := eventRecordWorldBloomChapterKey{eventID: eventID, gameCharacterID: gameCharacterID}
	return key, eventRecordHonorRankDisplay{text: fmt.Sprintf("T%d", bestTier), tier: bestTier}, true
}

func eventRecordWorldBloomBestHonorTier(ranges []masterdata.WorldBloomChapterRankingRewardRange, userHonorIDs map[int]struct{}, resourceBoxHonorIDs map[int][]int) int {
	bestTier := 0
	for _, item := range ranges {
		if item.ToRank <= 0 || item.ResourceBoxID <= 0 {
			continue
		}
		if !eventRecordUserHasAnyHonor(userHonorIDs, resourceBoxHonorIDs[item.ResourceBoxID]) {
			continue
		}
		if bestTier == 0 || item.ToRank < bestTier {
			bestTier = item.ToRank
		}
	}
	return bestTier
}

func eventRecordUserHasAnyHonor(userHonorIDs map[int]struct{}, honorIDs []int) bool {
	for _, honorID := range honorIDs {
		if honorID <= 0 {
			continue
		}
		if _, ok := userHonorIDs[honorID]; ok {
			return true
		}
	}
	return false
}

func eventRecordWorldBloomChapterClosed(chapter *masterdata.WorldBloom, now int64) bool {
	if chapter == nil {
		return false
	}
	closedAt := chapter.ChapterEndAt
	if closedAt <= 0 {
		closedAt = chapter.AggregateAt
	}
	return closedAt > 0 && now >= closedAt
}

func eventRecordAddHonorDetails(out map[int][]int, boxID int, details []eventRecordRankingRewardDetail) {
	if boxID <= 0 {
		return
	}
	for _, detail := range details {
		if eventRecordDetailResourceType(detail) != "honor" {
			continue
		}
		if id := eventRecordDetailResourceID(detail); id > 0 {
			out[boxID] = append(out[boxID], id)
		}
	}
}

func eventRecordDetailResourceType(detail eventRecordRankingRewardDetail) string {
	if value := strings.TrimSpace(detail.ResourceType); value != "" {
		return strings.ToLower(value)
	}
	return strings.ToLower(strings.TrimSpace(detail.ResourceType2))
}

func eventRecordDetailResourceID(detail eventRecordRankingRewardDetail) int {
	if detail.ResourceID > 0 {
		return detail.ResourceID
	}
	return detail.ResourceID2
}

func buildEventRecordHonorRankDisplays(rawData *rendersnapshot.RawUserData, eventMaster map[int]eventMasterEntry, eventHonorTiers map[int]map[int]int) map[int]eventRecordHonorRankDisplay {
	if rawData == nil || len(eventHonorTiers) == 0 {
		return nil
	}
	userHonorIDs := collectEventRecordUserHonorIDs(rawData)
	if len(userHonorIDs) == 0 {
		return nil
	}

	now := eventRecordSnapshotNow(rawData)
	out := make(map[int]eventRecordHonorRankDisplay)
	for eventID, tiers := range eventHonorTiers {
		if !eventRecordRankingSettled(eventMaster[eventID], now) {
			continue
		}
		bestTier := eventRecordBestHonorTier(userHonorIDs, tiers)
		if bestTier > 0 {
			out[eventID] = eventRecordHonorRankDisplay{
				text: fmt.Sprintf("T%d", bestTier),
				tier: bestTier,
			}
		}
	}
	return out
}

func eventRecordBestHonorTier(userHonorIDs map[int]struct{}, tiers map[int]int) int {
	bestTier := 0
	for honorID := range userHonorIDs {
		tier := tiers[honorID]
		if tier > 0 && (bestTier == 0 || tier < bestTier) {
			bestTier = tier
		}
	}
	return bestTier
}

func collectEventRecordUserHonorIDs(rawData *rendersnapshot.RawUserData) map[int]struct{} {
	if rawData == nil {
		return nil
	}
	ids := make(map[int]struct{}, len(rawData.UserHonors)+len(rawData.UserProfileHonors))
	for _, honor := range rawData.UserHonors {
		if honor.HonorID > 0 {
			ids[honor.HonorID] = struct{}{}
		}
	}
	for _, honor := range rawData.UserProfileHonors {
		if honor.HonorID > 0 {
			ids[honor.HonorID] = struct{}{}
		}
		if honor.HonorId2 > 0 {
			ids[honor.HonorId2] = struct{}{}
		}
	}
	return ids
}

func eventRecordRankingSettled(master map[string]any, now int64) bool {
	settledAt := int64Val(master, "rankingAnnounceAt")
	if settledAt <= 0 {
		settledAt = int64Val(master, "aggregateAt")
	}
	if settledAt <= 0 {
		settledAt = int64Val(master, "closedAt")
	}
	return settledAt > 0 && now >= settledAt
}

func eventRecordRankNote(region renderregion.Value) *string {
	switch renderregion.WithDefault(region) {
	case renderregion.CN, renderregion.KR, renderregion.TW:
		return stringPtr("CN/KR/TW服没有排名数据，仅显示Txxx名")
	default:
		return nil
	}
}

func dropUntrustedEventRecordSnapshotRanks(region renderregion.Value, rankByEvent map[int]int, boundariesByEvent map[int]map[int]struct{}) {
	if region == renderregion.JP || len(rankByEvent) == 0 || len(boundariesByEvent) == 0 {
		return
	}
	for eventID, rank := range rankByEvent {
		if rank < 1000 {
			continue
		}
		if boundaries, ok := boundariesByEvent[eventID]; ok {
			if _, suspicious := boundaries[rank]; suspicious {
				delete(rankByEvent, eventID)
			}
		}
	}
}

// buildEventHistoryFromMaster creates an EventHistory from master data.
func buildEventHistoryFromMaster(master map[string]any, eventID int, eventPoint *int, assetHelper *assets.AssetHelper, regionStr string) *drawing.EventHistory {
	if len(master) == 0 {
		return nil
	}
	assetBundle := stringVal(master, "assetbundleName")
	bannerPath := assets.ResolveEventBannerPath(assetHelper, regionStr, assetBundle)
	return &drawing.EventHistory{
		ID:         eventID,
		EventName:  stringVal(master, "name"),
		StartAt:    master["startAt"],
		EndAt:      master["closedAt"],
		EventPoint: eventPoint,
		BannerPath: bannerPath,
	}
}

func sortEventHistory(items []drawing.EventHistory) {
	sort.SliceStable(items, func(i, j int) bool {
		if ri, okI := eventHistorySortRank(items[i]); okI {
			if rj, okJ := eventHistorySortRank(items[j]); okJ {
				if ri != rj {
					return ri < rj
				}
			} else {
				return true
			}
		} else if _, okJ := eventHistorySortRank(items[j]); okJ {
			return false
		}
		pointI := eventHistoryPoint(items[i])
		pointJ := eventHistoryPoint(items[j])
		if pointI != pointJ {
			return pointI > pointJ
		}
		si, _ := items[i].StartAt.(float64)
		sj, _ := items[j].StartAt.(float64)
		return si > sj
	})
}

func eventHistoryPoint(item drawing.EventHistory) int {
	if item.EventPoint == nil {
		return 0
	}
	return *item.EventPoint
}

func eventHistorySortRank(item drawing.EventHistory) (int, bool) {
	if item.Rank != nil && *item.Rank > 0 {
		return *item.Rank, true
	}
	if item.RankTier != nil && *item.RankTier > 0 {
		return *item.RankTier, true
	}
	if item.RankDisplay != nil {
		return eventHistorySortRankDisplay(*item.RankDisplay)
	}
	return 0, false
}

func eventHistorySortRankDisplay(text string) (int, bool) {
	text = strings.TrimSpace(strings.ToUpper(text))
	if !strings.HasPrefix(text, "T") {
		return 0, false
	}
	rank, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(text, "T")))
	if err != nil || rank <= 0 {
		return 0, false
	}
	return rank, true
}

func stringVal(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func intVal(m map[string]any, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case json.Number:
			i, _ := n.Int64()
			return int(i)
		}
	}
	return 0
}

func int64Val(m map[string]any, key string) int64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int64(n)
		case int:
			return int64(n)
		case int64:
			return n
		case json.Number:
			i, _ := n.Int64()
			return i
		}
	}
	return 0
}
