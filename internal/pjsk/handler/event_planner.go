package handler

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/eventutil"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderprovider "haruki-cloud/internal/pjsk/render/provider"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

const eventPlannerHelp = `活动规划用法:
/cn活动规划 pt1000w
/cn活动规划 pt1000w 当前pt120w 歌 虾ex 龙hd
/jp活动规划 t100 event202 knd 歌 野车 10火
/cn活动规划 pt1200w wl3 mzk #123 456 789 101 112 队友综合25w 队友实效200

必须在指令前写区服: jp/cn/en/tw/kr。
参数: pt/目标, t排名, 当前pt, 1-10火, 歌曲/难度, 野车, event活动ID, wl章节角色, #固定卡/角色, 当前/顶配/画布/已读/队友综合/队友实效等活动组卡参数。
不写歌曲时默认算虾 EXPERT 和 龙 HARD；不写火数时默认算 5火 和 10火；不写卡组时默认使用最优卡组。`

const eventPlannerLostAndFoundMusicID = 226
const eventPlannerOmakaseMusicID = 10000

type eventPlannerCommandParams struct {
	EventID         int                         `json:"event_id,omitempty"`
	TargetPoint     int64                       `json:"target_point,omitempty"`
	TargetRank      int                         `json:"target_rank,omitempty"`
	CurrentPoint    int64                       `json:"current_point,omitempty"`
	CurrentPointSet bool                        `json:"current_point_set,omitempty"`
	Deck            deckAutoQueryParams         `json:"deck,omitempty"`
	Songs           []eventPlannerSongSelection `json:"songs,omitempty"`
	Boosts          []int                       `json:"boosts,omitempty"`
}

type eventPlannerSongSelection struct {
	Query      string `json:"query"`
	Difficulty string `json:"difficulty,omitempty"`
	MusicID    int    `json:"music_id,omitempty"`
}

var (
	eventPlannerTargetRankRE   = regexp.MustCompile(`(?i)(?:^|\s)t\s*([0-9]+)|([0-9]+)\s*名`)
	eventPlannerTargetPointRE  = regexp.MustCompile(`(?i)(?:目标pt|目标|pt|打到)\s*([0-9][0-9,._]*(?:万|億|亿|w|k)?)`)
	eventPlannerCurrentPointRE = regexp.MustCompile(`(?i)(?:当前pt|已有pt|已打|现在pt)\s*([0-9][0-9,._]*(?:万|億|亿|w|k)?)`)
	eventPlannerBoostRE        = regexp.MustCompile(`([0-9]{1,2})\s*火`)
)

var eventPlannerBoostMultipliers = map[int]int64{
	0:  1,
	1:  5,
	2:  10,
	3:  15,
	4:  19,
	5:  23,
	6:  26,
	7:  29,
	8:  31,
	9:  33,
	10: 35,
}

func (sekaiHandlers) EventPlannerHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "event/planner",
			Commands: []string{
				"/活动规划", "/pjsk event planner", "/event-planner",
			},
			Helper: eventPlannerHelp,
		},
		Regions: AllRegions,
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			if !ctx.HasExplicitRegion() {
				return nil, onebot11.NewReplayError("需要在指令前加区服，例如：/cn活动规划 pt500w 当前\n\n%s", eventPlannerHelp)
			}
			if ctx.Flags()["is_help"] {
				return makeCommandRequestWithParams(ctx, parser.ModuleEvent, "event-planner-help", nil), nil
			}
			params, err := parseEventPlannerParams(ctx.GetArgs(), ctx.originalTriggerCmd)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleEvent, "event-planner", params), nil
		},
	}, executeEvent)
}

