package handler

import (
	"context"
	"errors"
	"fmt"
	json "github.com/bytedance/sonic"
	"strconv"
	"strings"

	sekaidb "haruki-cloud/database/sekai"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/sk"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/logger"
)

var skTrackerDebugLogger = logger.NewLoggerFromGlobal("SKTracker")

func (sekaiHandlers) SKLineHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "sk/line",
			Commands: []string{
				"/sk-line", "/sk线", "/榜线", "/pjsk sk line", "/pjsk board line", "/skl",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKTrackerParams(ctx, true, true, false)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, "sk-line", params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKQueryHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{Path: "sk/query", Commands: []string{
			"/sk-query", "/sk查询", "/sk查分", "/pjsk sk board", "/pjsk board",
		},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKTrackerParams(ctx, false, true, true)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, "sk-query", params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKSpeedHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "sk/speed",
			Commands: []string{
				"/pjsk sk speed", "/pjsk board speed", "/时速", "/sks", "/skv", "/sk时速",
				"/sk-speed", "/sk时速", "/时速线", "/pjsk sk speed", "/pjsk board speed", "/sks", "/skv", "/sktime",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKSpeedTrackerParams(ctx, "h", 60, 60)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, "sk-speed", params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKCheckRoomHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "sk/check-room",
			Commands: []string{
				"/sk-check-room", "/sk查房", "/查房", "/cf", "/pjsk查房",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKTrackerParams(ctx, false, true, true)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, "sk-check-room", params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKCheckRoomLiteHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "sk/check-room",
			Commands: []string{
				"/cfl",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKTrackerParamsWithDefaultRanks(ctx, false, true, false, defaultSKCheckRoomLiteRanks)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, "sk-check-room", params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKPlayerTraceHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "sk/player-trace",
			Commands: []string{
				"/sk-player-trace", "/sk玩家轨迹", "/玩家轨迹", "/ptr", "/pjsk玩家追踪", "/pjsk ptr",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKPlayerTraceParams(ctx)
			if err != nil {
				return nil, err
			}
			if len(params) == 0 {
				return makeCommandRequest(ctx, parser.ModuleSK, "sk-player-trace"), nil
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, "sk-player-trace", params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKRankTraceHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "sk/rank-trace",
			Commands: []string{
				"/sk-rank-trace", "/sk档线轨迹", "/档线轨迹", "/rtr", "/skt", "/sklt", "/sktl", "/pjsk追踪", "/pjsk sk追踪",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, "sk-rank-trace", params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) WinratePredictHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{Path: "sk/winrate", Commands: []string{
			"/pjsk winrate predict", "/胜率预测", "/5v5预测", "/胜率", "/5v5胜率", "/预测胜率", "/预测5v5",
		}},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			return makeCommandRequest(ctx, parser.ModuleSK, "sk-winrate"), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKDailySpeedHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "sk/daily-speed",
			Commands: []string{
				"/pjsk sk daily speed", "/pjsk board daily speed", "/日速", "/skds", "/skdv", "/sk日速",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKSpeedTrackerParams(ctx, "d", 1, 24*60*60)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, "sk-daily-speed", params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKPredictHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "sk/predict",
			Commands: []string{
				"/pjsk sk predict", "/pjsk board predict", "/sk预测", "/榜线预测", "/skp",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, "sk-predict", params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKBoardHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "sk/query",
			Commands: []string{
				"/pjsk sk board", "/pjsk board", "/sk",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKTrackerParams(ctx, false, true, true)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, "sk-query", params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) CSBHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "sk/csb",
			Commands: []string{
				"/csb", "/查水表", "/pjsk查水表", "/停车时间",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKTrackerParams(ctx, false, true, true)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, "sk-csb", params), nil
		},
	}, executeSK)
}

func executeSK(rc *RequestContext) (message onebot11.Message, err error) {
	defer func() {
		err = normalizeSKUserFacingError(err)
	}()

	if rc == nil || rc.App == nil || rc.App.SK == nil {
		return nil, fmt.Errorf("sk service unavailable: tracker controller is not configured")
	}
	skCtrl := rc.App.SK.WithContext(rc.Ctx)
	data, err := executeSKMode(rc, skCtrl)
	if err != nil {
		return nil, err
	}
	return imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
}

func executeSKMode(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	switch rc.Cmd.Mode {
	case "sk-line":
		return executeSKLine(rc, skCtrl)
	case "sk-query":
		return executeSKQuery(rc, skCtrl)
	case "sk-check-room":
		return executeSKCheckRoom(rc, skCtrl)
	case "sk-csb":
		return executeSKCSB(rc, skCtrl)
	case "sk-speed", "sk-daily-speed":
		return executeSKSpeed(rc, skCtrl)
	case "sk-player-trace":
		return executeSKPlayerTrace(rc, skCtrl)
	case "sk-rank-trace":
		return executeSKRankTrace(rc, skCtrl)
	case "sk-predict":
		return executeSKPredict(rc, skCtrl)
	case "sk-winrate":
		return executeSKWinRate(rc, skCtrl)
	default:
		return nil, unsupportedModeError("sk", rc.Cmd.Mode)
	}
}

func executeSKLine(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		selfQuery := isSKSelfTrackerQuery(rc, trackerReq)
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return nil, err
		}
		payload, err := skCtrl.BuildLineRequestFromTracker(trackerReq)
		if err != nil {
			return nil, normalizeSKSelfRankingNotFoundError(selfQuery, trackerReq.Region, err)
		}
		return skCtrl.RenderLine(*payload)
	}
	req := sk.LineRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderLine(req)
}

func executeSKQuery(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		selfQuery := isSKSelfTrackerQuery(rc, trackerReq)
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return nil, err
		}
		payload, err := skCtrl.BuildQueryRequestFromTracker(trackerReq)
		if err != nil {
			return nil, normalizeSKSelfRankingNotFoundError(selfQuery, trackerReq.Region, err)
		}
		return skCtrl.RenderQuery(*payload)
	}
	req := drawing.SKRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderQuery(req)
}

func executeSKCheckRoom(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		selfQuery := isSKSelfTrackerQuery(rc, trackerReq)
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return nil, err
		}
		payload, err := skCtrl.BuildCheckRoomRequestFromTracker(trackerReq)
		if err != nil {
			return nil, normalizeSKSelfRankingNotFoundError(selfQuery, trackerReq.Region, err)
		}
		return skCtrl.RenderCheckRoom(*payload)
	}
	req := drawing.CFRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderCheckRoom(req)
}

