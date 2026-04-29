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
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

type eventMasterEntry = map[string]any

// buildEventRecordFromSnapshot constructs an EventRecordRequest from live
// Toolbox suite data, cross-referencing with master data for event metadata.
// Regular events come from userEvents; world bloom events come from userWorldBlooms.
func buildEventRecordFromSnapshot(rc *RequestContext, region renderregion.Value) (*drawing.EventRecordRequest, error) {
	var snapshot rendersnapshot.Snapshot
	if rc != nil && rc.App != nil && rc.App.Bindings != nil && strings.TrimSpace(rc.Platform) != "" && strings.TrimSpace(rc.PlatformUserID) != "" {
		binding, resolvedSnapshot, err := rc.requireVisibleSuiteSnapshot()
		if err != nil {
			return nil, err
		}
		if resolvedSnapshot == nil {
			return nil, newSuiteDataNotFoundReplayErrorForBinding(binding)
		}
		snapshot = resolvedSnapshot
	} else {
		snapshot = rc.ResolveSnapshot(false)
		if snapshot == nil {
			return nil, fmt.Errorf("event record requires user data (suite snapshot unavailable)")
		}
	}
	rawData := snapshot.RawData()
	if rawData == nil || (len(rawData.UserEvents) == 0 && len(rawData.UserWorldBlooms) == 0) {
		return nil, fmt.Errorf("event record requires at least one history entry")
	}

	// Build rank lookup from UserEventResults (if available), then fall back to
	// the legacy/original suite shape where rank is embedded inside userEvents.
	rankByEvent := make(map[int]int, len(rawData.UserEventResults))
	for _, result := range rawData.UserEventResults {
		rankByEvent[result.EventID] = result.Rank
	}
	for _, userEvent := range rawData.UserEvents {
		if userEvent.EventID <= 0 || userEvent.Rank <= 0 {
			continue
		}
		if _, exists := rankByEvent[userEvent.EventID]; !exists {
			rankByEvent[userEvent.EventID] = userEvent.Rank
		}
	}
	eventEntities, err := rc.App.Sekai.Event.Query().
		Where(eventdb.ServerRegionEQ(region.String())).
		All(rc.Ctx)
	if err != nil || len(eventEntities) == 0 {
		return nil, fmt.Errorf("event master data not available")
	}
	eventMaster := make(map[int]eventMasterEntry, len(eventEntities))
	eventRankBoundaries := make(map[int]map[int]struct{}, len(eventEntities))
	eventHonorTiers := make(map[int]map[int]int, len(eventEntities))
	resourceBoxHonorIDs := eventRecordResourceBoxHonorIDs(rc, region)
	for _, e := range eventEntities {
		id := int(e.GameID)
		eventMaster[id] = eventMasterEntry{
			"id":              id,
			"eventType":       e.EventType,
			"assetbundleName": e.AssetbundleName,
			"name":            e.Name,
			"startAt":         float64(e.StartAt),
			"closedAt":        float64(e.ClosedAt),
		}
		if boundaries := eventRecordRankBoundaries(e.EventRankingRewardRanges); len(boundaries) > 0 {
			eventRankBoundaries[id] = boundaries
		}
		if tiers := eventRecordHonorTiers(e.EventRankingRewardRanges, resourceBoxHonorIDs); len(tiers) > 0 {
			eventHonorTiers[id] = tiers
		}
	}

	// Identify world_bloom event IDs from master
	wlEventIDs := make(map[int]struct{})
	for _, ev := range eventMaster {
		if stringVal(ev, "eventType") == "world_bloom" {
			wlEventIDs[intVal(ev, "id")] = struct{}{}
		}
	}

	dropUntrustedEventRecordSnapshotRanks(region, rankByEvent, eventRankBoundaries)
	rankDisplayByEvent := buildEventRecordHonorRankDisplays(rawData, eventMaster, eventHonorTiers, rankByEvent)

	regionStr := regionWithDefault(region.String())

	// --- Build regular event entries from userEvents ---
	eventInfo := make([]drawing.EventHistory, 0)
	for _, ue := range rawData.UserEvents {
		hist := buildEventHistoryFromMaster(eventMaster[ue.EventID], ue.EventID, ue.EventPoint, rc.App.Assets, regionStr)
		if hist == nil {
			continue
		}
		if _, isWL := wlEventIDs[ue.EventID]; isWL {
			hist.IsWlEvent = true
		}
		if rank, ok := rankByEvent[ue.EventID]; ok {
			hist.Rank = &rank
		} else if display, ok := rankDisplayByEvent[ue.EventID]; ok {
			hist.RankDisplay = stringPtr(display.text)
			hist.RankTier = intPtr(display.tier)
		}
		eventInfo = append(eventInfo, *hist)
	}

	// --- Build world bloom single-board entries from userWorldBlooms ---
	wlEventInfo := make([]drawing.EventHistory, 0, len(rawData.UserWorldBlooms))
	for _, wb := range rawData.UserWorldBlooms {
		hist := buildEventHistoryFromMaster(eventMaster[wb.EventID], wb.EventID, wb.WorldBloomChapterPoint, rc.App.Assets, regionStr)
		if hist == nil {
			continue
		}
		hist.IsWlEvent = true
		if wb.Rank > 0 {
			hist.Rank = intPtr(wb.Rank)
		}
		if wb.GameCharacterID > 0 {
			hist.WlCharaIconPath = stringPtr(assets.ResolveRegionAssetPath(
				rc.App.Assets,
				regionStr,
				fmt.Sprintf("character/character_sd_l/chr_sp_%d.png", wb.GameCharacterID),
			))
		}
		wlEventInfo = append(wlEventInfo, *hist)
	}

	if len(eventInfo) == 0 && len(wlEventInfo) == 0 {
		return nil, fmt.Errorf("event record requires at least one history entry")
	}

	sortEventHistory(eventInfo)
	sortEventHistory(wlEventInfo)

	profile, _ := resolveCommandDisplayProfiles(rc, snapshot)
	if profile == nil {
		return nil, fmt.Errorf("event record requires user profile data")
	}

	return &drawing.EventRecordRequest{
		EventInfo:   eventInfo,
		WlEventInfo: wlEventInfo,
		UserInfo:    *profile,
		RankNote:    eventRecordRankNote(region),
	}, nil
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
	if rc == nil || rc.App == nil {
		return nil
	}
	if ids := eventRecordResourceBoxHonorIDsFromProvider(rc, region); len(ids) > 0 {
		return ids
	}
	if rc.App.Sekai == nil || rc.Ctx == nil {
		return nil
	}

	items, err := rc.App.Sekai.Resourceboxe.Query().
		Where(
			resourceboxe.ServerRegionEQ(region.String()),
			resourceboxe.ResourceBoxPurposeEQ("event_ranking_reward"),
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

func eventRecordResourceBoxHonorIDsFromProvider(rc *RequestContext, region renderregion.Value) map[int][]int {
	source := rc.App.ProviderForRegion(region)
	if source == nil || rc.Ctx == nil {
		return nil
	}
	education := source.Education()
	if education == nil {
		return nil
	}
	boxes := education.GetResourceBoxesByPurpose(rc.Ctx, "event_ranking_reward")
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

func buildEventRecordHonorRankDisplays(rawData *rendersnapshot.RawUserData, eventMaster map[int]eventMasterEntry, eventHonorTiers map[int]map[int]int, rankByEvent map[int]int) map[int]eventRecordHonorRankDisplay {
	if rawData == nil || len(eventHonorTiers) == 0 {
		return nil
	}
	userHonorIDs := collectEventRecordUserHonorIDs(rawData)
	if len(userHonorIDs) == 0 {
		return nil
	}

	now := rawData.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}

	out := make(map[int]eventRecordHonorRankDisplay)
	for eventID, tiers := range eventHonorTiers {
		if _, hasRank := rankByEvent[eventID]; hasRank {
			continue
		}
		if !eventRecordClosed(eventMaster[eventID], now) {
			continue
		}
		bestTier := 0
		for honorID := range userHonorIDs {
			tier, ok := tiers[honorID]
			if !ok || tier <= 0 {
				continue
			}
			if bestTier == 0 || tier < bestTier {
				bestTier = tier
			}
		}
		if bestTier > 0 {
			out[eventID] = eventRecordHonorRankDisplay{
				text: fmt.Sprintf("T%d", bestTier),
				tier: bestTier,
			}
		}
	}
	return out
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

func eventRecordClosed(master map[string]any, now int64) bool {
	closedAt := int64Val(master, "closedAt")
	return closedAt > 0 && now >= closedAt
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
func buildEventHistoryFromMaster(master map[string]any, eventID, eventPoint int, assetHelper *assets.AssetHelper, regionStr string) *drawing.EventHistory {
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
		if items[i].EventPoint != items[j].EventPoint {
			return items[i].EventPoint > items[j].EventPoint
		}
		si, _ := items[i].StartAt.(float64)
		sj, _ := items[j].StartAt.(float64)
		return si > sj
	})
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
