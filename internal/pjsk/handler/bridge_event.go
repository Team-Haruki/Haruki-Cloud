package handler

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"haruki-cloud/api/bot/onebot11"
	eventdb "haruki-cloud/database/sekai/event"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/event"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

func executeEvent(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App.Events == nil {
		return nil, fmt.Errorf("event service unavailable: sekai client not configured")
	}
	eventCtrl := rc.App.Events.WithContext(rc.Ctx)
	var data []byte
	region := renderregion.Value(rc.Cmd.Region)
	switch rc.Cmd.Mode {
	case "event-detail":
		q := event.DetailQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = eventCtrl.RenderEventDetail(q)
	case "event-list":
		q := event.ListQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = eventCtrl.RenderEventList(q)
	case "event-record":
		req, buildErr := buildEventRecordFromSnapshot(rc, region)
		if buildErr != nil {
			return nil, buildErr
		}
		data, err = eventCtrl.RenderEventRecord(*req)
	default:
		return nil, unsupportedModeError("event", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

// buildEventRecordFromSnapshot constructs an EventRecordRequest from live
// Toolbox suite data, cross-referencing with master data for event metadata.
// Regular events come from userEvents; world bloom events come from userWorldBlooms.
func buildEventRecordFromSnapshot(rc *RequestContext, region renderregion.Value) (*drawing.EventRecordRequest, error) {
	snapshot := resolveLiveSnapshot(rc, false)
	if snapshot == nil {
		return nil, fmt.Errorf("event record requires user data (suite snapshot unavailable)")
	}
	rawData := snapshot.RawData()
	if rawData == nil || (len(rawData.UserEvents) == 0 && len(rawData.UserWorldBlooms) == 0) {
		return nil, fmt.Errorf("event record requires at least one history entry")
	}

	// Build rank lookup from UserEventResults (if available)
	rankByEvent := make(map[int]int, len(rawData.UserEventResults))
	for _, result := range rawData.UserEventResults {
		rankByEvent[result.EventID] = result.Rank
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
			rank := wb.Rank
			hist.Rank = &rank
		}
		if wb.GameCharacterID > 0 {
			sdPath := assets.ResolveRegionAssetPath(
				rc.App.Assets,
				regionStr,
				fmt.Sprintf("character/character_sd_l/chr_sp_%d.png", wb.GameCharacterID),
			)
			hist.WlCharaIconPath = &sdPath
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
		return nil, fmt.Errorf("event record requires user profile data")
	}

	return &drawing.EventRecordRequest{
		EventInfo:   eventInfo,
		WlEventInfo: wlEventInfo,
		UserInfo:    *profile,
	}, nil
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