func executeSKCSB(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		selfQuery := isSKSelfTrackerQuery(rc, trackerReq)
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return nil, err
		}
		payload, err := skCtrl.BuildCSBRequestFromTracker(trackerReq)
		if err != nil {
			return nil, normalizeSKSelfRankingNotFoundError(selfQuery, trackerReq.Region, err)
		}
		return skCtrl.RenderCSB(*payload)
	}
	req := drawing.CSBRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderCSB(req)
}

func executeSKSpeed(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return nil, err
		}
		payload, err := skCtrl.BuildSpeedRequestFromTracker(trackerReq)
		if err != nil {
			return nil, err
		}
		return skCtrl.RenderSpeed(*payload)
	}
	req := drawing.SpeedRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderSpeed(req)
}

func executeSKPlayerTrace(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	trackerReq, ok := trackerRankQueryFromParams(rc.Cmd)
	if !ok {
		trackerReq = sk.TrackerRankQuery{Region: rc.Cmd.Region}
		if trackerReq.Region == "" {
			trackerReq.Region = DefaultRegionStr
		}
	}
	selfQuery := isSKSelfTrackerQuery(rc, trackerReq)
	if err := resolveTrackerCharacterSelection(rc.Ctx, rc.App, &trackerReq); err != nil {
		return nil, err
	}
	hasExplicitTarget := strings.TrimSpace(trackerReq.TargetUserID) != ""
	if trackerReq.UserID == nil {
		targetErr := resolveTrackerTargetUser(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID)
		if targetErr != nil && hasExplicitTarget {
			return nil, targetErr
		}
		if trackerReq.UserID == nil && len(trackerReq.Ranks) == 0 && !hasExplicitTarget {
			if uid := resolveRequesterGameUID(rc); uid > 0 {
				trackerReq.UserID = &uid
			}
		}
	}
	if trackerReq.UserID != nil || len(trackerReq.Ranks) > 0 {
		payload, err := skCtrl.BuildPlayerTraceFromTracker(trackerReq)
		if err != nil {
			return nil, normalizeSKSelfRankingNotFoundError(selfQuery, trackerReq.Region, err)
		}
		return skCtrl.RenderPlayerTrace(*payload)
	}
	req := drawing.PlayerTraceRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderPlayerTrace(req)
}

func executeSKRankTrace(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return nil, err
		}
		payload, err := skCtrl.BuildRankTraceRequestFromTracker(trackerReq)
		if err != nil {
			return nil, err
		}
		return skCtrl.RenderRankTrace(*payload)
	}
	req := drawing.RankTraceRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderRankTrace(req)
}

