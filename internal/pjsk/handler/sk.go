package handler

import (
	"context"
	"errors"
	"strconv"
	"strings"

	sekaidb "haruki-cloud/database/sekai"
	"haruki-cloud/internal/i18n"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/sk"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/logger"
	"haruki-cloud/utils/usererror"
)

var skTrackerDebugLogger = logger.NewLoggerFromGlobal("SKTracker")

type skExecutionResult struct {
	image   drawing.ImageResult
	warning string
}

func (sekaiHandlers) SKLineHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "sk/line",
		Commands: []string{
			"/sk-line", "/sk线", "/榜线", "/pjsk sk line", "/pjsk board line", "/skl",
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
		Path: "sk/query",
		Commands: []string{
			"/sk-query", "/sk查询", "/sk查分", "/pjsk sk board", "/pjsk board",
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKTrackerParams(ctx, false, true, true)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, skQueryCommand, params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKSpeedHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "sk/speed",
		Commands: []string{
			"/pjsk sk speed", "/pjsk board speed", "/时速", "/sks", "/skv", "/sk时速",
			"/sk-speed", "/sk时速", "/时速线", "/pjsk sk speed", "/pjsk board speed", "/sks", "/skv", "/sktime",
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
		Path: "sk/check-room",
		Commands: []string{
			"/sk-check-room", "/sk查房", "/查房", "/cf", "/pjsk查房",
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKTrackerParams(ctx, false, true, true)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, skCheckRoomCommand, params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKCheckRoomLiteHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "sk/check-room",
		Commands: []string{
			"/cfl",
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKTrackerParamsWithDefaultRanks(ctx, false, true, false, defaultSKCheckRoomLiteRanks)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, skCheckRoomCommand, params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKPlayerTraceHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "sk/player-trace",
		Commands: []string{
			"/sk-player-trace", "/sk玩家轨迹", "/玩家轨迹", "/ptr", "/pjsk玩家追踪", "/pjsk ptr", "/玩家追踪",
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKPlayerTraceParams(ctx)
			if err != nil {
				return nil, err
			}
			if len(params) == 0 {
				return makeCommandRequest(ctx, parser.ModuleSK, skPlayerTraceCommand), nil
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, skPlayerTraceCommand, params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKRankTraceHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "sk/rank-trace",
		Commands: []string{
			"/sk-rank-trace", "/sk档线轨迹", "/档线轨迹", "/rtr", "/skt", "/sklt", "/sktl", "/pjsk追踪", "/pjsk sk追踪", "/排名追踪",
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
		Path: "sk/winrate",
		Commands: []string{
			"/pjsk winrate predict", "/胜率预测", "/5v5预测", "/胜率", "/5v5胜率", "/预测胜率", "/预测5v5",
		},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			return makeCommandRequest(ctx, parser.ModuleSK, "sk-winrate"), nil
		},
	}, executeSK)
}

func (sekaiHandlers) SKDailySpeedHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "sk/daily-speed",
		Commands: []string{
			"/pjsk sk daily speed", "/pjsk board daily speed", "/日速", "/skds", "/skdv", "/sk日速", "/每日时速",
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
		Path: "sk/predict",
		Commands: []string{
			"/pjsk sk predict", "/pjsk board predict", "/sk预测", "/榜线预测", "/skp",
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
		Path: "sk/query",
		Commands: []string{
			"/pjsk sk board", "/pjsk board", "/sk",
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildSKTrackerParams(ctx, false, true, true)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleSK, skQueryCommand, params), nil
		},
	}, executeSK)
}

func (sekaiHandlers) CSBHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "sk/csb",
		Commands: []string{
			"/csb", "/查水表", "/pjsk查水表", "/停车时间",
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
		return nil, usererror.Misconfigured(errors.New("sk service unavailable: tracker controller is not configured"))
	}
	skCtrl := rc.App.SK.WithContext(rc.Ctx)
	result, err := executeSKMode(rc, skCtrl)
	if err != nil {
		return nil, err
	}
	message, err = rc.RenderedImageMessage(result.image)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(result.warning) != "" {
		return append(onebot11.Message{onebot11.Text(result.warning)}, message...), nil
	}
	return message, nil
}

func executeSKMode(rc *RequestContext, skCtrl *sk.Controller) (skExecutionResult, error) {
	switch rc.Cmd.Mode {
	case "sk-line":
		return skImageResult(executeSKLine(rc, skCtrl))
	case skQueryCommand:
		return executeSKQuery(rc, skCtrl)
	case skCheckRoomCommand:
		return executeSKCheckRoom(rc, skCtrl)
	case "sk-csb":
		return executeSKCSB(rc, skCtrl)
	case "sk-speed", "sk-daily-speed":
		return skImageResult(executeSKSpeed(rc, skCtrl))
	case skPlayerTraceCommand:
		return skImageResult(executeSKPlayerTrace(rc, skCtrl))
	case "sk-rank-trace":
		return skImageResult(executeSKRankTrace(rc, skCtrl))
	case "sk-predict":
		return skImageResult(executeSKPredict(rc, skCtrl))
	case "sk-winrate":
		return skImageResult(executeSKWinRate(rc, skCtrl))
	default:
		return skExecutionResult{}, unsupportedModeError("sk", rc.Cmd.Mode)
	}
}

func skImageResult(data drawing.ImageResult, err error) (skExecutionResult, error) {
	if err != nil {
		return skExecutionResult{}, err
	}
	return skExecutionResult{image: data}, nil
}

func executeSKLine(rc *RequestContext, skCtrl *sk.Controller) (drawing.ImageResult, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		selfQuery := isSKSelfTrackerQuery(rc, trackerReq)
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return drawing.ImageResult{}, err
		}
		payload, err := skCtrl.BuildLineRequestFromTracker(trackerReq)
		if err != nil {
			return drawing.ImageResult{}, normalizeSKSelfRankingNotFoundError(selfQuery, trackerReq.Region, err)
		}
		return skCtrl.RenderLineImage(*payload)
	}
	req := sk.LineRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderLineImage(req)
}