func parseEventPlannerParams(args string, trigger string) (eventPlannerCommandParams, error) {
	args = strings.TrimSpace(args)
	if args == "" || eventPlannerContainsAny(args, "help", "帮助", "用法", "?") {
		return eventPlannerCommandParams{}, onebot11.NewReplayError("%s", eventPlannerHelp)
	}

	params := eventPlannerCommandParams{
		Boosts: []int{5, 10},
	}
	params.TargetRank = parseEventPlannerRank(args)
	if value, ok := parseEventPlannerPointWithRE(eventPlannerCurrentPointRE, args); ok {
		params.CurrentPoint = value
		params.CurrentPointSet = true
	}
	targetArgs := eventPlannerCurrentPointRE.ReplaceAllString(args, " ")
	if value, ok := parseEventPlannerPointWithRE(eventPlannerTargetPointRE, targetArgs); ok {
		params.TargetPoint = value
	}
	deckArgs := eventPlannerCurrentPointRE.ReplaceAllString(args, " ")
	deckArgs = eventPlannerTargetPointRE.ReplaceAllString(deckArgs, " ")
	deckArgs = eventPlannerTargetRankRE.ReplaceAllString(deckArgs, " ")
	if params.TargetPoint == 0 {
		var bareTarget int64
		bareTarget, deckArgs = parseEventPlannerBareTargetPoint(deckArgs)
		params.TargetPoint = bareTarget
	}
	var err error
	params.Boosts, deckArgs, err = parseEventPlannerBoosts(deckArgs, params.Boosts)
	if err != nil {
		return eventPlannerCommandParams{}, err
	}
	params.Songs, deckArgs = parseEventPlannerSongs(deckArgs)
	if len(params.Songs) > 0 {
		applyEventPlannerDefaultSongDifficulties(params.Songs)
	}
	if err := parseEventPlannerDeckParams(deckArgs, &params.Deck, trigger); err != nil {
		return eventPlannerCommandParams{}, err
	}
	if params.Deck.EventID != nil {
		params.EventID = *params.Deck.EventID
	}

	if params.TargetPoint == 0 && params.TargetRank == 0 {
		return eventPlannerCommandParams{}, onebot11.NewReplayError("需要提供目标 pt 或目标排名，例如：/cn活动规划 pt500w 当前\n\n%s", eventPlannerHelp)
	}
	return params, nil
}

func parseEventPlannerDeckParams(args string, params *deckAutoQueryParams, trigger string) error {
	if params == nil {
		return nil
	}
	args = strings.TrimSpace(strings.ToLower(args))
	var err error
	args, err = buildEventDeckParams(args, params, trigger)
	if err != nil {
		return err
	}
	if err := validateDeckQueryParams(params); err != nil {
		return err
	}
	if args = strings.TrimSpace(args); args != "" {
		params.Args = args
	}
	return nil
}