func isSKSelfTrackerQuery(rc *RequestContext, req sk.TrackerRankQuery) bool {
	if rc == nil || rc.Cmd == nil {
		return false
	}
	targetPlatform := strings.TrimSpace(req.TargetPlatform)
	targetUserID := strings.TrimSpace(req.TargetUserID)
	requesterPlatform := strings.TrimSpace(rc.Cmd.RequesterPlatform)
	requesterUserID := strings.TrimSpace(rc.Cmd.RequesterUserID)
	if targetPlatform != "" && targetUserID != "" && requesterPlatform != "" && requesterUserID != "" {
		return strings.EqualFold(targetPlatform, requesterPlatform) && targetUserID == requesterUserID
	}
	if req.UserID != nil || len(req.Ranks) > 0 {
		return false
	}
	return targetPlatform == "" && targetUserID == ""
}

func normalizeSKSelfRankingNotFoundError(selfQuery bool, region string, err error) error {
	if !selfQuery || err == nil || !errors.Is(err, sekaiapi.ErrRankingNotFound) {
		return err
	}
	return onebot11.NewReplayError(
		"当前%s服活动没有找到你的排行榜数据",
		musicNotFoundRegionLabel(region),
	)
}

func executeSKPredict(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return nil, err
		}
		return skCtrl.RenderPredictLineFromTracker(trackerReq)
	}
	req := sk.LineRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderLine(req)
}

func executeSKWinRate(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	req := drawing.WinRateRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderWinRate(req)
}

func trackerRankQueryFromParams(r *CommandRequest) (sk.TrackerRankQuery, bool) {
	if r == nil || len(r.Params) == 0 {
		return sk.TrackerRankQuery{}, false
	}
	var req sk.TrackerRankQuery
	if err := json.Unmarshal(r.Params, &req); err != nil {
		return sk.TrackerRankQuery{}, false
	}
	resolvedRegion := strings.TrimSpace(r.Region)
	if !req.RegionExplicit && resolvedRegion != "" {
		req.Region = resolvedRegion
	} else if req.Region == "" {
		req.Region = resolvedRegion
	}
	if len(req.Ranks) == 0 &&
		req.EventID == 0 &&
		req.WlCharacterID == nil &&
		strings.TrimSpace(req.WlCharacterQuery) == "" &&
		req.UserID == nil &&
		strings.TrimSpace(req.TargetUserID) == "" {
		return sk.TrackerRankQuery{}, false
	}
	return req, true
}

func prepareTrackerRankQuery(ctx context.Context, app *renderapp.App, req *sk.TrackerRankQuery, requesterPlatform, requesterUserID string) error {
	if err := resolveTrackerCharacterSelection(ctx, app, req); err != nil {
		return err
	}
	return resolveTrackerTargetUser(ctx, app, req, requesterPlatform, requesterUserID)
}

func resolveTrackerCharacterSelection(ctx context.Context, app *renderapp.App, req *sk.TrackerRankQuery) error {
	if req == nil {
		return nil
	}

	if req.WlCharacterID != nil && *req.WlCharacterID <= 0 {
		req.WlCharacterID = nil
	}
	query := strings.TrimSpace(req.WlCharacterQuery)
	if req.WlCharacterID == nil && query == "" {
		return nil
	}

	region := renderregion.WithDefault(renderregion.Normalize(req.Region))
	eventInfo, chapters, err := resolveTrackerWorldBloomEvent(ctx, app, region, req.EventID)
	if err != nil {
		return err
	}

	req.EventID = int(eventInfo.GameID)
	if req.WlCharacterID != nil {
		chapter := findWorldBloomChapterByCharacterID(chapters, *req.WlCharacterID)
		if chapter == nil {
			return fmt.Errorf("活动 %s-%d 没有角色 %d 的 World Link 章节", strings.ToUpper(region.String()), req.EventID, *req.WlCharacterID)
		}
		applyTrackerWorldBloomChapterTiming(req, chapter)
		skTrackerDebugLogger.Debugf(
			"resolved wl tracker selection: region=%s event=%d char=%d query=%q source=explicit",
			region.String(), req.EventID, *req.WlCharacterID, query,
		)
		req.WlCharacterQuery = ""
		return nil
	}

	chapter, err := resolveTrackerWorldBloomChapterSelection(ctx, app, region, eventInfo, chapters, query)
	if err != nil {
		return err
	}
	if chapter.GameCharacterID <= 0 {
		return fmt.Errorf("活动 %s-%d 的 World Link 章节缺少角色信息", strings.ToUpper(region.String()), req.EventID)
	}

	charID := int(chapter.GameCharacterID)
	req.WlCharacterID = drawing.IntPtr(charID)
	req.WlCharacterQuery = ""
	applyTrackerWorldBloomChapterTiming(req, chapter)
	skTrackerDebugLogger.Debugf(
		"resolved wl tracker selection: region=%s event=%d chapter=%d char=%d query=%q",
		region.String(), req.EventID, chapter.ChapterNo, charID, query,
	)
	return nil
}