func executeSKQuery(rc *RequestContext, skCtrl *sk.Controller) (skExecutionResult, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		selfQuery := isSKSelfTrackerQuery(rc, trackerReq)
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return skExecutionResult{}, err
		}
		payload, err := skCtrl.BuildQueryRequestFromTracker(trackerReq)
		if err != nil {
			return skExecutionResult{}, normalizeSKSelfRankingNotFoundError(selfQuery, trackerReq.Region, err)
		}
		data, err := skCtrl.RenderQueryImage(*payload)
		if err != nil {
			return skExecutionResult{}, err
		}
		result := skExecutionResult{image: data}
		if selfQuery {
			result.warning = skCtrl.StaleSelfRecordWarning(trackerReq, payload.Ranks)
		}
		return result, nil
	}
	req := drawing.SKRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skImageResult(skCtrl.RenderQueryImage(req))
}

func executeSKCheckRoom(rc *RequestContext, skCtrl *sk.Controller) (skExecutionResult, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		selfQuery := isSKSelfTrackerQuery(rc, trackerReq)
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return skExecutionResult{}, err
		}
		payload, err := skCtrl.BuildCheckRoomRequestFromTracker(trackerReq)
		if err != nil {
			return skExecutionResult{}, normalizeSKSelfRankingNotFoundError(selfQuery, trackerReq.Region, err)
		}
		data, err := skCtrl.RenderCheckRoomImage(*payload)
		if err != nil {
			return skExecutionResult{}, err
		}
		result := skExecutionResult{image: data}
		if selfQuery {
			result.warning = skCtrl.StaleSelfRecordWarning(trackerReq, payload.Ranks)
		}
		return result, nil
	}
	req := drawing.CFRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skImageResult(skCtrl.RenderCheckRoomImage(req))
}

func executeSKCSB(rc *RequestContext, skCtrl *sk.Controller) (skExecutionResult, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		selfQuery := isSKSelfTrackerQuery(rc, trackerReq)
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return skExecutionResult{}, err
		}
		payload, err := skCtrl.BuildCSBRequestFromTracker(trackerReq)
		if err != nil {
			return skExecutionResult{}, normalizeSKSelfRankingNotFoundError(selfQuery, trackerReq.Region, err)
		}
		data, err := skCtrl.RenderCSBImage(*payload)
		if err != nil {
			return skExecutionResult{}, err
		}
		result := skExecutionResult{image: data}
		if selfQuery {
			result.warning = skCtrl.StaleSelfLatestRecordWarning(trackerReq)
		}
		return result, nil
	}
	req := drawing.CSBRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skImageResult(skCtrl.RenderCSBImage(req))
}

