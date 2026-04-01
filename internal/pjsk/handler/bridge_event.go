package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"haruki-cloud/api/bot/onebot11"
	eventdb "haruki-cloud/database/sekai/event"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/event"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

func executeEvent(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App.Events == nil {
		return nil, fmt.Errorf("event service unavailable: sekai client not configured")
	}
	var data []byte
	region := renderregion.Value(rc.Cmd.Region)
	switch rc.Cmd.Mode {
	case "event-detail":
		q := event.DetailQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = rc.App.Events.RenderEventDetail(q)
	case "event-list":
		q := event.ListQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = rc.App.Events.RenderEventList(q)
	case "event-record":
		req, buildErr := buildEventRecordFromSnapshot(rc.Ctx, rc.Cmd, rc.App, region)
		if buildErr != nil {
			return nil, buildErr
		}
		data, err = rc.App.Events.RenderEventRecord(*req)
	default:
		return nil, fmt.Errorf("bridge: unsupported event mode %q", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

// buildEventRecordFromSnapshot constructs an EventRecordRequest from live
// Toolbox suite data, cross-referencing with master data for event metadata.
// Regular events come from userEvents; world bloom events come from userWorldBlooms.
func buildEventRecordFromSnapshot(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App, region renderregion.Value) (*drawing.EventRecordRequest, error) {
	snapshot := resolveLiveSnapshot(ctx, r, app, false)
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

	eventEntities, err := app.Sekai.Event.Query().
		Where(eventdb.ServerRegionEQ(region.String())).
		All(ctx)
	if err != nil || len(eventEntities) == 0 {
		return nil, fmt.Errorf("event master data not available")
	}
	type eventMasterEntry = map[string]interface{}
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

	regionStr := region.String()
	if regionStr == "" {
		regionStr = "jp"
	}

	// --- Build regular event entries from userEvents ---
	eventInfo := make([]drawing.EventHistory, 0)
	for _, ue := range rawData.UserEvents {
		if _, isWL := wlEventIDs[ue.EventID]; isWL {
			continue // world bloom events handled below
		}
		hist := buildEventHistoryFromMaster(eventMaster[ue.EventID], ue.EventID, ue.EventPoint, app.Assets, regionStr)
		if hist == nil {
			continue
		}
		if rank, ok := rankByEvent[ue.EventID]; ok {
			hist.Rank = &rank
		}
		eventInfo = append(eventInfo, *hist)
	}

	// --- Build world bloom entries from userWorldBlooms ---
	// Aggregate by eventId: sum points, pick best rank, collect character IDs
	type wlAgg struct {
		totalPoint int
		bestRank   int
		charIDs    []int
	}
	wlMap := make(map[int]*wlAgg)
	for _, wb := range rawData.UserWorldBlooms {
		agg, ok := wlMap[wb.EventID]
		if !ok {
			agg = &wlAgg{bestRank: wb.Rank}
			wlMap[wb.EventID] = agg
		}
		agg.totalPoint += wb.WorldBloomChapterPoint
		if wb.Rank > 0 && (agg.bestRank == 0 || wb.Rank < agg.bestRank) {
			agg.bestRank = wb.Rank
		}
		agg.charIDs = append(agg.charIDs, wb.GameCharacterID)
	}

	wlEventInfo := make([]drawing.EventHistory, 0)
	for eventID, agg := range wlMap {
		hist := buildEventHistoryFromMaster(eventMaster[eventID], eventID, agg.totalPoint, app.Assets, regionStr)
		if hist == nil {
			continue
		}
		hist.IsWlEvent = true
		if agg.bestRank > 0 {
			rank := agg.bestRank
			hist.Rank = &rank
		}
		// Use first character for WL icon
		if len(agg.charIDs) > 0 && agg.charIDs[0] > 0 {
			icon := charaIconPath(app.Assets, agg.charIDs[0])
			hist.WlCharaIconPath = &icon
		}
		wlEventInfo = append(wlEventInfo, *hist)
	}

	// Also add any userEvents entries that are world bloom type
	for _, ue := range rawData.UserEvents {
		if _, isWL := wlEventIDs[ue.EventID]; !isWL {
			continue
		}
		if _, exists := wlMap[ue.EventID]; exists {
			continue
		}
		hist := buildEventHistoryFromMaster(eventMaster[ue.EventID], ue.EventID, ue.EventPoint, app.Assets, regionStr)
		if hist == nil {
			continue
		}
		hist.IsWlEvent = true
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
func buildEventHistoryFromMaster(master map[string]interface{}, eventID, eventPoint int, assetHelper *assets.AssetHelper, regionStr string) *drawing.EventHistory {
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

// loadMasterMapByID loads a JSON array file and indexes by "id" field.
func loadMasterMapByID(dir, filename string) map[int]map[string]interface{} {
	if dir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		return nil
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil
	}
	result := make(map[int]map[string]interface{}, len(items))
	for _, item := range items {
		id := intVal(item, "id")
		if id != 0 {
			result[id] = item
		}
	}
	return result
}

// loadMasterList loads a JSON array file.
func loadMasterList(dir, filename string) []map[string]interface{} {
	if dir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		return nil
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil
	}
	return items
}

func stringVal(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func intVal(m map[string]interface{}, key string) int {
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