func executeEventPlanner(rc *RequestContext) (onebot11.Message, error) {
	var params eventPlannerCommandParams
	mergeParams(rc.Cmd.Params, &params)

	if rc.App == nil || rc.App.Decks == nil {
		return nil, fmt.Errorf("deck service unavailable: deck controller not configured")
	}
	if rc.App.Music == nil {
		return nil, fmt.Errorf("music service unavailable: music controller not configured")
	}

	binding, snap, err := rc.requireVisibleSuiteSnapshot()
	if err != nil {
		if errors.Is(err, accountdata.ErrNoBinding) {
			return nil, newBindingRequiredReplayError()
		}
		return nil, err
	}
	if snap == nil {
		return nil, newSuiteDataNotFoundReplayErrorForBinding(binding)
	}

	region := renderregion.WithDefault(renderregion.Normalize(rc.Cmd.Region))
	baseQuery := buildEventPlannerBaseDeckQuery(region, params.Deck)
	if err := resolveDeckCharacterSelections(rc.Ctx, &baseQuery, rc.App); err != nil {
		return nil, err
	}
	eventInfo, eventWarning, err := resolveEventPlannerEventFromQuery(rc.Ctx, rc.App, region, baseQuery)
	if err != nil {
		return nil, err
	}
	if baseQuery.EventID == nil || *baseQuery.EventID <= 0 {
		if eventInfo.ID > 0 {
			baseQuery.EventID = drawing.IntPtr(eventInfo.ID)
		}
	}
	targetPoint, targetSource, err := resolveEventPlannerTargetPoint(rc, region, eventInfo.ID, params)
	if err != nil {
		return nil, err
	}
	currentPoint, currentWarning := resolveEventPlannerCurrentPoint(snap, eventInfo.ID, params)

	songs := eventPlannerSongsForRequest(params, baseQuery)
	req, err := buildEventPlannerDrawingRequest(rc, region, eventInfo, snap, baseQuery, params, songs, targetPoint, targetSource, currentPoint)
	if err != nil {
		return nil, err
	}
	if eventWarning != "" {
		req.Warnings = append(req.Warnings, eventWarning)
	}
	if currentWarning != "" {
		req.Warnings = append(req.Warnings, currentWarning)
	}

	data, err := rc.App.Drawing.GenerateEventPlanner(req)
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

func buildEventPlannerDrawingRequest(
	rc *RequestContext,
	region renderregion.Value,
	eventInfo *masterdata.Event,
	snap rendersnapshot.Snapshot,
	baseQuery renderdeck.AutoQuery,
	params eventPlannerCommandParams,
	songs []eventPlannerSongSelection,
	targetPoint int64,
	targetSource string,
	currentPoint int64,
) (*drawing.EventPlannerRequest, error) {
	if eventInfo == nil {
		return nil, fmt.Errorf("活动数据为空")
	}

	remaining := targetPoint - currentPoint
	if remaining < 0 {
		remaining = 0
	}
	req := &drawing.EventPlannerRequest{
		Title:           "活动规划",
		Region:          region.String(),
		EventID:         eventInfo.ID,
		EventName:       eventInfo.Name,
		EventBannerPath: eventPlannerEventBannerPath(rc.App, region, eventInfo),
		TargetPoint:     targetPoint,
		CurrentPoint:    currentPoint,
		RemainingPoint:  remaining,
		DailyPoint:      eventPlannerDailyPoint(targetPoint, eventInfo.StartAt, eventInfo.AggregateAt),
		TargetSource:    targetSource,
	}

	deckCtrl := rc.App.Decks.WithContext(rc.Ctx).WithSnapshot(snap)
	for _, selection := range songs {
		deckReq, err := buildEventPlannerSongDeckRequest(deckCtrl, rc.App, region, eventInfo.ID, baseQuery, selection)
		if err != nil {
			return nil, err
		}
		if len(deckReq.DeckData) == 0 {
			return nil, fmt.Errorf("组卡服务没有返回 %s 的卡组数据", selection.Query)
		}
		deckData := deckReq.DeckData[0]
		basePoint := eventPlannerIntValue(deckData.Score)
		if basePoint <= 0 {
			return nil, fmt.Errorf("组卡服务返回的 %s PT 为 0", selection.Query)
		}
		planSong := drawing.EventPlannerSong{
			Query:          selection.Query,
			Title:          eventPlannerStringValue(deckReq.MusicTitle, selection.Query),
			Difficulty:     eventPlannerStringValue(deckReq.MusicDiff, selection.Difficulty),
			MusicCoverPath: eventPlannerStringValue(deckReq.MusicCoverPath, ""),
		}
		if deckReq.MusicID != nil {
			planSong.MusicID = *deckReq.MusicID
		}
		for _, boost := range params.Boosts {
			point := int64(basePoint) * eventPlannerBoostMultiplier(boost)
			plays := int64(0)
			if remaining > 0 && point > 0 {
				plays = int64(math.Ceil(float64(remaining) / float64(point)))
			}
			planSong.Rows = append(planSong.Rows, drawing.EventPlannerBoostRow{
				Boost:        boost,
				PointPerPlay: point,
				Plays:        plays,
				Energy:       plays * int64(boost),
			})
		}
		req.Songs = append(req.Songs, planSong)

		if len(req.DeckCards) == 0 {
			req.DeckRequest = deckReq
			req.Profile = &deckReq.Profile
			if deckReq.EventBannerPath != nil && strings.TrimSpace(*deckReq.EventBannerPath) != "" {
				req.EventBannerPath = *deckReq.EventBannerPath
			}
			req.LiveName = eventPlannerStringValue(deckReq.LiveName, "")
			req.DeckCards = eventPlannerDeckCards(deckData.CardData)
			req.DeckTotalPower = eventPlannerIntValue(deckData.TotalPower)
			req.DeckEventBonus = eventPlannerFloatValue(deckData.EventBonusRate)
			req.DeckSkillUp = eventPlannerFloatValue(deckData.MultiLiveScoreUp)
			req.DeckSummary = buildEventPlannerDeckSummary(baseQuery, req.DeckTotalPower, req.DeckEventBonus, req.DeckSkillUp)
		}
	}
	if len(req.Songs) == 0 {
		return nil, fmt.Errorf("没有可计算的歌曲")
	}
	return req, nil
}

func buildEventPlannerSongDeckRequest(
	deckCtrl *renderdeck.Controller,
	app *renderapp.App,
	region renderregion.Value,
	eventID int,
	baseQuery renderdeck.AutoQuery,
	selection eventPlannerSongSelection,
) (*drawing.DeckRequest, error) {
	query := baseQuery
	query.Region = region.String()
	query.RecommendType = "event"
	if eventID > 0 {
		query.EventID = drawing.IntPtr(eventID)
	} else if query.EventID != nil && *query.EventID <= 0 {
		query.EventID = nil
	}
	query.Limit = 1
	query.MusicCompare = false
	query.MusicCompareQueries = nil
	query.MusicCompareSelections = nil
	query.MusicID = nil
	query.MusicTitle = ""
	query.MusicCoverPath = ""
	query.MusicQuery = strings.TrimSpace(selection.Query)
	query.MusicDiff = strings.TrimSpace(selection.Difficulty)
	if selection.MusicID > 0 {
		query.MusicID = drawing.IntPtr(selection.MusicID)
		query.MusicQuery = ""
	} else if musicID := eventPlannerMusicIDOverride(selection.Query); musicID > 0 {
		query.MusicID = drawing.IntPtr(musicID)
		query.MusicQuery = ""
	}
	if err := resolveDeckMusicSelection(&query, app); err != nil {
		return nil, err
	}
	return deckCtrl.BuildAutoRecommendRequest(query)
}

func buildEventPlannerBaseDeckQuery(region renderregion.Value, params deckAutoQueryParams) renderdeck.AutoQuery {
	query := renderdeck.AutoQuery{
		Region:                       region.String(),
		RecommendType:                "event",
		EventID:                      params.EventID,
		Limit:                        1,
		TargetBonuses:                slices.Clone(params.TargetBonuses),
		AreaItemLevel:                params.AreaItemLevel,
		UnitFilter:                   params.UnitFilter,
		AttrFilter:                   params.AttrFilter,
		ExcludedCards:                slices.Clone(params.ExcludedCards),
		UseCurrentDeck:               params.UseCurrentDeck,
		MaxProfile:                   params.MaxProfile,
		SubMaxProfile:                params.SubMaxProfile,
		SupportMasterMax:             params.SupportMasterMax,
		SupportSkillMax:              params.SupportSkillMax,
		SpecificSkillOrder:           slices.Clone(params.SpecificSkillOrder),
		Args:                         params.Args,
		Algorithm:                    params.Algorithm,
		LiveType:                     params.LiveType,
		Target:                       params.Target,
		MusicQuery:                   params.MusicQuery,
		MusicID:                      params.MusicID,
		MusicDiff:                    params.MusicDiff,
		EventAttr:                    params.EventAttr,
		EventUnit:                    params.EventUnit,
		WorldBloomCharacterID:        params.WorldBloomCharacterID,
		WorldBloomCharacterQuery:     params.WorldBloomCharacterQuery,
		WorldBloomEventTurn:          params.WorldBloomEventTurn,
		ForcedLeaderCharacterID:      params.ForcedLeaderCharacterID,
		ForcedLeaderCharacterQuery:   params.ForcedLeaderCharacterQuery,
		ChallengeLiveCharacterID:     params.ChallengeLiveCharacterID,
		ChallengeLiveCharacterQuery:  params.ChallengeLiveCharacterQuery,
		FixedCards:                   slices.Clone(params.FixedCards),
		FixedCharacters:              slices.Clone(params.FixedCharacters),
		FixedCharacterQueries:        slices.Clone(params.FixedCharacterQueries),
		Rarity1Config:                params.Rarity1Config,
		Rarity2Config:                params.Rarity2Config,
		Rarity3Config:                params.Rarity3Config,
		Rarity4Config:                params.Rarity4Config,
		RarityBirthdayConfig:         params.RarityBirthdayConfig,
		SingleCardConfigs:            slices.Clone(params.SingleCardConfigs),
		MultiLiveTeammatePower:       params.MultiLiveTeammatePower,
		MultiLiveTeammateScoreUp:     params.MultiLiveTeammateScoreUp,
		MultiLiveScoreUpLowerBound:   params.MultiLiveScoreUpLowerBound,
		SkillOrderChooseStrategy:     params.SkillOrderChooseStrategy,
		SkillReferenceChooseStrategy: params.SkillReferenceChooseStrategy,
		KeepAfterTrainingState:       params.KeepAfterTrainingState,
	}
	if query.Algorithm == "" {
		query.Algorithm = "dfs_ga"
	}
	if query.LiveType == "" {
		query.LiveType = "multi"
	}
	if query.Target == "" {
		query.Target = "score"
	}
	if query.UseCurrentDeck {
		query.UseExactCardState = true
	}
	return query
}

func eventPlannerSongsForRequest(params eventPlannerCommandParams, query renderdeck.AutoQuery) []eventPlannerSongSelection {
	if len(params.Songs) > 0 {
		songs := slices.Clone(params.Songs)
		applyEventPlannerDefaultSongDifficulties(songs)
		return songs
	}
	if query.MusicID != nil && *query.MusicID > 0 {
		diff := strings.TrimSpace(query.MusicDiff)
		if diff == "" {
			diff = "master"
		}
		return []eventPlannerSongSelection{{Query: query.MusicQuery, Difficulty: diff, MusicID: *query.MusicID}}
	}
	if strings.TrimSpace(query.MusicQuery) != "" {
		selection := eventPlannerSongSelection{Query: strings.TrimSpace(query.MusicQuery), Difficulty: strings.TrimSpace(query.MusicDiff)}
		if selection.Difficulty == "" {
			selection.Difficulty = defaultEventPlannerSongDifficulty(selection.Query)
		}
		if musicID := eventPlannerMusicIDOverride(selection.Query); musicID > 0 {
			selection.MusicID = musicID
		}
		return []eventPlannerSongSelection{selection}
	}
	return []eventPlannerSongSelection{
		{Query: "虾", Difficulty: "expert"},
		{Query: "龙", Difficulty: "hard", MusicID: eventPlannerLostAndFoundMusicID},
	}
}

func eventPlannerEventBannerPath(app *renderapp.App, region renderregion.Value, eventInfo *masterdata.Event) string {
	if app == nil || eventInfo == nil {
		return ""
	}
	return renderassets.ResolveEventBannerPath(app.Assets, region.String(), eventInfo.AssetBundleName)
}

func eventPlannerDailyPoint(targetPoint, startAt, aggregateAt int64) int64 {
	if targetPoint <= 0 || aggregateAt <= startAt {
		return 0
	}
	durationDays := float64(aggregateAt-startAt) / float64(24*time.Hour/time.Millisecond)
	if durationDays <= 0 {
		return 0
	}
	return int64(math.Ceil(float64(targetPoint) / durationDays))
}

func resolveEventPlannerEvent(ctx context.Context, app *renderapp.App, region renderregion.Value, eventID int) (*masterdata.Event, string, error) {
	provider := eventPlannerProvider(app, region)
	if provider == nil {
		return nil, "", fmt.Errorf("当前%s服未找到活动数据", strings.ToUpper(region.String()))
	}
	if eventID > 0 {
		eventInfo, err := provider.Events().GetByID(ctx, eventID)
		if err != nil {
			return nil, "", err
		}
		if eventInfo == nil {
			return nil, "", fmt.Errorf("当前%s服未找到该活动数据", strings.ToUpper(region.String()))
		}
		return eventInfo, "", nil
	}

	now := time.Now().UnixMilli()
	var current *masterdata.Event
	for _, item := range provider.Events().GetAll(ctx) {
		if item == nil || !eventutil.IsRankingOpen(item.StartAt, item.AggregateAt, now) {
			continue
		}
		if current == nil || item.StartAt > current.StartAt {
			current = item
		}
	}
	if current == nil {
		return nil, "", fmt.Errorf("当前%s服没有进行中的活动，请在指令里加 eventID", strings.ToUpper(region.String()))
	}
	return current, fmt.Sprintf("未指定活动，已使用当前活动 %s", current.Name), nil
}

func resolveEventPlannerEventFromQuery(ctx context.Context, app *renderapp.App, region renderregion.Value, query renderdeck.AutoQuery) (*masterdata.Event, string, error) {
	if query.EventID != nil && *query.EventID > 0 {
		return resolveEventPlannerEvent(ctx, app, region, *query.EventID)
	}
	if strings.TrimSpace(query.EventAttr) != "" || strings.TrimSpace(query.EventUnit) != "" ||
		(query.WorldBloomEventTurn != nil && *query.WorldBloomEventTurn > 0) {
		return &masterdata.Event{
			ID:          0,
			Name:        eventPlannerSimulatedEventName(query),
			EventType:   eventPlannerSimulatedEventType(query),
			StartAt:     time.Now().UnixMilli(),
			AggregateAt: time.Now().Add(7 * 24 * time.Hour).UnixMilli(),
		}, "已使用模拟活动设置进行规划", nil
	}
	return resolveEventPlannerEvent(ctx, app, region, 0)
}

func eventPlannerSimulatedEventName(query renderdeck.AutoQuery) string {
	if query.WorldBloomEventTurn != nil && *query.WorldBloomEventTurn > 0 {
		return fmt.Sprintf("WL%d模拟活动", *query.WorldBloomEventTurn)
	}
	return "模拟活动"
}

func eventPlannerSimulatedEventType(query renderdeck.AutoQuery) string {
	if query.WorldBloomEventTurn != nil && *query.WorldBloomEventTurn > 0 {
		return "world_bloom"
	}
	return ""
}

func eventPlannerProvider(app *renderapp.App, region renderregion.Value) renderprovider.MasterDataProvider {
	if app == nil {
		return nil
	}
	region = renderregion.WithDefault(region)
	if app.Providers != nil {
		if provider := app.Providers[region]; provider != nil {
			return provider
		}
	}
	if app.Provider != nil && app.Provider.Region() == region {
		return app.Provider
	}
	return app.Provider
}

func resolveEventPlannerTargetPoint(rc *RequestContext, region renderregion.Value, eventID int, params eventPlannerCommandParams) (int64, string, error) {
	if params.TargetPoint > 0 {
		return params.TargetPoint, "直接输入", nil
	}
	if params.TargetRank <= 0 {
		return 0, "", fmt.Errorf("缺少目标 pt")
	}
	if rc.App == nil || rc.App.Tracker == nil {
		return 0, "", fmt.Errorf("未配置 Tracker，不能按 t%d 读取榜线", params.TargetRank)
	}
	tracker := rc.App.Tracker.WithContext(rc.Ctx)
	if lines, err := tracker.GetRankingLines(region.String(), eventID); err == nil {
		for _, line := range lines {
			if line.Rank == params.TargetRank && line.Score > 0 {
				return int64(line.Score), fmt.Sprintf("Tracker实时榜线:t%d", params.TargetRank), nil
			}
		}
	}
	latest, err := tracker.GetLatestRankingByRank(region.String(), eventID, params.TargetRank)
	if err != nil {
		return 0, "", err
	}
	if latest == nil || latest.RankData.Score <= 0 {
		return 0, "", fmt.Errorf("tracker 未返回 t%d 的有效榜线", params.TargetRank)
	}
	return int64(latest.RankData.Score), fmt.Sprintf("Tracker实时榜线:t%d", params.TargetRank), nil
}

func resolveEventPlannerCurrentPoint(snap rendersnapshot.Snapshot, eventID int, params eventPlannerCommandParams) (int64, string) {
	if params.CurrentPointSet {
		return params.CurrentPoint, ""
	}
	raw := snap.RawData()
	if raw != nil {
		for _, item := range raw.UserEvents {
			if item.EventID == eventID && item.EventPoint > 0 {
				return int64(item.EventPoint), ""
			}
		}
	}
	return 0, "未指定当前pt且 suite 中未找到该活动 pt，当前按 0 计算"
}

func parseEventPlannerRank(args string) int {
	match := eventPlannerTargetRankRE.FindStringSubmatch(args)
	if len(match) < 2 {
		return 0
	}
	for _, group := range match[1:] {
		if group == "" {
			continue
		}
		value, _ := strconv.Atoi(group)
		return value
	}
	return 0
}

func parseEventPlannerPointWithRE(re *regexp.Regexp, text string) (int64, bool) {
	match := re.FindStringSubmatch(text)
	if len(match) < 2 || match[1] == "" {
		return 0, false
	}
	value, err := parseEventPlannerHumanNumber(match[1])
	return value, err == nil
}

func parseEventPlannerBareTargetPoint(args string) (int64, string) {
	fields := strings.Fields(args)
	remaining := make([]string, 0, len(fields))
	var target int64
	inFixedTargets := false
	for _, token := range fields {
		clean := strings.Trim(token, " ,，。")
		if clean == "" {
			remaining = append(remaining, token)
			continue
		}
		if strings.HasPrefix(clean, "#") {
			inFixedTargets = true
		}
		if inFixedTargets {
			remaining = append(remaining, token)
			continue
		}
		lower := strings.ToLower(clean)
		if strings.HasPrefix(lower, "t") || strings.Contains(lower, "火") ||
			strings.HasPrefix(lower, "event") || strings.HasPrefix(lower, "活动") {
			remaining = append(remaining, token)
			continue
		}
		value, err := parseEventPlannerHumanNumber(clean)
		if err == nil && value >= 10000 && target == 0 {
			target = value
			continue
		}
		remaining = append(remaining, token)
	}
	return target, strings.TrimSpace(strings.Join(remaining, " "))
}

func parseEventPlannerHumanNumber(raw string) (int64, error) {
	clean := strings.ToLower(strings.TrimSpace(raw))
	clean = strings.NewReplacer(",", "", "_", "", "，", "", " ", "").Replace(clean)
	if clean == "" {
		return 0, fmt.Errorf("empty number")
	}
	multiplier := float64(1)
	for _, suffix := range []struct {
		text string
		mul  float64
	}{
		{"亿", 100000000}, {"億", 100000000}, {"万", 10000}, {"w", 10000}, {"k", 1000},
	} {
		if strings.HasSuffix(clean, suffix.text) {
			multiplier = suffix.mul
			clean = strings.TrimSuffix(clean, suffix.text)
			break
		}
	}
	value, err := strconv.ParseFloat(clean, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid number %q", raw)
	}
	return int64(value*multiplier + 0.5), nil
}

func parseEventPlannerBoosts(args string, fallback []int) ([]int, string, error) {
	matches := eventPlannerBoostRE.FindAllStringSubmatch(args, -1)
	if len(matches) == 0 {
		return fallback, args, nil
	}
	seen := map[int]struct{}{}
	boosts := make([]int, 0, len(matches))
	for _, match := range matches {
		value, _ := strconv.Atoi(match[1])
		if value < 1 || value > 10 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		boosts = append(boosts, value)
	}
	if len(boosts) == 0 {
		return nil, "", onebot11.NewReplayError("火数只能指定 1-10 火")
	}
	return boosts, eventPlannerBoostRE.ReplaceAllString(args, " "), nil
}

func parseEventPlannerSongs(args string) ([]eventPlannerSongSelection, string) {
	if prefix, after, ok := eventPlannerAfterSongMarker(args); ok {
		songs, remaining := eventPlannerSongSelectionsFromText(after)
		return songs, normalizeDeckSpaces(strings.TrimSpace(prefix + " " + remaining))
	}
	return nil, args
}

func eventPlannerAfterSongMarker(args string) (string, string, bool) {
	markers := []string{"歌曲", "歌", "曲"}
	best := -1
	markerLen := 0
	for _, marker := range markers {
		if idx := strings.Index(args, marker); idx >= 0 && (best < 0 || idx < best) {
			best = idx
			markerLen = len(marker)
		}
	}
	if best < 0 {
		return "", "", false
	}
	return strings.TrimSpace(args[:best]), strings.TrimSpace(args[best+markerLen:]), true
}

func eventPlannerSongSelectionsFromText(text string) ([]eventPlannerSongSelection, string) {
	text = strings.NewReplacer("/", " ", "|", " ", "，", " ", ",", " ", "、", " ").Replace(text)
	var songs []eventPlannerSongSelection
	var remaining []string
	for _, token := range strings.Fields(text) {
		token = strings.Trim(token, " ,，。")
		if !eventPlannerLooksLikeSongToken(token) {
			remaining = append(remaining, token)
			continue
		}
		query, diff := splitEventPlannerDifficulty(token)
		if query == "" {
			remaining = append(remaining, token)
			continue
		}
		songs = append(songs, eventPlannerSongSelection{Query: query, Difficulty: diff})
	}
	return songs, strings.TrimSpace(strings.Join(remaining, " "))
}

func applyEventPlannerDefaultSongDifficulties(songs []eventPlannerSongSelection) {
	for i := range songs {
		if songs[i].MusicID <= 0 {
			songs[i].MusicID = eventPlannerMusicIDOverride(songs[i].Query)
		}
		if strings.TrimSpace(songs[i].Difficulty) != "" {
			continue
		}
		songs[i].Difficulty = defaultEventPlannerSongDifficulty(songs[i].Query)
	}
}

func defaultEventPlannerSongDifficulty(query string) string {
	switch strings.TrimSpace(strings.ToLower(query)) {
	case "虾":
		return "expert"
	case "龙":
		return "hard"
	default:
		return "master"
	}
}

func eventPlannerMusicIDOverride(query string) int {
	switch strings.TrimSpace(strings.ToLower(query)) {
	case "龙":
		return eventPlannerLostAndFoundMusicID
	case "野车", "omakase", "随机":
		return eventPlannerOmakaseMusicID
	default:
		return 0
	}
}

func eventPlannerLooksLikeSongToken(token string) bool {
	if token == "" || strings.HasPrefix(token, "#") {
		return false
	}
	lower := strings.ToLower(token)
	if strings.Contains(lower, "火") || strings.HasPrefix(lower, "event") || strings.HasPrefix(lower, "活动") ||
		strings.HasPrefix(lower, "pt") {
		return false
	}
	switch lower {
	case "solo", "单人", "auto", "自动", "multi", "多人", "协力":
		return false
	}
	if strings.HasPrefix(lower, "t") && len(lower) > 1 && eventPlannerIsDigits(lower[1:]) {
		return false
	}
	if eventPlannerContainsAny(lower, "当前", "最优", "最佳", "目标", "已有", "现在", "卡组", "主队", "队友", "实效", "综合", "画布", "已读") {
		return false
	}
	if value, err := parseEventPlannerHumanNumber(lower); err == nil && value > 0 {
		return false
	}
	return true
}

func eventPlannerContainsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func splitEventPlannerDifficulty(token string) (string, string) {
	lower := strings.ToLower(token)
	suffixes := []struct {
		raw  string
		diff string
	}{
		{"append", "append"}, {"apd", "append"}, {"app", "append"},
		{"master", "master"}, {"mas", "master"}, {"ma", "master"},
		{"expert", "expert"}, {"exp", "expert"}, {"ex", "expert"},
		{"hard", "hard"}, {"hd", "hard"},
		{"normal", "normal"}, {"nm", "normal"},
		{"easy", "easy"}, {"ez", "easy"},
	}
	for _, suffix := range suffixes {
		if strings.HasSuffix(lower, suffix.raw) && len(token) > len(suffix.raw) {
			return strings.TrimSpace(token[:len(token)-len(suffix.raw)]), suffix.diff
		}
	}
	return token, ""
}

func eventPlannerIsDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func eventPlannerBoostMultiplier(boost int) int64 {
	if value := eventPlannerBoostMultipliers[boost]; value > 0 {
		return value
	}
	return 1
}

func eventPlannerDeckCards(cards []drawing.DeckCardData) []drawing.EventPlannerDeckCard {
	result := make([]drawing.EventPlannerDeckCard, 0, len(cards))
	for _, card := range cards {
		result = append(result, drawing.EventPlannerDeckCard{
			CardThumbnail:  card.CardThumbnail,
			SkillLevel:     card.SkillLevel,
			SkillRate:      card.SkillRate,
			EventBonusRate: card.EventBonusRate,
		})
	}
	return result
}

func buildEventPlannerDeckSummary(query renderdeck.AutoQuery, totalPower int, eventBonus, skillUp float64) string {
	label := "最优组卡"
	switch {
	case len(query.FixedCards) > 0 || len(query.FixedCharacters) > 0:
		label = "指定卡组"
	case query.UseCurrentDeck:
		label = "当前主队"
	case query.MaxProfile:
		label = "顶配组卡"
	case query.SubMaxProfile:
		label = "次顶配组卡"
	}
	parts := []string{label}
	if totalPower > 0 {
		parts = append(parts, fmt.Sprintf("综合力 %s", formatEventPlannerPlainInt(int64(totalPower))))
	}
	if eventBonus > 0 {
		parts = append(parts, fmt.Sprintf("活动加成 %s%%", formatEventPlannerRate(eventBonus)))
	}
	if skillUp > 0 {
		parts = append(parts, fmt.Sprintf("协力实效 %s%%", formatEventPlannerRate(skillUp)))
	}
	return strings.Join(parts, " / ")
}

func eventPlannerIntValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func eventPlannerFloatValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func eventPlannerStringValue(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return *value
}

func formatEventPlannerPlainInt(value int64) string {
	raw := strconv.FormatInt(value, 10)
	if len(raw) <= 3 {
		return raw
	}
	var parts []string
	for len(raw) > 3 {
		parts = append([]string{raw[len(raw)-3:]}, parts...)
		raw = raw[:len(raw)-3]
	}
	parts = append([]string{raw}, parts...)
	return strings.Join(parts, ",")
}

func formatEventPlannerRate(value float64) string {
	rounded := math.Round(value*10) / 10
	if math.Abs(rounded-math.Round(rounded)) < 1e-9 {
		return strconv.FormatInt(int64(math.Round(rounded)), 10)
	}
	return strconv.FormatFloat(rounded, 'f', 1, 64)
}