func executeSKSpeed(rc *RequestContext, skCtrl *sk.Controller) (drawing.ImageResult, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return drawing.ImageResult{}, err
		}
		payload, err := skCtrl.BuildSpeedRequestFromTracker(trackerReq)
		if err != nil {
			return drawing.ImageResult{}, err
		}
		return skCtrl.RenderSpeedImage(*payload)
	}
	req := drawing.SpeedRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderSpeedImage(req)
}

func executeSKPlayerTrace(rc *RequestContext, skCtrl *sk.Controller) (drawing.ImageResult, error) {
	trackerReq, ok := trackerRankQueryFromParams(rc.Cmd)
	if !ok {
		trackerReq = sk.TrackerRankQuery{Region: rc.Cmd.Region}
		if trackerReq.Region == "" {
			trackerReq.Region = DefaultRegionStr
		}
	}
	selfQuery := isSKSelfTrackerQuery(rc, trackerReq)
	if err := resolveTrackerCharacterSelection(rc.Ctx, rc.App, &trackerReq); err != nil {
		return drawing.ImageResult{}, err
	}
	hasExplicitTarget := strings.TrimSpace(trackerReq.TargetUserID) != ""
	if trackerReq.UserID == nil {
		targetErr := resolveTrackerTargetUser(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID)
		if targetErr != nil && hasExplicitTarget {
			return drawing.ImageResult{}, targetErr
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
			return drawing.ImageResult{}, normalizeSKSelfRankingNotFoundError(selfQuery, trackerReq.Region, err)
		}
		data, err := skCtrl.RenderPlayerTraceImage(*payload)
		if err != nil {
			return drawing.ImageResult{}, normalizeSKPlayerTraceDrawingError(err)
		}
		return data, nil
	}
	req := drawing.PlayerTraceRequest{}
	mergeParams(rc.Cmd.Params, &req)
	data, err := skCtrl.RenderPlayerTraceImage(req)
	if err != nil {
		return drawing.ImageResult{}, normalizeSKPlayerTraceDrawingError(err)
	}
	return data, nil
}

func executeSKRankTrace(rc *RequestContext, skCtrl *sk.Controller) (drawing.ImageResult, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return drawing.ImageResult{}, err
		}
		payload, err := skCtrl.BuildRankTraceRequestFromTracker(trackerReq)
		if err != nil {
			return drawing.ImageResult{}, err
		}
		return skCtrl.RenderRankTraceImage(*payload)
	}
	req := drawing.RankTraceRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderRankTraceImage(req)
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
	return usererror.Wrap(usererror.CodeNotFound, i18n.M("sk.self_not_ranked", i18n.Data{"Region": i18n.RegionLabel(regionWithDefault(region))}), err)
}

func executeSKPredict(rc *RequestContext, skCtrl *sk.Controller) (drawing.ImageResult, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return drawing.ImageResult{}, err
		}
		return skCtrl.RenderPredictLineFromTrackerImage(trackerReq)
	}
	req := sk.LineRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderLineImage(req)
}

