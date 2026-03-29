package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/api/bot/onebot11"
	harukiConfig "haruki-cloud/config"
	sekaidb "haruki-cloud/database/sekai"
	bonddb "haruki-cloud/database/sekai/bond"
	gamecharacterdb "haruki-cloud/database/sekai/gamecharacter"
	gamecharacterunitdb "haruki-cloud/database/sekai/gamecharacterunit"
	leveldb "haruki-cloud/database/sekai/level"
	pjskalias "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/card"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/education"
	"haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/gacha"
	"haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/internal/pjsk/render/mysekai"
	"haruki-cloud/internal/pjsk/render/profile"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/sk"
	"haruki-cloud/internal/pjsk/render/stamp"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/internal/pjsk/render/vlive"
	"haruki-cloud/internal/pjsk/requestbuilder"
	accountdata "haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/drawing"
	"haruki-cloud/utils/query"
	sekaiutils "haruki-cloud/utils/sekai"
)

// Execute routes a ResolvedCommand to the corresponding execution controller
// and returns a OneBot message or an error.
func Execute(ctx context.Context, resolved *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	if resolved == nil {
		return nil, fmt.Errorf("bridge: nil resolved command")
	}
	if app == nil {
		return nil, fmt.Errorf("bridge: nil render app")
	}

	// Check requester ban before dispatching.
	if platform := strings.TrimSpace(resolved.RequesterPlatform); platform != "" {
		if userID := strings.TrimSpace(resolved.RequesterUserID); userID != "" {
			if err := app.BanChecker.CheckBan(ctx, platform, userID, resolved.Module); err != nil {
				message = append(message, onebot11.Text(err.Error()))
				return message, nil
			}
		}
	}

	// When no region was explicitly specified (no prefix like /jp, no -r flag),
	// resolve it from the user's global default binding so e.g. a TW player
	// doesn't always get JP results when typing bare commands.
	resolved.Region = resolveRegionFromDefaultBinding(ctx, resolved, app)

	switch resolved.Module {
	case parser.ModuleCard:
		message, err = executeCard(resolved, app)
	case parser.ModuleEvent:
		message, err = executeEvent(resolved, app)
	case parser.ModuleMusic:
		message, err = executeMusic(resolved, app)
	case parser.ModuleAlias:
		message, err = executeAlias(ctx, resolved, app)
	case parser.ModuleGacha:
		message, err = executeGacha(resolved, app)
	case parser.ModuleDeck:
		message, err = executeDeck(resolved, app)
	case parser.ModuleEducation:
		message, err = executeEducation(resolved, app)
	case parser.ModuleSK:
		message, err = executeSK(ctx, resolved, app)
	case parser.ModuleScore:
		message, err = executeScore(resolved, app)
	case parser.ModuleProfile:
		message, err = executeProfile(ctx, resolved, app)
	case parser.ModuleArrest:
		message, err = executeArrest(ctx, resolved, app)
	case parser.ModuleRegTime:
		message, err = executeRegTime(ctx, resolved, app)
	case parser.ModuleCheckData:
		message, err = executeCheckData(ctx, resolved, app)
	case parser.ModuleMysekai:
		message, err = executeMysekai(resolved, app)
	case parser.ModuleStamp:
		message, err = executeStamp(resolved, app)
	case parser.ModuleMisc:
		message, err = executeMisc(resolved, app)
	case parser.ModuleVLive:
		message, err = executeVLive(resolved, app)

	default:
		return nil, fmt.Errorf("bridge: unsupported module %v", resolved.Module)
	}
	if err != nil {
		return nil, err
	}
	return message, nil
}

func imageMessage(img []byte, app *renderapp.App, group string) (onebot11.Message, error) {
	url, err := app.ImageCache.StoreAndGetURL(img, group)
	if err != nil {
		return nil, err
	}
	return onebot11.Message{onebot11.Image(url, "")}, nil
}

