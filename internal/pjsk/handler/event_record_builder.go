package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	eventdb "haruki-cloud/database/sekai/event"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/logger"

	"golang.org/x/sync/errgroup"
)

var eventRecordDebugLogger = logger.NewLoggerFromGlobal("EventRecord")

var eventRecordTrackerRankLookup = defaultEventRecordTrackerRankLookup

const eventRecordTrackerFallbackEventLimit = 2

var errStopEventRecordTrackerFallback = errors.New("event record tracker fallback aborted")

// buildEventRecordFromSnapshot constructs an EventRecordRequest from live
// Toolbox suite data, cross-referencing with master data for event metadata.
// Regular events come from userEvents; world bloom events come from userWorldBlooms.
func buildEventRecordFromSnapshot(rc *RequestContext, region renderregion.Value) (*drawing.EventRecordRequest, error) {
	snapshot := rc.ResolveSnapshot(false)
	if snapshot == nil {
		return nil, fmt.Errorf("event record requires user data (suite snapshot unavailable)")
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
	type eventMasterEntry = map[string]any
	eventMaster := make(map[int]eventMasterEntry, len(eventEntities))
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
	}

	// Identify world_bloom event IDs from master
	wlEventIDs := make(map[int]struct{})
	for _, ev := range eventMaster {
		if stringVal(ev, "eventType") == "world_bloom" {
			wlEventIDs[intVal(ev, "id")] = struct{}{}
		}
	}

	fillEventRecordTrackerRanks(rc.Ctx, rc.App.Tracker, region, rawData.UserGamedata.UserID, rawData.UserEvents, rankByEvent, wlEventIDs)

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

	profile := snapshot.DetailedProfile(region)
	if profile == nil {
		profile = rc.GetDetailedProfile()
	}
	if profile == nil {
		return nil, fmt.Errorf("event record requires user profile data")
	}

	return &drawing.EventRecordRequest{
		EventInfo:   eventInfo,
		WlEventInfo: wlEventInfo,
		UserInfo:    *profile,
	}, nil
}

func defaultEventRecordTrackerRankLookup(ctx context.Context, tracker *sekaiapi.TrackerClient, region string, eventID int, userID int64) (*int, error) {
	if tracker == nil {
		return nil, nil
	}

	resp, err := tracker.WithContext(ctx).GetLatestRankingByUser(region, eventID, userID)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.RankData.Rank <= 0 {
		return nil, nil
	}

	return intPtr(resp.RankData.Rank), nil
}

func fillEventRecordTrackerRanks(
	ctx context.Context,
	tracker *sekaiapi.TrackerClient,
	region renderregion.Value,
	userID int64,
	userEvents []rendersnapshot.RawUserEvent,
	rankByEvent map[int]int,
	wlEventIDs map[int]struct{},
) {
	if userID <= 0 || len(userEvents) == 0 || len(rankByEvent) >= len(userEvents) {
		return
	}

	regionStr := regionWithDefault(region.String())
	if regionStr == "" {
		return
	}

	eventIDs := collectEventRecordTrackerFallbackEventIDs(userEvents, rankByEvent, wlEventIDs)
	if len(eventIDs) == 0 {
		return
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(1)

	var (
		mu      sync.Mutex
		logOnce sync.Once
	)
	for _, eventID := range eventIDs {
		eventIDCopy := eventID
		group.Go(func() error {
			if groupCtx.Err() != nil {
				return nil
			}

			rank, err := eventRecordTrackerRankLookup(groupCtx, tracker, regionStr, eventIDCopy, userID)
			if err != nil {
				if !errors.Is(err, sekaiapi.ErrRankingNotFound) {
					logOnce.Do(func() {
						eventRecordDebugLogger.Debugf("event record tracker rank fallback unavailable: region=%s user=%s err=%v",
							regionStr, maskDebugID(strconv.FormatInt(userID, 10)), err)
					})
					if shouldStopEventRecordTrackerFallback(err) {
						return errStopEventRecordTrackerFallback
					}
				}
				return nil
			}
			if rank == nil || *rank <= 0 {
				return nil
			}

			mu.Lock()
			defer mu.Unlock()
			if _, exists := rankByEvent[eventIDCopy]; !exists {
				rankByEvent[eventIDCopy] = *rank
			}
			return nil
		})
	}

	waitErr := group.Wait()
	if waitErr != nil && !errors.Is(waitErr, errStopEventRecordTrackerFallback) {
		eventRecordDebugLogger.Debugf("event record tracker rank fallback ended early: region=%s user=%s err=%v",
			regionStr, maskDebugID(strconv.FormatInt(userID, 10)), waitErr)
	}
}

func collectEventRecordTrackerFallbackEventIDs(
	userEvents []rendersnapshot.RawUserEvent,
	rankByEvent map[int]int,
	wlEventIDs map[int]struct{},
) []int {
	seen := make(map[int]struct{}, len(userEvents))
	eventIDs := make([]int, 0, len(userEvents))
	for _, userEvent := range userEvents {
		eventID := userEvent.EventID
		if eventID <= 0 {
			continue
		}
		if _, isWL := wlEventIDs[eventID]; isWL {
			continue
		}
		if _, ok := seen[eventID]; ok {
			continue
		}
		seen[eventID] = struct{}{}
		if _, ok := rankByEvent[eventID]; ok {
			continue
		}
		eventIDs = append(eventIDs, eventID)
	}

	sort.Slice(eventIDs, func(i, j int) bool {
		return eventIDs[i] > eventIDs[j]
	})
	if len(eventIDs) > eventRecordTrackerFallbackEventLimit {
		eventIDs = eventIDs[:eventRecordTrackerFallbackEventLimit]
	}
	return eventIDs
}

func shouldStopEventRecordTrackerFallback(err error) bool {
	if err == nil || errors.Is(err, sekaiapi.ErrRankingNotFound) {
		return false
	}
	if errors.Is(err, sekaiapi.ErrServerMaintenance) {
		return true
	}

	var trackerErr *sekaiapi.TrackerAPIError
	if errors.As(err, &trackerErr) {
		if trackerErr.StatusCode >= 500 || trackerErr.StatusCode == 429 {
			return true
		}
		message := strings.ToLower(strings.TrimSpace(trackerErr.Message))
		return strings.Contains(message, "doesn't exist") ||
			strings.Contains(message, "failed to fetch latest ranking")
	}

	return false
}

// buildEventHistoryFromMaster creates an EventHistory from master data.
func buildEventHistoryFromMaster(master map[string]any, eventID, eventPoint int, assetHelper *assets.AssetHelper, regionStr string) *drawing.EventHistory {
	if len(master) == 0 {
		return nil
	}
	assetBundle := stringVal(master, "assetbundleName")
	bannerPath := assets.ResolveRegionAssetPath(assetHelper, regionStr,
		filepath.Join("home", "banner", assetBundle, assetBundle+".png"),
		filepath.Join("event", assetBundle, "banner.png"))
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
		si, _ := items[i].StartAt.(float64)
		sj, _ := items[j].StartAt.(float64)
		return si > sj
	})
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