func executeSKWinRate(rc *RequestContext, skCtrl *sk.Controller) (drawing.ImageResult, error) {
	req := drawing.WinRateRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderWinRateImage(req)
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
	if r.RegionExplicit {
		req.RegionExplicit = true
		if resolvedRegion != "" {
			req.Region = resolvedRegion
		}
	} else if !req.RegionExplicit && resolvedRegion != "" {
		req.Region = resolvedRegion
	} else if req.Region == "" {
		req.Region = resolvedRegion
	}
	if len(req.Ranks) == 0 &&
		req.EventID == 0 &&
		req.WlCharacterID == nil &&
		strings.TrimSpace(req.WlCharacterQuery) == "" &&
		req.CompareRank == 0 &&
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
			return usererror.Invalid(i18n.M("sk.wl.no_character_chapter", i18n.Data{"Event": eventLabel(region.String(), req.EventID), "Character": strconv.Itoa(*req.WlCharacterID)}))
		}
		applyTrackerWorldBloomChapterTiming(req, chapter)
		skTrackerDebugLogger.DebugContext(ctx, "world link tracker selection resolved",
			"region", region.String(),
			"event_id", req.EventID,
			"character_id", *req.WlCharacterID,
			"source", "explicit",
		)
		req.WlCharacterQuery = ""
		return nil
	}

	chapter, err := resolveTrackerWorldBloomChapterSelection(ctx, app, region, eventInfo, chapters, query)
	if err != nil {
		return err
	}
	if chapter.GameCharacterID <= 0 {
		return usererror.New(usererror.CodeUnavailable, i18n.M("sk.wl.chapter_incomplete", i18n.Data{"Event": eventLabel(region.String(), req.EventID)}))
	}

	charID := int(chapter.GameCharacterID)
	req.WlCharacterID = drawing.IntPtr(charID)
	req.WlCharacterQuery = ""
	applyTrackerWorldBloomChapterTiming(req, chapter)
	skTrackerDebugLogger.DebugContext(ctx, "world link tracker selection resolved",
		"region", region.String(),
		"event_id", req.EventID,
		"chapter", chapter.ChapterNo,
		"character_id", charID,
		"source", "query",
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

	isSelfTarget := trackerTargetIsRequester(targetPlatform, targetUserID, requesterPlatform, requesterUserID)
	binding, err := resolveTrackerTargetBinding(ctx, app.Bindings, req, targetPlatform, targetUserID, targetSelector, isSelfTarget)
	if err != nil {
		return err
	}
	if binding == nil {
		return accountdata.ErrNoBinding
	}
	if targetSelector == "" && !binding.Visible && !isSelfTarget {
		return usererror.Forbidden(i18n.M("binding.target_hidden"))
	}

	uid, parseErr := strconv.ParseInt(strings.TrimSpace(binding.PJSKUserID), 10, 64)
	if parseErr != nil || uid <= 0 {
		return usererror.New(usererror.CodeInternal, i18n.M("binding.target_uid_invalid"))
	}
	req.UserID = &uid
	if !req.RegionExplicit {
		req.Region = normalizeTrackerRegion(binding.Server)
	}
	return nil
}

func resolveTrackerTargetBinding(
	ctx context.Context,
	bindings *accountdata.BindingService,
	req *sk.TrackerRankQuery,
	targetPlatform, targetUserID, targetSelector string,
	self bool,
) (*accountdata.ResolvedBinding, error) {
	// The requester's own lookup gets the usual "please bind first" reply;
	// another user's names that user.
	notBound := func(message i18n.Message) i18n.Message {
		if self {
			return i18n.Message{}
		}
		return message
	}
	if targetSelector != "" {
		_, binding, err := bindings.ResolveUserBindingBySelector(ctx, targetPlatform, targetUserID, selectorBindingServer(normalizeTrackerRegion(req.Region), req.RegionExplicit), targetSelector)
		if err != nil {
			return nil, normalizeBindingLookupError(err, notBound(i18n.M("binding.target_not_bound")))
		}
		return binding, nil
	}
	if req.RegionExplicit {
		region := normalizeTrackerRegion(req.Region)
		_, binding, err := bindings.ResolveUserBinding(ctx, targetPlatform, targetUserID, region)
		if err != nil {
			return nil, normalizeBindingLookupError(err, notBound(i18n.M("binding.target_not_bound_region", i18n.Data{"Region": i18n.RegionLabel(region)})))
		}
		return binding, nil
	}
	_, binding, err := bindings.ResolveUserBinding(ctx, targetPlatform, targetUserID, accountdata.GlobalDefaultBindingScope)
	if err == nil && binding != nil {
		return binding, nil
	}
	_, binding, err = bindings.ResolveUserBinding(ctx, targetPlatform, targetUserID, DefaultRegionStr)
	if err != nil {
		return nil, normalizeBindingLookupError(err, notBound(i18n.M("binding.target_not_bound")))
	}
	return binding, nil
}

func trackerTargetIsRequester(targetPlatform, targetUserID, requesterPlatform, requesterUserID string) bool {
	targetUserID = strings.TrimSpace(targetUserID)
	return targetUserID != "" &&
		strings.EqualFold(strings.TrimSpace(targetPlatform), strings.TrimSpace(requesterPlatform)) &&
		targetUserID == strings.TrimSpace(requesterUserID)
}

func normalizeTrackerRegion(region string) string {
	return regionWithDefault(region)
}