func executeCard(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	if app.Cards == nil {
		return nil, fmt.Errorf("card service unavailable: sekai client not configured")
	}
	var (
		data []byte
		url  string
	)
	publicDetailedProfile, _ := buildPublicMusicProfiles(r, app)
	if snapshot := resolveLiveSnapshot(r, app, false); snapshot != nil {
		if detail := snapshot.DetailedProfile(renderregion.Normalize(r.Region)); detail != nil {
			publicDetailedProfile = detail
		}
	}
	switch r.Mode {
	case "card-detail":
		q := card.Query{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		data, err = app.Cards.RenderCardDetail(q)
	case "card-list":
		q := card.ListRequest{Region: r.Region}
		mergeParams(r.Params, &q)
		q.DetailedProfile = publicDetailedProfile
		data, err = app.Cards.RenderCardList(q)
	case "card-box":
		useAfterTraining := true
		q := card.Query{
			Query:            r.Query,
			Region:           r.Region,
			UseAfterTraining: &useAfterTraining,
			DetailedProfile:  resolveCardBoxDetailedProfile(r, app),
		}
		mergeParams(r.Params, &q)
		queries := []card.Query{q}
		data, err = app.Cards.RenderCardBox(queries)
	case "card-image":
		q := card.Query{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		result, resolveErr := app.Cards.ResolveCardImages(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		message = make(onebot11.Message, 0, len(result.Paths))
		for _, path := range result.Paths {
			image, imageErr := assetImageMessage(path, app, BotModulePJSK)
			if imageErr != nil {
				return nil, imageErr
			}
			message = append(message, image...)
		}
		if len(message) == 0 {
			return nil, fmt.Errorf("bridge: card %d did not resolve any images", result.Card.ID)
		}
		return message, nil
	default:
		return nil, fmt.Errorf("bridge: unsupported card mode %q", r.Mode)
	}
	if err != nil {
		return
	}
	url, err = app.ImageCache.StoreAndGetURL(data, BotModulePJSK)
	return onebot11.Message{onebot11.Image(url, "")}, err
}

func executeEvent(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	if app.Events == nil {
		return nil, fmt.Errorf("event service unavailable: sekai client not configured")
	}
	var data []byte
	region := renderregion.Value(r.Region)
	switch r.Mode {
	case "event-detail":
		q := event.DetailQuery{Region: region}
		mergeParams(r.Params, &q)
		data, err = app.Events.RenderEventDetail(q)
	case "event-list":
		q := event.ListQuery{Region: region}
		mergeParams(r.Params, &q)
		data, err = app.Events.RenderEventList(q)
	case "event-record":
		req, buildErr := buildEventRecordFromSnapshot(r, app, region)
		if buildErr != nil {
			return nil, buildErr
		}
		data, err = app.Events.RenderEventRecord(*req)
	default:
		return nil, fmt.Errorf("bridge: unsupported event mode %q", r.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

// buildEventRecordFromSnapshot constructs an EventRecordRequest from live
// Toolbox suite data, cross-referencing with master data for event metadata.
// Regular events come from userEvents; world bloom events come from userWorldBlooms.
func buildEventRecordFromSnapshot(r *parser.ResolvedCommand, app *renderapp.App, region renderregion.Value) (*drawing.EventRecordRequest, error) {
	snapshot := resolveLiveSnapshot(r, app, false)
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

	masterDir := app.Config.LocalMasterdata.Dir
	eventMaster := loadMasterMapByID(masterDir, "events.json")
	if len(eventMaster) == 0 {
		return nil, fmt.Errorf("event master data not available")
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

func executeMusic(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	if app == nil || app.Music == nil {
		return nil, fmt.Errorf("music service unavailable: music controller is not configured")
	}
	if app.Aliases != nil {
		app.Music.SetAliasResolver(app.Aliases)
	}
	publicDetailedProfile, publicProfileCard := buildPublicMusicProfiles(r, app)
	var data []byte
	switch r.Mode {
	case "music-detail":
		q := music.Query{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		data, err = app.Music.RenderMusicDetail(q)
	case "music-list":
		q := music.ListQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		if strings.TrimSpace(q.Keyword) == "" {
			q.Keyword = strings.TrimSpace(r.Query)
		}
		q.DetailedProfile = publicDetailedProfile
		data, err = app.Music.RenderMusicList(q)
	case "music-chart":
		q := music.ChartQuery{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		data, err = app.Music.RenderMusicChart(q)
	case "music-progress":
		q := music.ProgressQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		q.Profile = publicProfileCard
		data, err = app.Music.RenderMusicProgress(q)
	case "music-rewards":
		data, err = renderMusicRewards(r, app, publicProfileCard)
	case "music-note-count":
		q := music.NoteCountQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		matches, resolveErr := app.Music.FindMusicChartsByNoteCount(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		lines := make([]string, 0, len(matches))
		for _, item := range matches {
			lines = append(lines, fmt.Sprintf("銆?d銆?s - %s %d", item.Music.ID, item.Music.Title, strings.ToUpper(item.Difficulty), item.PlayLevel))
		}
		return onebot11.Message{onebot11.Text(strings.Join(lines, "\n"))}, nil
	case "music-cover":
		q := music.Query{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		result, resolveErr := app.Music.ResolveMusicCover(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		image, imageErr := assetImageMessage(result.JacketPath, app, BotModulePJSK)
		if imageErr != nil {
			return nil, imageErr
		}
		text := fmt.Sprintf("銆?d銆?s", result.Music.ID, result.Music.Title)
		return append(image, onebot11.Text(text)), nil
	case "music-bpm":
		q := music.Query{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		result, resolveErr := app.Music.ResolveMusicBPM(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		image, imageErr := assetImageMessage(result.JacketPath, app, BotModulePJSK)
		if imageErr != nil {
			return nil, imageErr
		}
		textLines := []string{
			fmt.Sprintf("銆?d銆?s", result.Music.ID, result.Music.Title),
			fmt.Sprintf("涓?BPM: %s", formatMusicBPM(result.MainBPM)),
			fmt.Sprintf("BPM 鍙樺寲: %s", formatBPMEvents(result.Events)),
			fmt.Sprintf("璋遍潰鏉ユ簮: %s", strings.ToUpper(result.Difficulty)),
		}
		return append(image, onebot11.Text(strings.Join(textLines, "\n"))), nil
	default:
		return nil, fmt.Errorf("bridge: unsupported music mode %q", r.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

func executeAlias(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) (onebot11.Message, error) {
	if app == nil || app.Aliases == nil {
		return nil, fmt.Errorf("鍒悕鏈嶅姟鏈氨缁紝璇风◢鍚庡啀璇?)
	}
	data, err := pjskalias.ExecuteCommand(ctx, app.Aliases, r.Mode, r.Params)
	if err != nil {
		return nil, err
	}
	return onebot11.Message{onebot11.Text(string(data))}, nil
}

func assetImageMessage(path string, app *renderapp.App, group string) (onebot11.Message, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("asset path is empty")
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return onebot11.Message{onebot11.Image(path, "")}, nil
	}
	// When a CDN base URL is configured, build a direct URL by extracting the
	// "{region}-assets/..." portion of the path (e.g. "jp-assets/startapp/...").
	if app != nil {
		if base := strings.TrimRight(app.Config.AssetsBaseURL, "/"); base != "" {
			rel := filepath.ToSlash(path)
			if idx := strings.Index(rel, "-assets/"); idx > 0 {
				start := strings.LastIndex(rel[:idx], "/") + 1
				rel = rel[start:]
			}
			return onebot11.Message{onebot11.Image(base+"/"+rel, "")}, nil
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, group)
}

func formatBPMEvents(events []music.BPMEvent) string {
	if len(events) == 0 {
		return "鏃犳暟鎹?
	}
	parts := make([]string, 0, len(events))
	for _, item := range events {
		parts = append(parts, formatMusicBPM(item.BPM))
	}
	return strings.Join(parts, " -> ")
}

func formatMusicBPM(value float64) string {
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return strconv.FormatInt(int64(math.Round(value)), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func buildPublicMusicProfiles(r *parser.ResolvedCommand, app *renderapp.App) (*drawing.DetailedProfileCardRequest, *drawing.ProfileCardRequest) {
	if r == nil || app == nil || app.Profiles == nil || app.Bindings == nil {
		return nil, nil
	}
	if strings.TrimSpace(r.RequesterPlatform) == "" || strings.TrimSpace(r.RequesterUserID) == "" {
		return nil, nil
	}

	region := strings.TrimSpace(r.Region)
	if region == "" {
		region = string(renderregion.JP)
	}

	queryParams := userQueryParams{
		Mode:           "self",
		Platform:       strings.TrimSpace(r.RequesterPlatform),
		PlatformUserID: strings.TrimSpace(r.RequesterUserID),
	}
	target, err := resolveGameTarget(context.Background(), queryParams, region, r.RegionExplicit, app)
	if err != nil {
		return nil, nil
	}

	resp, err := sekaiutils.GetSekaiAPIClient().GetUserProfile(region, target.PJSKUserID)
	if err != nil {
		return nil, nil
	}

	var framesJSON []byte
	if hasUsableSuiteData(target.Binding) {
		if uid, convErr := strconv.ParseInt(target.PJSKUserID, 10, 64); convErr == nil {
			framesJSON, _ = sekaiutils.GetToolboxClient().GetPrivateDataValue(
				region, sekaiutils.ToolboxDataTypeSuite, uid, queryParams.Platform, queryParams.PlatformUserID, "userPlayerFrames")
		}
	}

	q := profile.Query{
		Region:     region,
		Visible:    target.Visible,
		BgSettings: target.BgSettings,
	}
	detail, err := app.Profiles.BuildDetailedProfileCardFromAPI(q, resp, framesJSON)
	if err != nil {
		return nil, nil
	}
	card, err := app.Profiles.BuildProfileCardFromAPI(q, resp, framesJSON)
	if err != nil {
		return detail, nil
	}
	return detail, card
}

func resolveCardBoxDetailedProfile(r *parser.ResolvedCommand, app *renderapp.App) *drawing.DetailedProfileCardRequest {
	if r == nil || app == nil {
		return nil
	}
	region := renderregion.Normalize(r.Region)
	if snapshot := resolveLiveSnapshot(r, app, false); snapshot != nil {
		if detail := snapshot.DetailedProfile(region); detail != nil && len(detail.UserCards) > 0 {
			return detail
		}
	}
	if app.Profiles != nil {
		if detail := app.Profiles.SnapshotDetailedProfile(region); detail != nil && len(detail.UserCards) > 0 {
			return detail
		}
	}
	return nil
}

func renderMusicRewards(r *parser.ResolvedCommand, app *renderapp.App, publicProfileCard *drawing.ProfileCardRequest) ([]byte, error) {
	q := music.RewardsBasicQuery{Region: r.Region}
	mergeParams(r.Params, &q)
	q.Profile = publicProfileCard

	reason := ""
	region := strings.TrimSpace(r.Region)
	if region == "" {
		region = string(renderregion.JP)
	}

	if strings.TrimSpace(r.RequesterPlatform) != "" && strings.TrimSpace(r.RequesterUserID) != "" {
		queryParams := userQueryParams{
			Mode:           "self",
			Platform:       strings.TrimSpace(r.RequesterPlatform),
			PlatformUserID: strings.TrimSpace(r.RequesterUserID),
		}
		target, err := resolveGameTarget(context.Background(), queryParams, region, r.RegionExplicit, app)
		if err == nil && target.Binding != nil {
			if !hasUsableSuiteData(target.Binding) {
				reason = "褰撳墠宸插叧闂?Suite 鎶撳寘鏁版嵁锛屼互涓嬩负鍩轰簬鍏紑淇℃伅鐨勪及绠楃粨鏋溿€?
			} else if uid, convErr := strconv.ParseInt(target.PJSKUserID, 10, 64); convErr == nil {
				raw, toolboxErr := sekaiutils.GetToolboxClient().GetPrivateDataValue(
					region, sekaiutils.ToolboxDataTypeSuite, uid, queryParams.Platform, queryParams.PlatformUserID, "userMusicAchievements")
				if toolboxErr == nil && len(raw) > 0 {
					detailQuery := music.RewardsDetailQuery{
						Region:        q.Region,
						Title:         q.Title,
						TitleStyle:    q.TitleStyle,
						JewelIconPath: q.JewelIconPath,
						ShardIconPath: q.ShardIconPath,
						Profile:       q.Profile,
					}
					if _, buildErr := app.Music.BuildMusicRewardsDetailRequestFromAchievements(detailQuery, raw); buildErr == nil {
						return app.Music.RenderMusicRewardsDetailFromAchievements(detailQuery, raw)
					}
					reason = "Suite 鎶撳寘鏁版嵁鍙敤锛屼絾濂栧姳鏄庣粏鏋勫缓澶辫触锛屼互涓嬩负鍩轰簬鍏紑淇℃伅鐨勪及绠楃粨鏋溿€?
				} else {
					reason = "褰撳墠鏃犳硶璇诲彇 Suite 鎶撳寘濂栧姳鏄庣粏锛屼互涓嬩负鍩轰簬鍏紑淇℃伅鐨勪及绠楃粨鏋溿€?
				}
			}
		}
	}

	var clearCounts []sekaiutils.AnotherUserMusicDifficultyClearCount
	if publicProfileCard != nil && publicProfileCard.Profile != nil {
		if userID := strings.TrimSpace(publicProfileCard.Profile.ID); userID != "" {
			if resp, err := sekaiutils.GetSekaiAPIClient().GetUserProfile(region, userID); err == nil && resp != nil {
				clearCounts = resp.UserMusicDifficultyClearCount
			}
		}
	}

	return app.Music.RenderMusicRewardsBasicEstimate(q, clearCounts, reason)
}

func executeGacha(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	if app.Gachas == nil {
		return nil, fmt.Errorf("gacha service unavailable: sekai client not configured")
	}
	var data []byte
	region := renderregion.Value(r.Region)
	switch r.Mode {
	case "gacha":
		q := gacha.ListQuery{Region: region}
		mergeParams(r.Params, &q)
		data, err = app.Gachas.RenderGachaList(q)
	default:
		return nil, fmt.Errorf("bridge: unsupported gacha mode %q", r.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

func executeDeck(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	var data []byte
	recommendType := ""
	switch r.Mode {
	case "deck-event":
		recommendType = "event"
	case "deck-challenge":
		recommendType = "challenge"
	case "deck-no-event":
		recommendType = "no_event"
	case "deck-bonus":
		recommendType = "bonus"
	case "deck-mysekai":
		recommendType = "mysekai"
	case "deck-score-up":
		var msg string
		err := json.Unmarshal(r.Params, &msg)
		if err != nil {
			return nil, err
		}
		return onebot11.Message{onebot11.Text(msg)}, nil
	default:
		return nil, fmt.Errorf("bridge: unsupported deck mode %q", r.Mode)
	}
	q := deck.AutoQuery{Region: r.Region, RecommendType: recommendType}
	mergeParams(r.Params, &q)
	if err := resolveDeckCharacterSelections(&q, app); err != nil {
		return nil, err
	}
	if err := resolveDeckMusicSelection(&q, app); err != nil {
		return nil, err
	}
	if detail, _ := buildPublicMusicProfiles(r, app); detail != nil {
		q.Profile = detail
	}

	// Try to inject live Toolbox snapshot so the deck controller can operate
	// on real user data even when no local snapshot file is configured.
	deckCtrl := app.Decks
	if snapshot := resolveLiveSnapshot(r, app, false); snapshot != nil {
		deckCtrl = deckCtrl.WithSnapshot(snapshot)
	}

	data, err = deckCtrl.RenderAutoRecommend(q)
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

func resolveDeckMusicSelection(q *deck.AutoQuery, app *renderapp.App) error {
	if q == nil {
		return nil
	}
	if app == nil || app.Music == nil {
		return fmt.Errorf("deck music resolve requires music controller")
	}

	var (
		result *music.CoverResult
		err    error
	)
	if q.MusicID != nil && *q.MusicID > 0 {
		result, err = app.Music.ResolveMusicCover(music.Query{
			Query:  fmt.Sprintf("music%d", *q.MusicID),
			Region: q.Region,
		})
	} else {
		query := strings.TrimSpace(q.MusicQuery)
		if query == "" {
			return nil
		}
		result, err = app.Music.ResolveMusicCoverByTitleOrAlias(music.Query{
			Query:  query,
			Region: q.Region,
		})
	}
	if err != nil {
		return err
	}
	if result == nil || result.Music == nil || result.Music.ID <= 0 {
		return fmt.Errorf("failed to resolve deck music selection")
	}

	q.MusicID = drawing.IntPtr(result.Music.ID)
	q.MusicTitle = result.Music.Title
	q.MusicCoverPath = result.JacketPath
	return nil
}

func resolveDeckCharacterSelections(q *deck.AutoQuery, app *renderapp.App) error {
	if q == nil {
		return nil
	}

	region := renderregion.WithDefault(renderregion.Normalize(q.Region))

	if q.WorldBloomCharacterID == nil && strings.TrimSpace(q.WorldBloomCharacterQuery) != "" {
		charID, err := resolveGameCharacterIDByQuery(context.Background(), app, region, q.WorldBloomCharacterQuery, "deck")
		if err != nil {
			if q.WorldBloomEventTurn == nil && strings.TrimSpace(q.MusicQuery) == "" && isCharacterNotFoundError(err) {
				q.MusicQuery = strings.TrimSpace(q.WorldBloomCharacterQuery)
				q.WorldBloomCharacterQuery = ""
			} else {
				return err
			}
		} else {
			q.WorldBloomCharacterID = drawing.IntPtr(charID)
			q.WorldBloomCharacterQuery = ""
			if strings.TrimSpace(q.EventUnit) == "" {
				q.EventUnit = resolveDeckCharacterUnit(charID)
			}
		}
	}

	if q.ChallengeLiveCharacterID == nil && strings.TrimSpace(q.ChallengeLiveCharacterQuery) != "" {
		charID, err := resolveGameCharacterIDByQuery(context.Background(), app, region, q.ChallengeLiveCharacterQuery, "deck")
		if err != nil {
			if strings.TrimSpace(q.MusicQuery) == "" && isCharacterNotFoundError(err) {
				q.MusicQuery = strings.TrimSpace(q.ChallengeLiveCharacterQuery)
				q.ChallengeLiveCharacterQuery = ""
			} else {
				return err
			}
		} else {
			q.ChallengeLiveCharacterID = drawing.IntPtr(charID)
			q.ChallengeLiveCharacterQuery = ""
		}
	}

	if len(q.FixedCharacterQueries) > 0 {
		for _, raw := range q.FixedCharacterQueries {
			charID, err := resolveGameCharacterIDByQuery(context.Background(), app, region, raw, "deck")
			if err != nil {
				return err
			}
			q.FixedCharacters = append(q.FixedCharacters, charID)
		}
		q.FixedCharacterQueries = nil
	}

	if err := validateDeckCharacterIDs(q.FixedCharacters); err != nil {
		return err
	}
	return nil
}

func validateDeckCharacterIDs(values []int) error {
	if len(values) == 0 {
		return nil
	}
	if len(values) > 5 {
		return fmt.Errorf("鍥哄畾瑙掕壊鏁伴噺涓嶈兘瓒呰繃5涓?)
	}
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			return fmt.Errorf("鍥哄畾瑙掕壊涓嶈兘涓虹┖")
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("鍥哄畾瑙掕壊涓嶈兘閲嶅")
		}
		seen[value] = struct{}{}
	}
	return nil
}

func isCharacterNotFoundError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "鏈壘鍒板搴旇鑹?)
}

func resolveDeckCharacterUnit(charID int) string {
	switch {
	case charID >= 1 && charID <= 4:
		return "light_sound"
	case charID >= 5 && charID <= 8:
		return "idol"
	case charID >= 9 && charID <= 12:
		return "street"
	case charID >= 13 && charID <= 16:
		return "theme_park"
	case charID >= 17 && charID <= 20:
		return "school_refusal"
	case charID >= 21 && charID <= 26:
		return "piapro"
	default:
		return ""
	}
}

func executeEducation(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	var data []byte
	region := renderregion.Value(r.Region)
	regionStr := strings.TrimSpace(r.Region)
	if regionStr == "" {
		regionStr = "jp"
	}
	publicDetailedProfile, _ := buildPublicMusicProfiles(r, app)

	// Resolve user binding and fetch suite data from Toolbox.
	platform := strings.TrimSpace(r.RequesterPlatform)
	platformUserID := strings.TrimSpace(r.RequesterUserID)
	var suiteUID int64
	var suitePlatform, suitePlatformUserID string

	if platform != "" && platformUserID != "" && app.Bindings != nil {
		_, binding, resolveErr := app.Bindings.ResolveUserBinding(
			context.Background(), platform, platformUserID, regionStr)
		if resolveErr == nil && binding != nil && hasUsableSuiteData(binding) {
			if uid, convErr := strconv.ParseInt(binding.PJSKUserID, 10, 64); convErr == nil {
				suiteUID = uid
				suitePlatform = platform
				suitePlatformUserID = platformUserID
			}
		}
	}

	switch r.Mode {
	case "education-challenge":
		q := education.ChallengeLiveQuery{Region: region}
		mergeParams(r.Params, &q)
		q.Profile = publicDetailedProfile
		if suiteUID > 0 {
			suiteJSON, suiteErr := sekaiutils.GetToolboxClient().GetSuiteData(
				regionStr, suiteUID, suitePlatform, suitePlatformUserID)
			if suiteErr == nil && len(suiteJSON) > 0 {
				snapshot, buildErr := buildEducationSnapshot(app, region, suiteJSON)
				if buildErr == nil {
					q.Snapshot = snapshot
				}
			}
		}
		data, err = app.Edu.RenderChallengeLiveDetails(q)

	case "education-bonds":
		req := drawing.BondsRequest{}
		mergeParams(r.Params, &req)
		if len(req.Bonds) == 0 && suiteUID > 0 {
			bondsReq, buildErr := buildBondsRequestFromSuite(
				app, regionStr, suiteUID, suitePlatform, suitePlatformUserID, publicDetailedProfile)
			if buildErr == nil {
				req = *bondsReq
			}
		}
		data, err = app.Edu.RenderBonds(req)

	case "education-leader":
		req := drawing.LeaderCountRequest{}
		mergeParams(r.Params, &req)
		if len(req.LeaderCounts) == 0 && suiteUID > 0 {
			leaderReq, buildErr := buildLeaderCountRequestFromSuite(
				app, regionStr, suiteUID, suitePlatform, suitePlatformUserID, publicDetailedProfile)
			if buildErr == nil {
				req = *leaderReq
			}
		}
		data, err = app.Edu.RenderLeaderCount(req)

	case "education-power":
		req := drawing.PowerBonusDetailRequest{}
		mergeParams(r.Params, &req)
		if len(req.CharaBonuses) == 0 && len(req.UnitBonuses) == 0 && len(req.AttrBonuses) == 0 && suiteUID > 0 {
			builtReq, buildErr := buildPowerBonusRequestFromSuite(
				app, region, regionStr, suiteUID, suitePlatform, suitePlatformUserID, publicDetailedProfile)
			if buildErr != nil {
				return nil, buildErr
			}
			req = *builtReq
		}
		data, err = app.Edu.RenderPowerBonusDetail(req)

	case "education-area":
		query := education.AreaItemQuery{Region: region}
		mergeParams(r.Params, &query)
		if query.Cid <= 0 && strings.TrimSpace(query.CharacterQuery) != "" {
			query.Cid, err = resolveEducationAreaCharacterID(context.Background(), app, region, query.CharacterQuery)
			if err != nil {
				return nil, err
			}
		}
		query.Profile = publicDetailedProfile
		if suiteUID > 0 {
			builtReq, buildErr := buildAreaItemUpgradeMaterialsRequestFromSuite(
				app, query, regionStr, suiteUID, suitePlatform, suitePlatformUserID)
			if buildErr != nil {
				return nil, buildErr
			}
			data, err = app.Edu.RenderAreaItemUpgradeMaterials(*builtReq)
			break
		}
		data, err = app.Edu.RenderAreaItemUpgradeMaterials(drawing.AreaItemUpgradeMaterialsRequest{})

	default:
		return nil, fmt.Errorf("bridge: unsupported education mode %q", r.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

// buildEducationSnapshot creates a userdata.Service from live Toolbox suite data.
func buildEducationSnapshot(app *renderapp.App, region renderregion.Value, suiteJSON []byte) (*userdata.Service, error) {
	return userdata.NewFromBytes(app.Sekai, app.Assets, region, suiteJSON, nil, nil)
}

func buildPowerBonusRequestFromSuite(
	app *renderapp.App, region renderregion.Value, regionStr string, uid int64, platform, platformUserID string,
	profile *drawing.DetailedProfileCardRequest,
) (*drawing.PowerBonusDetailRequest, error) {
	snapshot, err := buildEducationSnapshotFromSuite(app, region, regionStr, uid, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	return app.Edu.BuildPowerBonusDetailRequestFromSnapshot(education.PowerBonusQuery{
		Region:   region,
		Profile:  profile,
		Snapshot: snapshot,
	})
}

func buildAreaItemUpgradeMaterialsRequestFromSuite(
	app *renderapp.App, query education.AreaItemQuery, regionStr string, uid int64, platform, platformUserID string,
) (*drawing.AreaItemUpgradeMaterialsRequest, error) {
	snapshot, err := buildEducationSnapshotFromSuite(app, query.Region, regionStr, uid, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	query.Snapshot = snapshot
	return app.Edu.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(query)
}

func buildEducationSnapshotFromSuite(
	app *renderapp.App, region renderregion.Value, regionStr string, uid int64, platform, platformUserID string,
) (*userdata.Service, error) {
	suiteJSON, err := sekaiutils.GetToolboxClient().GetSuiteData(regionStr, uid, platform, platformUserID)
	if err != nil {
		return nil, fmt.Errorf("fetch suite data: %w", err)
	}
	if len(suiteJSON) == 0 {
		return nil, fmt.Errorf("suite data is empty")
	}
	snapshot, err := buildEducationSnapshot(app, region, suiteJSON)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// resolveLiveSnapshot resolves the requester's Toolbox binding and fetches a
// live snapshot. If needMySekai is true it also fetches mysekai data and merges
// it into the snapshot. Returns nil if the user has no usable binding or if any
// API call fails (callers fall back to the controller-level static snapshot).
func resolveLiveSnapshot(r *parser.ResolvedCommand, app *renderapp.App, needMySekai bool) *userdata.Service {
	platform := strings.TrimSpace(r.RequesterPlatform)
	platformUserID := strings.TrimSpace(r.RequesterUserID)
	if platform == "" || platformUserID == "" || app.Bindings == nil {
		return nil
	}

	regionStr := strings.TrimSpace(r.Region)
	if regionStr == "" {
		regionStr = "jp"
	}

	_, binding, resolveErr := app.Bindings.ResolveUserBinding(
		context.Background(), platform, platformUserID, regionStr)
	if resolveErr != nil || binding == nil || !hasUsableSuiteData(binding) {
		return nil
	}
	uid, convErr := strconv.ParseInt(binding.PJSKUserID, 10, 64)
	if convErr != nil {
		return nil
	}

	tc := sekaiutils.GetToolboxClient()
	suiteJSON, suiteErr := tc.GetSuiteData(regionStr, uid, platform, platformUserID)
	if suiteErr != nil || len(suiteJSON) == 0 {
		return nil
	}

	var mysekaiJSON []byte
	if needMySekai && hasUsableMySekaiData(binding) {
		mysekaiJSON, _ = tc.GetMySekaiData(regionStr, uid, platform, platformUserID)
	}

	region := renderregion.Normalize(regionStr)
	snapshot, err := userdata.NewFromBytes(app.Sekai, app.Assets, region, suiteJSON, mysekaiJSON, nil)
	if err != nil {
		return nil
	}
	return snapshot
}

// resolveMySekaiOnly fetches only the mysekai data from Toolbox, without
// requiring suite data. This is the lightweight fallback when the full
// merged snapshot is unavailable (e.g. the user has mysekai data uploaded
// but GetSuiteData fails). Returns nil on any error.
func resolveMySekaiOnly(r *parser.ResolvedCommand, app *renderapp.App) []byte {
	platform := strings.TrimSpace(r.RequesterPlatform)
	platformUserID := strings.TrimSpace(r.RequesterUserID)
	if platform == "" || platformUserID == "" || app.Bindings == nil {
		return nil
	}

	regionStr := strings.TrimSpace(r.Region)
	if regionStr == "" {
		regionStr = "jp"
	}

	_, binding, resolveErr := app.Bindings.ResolveUserBinding(
		context.Background(), platform, platformUserID, regionStr)
	if resolveErr != nil || binding == nil || !hasUsableMySekaiData(binding) {
		return nil
	}
	uid, convErr := strconv.ParseInt(binding.PJSKUserID, 10, 64)
	if convErr != nil {
		return nil
	}

	tc := sekaiutils.GetToolboxClient()
	data, err := tc.GetMySekaiData(regionStr, uid, platform, platformUserID)
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}

// buildBondsRequestFromSuite fetches bonds data from the Toolbox and builds a BondsRequest.
func buildBondsRequestFromSuite(
	app *renderapp.App, region string, uid int64, platform, platformUserID string,
	profile *drawing.DetailedProfileCardRequest,
) (*drawing.BondsRequest, error) {
	tc := sekaiutils.GetToolboxClient()

	bondsRaw, err := tc.GetPrivateDataValue(region, sekaiutils.ToolboxDataTypeSuite, uid, platform, platformUserID, "userBonds")
	if err != nil {
		return nil, fmt.Errorf("fetch userBonds: %w", err)
	}
	charsRaw, err := tc.GetPrivateDataValue(region, sekaiutils.ToolboxDataTypeSuite, uid, platform, platformUserID, "userCharacters")
	if err != nil {
		return nil, fmt.Errorf("fetch userCharacters: %w", err)
	}

	var suiteBonds []struct {
		BondsGroupID int `json:"bondsGroupId"`
		Rank         int `json:"rank"`
		Exp          int `json:"exp"`
	}
	if err := json.Unmarshal(bondsRaw, &suiteBonds); err != nil {
		return nil, fmt.Errorf("decode userBonds: %w", err)
	}

	var suiteChars []struct {
		CharacterID   int `json:"characterId"`
		CharacterRank int `json:"characterRank"`
	}
	if err := json.Unmarshal(charsRaw, &suiteChars); err != nil {
		return nil, fmt.Errorf("decode userCharacters: %w", err)
	}

	charRankMap := make(map[int]int, len(suiteChars))
	for _, c := range suiteChars {
		charRankMap[c.CharacterID] = c.CharacterRank
	}

	// Look up bonds master data to map group IDs to character pairs.
	ctx := context.Background()
	normalizedRegion := strings.ToLower(strings.TrimSpace(region))
	if normalizedRegion == "" {
		normalizedRegion = "jp"
	}

	bondsMaster, err := app.Sekai.Bond.Query().
		Where(bonddb.ServerRegionEQ(normalizedRegion)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query bonds master: %w", err)
	}
	type bondPair struct {
		CharID1 int
		CharID2 int
	}
	groupToPair := make(map[int]bondPair, len(bondsMaster))
	for _, b := range bondsMaster {
		groupToPair[int(b.GroupID)] = bondPair{CharID1: int(b.CharacterId1), CharID2: int(b.CharacterId2)}
	}

	bondLevels, err := app.Sekai.Level.Query().
		Where(
			leveldb.ServerRegionEQ(normalizedRegion),
			leveldb.LevelTypeEQ("bonds"),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query bonds levels: %w", err)
	}
	levelTotalExp := make(map[int]int, len(bondLevels))
	maxLevel := 0
	for _, item := range bondLevels {
		levelValue := int(item.Level)
		levelTotalExp[levelValue] = int(item.TotalExp)
		if levelValue > maxLevel {
			maxLevel = levelValue
		}
	}

	requiredCharIDs := make(map[int]struct{}, len(suiteBonds)*2)
	for _, sb := range suiteBonds {
		pair, ok := groupToPair[sb.BondsGroupID]
		if !ok {
			continue
		}
		requiredCharIDs[pair.CharID1] = struct{}{}
		requiredCharIDs[pair.CharID2] = struct{}{}
	}

	// Map game_id 鈫?game_character_id for icon paths (e.g., 46 鈫?actual 1-26 range ID)
	gameIDToCharID := make(map[int]int, len(requiredCharIDs))
	charColorMap := make(map[int][]int, len(requiredCharIDs))
	if len(requiredCharIDs) > 0 {
		charIDs := make([]int64, 0, len(requiredCharIDs))
		for charID := range requiredCharIDs {
			charIDs = append(charIDs, int64(charID))
		}
		sort.Slice(charIDs, func(i, j int) bool { return charIDs[i] < charIDs[j] })

		colorRows, err := app.Sekai.Gamecharacterunit.Query().
			Where(
				gamecharacterunitdb.ServerRegionEQ(normalizedRegion),
				gamecharacterunitdb.GameIDIn(charIDs...),
			).
			Order(gamecharacterunitdb.ByID()).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("query gamecharacterunit colors: %w", err)
		}
		for _, row := range colorRows {
			gameID := int(row.GameID)
			charID := int(row.GameCharacterID)
			if charID > 0 {
				gameIDToCharID[gameID] = charID
			}
			if _, ok := charColorMap[gameID]; ok {
				continue
			}
			charColorMap[gameID] = parseBondColorCode(row.ColorCode)
		}
	}

	// resolveCharIcon maps a game_id to its icon path via game_character_id
	resolveCharIcon := func(gameID int) string {
		if mapped, ok := gameIDToCharID[gameID]; ok {
			return charaIconPath(app.Assets, mapped)
		}
		return charaIconPath(app.Assets, gameID)
	}

	bonds := make([]drawing.BondInfo, 0, len(suiteBonds))
	userMaxLevel := 0
	for _, sb := range suiteBonds {
		pair, ok := groupToPair[sb.BondsGroupID]
		if !ok {
			continue
		}

		if sb.Rank > userMaxLevel {
			userMaxLevel = sb.Rank
		}

		info := drawing.BondInfo{
			CharaID1:       pair.CharID1,
			CharaID2:       pair.CharID2,
			CharaIconPath1: resolveCharIcon(pair.CharID1),
			CharaIconPath2: resolveCharIcon(pair.CharID2),
			CharaRank1:     charRankMap[pair.CharID1],
			CharaRank2:     charRankMap[pair.CharID2],
			BondLevel:      sb.Rank,
			HasBond:        true,
			Color1:         defaultBondColor(),
			Color2:         defaultBondColor(),
		}
		if color, ok := charColorMap[pair.CharID1]; ok {
			info.Color1 = color
		}
		if color, ok := charColorMap[pair.CharID2]; ok {
			info.Color2 = color
		}
		if sb.Rank > 0 && sb.Rank < maxLevel {
			currentTotalExp, okCurrent := levelTotalExp[sb.Rank]
			nextTotalExp, okNext := levelTotalExp[sb.Rank+1]
			if okCurrent && okNext {
				needExp := nextTotalExp - currentTotalExp - sb.Exp
				if needExp < 0 {
					needExp = 0
				}
				info.NeedExp = &needExp
			}
		}
		bonds = append(bonds, info)
	}
	if maxLevel == 0 {
		maxLevel = userMaxLevel
	}
	sort.Slice(bonds, func(i, j int) bool {
		if bonds[i].BondLevel != bonds[j].BondLevel {
			return bonds[i].BondLevel > bonds[j].BondLevel
		}
		if bonds[i].CharaID1 != bonds[j].CharaID1 {
			return bonds[i].CharaID1 < bonds[j].CharaID1
		}
		return bonds[i].CharaID2 < bonds[j].CharaID2
	})

	req := &drawing.BondsRequest{
		Bonds:    bonds,
		MaxLevel: maxLevel,
	}
	if profile != nil {
		req.Profile = *profile
	}
	return req, nil
}

// buildLeaderCountRequestFromSuite fetches leader usage data from Toolbox and builds a LeaderCountRequest.
func buildLeaderCountRequestFromSuite(
	app *renderapp.App, region string, uid int64, platform, platformUserID string,
	profile *drawing.DetailedProfileCardRequest,
) (*drawing.LeaderCountRequest, error) {
	tc := sekaiutils.GetToolboxClient()

	raw, err := tc.GetPrivateDataValue(region, sekaiutils.ToolboxDataTypeSuite, uid, platform, platformUserID, "userCharacterLiveUsageCounts")
	if err != nil {
		return nil, fmt.Errorf("fetch userCharacterLiveUsageCounts: %w", err)
	}

	var usageCounts []struct {
		CharacterID            int    `json:"characterId"`
		CharacterLiveUsageType string `json:"characterLiveUsageType"`
		UsageCount             int    `json:"usageCount"`
	}
	if err := json.Unmarshal(raw, &usageCounts); err != nil {
		return nil, fmt.Errorf("decode userCharacterLiveUsageCounts: %w", err)
	}

	// Group by character, pick leader counts.
	type charEntry struct {
		LeaderCount int
		MemberCount int
	}
	charMap := make(map[int]*charEntry)
	for _, u := range usageCounts {
		entry, ok := charMap[u.CharacterID]
		if !ok {
			entry = &charEntry{}
			charMap[u.CharacterID] = entry
		}
		switch u.CharacterLiveUsageType {
		case "leader":
			entry.LeaderCount = u.UsageCount
		case "member":
			entry.MemberCount = u.UsageCount
		}
	}

	leaders := make([]drawing.LeaderCountInfo, 0, len(charMap))
	maxPlay := 0
	for charID, entry := range charMap {
		if entry.LeaderCount > maxPlay {
			maxPlay = entry.LeaderCount
		}
		leaders = append(leaders, drawing.LeaderCountInfo{
			CharaID:       charID,
			CharaIconPath: charaIconPath(app.Assets, charID),
			PlayCount:     entry.LeaderCount,
		})
	}
	sort.Slice(leaders, func(i, j int) bool { return leaders[i].PlayCount > leaders[j].PlayCount })

	req := &drawing.LeaderCountRequest{
		LeaderCounts: leaders,
		MaxPlayCount: maxPlay,
	}
	if profile != nil {
		req.Profile = *profile
	}
	return req, nil
}

// charaIconPath resolves a character icon path using the asset helper.
func charaIconPath(helper *assets.AssetHelper, charID int) string {
	if nickname, ok := assets.CharacterIDToNickname[charID]; ok {
		return assets.ResolveAssetPath(helper, assets.StaticImagesDir,
			filepath.Join("chara_icon", nickname+".png"),
			filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", charID)))
	}
	return assets.ResolveAssetPath(helper, assets.StaticImagesDir,
		filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", charID)))
}

func defaultBondColor() []int {
	return []int{100, 100, 100}
}

func parseBondColorCode(code string) []int {
	colorCode := strings.TrimSpace(strings.TrimPrefix(code, "#"))
	if len(colorCode) != 6 {
		return defaultBondColor()
	}

	result := make([]int, 3)
	for idx := 0; idx < 3; idx++ {
		value, err := strconv.ParseInt(colorCode[idx*2:idx*2+2], 16, 64)
		if err != nil {
			return defaultBondColor()
		}
		result[idx] = int(value)
	}
	return result
}

func executeSK(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	var data []byte
	switch r.Mode {
	case "sk-line":
		if trackerReq, ok := trackerRankQueryFromParams(r); ok {
			if err := prepareTrackerRankQuery(ctx, app, &trackerReq); err != nil {
				return nil, err
			}
			payload, err := app.SK.BuildLineRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			data, err = app.SK.RenderLine(*payload)
			break
		}
		req := sk.LineRequest{}
		mergeParams(r.Params, &req)
		data, err = app.SK.RenderLine(req)
	case "sk-query":
		if trackerReq, ok := trackerRankQueryFromParams(r); ok {
			if err := prepareTrackerRankQuery(ctx, app, &trackerReq); err != nil {
				return nil, err
			}
			payload, err := app.SK.BuildQueryRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			data, err = app.SK.RenderQuery(*payload)
			break
		}
		req := drawing.SKRequest{}
		mergeParams(r.Params, &req)
		data, err = app.SK.RenderQuery(req)
	case "sk-check-room":
		if trackerReq, ok := trackerRankQueryFromParams(r); ok {
			if err := prepareTrackerRankQuery(ctx, app, &trackerReq); err != nil {
				return nil, err
			}
			payload, err := app.SK.BuildCheckRoomRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			data, err = app.SK.RenderCheckRoom(*payload)
			break
		}
		req := drawing.CFRequest{}
		mergeParams(r.Params, &req)
		data, err = app.SK.RenderCheckRoom(req)
	case "sk-speed":
		if trackerReq, ok := trackerRankQueryFromParams(r); ok {
			if err := prepareTrackerRankQuery(ctx, app, &trackerReq); err != nil {
				return nil, err
			}
			payload, err := app.SK.BuildSpeedRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			data, err = app.SK.RenderSpeed(*payload)
			break
		}
		req := drawing.SpeedRequest{}
		mergeParams(r.Params, &req)
		data, err = app.SK.RenderSpeed(req)
	case "sk-player-trace":
		trackerReq, ok := trackerRankQueryFromParams(r)
		if !ok {
			// No params: build a basic query for the requester's own user
			trackerReq = sk.TrackerRankQuery{Region: r.Region}
			if trackerReq.Region == "" {
				trackerReq.Region = "jp"
			}
		}
		if err := resolveTrackerCharacterSelection(ctx, app, &trackerReq); err != nil {
			return nil, err
		}
		// Resolve user ID if not provided
		if trackerReq.UserID == nil {
			if err := resolveTrackerTargetUser(ctx, app, &trackerReq); err != nil || trackerReq.UserID == nil {
				if uid := resolveRequesterGameUID(r, app); uid > 0 {
					trackerReq.UserID = &uid
				}
			}
		}
		if trackerReq.UserID != nil {
			payload, buildErr := app.SK.BuildPlayerTraceFromTracker(trackerReq)
			if buildErr != nil {
				return nil, buildErr
			}
			data, err = app.SK.RenderPlayerTrace(*payload)
			break
		}
		req := drawing.PlayerTraceRequest{}
		mergeParams(r.Params, &req)
		data, err = app.SK.RenderPlayerTrace(req)
	case "sk-rank-trace":
		if trackerReq, ok := trackerRankQueryFromParams(r); ok {
			if err := prepareTrackerRankQuery(ctx, app, &trackerReq); err != nil {
				return nil, err
			}
			payload, err := app.SK.BuildRankTraceRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			data, err = app.SK.RenderRankTrace(*payload)
			break
		}
		req := drawing.RankTraceRequest{}
		mergeParams(r.Params, &req)
		data, err = app.SK.RenderRankTrace(req)
	case "sk-winrate":
		req := drawing.WinRateRequest{}
		mergeParams(r.Params, &req)
		data, err = app.SK.RenderWinRate(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported sk mode %q", r.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

func trackerRankQueryFromParams(r *parser.ResolvedCommand) (sk.TrackerRankQuery, bool) {
	if r == nil || len(r.Params) == 0 {
		return sk.TrackerRankQuery{}, false
	}
	var req sk.TrackerRankQuery
	if err := json.Unmarshal(r.Params, &req); err != nil {
		return sk.TrackerRankQuery{}, false
	}
	if req.Region == "" {
		req.Region = r.Region
	}
	if len(req.Ranks) == 0 && req.EventID == 0 && req.WlCharacterID == nil && req.UserID == nil {
		return sk.TrackerRankQuery{}, false
	}
	return req, true
}

func prepareTrackerRankQuery(ctx context.Context, app *renderapp.App, req *sk.TrackerRankQuery) error {
	if err := resolveTrackerCharacterSelection(ctx, app, req); err != nil {
		return err
	}
	return resolveTrackerTargetUser(ctx, app, req)
}

func resolveTrackerCharacterSelection(ctx context.Context, app *renderapp.App, req *sk.TrackerRankQuery) error {
	if req == nil || req.WlCharacterID != nil {
		return nil
	}
	query := strings.TrimSpace(req.WlCharacterQuery)
	if query == "" {
		return nil
	}
	region := renderregion.WithDefault(renderregion.Normalize(req.Region))
	charID, err := resolveGameCharacterIDByQuery(ctx, app, region, query, "sk")
	if err != nil {
		return err
	}
	req.WlCharacterID = drawing.IntPtr(charID)
	req.WlCharacterQuery = ""
	return nil
}

// resolveRequesterGameUID resolves the game user ID from the requester's binding.
func resolveRequesterGameUID(r *parser.ResolvedCommand, app *renderapp.App) int64 {
	platform := strings.TrimSpace(r.RequesterPlatform)
	platformUserID := strings.TrimSpace(r.RequesterUserID)
	if platform == "" || platformUserID == "" || app.Bindings == nil {
		return 0
	}
	regionStr := strings.TrimSpace(r.Region)
	if regionStr == "" {
		regionStr = "jp"
	}
	_, binding, err := app.Bindings.ResolveUserBinding(context.Background(), platform, platformUserID, regionStr)
	if err != nil || binding == nil {
		return 0
	}
	uid, parseErr := strconv.ParseInt(strings.TrimSpace(binding.PJSKUserID), 10, 64)
	if parseErr != nil || uid <= 0 {
		return 0
	}
	return uid
}

func resolveTrackerTargetUser(ctx context.Context, app *renderapp.App, req *sk.TrackerRankQuery) error {
	if req == nil || req.UserID != nil {
		return nil
	}

	targetPlatform := strings.TrimSpace(req.TargetPlatform)
	targetUserID := strings.TrimSpace(req.TargetUserID)
	if targetPlatform == "" || targetUserID == "" {
		return nil
	}

	if app == nil || app.Bindings == nil || !app.Bindings.IsReady() {
		return fmt.Errorf("鏆備笉鏀寔@鐢ㄦ埛鏌ヨ锛氱粦瀹氭湇鍔℃湭灏辩华锛岃鏀圭敤娓告垙UID")
	}

	bindings, err := app.Bindings.List(ctx, targetPlatform, targetUserID)
	if err != nil {
		return fmt.Errorf("鏃犳硶瑙ｆ瀽@鐢ㄦ埛 %s 鐨勭粦瀹? %w", targetUserID, err)
	}

	selected, ok := pickTrackerBindingByRegion(bindings, req.Region)
	if !ok {
		return fmt.Errorf("@鐢ㄦ埛 %s 鍦?%s 鏈嶆病鏈夊彲鐢ㄧ粦瀹?, targetUserID, strings.ToUpper(strings.TrimSpace(req.Region)))
	}

	uid, parseErr := strconv.ParseInt(strings.TrimSpace(selected.UserID), 10, 64)
	if parseErr != nil || uid <= 0 {
		return fmt.Errorf("@鐢ㄦ埛 %s 鐨勭粦瀹歎ID鏃犳晥: %s", targetUserID, selected.UserID)
	}
	req.UserID = &uid
	return nil
}

func pickTrackerBindingByRegion(bindings []accountdata.BindingListItem, region string) (accountdata.BindingListItem, bool) {
	normalizedRegion := strings.ToLower(strings.TrimSpace(region))
	var (
		serverDefault accountdata.BindingListItem
		hasServer     bool
		globalDefault accountdata.BindingListItem
		hasGlobal     bool
		fallback      accountdata.BindingListItem
		hasFallback   bool
	)

	for _, item := range bindings {
		if !item.Visible {
			continue
		}
		if strings.ToLower(strings.TrimSpace(item.Server)) != normalizedRegion {
			continue
		}
		if item.IsServerDefault {
			serverDefault = item
			hasServer = true
			continue
		}
		if item.IsGlobalDefault && !hasGlobal {
			globalDefault = item
			hasGlobal = true
		}
		if !hasFallback {
			fallback = item
			hasFallback = true
		}
	}

	switch {
	case hasServer:
		return serverDefault, true
	case hasGlobal:
		return globalDefault, true
	case hasFallback:
		return fallback, true
	default:
		return accountdata.BindingListItem{}, false
	}
}

func executeScore(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	if app != nil && app.Music != nil && app.Aliases != nil {
		app.Music.SetAliasResolver(app.Aliases)
	}
	var data []byte
	switch r.Mode {
	case "score-control":
		req := drawing.ScoreControlRequest{}
		mergeParams(r.Params, &req)
		if req.MusicID <= 0 || req.TargetPoint <= 0 || len(req.ValidScores) == 0 {
			reqPtr, resolveErr := requestbuilder.BuildScoreControlRequest(r, app)
			if resolveErr != nil {
				return nil, resolveErr
			}
			req = *reqPtr
		}
		data, err = app.Score.RenderScoreControl(req)
	case "score-custom-room":
		req := drawing.CustomRoomScoreRequest{}
		mergeParams(r.Params, &req)
		if req.TargetPoint <= 0 || len(req.CandidatePairs) == 0 {
			reqPtr, resolveErr := requestbuilder.BuildCustomRoomScoreRequest(r, app)
			if resolveErr != nil {
				return nil, resolveErr
			}
			req = *reqPtr
		}
		data, err = app.Score.RenderCustomRoomScore(req)
	case "score-music-meta":
		var params struct {
			Queries []string `json:"queries"`
		}
		if r.Params != nil {
			if err := json.Unmarshal(r.Params, &params); err != nil {
				return nil, fmt.Errorf("bridge: unmarshal music-meta params: %w", err)
			}
		}
		if len(params.Queries) == 0 {
			params.Queries = splitScoreMusicMetaQueries(r.Query)
		}
		req, resolveErr := app.Music.ResolveMusicMetaRequests(r.Region, params.Queries)
		if resolveErr != nil {
			return nil, resolveErr
		}
		data, err = app.Score.RenderMusicMeta(req)
	case "score-music-board":
		req := drawing.MusicBoardRequest{}
		mergeParams(r.Params, &req)
		if len(req.Items) == 0 {
			if app == nil || app.Music == nil {
				return nil, fmt.Errorf("music board service unavailable: music controller is not configured")
			}
			boardQuery := music.BoardQuery{}
			mergeParams(r.Params, &boardQuery)
			if len(boardQuery.SpecQueries) == 0 {
				boardQuery.SpecQueries = splitScoreMusicMetaQueries(r.Query)
			}
			reqPtr, resolveErr := app.Music.ResolveMusicBoardRequest(r.Region, boardQuery)
			if resolveErr != nil {
				return nil, resolveErr
			}
			req = *reqPtr
		}
		data, err = app.Score.RenderMusicBoard(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported score mode %q", r.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

func splitScoreMusicMetaQueries(args string) []string {
	segments := strings.Split(strings.ReplaceAll(strings.TrimSpace(args), "/", "|"), "|")
	clean := make([]string, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg != "" {
			clean = append(clean, seg)
		}
	}
	return clean
}

func executeProfile(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) (onebot11.Message, error) {
	switch r.Mode {
	case accountdata.ProfileModeRender:
		var p userQueryParams
		mergeParams(r.Params, &p)

		region := r.Region
		if region == "" {
			region = string(renderregion.JP)
		}

		target, err := resolveGameTarget(ctx, p, region, r.RegionExplicit, app)
		if err != nil {
			return nil, err
		}

		resp, err := sekaiutils.GetSekaiAPIClient().GetUserProfile(region, target.PJSKUserID)
		if err != nil {
			return nil, fmt.Errorf("鑾峰彇鐜╁淇℃伅澶辫触锛?w", err)
		}

		if app.Censor != nil {
			harukiID := target.HarukiUserID
			if !app.Censor.CensorName(ctx, harukiID, target.PJSKUserID, resp.User.Name, region) {
				resp.User.Name = ""
			}
			if !app.Censor.CensorShortBio(ctx, harukiID, target.PJSKUserID, resp.UserProfile.Word, region) {
				resp.UserProfile.Word = ""
			}
		}

		// Fetch player frames from the suite snapshot (best-effort; nil = no frame rendered).
		var framesJSON []byte
		if p.Mode == "self" && hasUsableSuiteData(target.Binding) {
			if platform, platformUserID := platformCredentials(p); platform != "" {
				if uid, convErr := strconv.ParseInt(target.PJSKUserID, 10, 64); convErr == nil {
					framesJSON, _ = sekaiutils.GetToolboxClient().GetPrivateDataValue(
						region, sekaiutils.ToolboxDataTypeSuite, uid, platform, platformUserID, "userPlayerFrames")
				}
			}
		}

		q := profile.Query{
			Region:     r.Region,
			Visible:    target.Visible,
			BgSettings: target.BgSettings,
		}
		data, err := app.Profiles.RenderProfileFromAPI(q, resp, framesJSON)
		if err != nil {
			return nil, err
		}
		return imageMessage(data, app, BotModulePJSK)
	case accountdata.ProfileModeBind, accountdata.ProfileModeBindList, accountdata.ProfileModeUnbind, accountdata.ProfileModeDefaultSet, accountdata.ProfileModeDefaultClear:
		if app.Bindings == nil {
			return nil, fmt.Errorf("缁戝畾鏈嶅姟鏈氨缁紝璇风◢鍚庡啀璇?)
		}
		params, err := accountdata.DecodeProfileBindingParams(r.Params)
		if err != nil {
			return nil, err
		}
		data, err := accountdata.ExecuteProfileBindingCommand(ctx, app.Bindings, r.Mode, params)
		if err != nil {
			return nil, err
		}
		return onebot11.Message{onebot11.Text(string(data))}, nil
	case accountdata.ProfileModeHideID, accountdata.ProfileModeShowID,
		accountdata.ProfileModeHideSuite, accountdata.ProfileModeShowSuite,
		accountdata.ProfileModeHideMySekai, accountdata.ProfileModeShowMySekai,
		accountdata.ProfileModeVerify, accountdata.ProfileModeVerifyList,
		accountdata.ProfileModeBGUpload, accountdata.ProfileModeBGClear, accountdata.ProfileModeBGAdjust:
		if app.Bindings == nil {
			return nil, fmt.Errorf("缁戝畾鏈嶅姟鏈氨缁紝璇风◢鍚庡啀璇?)
		}
		params, err := accountdata.DecodeProfileSettingsParams(r.Params)
		if err != nil {
			return nil, err
		}
		data, err := accountdata.ExecuteProfileSettingsCommand(ctx, app.Bindings, r.Mode, params)
		if err != nil {
			return nil, err
		}
		return onebot11.Message{onebot11.Text(string(data))}, nil
	default:
		return nil, fmt.Errorf("bridge: unsupported profile mode %q", r.Mode)
	}
}

func executeMysekai(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	// MySekai is disabled for CN region unless the requester's
	// platform+group is on the whitelist.
	if strings.EqualFold(r.Region, "cn") {
		allowed := false
		for _, entry := range harukiConfig.Cfg.PJSK.AllowCNMySekai {
			if strings.EqualFold(entry.Platform, r.RequesterPlatform) &&
				entry.GroupID == r.RequesterGroupID {
				allowed = true
				break
			}
		}
		if !allowed {
			return onebot11.Message{onebot11.Text("MySekai 鍔熻兘鏆備笉鏀寔鍥芥湇鍖哄煙")}, nil
		}
	}

	var data []byte
	_, publicProfileCard := buildPublicMusicProfiles(r, app)

	// Inject live Toolbox data. Prefer the full snapshot (suite + mysekai
	// merged); fall back to mysekai-only data which is sufficient for all
	// mysekai render modes (profile card comes from the public API override).
	msCtrl := app.MySekai
	if snapshot := resolveLiveSnapshot(r, app, true); snapshot != nil {
		msCtrl = msCtrl.WithSnapshot(snapshot)
	} else if mysekaiData := resolveMySekaiOnly(r, app); mysekaiData != nil {
		msCtrl = msCtrl.WithMySekaiData(mysekaiData)
	}

	switch r.Mode {
	case "mysekai-resource":
		q := mysekai.ResourceQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		q.Profile = publicProfileCard
		data, err = msCtrl.RenderResource(q)
	case "mysekai-map":
		q := mysekai.MapQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		data, err = msCtrl.RenderMap(q)
	case "mysekai-fixture-list":
		q := mysekai.FixtureListQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		q.Profile = publicProfileCard
		data, err = msCtrl.RenderFixtureList(q)
	case "mysekai-fixture-detail":
		q := mysekai.FixtureDetailQuery{Region: r.Region, Query: r.Query}
		mergeParams(r.Params, &q)
		data, err = msCtrl.RenderFixtureDetail(q)
	case "mysekai-door-upgrade":
		q := mysekai.DoorUpgradeQuery{Region: r.Region, Query: r.Query}
		mergeParams(r.Params, &q)
		q.Profile = publicProfileCard
		data, err = msCtrl.RenderDoorUpgrade(q)
	case "mysekai-music-record":
		q := mysekai.MusicRecordQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		q.Profile = publicProfileCard
		data, err = msCtrl.RenderMusicRecord(q)
	case "mysekai-photo":
		q := mysekai.PhotoQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		result, resolveErr := msCtrl.ResolvePhoto(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		data, err = sekaiutils.GetSekaiAPIClient().GetMySekaiImage(result.Region, result.ImagePath)
		if err != nil {
			return nil, fmt.Errorf("鑾峰彇 MySekai 鐓х墖澶辫触锛?w", err)
		}
		image, imageErr := imageMessage(data, app, BotModulePJSK)
		if imageErr != nil {
			return nil, imageErr
		}
		photoTime := "鏈煡"
		if !result.ObtainedAt.IsZero() {
			photoTime = result.ObtainedAt.Format("2006-01-02 15:04")
		}
		return append(image, onebot11.Text(fmt.Sprintf("鎷嶆憚鏃堕棿: %s", photoTime))), nil
	case "mysekai-talk-list":
		q := mysekai.TalkListQuery{Region: r.Region, Query: r.Query}
		mergeParams(r.Params, &q)
		q.Profile = publicProfileCard
		data, err = msCtrl.RenderTalkList(q)
	default:
		return nil, fmt.Errorf("bridge: unsupported mysekai mode %q", r.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

func executeStamp(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	if app.Stamps == nil {
		return nil, fmt.Errorf("stamp service unavailable: sekai client not configured")
	}
	region := renderregion.Value(r.Region)
	switch r.Mode {
	case "stamp-list":
		q := stamp.ListQuery{Region: region}
		mergeParams(r.Params, &q)
		if q.All {
			images, renderErr := app.Stamps.RenderStampListPages(q)
			if renderErr != nil {
				return nil, renderErr
			}
			message = make(onebot11.Message, 0, len(images))
			for _, img := range images {
				segment, imageErr := imageMessage(img, app, BotModulePJSK)
				if imageErr != nil {
					return nil, imageErr
				}
				message = append(message, segment...)
			}
			if len(message) == 0 {
				return nil, fmt.Errorf("stamp all mode did not produce any images")
			}
			return message, nil
		}
		data, renderErr := app.Stamps.RenderStampList(q)
		if renderErr != nil {
			return nil, renderErr
		}
		return imageMessage(data, app, BotModulePJSK)
	default:
		return nil, fmt.Errorf("bridge: unsupported stamp mode %q", r.Mode)
	}
	return nil, fmt.Errorf("bridge: unsupported stamp mode %q", r.Mode)
}

func executeMisc(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	var data []byte
	switch r.Mode {
	case "misc-birthday":
		req := drawing.CharaBirthdayRequest{}
		mergeParams(r.Params, &req)
		if req.Cid <= 0 || req.Month <= 0 || req.Day <= 0 || len(req.Cards) == 0 {
			reqPtr, resolveErr := requestbuilder.BuildMiscBirthdayRequest(r, app)
			if resolveErr != nil {
				return nil, resolveErr
			}
			req = *reqPtr
		}
		data, err = app.Misc.RenderCharaBirthday(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported misc mode %q", r.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

func executeVLive(r *parser.ResolvedCommand, app *renderapp.App) (onebot11.Message, error) {
	if app == nil || app.VLive == nil {
		return nil, fmt.Errorf("vlive service unavailable: sekai client not configured")
	}
	query := vlive.ListQuery{Region: r.Region}
	mergeParams(r.Params, &query)
	text, err := app.VLive.RenderText(query)
	if err != nil {
		return nil, err
	}
	return onebot11.Message{onebot11.Text(text)}, nil
}

// userQueryParams mirrors sekai.UserQueryParams for bridge-side decoding.
type userQueryParams struct {
	Mode           string `json:"mode"`
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	AtUserID       string `json:"at_user_id"`
	PJSKUserID     string `json:"pjsk_user_id"`
	Selector       string `json:"selector,omitempty"`
}

type resolvedGameTarget struct {
	HarukiUserID int
	PJSKUserID   string
	Visible      bool
	BgSettings   *drawing.ProfileBgSettings
	Binding      *accountdata.ResolvedBinding
}

func resolveGameTarget(ctx context.Context, p userQueryParams, region string, regionExplicit bool, app *renderapp.App) (resolvedGameTarget, error) {
	if app == nil || app.Bindings == nil {
		return resolvedGameTarget{}, fmt.Errorf("缁戝畾鏈嶅姟鏈氨缁?)
	}
	switch p.Mode {
	case "self":
		var hid int
		var binding *accountdata.ResolvedBinding
		var err error
		if p.Selector != "" {
			hid, binding, err = app.Bindings.ResolveUserBindingBySelector(ctx, p.Platform, p.PlatformUserID, p.Selector)
		} else if !regionExplicit {
			// No explicit region prefix 鈫?use global default binding directly,
			// so the user's global default account is picked instead of a
			// potentially different server-specific default.
			hid, binding, err = app.Bindings.ResolveUserBinding(ctx, p.Platform, p.PlatformUserID, accountdata.GlobalDefaultBindingScope)
			if err != nil {
				hid, binding, err = app.Bindings.ResolveUserBinding(ctx, p.Platform, p.PlatformUserID, region)
			}
		} else {
			hid, binding, err = app.Bindings.ResolveUserBinding(ctx, p.Platform, p.PlatformUserID, region)
		}
		if err != nil {
			return resolvedGameTarget{}, fmt.Errorf("鏈壘鍒扮粦瀹氳处鍙凤細%w", err)
		}
		return resolvedGameTarget{
			HarukiUserID: hid,
			PJSKUserID:   binding.PJSKUserID,
			Visible:      binding.Visible,
			BgSettings:   binding.Bg,
			Binding:      binding,
		}, nil
	case "at_user":
		_, binding, err := app.Bindings.ResolveUserBinding(ctx, p.Platform, p.AtUserID, region)
		if err != nil {
			return resolvedGameTarget{}, fmt.Errorf("鏈壘鍒拌鐢ㄦ埛鐨勭粦瀹氳处鍙凤細%w", err)
		}
		if !binding.Visible {
			return resolvedGameTarget{}, fmt.Errorf("璇ョ敤鎴峰凡闅愯棌涓汉淇℃伅")
		}
		return resolvedGameTarget{
			PJSKUserID: binding.PJSKUserID,
			Visible:    binding.Visible,
			BgSettings: binding.Bg,
			Binding:    binding,
		}, nil
	case "uid":
		return resolvedGameTarget{
			PJSKUserID: p.PJSKUserID,
			Visible:    true,
		}, nil
	default:
		return resolvedGameTarget{}, fmt.Errorf("鏈煡鐨勬煡璇㈡ā寮忥細%q", p.Mode)
	}
}

// resolveGameUID resolves a game UID from userQueryParams using the identity and
// binding layers in the renderapp. For "at_user" mode the caller's visibility
// is checked; if the target has hidden their profile an error is returned.
//
// Returns (harukiUserID, pjskUserID, visible, error).
// harukiUserID is 0 and visible is true when the mode is "uid".
func resolveGameUID(ctx context.Context, p userQueryParams, region string, regionExplicit bool, app *renderapp.App) (int, string, bool, error) {
	target, err := resolveGameTarget(ctx, p, region, regionExplicit, app)
	if err != nil {
		return 0, "", false, err
	}
	return target.HarukiUserID, target.PJSKUserID, target.Visible, nil
}

func hasUsableSuiteData(binding *accountdata.ResolvedBinding) bool {
	return binding != nil && binding.SuiteVisible
}

// MySekai availability is governed by a dedicated binding flag rather than
// reusing SuiteVisible. This matches the split semantics we want in Cloud and
// keeps suite/mysekai private-data visibility independently configurable.
func hasUsableMySekaiData(binding *accountdata.ResolvedBinding) bool {
	return binding != nil && binding.MySekaiVisible
}

// platformCredentials returns the (platform, platformUserID) pair for toolbox
// key queries. Returns empty strings for "uid" mode (no credentials available).
func platformCredentials(p userQueryParams) (string, string) {
	switch p.Mode {
	case "self":
		return p.Platform, p.PlatformUserID
	case "at_user":
		return p.Platform, p.AtUserID
	default:
		return "", ""
	}
}

func executeArrest(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) (onebot11.Message, error) {
	var p userQueryParams
	mergeParams(r.Params, &p)

	region := r.Region
	if region == "" {
		region = string(renderregion.JP)
	}

	harukiUserID, pjskUserID, visible, err := resolveGameUID(ctx, p, region, r.RegionExplicit, app)
	if err != nil {
		return nil, err
	}
	resp, err := sekaiutils.GetSekaiAPIClient().GetUserProfile(region, pjskUserID)
	if err != nil {
		return nil, fmt.Errorf("鑾峰彇鐜╁淇℃伅澶辫触锛?w", err)
	}

	// Censor user-controlled text (name shown in text output).
	if app.Censor != nil {
		if !app.Censor.CensorName(ctx, harukiUserID, pjskUserID, resp.User.Name, region) {
			resp.User.Name = ""
		}
	}
	queryClient := query.NewClient(nil, nil, app.PJSK, nil)
	// Load the caller's enabled difficulties for self-mode; default for others.
	enabledDiffs := defaultEnabledDiffs()
	if p.Mode == "self" && harukiUserID > 0 && app.PJSK != nil {
		if settings, sErr := queryClient.GetPJSKSettings(ctx, harukiUserID); sErr == nil && settings != nil {
			if len(settings.PJSKEnabledDifficulties) > 0 {
				enabledDiffs = settings.PJSKEnabledDifficulties
			}
		}
	}
	text := formatArrestText(resp, enabledDiffs, resolveArrestChallengeCharacterName(ctx, app, resp.UserChallengeLiveSoloResult.CharacterID), visible)
	return onebot11.Message{onebot11.Text(text)}, nil
}

func defaultEnabledDiffs() []sekaiutils.MusicDifficultyType {
	return []sekaiutils.MusicDifficultyType{
		sekaiutils.MusicDifficultyMaster,
		sekaiutils.MusicDifficultyExpert,
	}
}

func formatArrestText(resp *sekaiutils.GetAnotherProfileResponse, diffs []sekaiutils.MusicDifficultyType, challengeCharacterName string, uidVisible bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("閫崟: %s (UID: %s) Lv.%d\n",
		resp.User.Name, arrestDisplayUID(resp.User.UserID, uidVisible), resp.User.Rank))

	// Index clear counts by difficulty.
	countByDiff := make(map[sekaiutils.MusicDifficultyType]sekaiutils.AnotherUserMusicDifficultyClearCount)
	for _, c := range resp.UserMusicDifficultyClearCount {
		countByDiff[c.MusicDifficultyType] = c
	}

	for _, diff := range diffs {
		c, ok := countByDiff[diff]
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("[%s] Clear:%d FC:%d AP:%d\n",
			diff, c.LiveClear, c.FullCombo, c.AllPerfect))
	}

	if resp.UserChallengeLiveSoloResult.HighScore > 0 {
		sb.WriteString(fmt.Sprintf("鎸戞垬Live(%s): %s鍒?,
			arrestChallengeCharacterLabel(resp.UserChallengeLiveSoloResult.CharacterID, challengeCharacterName),
			formatInt(resp.UserChallengeLiveSoloResult.HighScore)))
	}

	return strings.TrimRight(sb.String(), "\n")
}

func resolveArrestChallengeCharacterName(ctx context.Context, app *renderapp.App, characterID int) string {
	if characterID <= 0 || app == nil || app.Sekai == nil {
		return ""
	}

	rows, err := app.Sekai.Gamecharacter.Query().
		Where(gamecharacterdb.GameIDEQ(int64(characterID))).
		All(ctx)
	if err != nil || len(rows) == 0 {
		return ""
	}

	bestName := ""
	bestRank := 999
	for _, row := range rows {
		candidates := []string{
			strings.TrimSpace(row.FirstName + row.GivenName),
			strings.TrimSpace(strings.TrimSpace(row.FirstName) + " " + strings.TrimSpace(row.GivenName)),
			strings.TrimSpace(row.FirstNameEnglish + row.GivenNameEnglish),
			strings.TrimSpace(strings.TrimSpace(row.FirstNameEnglish) + " " + strings.TrimSpace(row.GivenNameEnglish)),
		}
		for _, candidate := range candidates {
			if candidate == "" {
				continue
			}
			rank := arrestCharacterRegionRank(row.ServerRegion)
			if rank < bestRank {
				bestRank = rank
				bestName = candidate
				break
			}
		}
	}
	return strings.TrimSpace(bestName)
}

func arrestChallengeCharacterLabel(characterID int, resolvedName string) string {
	if name := strings.TrimSpace(resolvedName); name != "" {
		return name
	}
	return fmt.Sprintf("瑙掕壊#%d", characterID)
}

func resolveEducationAreaCharacterID(ctx context.Context, app *renderapp.App, region renderregion.Value, query string) (int, error) {
	return resolveGameCharacterIDByQuery(ctx, app, region, query, "education area")
}

func resolveGameCharacterIDByQuery(
	ctx context.Context,
	app *renderapp.App,
	region renderregion.Value,
	query string,
	serviceLabel string,
) (int, error) {
	if app == nil || app.Sekai == nil {
		return 0, fmt.Errorf("%s service unavailable: sekai client not configured", serviceLabel)
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return 0, fmt.Errorf("璇疯緭鍏ヨ鑹插悕")
	}

	target := normalizeGameCharacterText(query)
	if target == "" {
		return 0, fmt.Errorf("璇疯緭鍏ヨ鑹插悕")
	}
	if app.Aliases != nil {
		if charID, ok, err := app.Aliases.TryResolveCharacterID(ctx, query); err != nil {
			return 0, err
		} else if ok && charID > 0 {
			return charID, nil
		}
	}

	rows, err := app.Sekai.Gamecharacter.Query().
		Where(gamecharacterdb.ServerRegionEQ(region.String())).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query %s characters failed: %w", serviceLabel, err)
	}
	ids := matchGameCharacterIDs(rows, target)
	if len(ids) == 0 {
		rows, err = app.Sekai.Gamecharacter.Query().
			Where(gamecharacterdb.GameIDGT(0)).
			All(ctx)
		if err != nil {
			return 0, fmt.Errorf("query %s characters failed: %w", serviceLabel, err)
		}
		ids = matchGameCharacterIDs(rows, target)
	}

	switch len(ids) {
	case 0:
		return 0, fmt.Errorf("鏈壘鍒板搴旇鑹? %s", query)
	case 1:
		return ids[0], nil
	default:
		return 0, fmt.Errorf("瑙掕壊鍚嶅瓨鍦ㄦ涔? %s", query)
	}
}

func matchGameCharacterIDs(rows []*sekaidb.Gamecharacter, target string) []int {
	if target == "" {
		return nil
	}

	matched := make(map[int]struct{})
	for _, row := range rows {
		if row == nil || row.GameID <= 0 {
			continue
		}
		for _, candidate := range gameCharacterNames(row) {
			if normalizeGameCharacterText(candidate) != target {
				continue
			}
			matched[int(row.GameID)] = struct{}{}
			break
		}
	}
	if len(matched) == 0 {
		return nil
	}

	ids := make([]int, 0, len(matched))
	for id := range matched {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func gameCharacterNames(row *sekaidb.Gamecharacter) []string {
	values := make([]string, 0, 8)
	appendGameCharacterName(&values, strings.TrimSpace(row.FirstName+row.GivenName))
	appendGameCharacterName(&values, strings.TrimSpace(strings.TrimSpace(row.FirstName)+" "+strings.TrimSpace(row.GivenName)))
	appendGameCharacterName(&values, strings.TrimSpace(row.FirstNameEnglish+row.GivenNameEnglish))
	appendGameCharacterName(&values, strings.TrimSpace(strings.TrimSpace(row.FirstNameEnglish)+" "+strings.TrimSpace(row.GivenNameEnglish)))
	appendGameCharacterName(&values, row.FirstName)
	appendGameCharacterName(&values, row.GivenName)
	appendGameCharacterName(&values, row.FirstNameEnglish)
	appendGameCharacterName(&values, row.GivenNameEnglish)
	return values
}

func appendGameCharacterName(values *[]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, existing := range *values {
		if strings.EqualFold(existing, value) {
			return
		}
	}
	*values = append(*values, value)
}

func normalizeGameCharacterText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), ""))
}

func arrestDisplayUID(uid int64, visible bool) string {
	raw := strconv.FormatInt(uid, 10)
	if visible || len(raw) <= 6 {
		return raw
	}
	return raw[:3] + strings.Repeat("*", len(raw)-6) + raw[len(raw)-3:]
}

func arrestCharacterRegionRank(region string) int {
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "jp":
		return 0
	case "cn":
		return 1
	case "tw":
		return 2
	case "en":
		return 3
	case "kr":
		return 4
	default:
		return 999
	}
}

// formatInt formats an integer with comma separators (e.g. 3011947 鈫?"3,011,947").
func formatInt(n int) string {
	if n < 0 {
		return "-" + formatInt(-n)
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var buf strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		buf.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += 3 {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(s[i : i+3])
	}
	return buf.String()
}

func executeRegTime(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) (onebot11.Message, error) {
	var p userQueryParams
	mergeParams(r.Params, &p)

	region := r.Region
	if region == "" {
		region = string(renderregion.JP)
	}

	target, err := resolveGameTarget(ctx, p, region, r.RegionExplicit, app)
	if err != nil {
		return nil, err
	}
	pjskUserID := target.PJSKUserID
	bindingServer := region
	if target.Binding != nil && target.Binding.Server != "" {
		bindingServer = target.Binding.Server
	}

	ts, err := calcRegistrationTime(pjskUserID, bindingServer)
	if err != nil {
		return nil, err
	}

	var tzOffset string
	if app.PJSK != nil && target.HarukiUserID > 0 {
		if settings, sErr := query.NewClient(nil, nil, app.PJSK, nil).GetPJSKSettings(ctx, target.HarukiUserID); sErr == nil && settings != nil {
			tzOffset = settings.TimeZoneOffset
		}
	}
	tz, tzLabel := parseUserTimeZone(tzOffset)
	regTime := time.Unix(ts, 0).In(tz)
	relDur := formatRelativeDuration(time.Since(time.Unix(ts, 0)))
	maskedUID := maskPJSKUID(pjskUserID, target.Visible)

	text := fmt.Sprintf("UID %s 鐨勬敞鍐屾椂闂?\n%s (%s) (%s)",
		maskedUID, regTime.Format("2006-01-02 15:04:05"), tzLabel, relDur)
	return onebot11.Message{onebot11.Text(text)}, nil
}

func executeCheckData(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) (onebot11.Message, error) {
	var p userQueryParams
	mergeParams(r.Params, &p)

	region := r.Region
	if region == "" {
		region = string(renderregion.JP)
	}

	var dataType sekaiutils.ToolboxDataType
	var label string
	var uid int64
	var platform string
	var platformUserID string
	var pjskUID string
	var bindingVisible bool
	var resolvedHarukiID int
	var bindingServer string

	// Helper to resolve user binding, supporting u[i] selector.
	// Returns (binding, harukiUserID, error).
	resolveCheckDataBinding := func() (*accountdata.ResolvedBinding, int, error) {
		var hid int
		var binding *accountdata.ResolvedBinding
		var err error
		if p.Selector != "" {
			hid, binding, err = app.Bindings.ResolveUserBindingBySelector(ctx, p.Platform, p.PlatformUserID, p.Selector)
		} else if !r.RegionExplicit {
			hid, binding, err = app.Bindings.ResolveUserBinding(ctx, p.Platform, p.PlatformUserID, accountdata.GlobalDefaultBindingScope)
			if err != nil {
				hid, binding, err = app.Bindings.ResolveUserBinding(ctx, p.Platform, p.PlatformUserID, region)
			}
		} else {
			hid, binding, err = app.Bindings.ResolveUserBinding(ctx, p.Platform, p.PlatformUserID, region)
		}
		if err != nil {
			return nil, 0, fmt.Errorf("鏈壘鍒扮粦瀹氳处鍙凤細%w", err)
		}
		return binding, hid, nil
	}

	switch r.Mode {
	case "mysekai":
		if p.Mode != "self" {
			return nil, fmt.Errorf("MySekai鎶撳寘鐩稿叧鍐呭浠呮敮鎸佹煡璇㈣嚜宸辩殑鏁版嵁")
		}
		binding, hid, err := resolveCheckDataBinding()
		if err != nil {
			return nil, err
		}
		if !hasUsableMySekaiData(binding) {
			return nil, fmt.Errorf("褰撳墠璐﹀彿娌℃湁鍙敤鐨?MySekai 鎶撳寘鏁版嵁")
		}
		uid, err = strconv.ParseInt(binding.PJSKUserID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("鏃犳晥鐨勮处鍙稩D锛?w", err)
		}
		platform = p.Platform
		platformUserID = p.PlatformUserID
		dataType = sekaiutils.ToolboxDataTypeMySekai
		label = "MySekai"
		pjskUID = binding.PJSKUserID
		bindingVisible = binding.Visible
		resolvedHarukiID = hid
		bindingServer = binding.Server
	default:
		if p.Mode != "self" {
			return nil, fmt.Errorf("Suite鎶撳寘鐩稿叧鍐呭浠呮敮鎸佹煡璇㈣嚜宸辩殑鏁版嵁")
		}
		binding, hid, err := resolveCheckDataBinding()
		if err != nil {
			return nil, err
		}
		if !hasUsableSuiteData(binding) {
			return nil, fmt.Errorf("褰撳墠璐﹀彿娌℃湁鍙敤鐨?Suite 鎶撳寘鏁版嵁")
		}
		uid, err = strconv.ParseInt(binding.PJSKUserID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("鏃犳晥鐨勮处鍙稩D锛?w", err)
		}
		platform = p.Platform
		platformUserID = p.PlatformUserID
		dataType = sekaiutils.ToolboxDataTypeSuite
		label = "Suite"
		pjskUID = binding.PJSKUserID
		bindingVisible = binding.Visible
		resolvedHarukiID = hid
		bindingServer = binding.Server
	}

	if bindingServer == "" {
		bindingServer = region
	}

	raw, err := sekaiutils.GetToolboxClient().GetUploadTime(bindingServer, dataType, uid, platform, platformUserID)
	if err != nil {
		return nil, fmt.Errorf("鑾峰彇%s鏇存柊鏃堕棿澶辫触锛?w", label, err)
	}

	ts, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("瑙ｆ瀽鏇存柊鏃堕棿澶辫触锛?w", err)
	}

	var tzOffset string
	if app.PJSK != nil && resolvedHarukiID > 0 {
		if settings, sErr := query.NewClient(nil, nil, app.PJSK, nil).GetPJSKSettings(ctx, resolvedHarukiID); sErr == nil && settings != nil {
			tzOffset = settings.TimeZoneOffset
		}
	}
	tz, tzLabel := parseUserTimeZone(tzOffset)
	uploadTime := time.Unix(ts, 0).In(tz)
	relDur := formatRelativeDuration(time.Since(time.Unix(ts, 0)))
	maskedUID := maskPJSKUID(pjskUID, bindingVisible)

	text := fmt.Sprintf("UID %s 鐨?s鏁版嵁鏇存柊鏃堕棿:\n%s (%s) (%s)",
		maskedUID, label, uploadTime.Format("2006-01-02 15:04:05"), tzLabel, relDur)
	return onebot11.Message{onebot11.Text(text)}, nil
}

// maskPJSKUID masks the middle digits of a PJSK user ID when visible is false.
// Shows first 3 and last 3 digits with asterisks in between.
func maskPJSKUID(uid string, visible bool) string {
	if visible || len(uid) <= 6 {
		return uid
	}
	return uid[:3] + strings.Repeat("*", len(uid)-6) + uid[len(uid)-3:]
}

// parseUserTimeZone parses a timezone offset string (e.g. "+09:00") into a
// time.Location and human-readable label. Empty string defaults to UTC+8.
func parseUserTimeZone(offset string) (*time.Location, string) {
	offset = strings.TrimSpace(offset)
	if offset == "" {
		return time.FixedZone("UTC+8", 8*3600), "UTC+8"
	}
	sign := 1
	raw := offset
	if strings.HasPrefix(raw, "-") {
		sign = -1
		raw = raw[1:]
	} else if strings.HasPrefix(raw, "+") {
		raw = raw[1:]
	}
	parts := strings.SplitN(raw, ":", 2)
	hours, err := strconv.Atoi(parts[0])
	if err != nil || hours < 0 || hours > 14 {
		return time.FixedZone("UTC+8", 8*3600), "UTC+8"
	}
	minutes := 0
	if len(parts) == 2 {
		minutes, _ = strconv.Atoi(parts[1])
	}
	totalSecs := sign * (hours*3600 + minutes*60)
	var label string
	if sign < 0 {
		label = fmt.Sprintf("UTC-%d", hours)
	} else {
		label = fmt.Sprintf("UTC+%d", hours)
	}
	if minutes != 0 {
		label = fmt.Sprintf("%s:%02d", label, minutes)
	}
	return time.FixedZone(label, totalSecs), label
}

// formatRelativeDuration formats a duration as a human-readable Chinese relative time.
// Only shows units starting from the largest non-zero unit down to minutes.
// e.g. "绾?澶?灏忔椂30鍒嗛挓鍓?, "绾?灏忔椂30鍒嗛挓鍓?, "绾?0鍒嗛挓鍓?, "鍒氬垰"
func formatRelativeDuration(d time.Duration) string {
	if d < time.Minute {
		return "鍒氬垰"
	}
	mins := int(d.Minutes()) % 60
	hrs := int(d.Hours()) % 24
	days := int(d.Hours()) / 24
	if days > 0 {
		if hrs == 0 && mins == 0 {
			return fmt.Sprintf("绾?d澶╁墠", days)
		}
		return fmt.Sprintf("绾?d澶?d灏忔椂%d鍒嗛挓鍓?, days, hrs, mins)
	}
	if hrs > 0 {
		if mins == 0 {
			return fmt.Sprintf("绾?d灏忔椂鍓?, hrs)
		}
		return fmt.Sprintf("绾?d灏忔椂%d鍒嗛挓鍓?, hrs, mins)
	}
	return fmt.Sprintf("绾?d鍒嗛挓鍓?, mins)
}

// calcRegistrationTime derives the approximate Unix registration timestamp from
// a PJSK game user ID and server region.
//
// JP/EN: the upper bits encode seconds since 2020-09-16T03:00:00 UTC.
// TW/KR/CN: the raw bits encode an absolute Unix timestamp.
func calcRegistrationTime(userID string, server string) (int64, error) {
	switch strings.ToLower(server) {
	case "jp", "en":
		if len(userID) <= 3 {
			return 0, fmt.Errorf("璐﹀彿ID鏍煎紡涓嶆纭?)
		}
		n, err := strconv.ParseInt(userID[:len(userID)-3], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("鏃犳晥鐨勮处鍙稩D锛?w", err)
		}
		return 1600218000 + int64(float64(n)/(1024*4096)), nil
	case "tw", "kr", "cn":
		n, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("鏃犳晥鐨勮处鍙稩D锛?w", err)
		}
		return int64(float64(n) / (1024 * 1024 * 4096)), nil
	default:
		return 0, fmt.Errorf("涓嶆敮鎸佺殑鏈嶅姟鍣細%s", server)
	}
}

// mergeParams unmarshals the JSON params from ResolvedCommand into the target struct,
// allowing handler-set fields to override defaults. Fields not present in params
// remain at their zero/pre-set values.
func mergeParams(params json.RawMessage, target interface{}) {
	if len(params) == 0 {
		return
	}
	_ = json.Unmarshal(params, target)
}

// resolveRegionFromDefaultBinding resolves the region for a command where the
// user did not provide an explicit region prefix or -r flag. It looks up the
// user's global default binding (server = "default") and returns the server of
// the bound account (e.g. "jp", "tw", "kr", "en"). Falls back to "jp" if the
// user has no global default binding or if any lookup fails.
func resolveRegionFromDefaultBinding(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) string {
	if r.RegionExplicit {
		return r.Region
	}
	platform := strings.TrimSpace(r.RequesterPlatform)
	platformUserID := strings.TrimSpace(r.RequesterUserID)
	if platform == "" || platformUserID == "" || app.Bindings == nil {
		return r.Region
	}
	_, binding, err := app.Bindings.ResolveUserBinding(ctx, platform, platformUserID, accountdata.GlobalDefaultBindingScope)
	if err != nil || binding == nil || strings.TrimSpace(binding.Server) == "" {
		return r.Region
	}
	normalized := renderregion.Normalize(binding.Server)
	if normalized.IsZero() {
		return r.Region
	}
	return normalized.String()
}