func applyTrackerWorldBloomChapterTiming(req *sk.TrackerRankQuery, chapter *sekaidb.Worldbloom) {
	if req == nil || chapter == nil {
		return
	}
	if chapter.ChapterStartAt > 0 {
		req.EventStartAt = drawing.Int64Ptr(chapter.ChapterStartAt)
	}
	if chapter.AggregateAt > 0 {
		req.EventAggregateAt = drawing.Int64Ptr(chapter.AggregateAt)
	}
}

func resolveRequesterGameUID(rc *RequestContext) int64 {
	_, binding, _ := resolveBindingWithFallback(
		rc.Ctx, rc.App.Bindings, rc.Platform, rc.PlatformUserID, rc.RegionStr, rc.Cmd.RegionExplicit,
		bindingResolutionOptions{},
	)
	if binding == nil {
		return 0
	}
	uid, parseErr := strconv.ParseInt(strings.TrimSpace(binding.PJSKUserID), 10, 64)
	if parseErr != nil || uid <= 0 {
		return 0
	}
	return uid
}

func resolveTrackerTargetUser(ctx context.Context, app *renderapp.App, req *sk.TrackerRankQuery, requesterPlatform, requesterUserID string) error {
	if req == nil || req.UserID != nil {
		return nil
	}

	targetPlatform := strings.TrimSpace(req.TargetPlatform)
	targetUserID := strings.TrimSpace(req.TargetUserID)
	targetSelector := strings.TrimSpace(req.TargetSelector)
	if targetPlatform == "" || targetUserID == "" {
		return nil
	}

	if app == nil || app.Bindings == nil || !app.Bindings.IsReady() {
		return accountdata.ErrBindingServiceUnavailable
	}

	var (
		binding *accountdata.ResolvedBinding
		err     error
	)

	if targetSelector != "" {
		_, binding, err = app.Bindings.ResolveUserBindingBySelector(ctx, targetPlatform, targetUserID, selectorBindingServer(normalizeTrackerRegion(req.Region), req.RegionExplicit), targetSelector)
		if err != nil {
			return normalizeBindingLookupError(err, fmt.Sprintf("无法解析账号选择器 %s", targetSelector))
		}
	} else if req.RegionExplicit {
		region := normalizeTrackerRegion(req.Region)
		_, binding, err = app.Bindings.ResolveUserBinding(ctx, targetPlatform, targetUserID, region)
		if err != nil {
			return normalizeBindingLookupError(err, fmt.Sprintf("@用户 %s 在 %s 服没有绑定账号", targetUserID, strings.ToUpper(region)))
		}
	} else {
		_, binding, err = app.Bindings.ResolveUserBinding(ctx, targetPlatform, targetUserID, accountdata.GlobalDefaultBindingScope)
		if err != nil || binding == nil {
			_, binding, err = app.Bindings.ResolveUserBinding(ctx, targetPlatform, targetUserID, DefaultRegionStr)
			if err != nil {
				return normalizeBindingLookupError(err, fmt.Sprintf("@用户 %s 没有可用绑定", targetUserID))
			}
		}
	}

	if binding == nil {
		return accountdata.ErrNoBinding
	}

	isSelfTarget := strings.EqualFold(strings.TrimSpace(targetPlatform), strings.TrimSpace(requesterPlatform)) &&
		strings.TrimSpace(targetUserID) != "" &&
		strings.TrimSpace(targetUserID) == strings.TrimSpace(requesterUserID)

	if targetSelector == "" && !binding.Visible && !isSelfTarget {
		return fmt.Errorf("@用户 %s 已隐藏个人信息，无法查询", targetUserID)
	}

	uid, parseErr := strconv.ParseInt(strings.TrimSpace(binding.PJSKUserID), 10, 64)
	if parseErr != nil || uid <= 0 {
		return fmt.Errorf("@用户 %s 的绑定UID无效: %s", targetUserID, binding.PJSKUserID)
	}
	req.UserID = &uid
	if !req.RegionExplicit {
		req.Region = normalizeTrackerRegion(binding.Server)
	}
	return nil
}

func normalizeTrackerRegion(region string) string {
	return regionWithDefault(region)
}
