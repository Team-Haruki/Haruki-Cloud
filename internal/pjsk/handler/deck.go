package handler

import (
	"fmt"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/profile"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	"strconv"
	"strings"
)

func buildDeckParamsWithSelfQuery(ctx HarrukiSekaiHandlerContext, mode string) (deckAutoQueryParams, UserQueryParams, error) {
	params, err := buildDeckQueryParams(ctx, mode)
	if err != nil {
		return deckAutoQueryParams{}, UserQueryParams{}, err
	}
	query, err := resolveSelfOnlyQueryParams(ctx)
	if err != nil {
		return deckAutoQueryParams{}, UserQueryParams{}, err
	}
	params.Selector = query.Selector
	return params, query, nil
}

func (sekaiHandlers) EventDeckHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "deck/event",
		Commands: []string{
			"/pjsk event card", "/pjsk event deck", "/pjsk deck",
			"/活动组卡", "/活动组队", "/活动卡组", "/活动配队",
			"/组卡", "/组队", "/配队",
			"/指定属性组卡", "/指定属性组队", "/指定属性卡组", "/指定属性配队",
			"/模拟组卡", "/模拟配队", "/模拟组队", "/模拟卡组",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, _, err := buildDeckParamsWithSelfQuery(ctx, deckEventCommand)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleDeck, deckEventCommand, params), nil
		},
	}, executeDeck)
}

func (sekaiHandlers) ChallengeDeckHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "deck/challenge",
		Commands: []string{
			"/pjsk challenge card", "/pjsk challenge deck",
			"/挑战组卡", "/挑战组队", "/挑战卡组", "/挑战配队",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, _, err := buildDeckParamsWithSelfQuery(ctx, deckChallengeCommand)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleDeck, deckChallengeCommand, params), nil
		},
	}, executeDeck)
}

func (sekaiHandlers) NoEventDeckHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "deck/no-event",
		Commands: []string{
			"/pjsk no event deck", "/pjsk best deck",
			"/长草组卡", "/长草组队", "/长草卡组", "/长草配队",
			"/最强卡组", "/最强组卡", "/最强组队", "/最强配队",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, _, err := buildDeckParamsWithSelfQuery(ctx, deckNoEventCommand)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleDeck, deckNoEventCommand, params), nil
		},
	}, executeDeck)
}

func (sekaiHandlers) BonusDeckHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "deck/bonus",
		Commands: []string{
			"/pjsk bonus deck", "/pjsk bonus card",
			"/加成组卡", "/加成组队", "/加成卡组", "/加成配队",
			"/控分组卡", "/控分组队", "/控分卡组", "/控分配队",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, _, err := buildDeckParamsWithSelfQuery(ctx, deckBonusCommand)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleDeck, deckBonusCommand, params), nil
		},
	}, executeDeck)
}

func (sekaiHandlers) MysekaiDeckHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "deck/mysekai",
		Commands: []string{
			"/mysekai deck", "/pjsk mysekai deck",
			"/烤森组卡", "/烤森组队", "/烤森卡组", "/烤森配队",
			"/ms组卡", "/ms组队", "/ms卡组", "/ms配队",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, p, err := buildDeckParamsWithSelfQuery(ctx, deckMySekaiCommand)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleDeck, deckMySekaiCommand, mysekaiDeckCombinedParams{
				Deck:  params,
				Query: p,
			}), nil
		},
	}, executeDeck)
}

func (sekaiHandlers) ScoreUpHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "deck/score-up",
		Commands: []string{
			"/实效", "/倍率", "/时效", "/pjsk score up",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			parts := strings.Fields(strings.TrimSpace(ctx.GetArgs()))
			if len(parts) != 5 {
				return nil, onebot11.NewReplayError("使用方式: %s 队长技能 技能2 技能3 技能4 技能5\n例: %s 160 160 150 150 150", ctx.GetTriggerCmd(), ctx.GetTriggerCmd())
			}

			values := make([]float64, 0, 5)
			for _, p := range parts {
				v, err := strconv.ParseFloat(p, 64)
				if err != nil || v < 0 {
					return nil, onebot11.NewReplayError("使用方式: %s 队长技能 技能2 技能3 技能4 技能5\n例: %s 160 160 150 150 150", ctx.GetTriggerCmd(), ctx.GetTriggerCmd())
				}
				values = append(values, v)
			}

			leader := values[0]
			others := values[1] + values[2] + values[3] + values[4]
			internalValue := values[0] + values[1] + values[2] + values[3] + values[4]
			scoreUp := leader + others*0.2
			multiplier := scoreUp/100.0 + 1.0
			return makeCommandRequestWithParams(
				ctx,
				parser.ModuleDeck,
				"deck-score-up",
				fmt.Sprintf(
					"队长技能加成: %.4g%%\n内部值: %.4g\n实效: %.4g%%\n倍率: %.4g",
					leader, internalValue, scoreUp, multiplier,
				)), nil
		},
	}, executeDeck)
}

type deckUserTargetParams struct {
	Selector string `json:"selector,omitempty"`
}

func executeDeck(rc *RequestContext) (message onebot11.Message, err error) {
	defer func() {
		region := ""
		mode := ""
		if rc != nil {
			region = rc.RegionStr
			if rc.Cmd != nil {
				mode = rc.Cmd.Mode
			}
		}
		err = normalizeDeckUserFacingErrorForCommand(err, region, mode)
	}()

	if msg, disabled := deckRecommendDisabledMessage(rc); disabled {
		return onebot11.Message{onebot11.Text(msg)}, nil
	}
	switch rc.Cmd.Mode {
	case deckMySekaiCommand:
		return executeMySekaiDeck(rc)
	case "deck-score-up":
		var msg string
		if err := json.Unmarshal(rc.Cmd.Params, &msg); err != nil {
			return nil, err
		}
		return onebot11.Message{onebot11.Text(msg)}, nil
	}
	recommendType, ok := deckRecommendType(rc.Cmd.Mode)
	if !ok {
		return nil, unsupportedModeError("deck", rc.Cmd.Mode)
	}
	return executeStandardDeck(rc, recommendType)
}

func deckRecommendType(mode string) (string, bool) {
	switch mode {
	case deckEventCommand:
		return "event", true
	case deckChallengeCommand:
		return "challenge", true
	case deckNoEventCommand:
		return "no_event", true
	case deckBonusCommand:
		return "bonus", true
	default:
		return "", false
	}
}

func buildDeckDoneText(query deck.AutoQuery) string {
	text := fmt.Sprintf("已处理%s。", formatDeckQuerySummary(query))
	if query.RecommendType == "event" {
		text += "\n如需更加精确、更快、更多可自定义参数的组卡功能，请前往Haruki工具箱使用组卡推荐功能"
	}
	return text
}

func executeStandardDeck(rc *RequestContext, recommendType string) (onebot11.Message, error) {
	q := deck.AutoQuery{Region: rc.Cmd.Region, RecommendType: recommendType}
	mergeParams(rc.Cmd.Params, &q)
	var targetParams deckUserTargetParams
	mergeParams(rc.Cmd.Params, &targetParams)
	detail, snapshot, region, publicResp, err := resolveDeckRenderProfileSnapshotAndPublic(rc, targetParams.Selector)
	if err != nil {
		return nil, err
	}
	q.Region = region
	if detail != nil {
		q.Profile = detail
	}
	q.PublicProfileResp = publicResp
	if err := resolveDeckCharacterSelections(rc.Ctx, &q, rc.App); err != nil {
		return nil, err
	}
	applyDefaultChallengeDeckAutoQueryMusic(&q)
	if err := resolveDeckMusicSelection(&q, rc.App); err != nil {
		return nil, err
	}

	deckCtrl := rc.App.Decks.WithContext(rc.Ctx)
	if snapshot != nil {
		deckCtrl = deckCtrl.WithSnapshot(snapshot)
	}

	data, err := deckCtrl.RenderAutoRecommend(q)
	if err != nil {
		return nil, err
	}
	image, imageErr := rc.ImageMessage(data)
	if imageErr != nil {
		return nil, imageErr
	}
	return append(onebot11.Message{onebot11.Text(buildDeckDoneText(q))}, image...), nil
}

func executeMySekaiDeck(rc *RequestContext) (onebot11.Message, error) {
	var combined struct {
		Deck  json.RawMessage `json:"deck"`
		Query userQueryParams `json:"query"`
	}
	mergeParams(rc.Cmd.Params, &combined)
	regionStr := regionWithDefault(rc.Cmd.Region)
	if !isMySekaiDeckRegionAllowed(rc.Cmd, regionStr) {
		return rejectCNMySekai(rc)
	}
	query := deck.AutoQuery{Region: regionStr, RecommendType: "mysekai"}
	mergeParams(combined.Deck, &query)
	if isTheoreticalDeckQuery(query) {
		return executeTheoreticalMySekaiDeck(rc, query)
	}
	return executeTargetMySekaiDeck(rc, query, combined.Query, regionStr)
}

func executeTheoreticalMySekaiDeck(rc *RequestContext, query deck.AutoQuery) (onebot11.Message, error) {
	if err := prepareMySekaiDeckQuery(rc, &query); err != nil {
		return nil, err
	}
	return renderMySekaiDeck(rc, query, nil)
}

func executeTargetMySekaiDeck(rc *RequestContext, query deck.AutoQuery, params userQueryParams, regionStr string) (onebot11.Message, error) {
	if params.Mode == "" {
		params.Mode = "self"
		params.Platform = strings.TrimSpace(rc.Cmd.RequesterPlatform)
		params.PlatformUserID = strings.TrimSpace(rc.Cmd.RequesterUserID)
	}
	target, err := resolveGameTarget(rc.Ctx, params, regionStr, rc.Cmd.RegionExplicit, rc.App)
	if err != nil {
		return nil, err
	}
	regionStr = resolvedTargetRegion(regionStr, target)
	if !isMySekaiDeckRegionAllowed(rc.Cmd, regionStr) {
		return rejectCNMySekai(rc)
	}
	platform, platformUserID := platformCredentials(params)
	targetSnapshot, err := resolveTargetSnapshotWithError(rc.Ctx, rc.App, regionStr, platform, platformUserID, target.PJSKUserID, false)
	if err != nil {
		return nil, normalizeToolboxDataFetchError(err, "suite", target.Binding)
	}
	if target.Binding != nil && targetSnapshot == nil {
		return nil, newSuiteDataNotFoundReplayErrorForBinding(target.Binding)
	}

	query.Region = regionStr
	if err := prepareMySekaiDeckQuery(rc, &query); err != nil {
		return nil, err
	}
	applyMySekaiDeckProfile(rc, &query, target, regionStr, targetSnapshot)
	return renderMySekaiDeck(rc, query, targetSnapshot)
}

func prepareMySekaiDeckQuery(rc *RequestContext, query *deck.AutoQuery) error {
	explicitEventSelection := hasExplicitMySekaiEventSelection(*query)
	if err := resolveDeckCharacterSelections(rc.Ctx, query, rc.App); err != nil {
		return err
	}
	applyDefaultChallengeDeckAutoQueryMusic(query)
	if !explicitEventSelection {
		preserveImplicitMysekaiWorldBloomMetadata(query)
	}
	return resolveDeckMusicSelection(query, rc.App)
}

func hasExplicitMySekaiEventSelection(query deck.AutoQuery) bool {
	return query.EventID != nil ||
		strings.TrimSpace(query.EventUnit) != "" ||
		strings.TrimSpace(query.EventAttr) != "" ||
		query.WorldBloomEventTurn != nil ||
		query.WorldBloomFinaleTurn != nil ||
		query.WorldBloomCharacterID != nil ||
		strings.TrimSpace(query.WorldBloomCharacterQuery) != ""
}

func applyMySekaiDeckProfile(rc *RequestContext, query *deck.AutoQuery, target ResolvedGameTarget, regionStr string, targetSnapshot rendersnapshot.Snapshot) {
	resp := resolveDeckPublicProfileForTarget(rc, target, regionStr)
	if resp == nil {
		return
	}
	query.PublicProfileResp = resp
	if rc.App.Profiles == nil {
		return
	}
	profileQuery := profile.Query{Region: regionStr, Visible: target.Visible, BgSettings: target.BgSettings}
	finishBuild := measurePayloadBuild(rc.Ctx)
	detail, err := rc.App.Profiles.WithContext(rc.Ctx).BuildDetailedProfileCardFromAPIWithSnapshot(profileQuery, resp, targetSnapshot)
	finishBuild()
	if err == nil {
		query.Profile = detail
	}
}

func renderMySekaiDeck(rc *RequestContext, query deck.AutoQuery, targetSnapshot rendersnapshot.Snapshot) (onebot11.Message, error) {
	deckCtrl := rc.App.Decks.WithContext(rc.Ctx)
	if targetSnapshot != nil {
		deckCtrl = deckCtrl.WithSnapshot(targetSnapshot)
	}
	data, err := deckCtrl.RenderAutoRecommend(query)
	if err != nil {
		return nil, err
	}
	image, err := rc.ImageMessage(data)
	if err != nil {
		return nil, err
	}
	return append(onebot11.Message{onebot11.Text(buildDeckDoneText(query))}, image...), nil
}

func deckRecommendDisabledMessage(rc *RequestContext) (string, bool) {
	if rc == nil || rc.Cmd == nil || rc.App == nil || !isDeckRecommendMode(rc.Cmd.Mode) {
		return "", false
	}
	if !rc.App.Config.DeckRecommend.Disable {
		return "", false
	}
	reason := strings.TrimSpace(rc.App.Config.DeckRecommend.DisableReason)
	return fmt.Sprintf("组卡功能已被禁用\n原因: %s\n如有组卡功能需求，请临时前往Haruki工具箱使用组卡推荐", reason), true
}

func isDeckRecommendMode(mode string) bool {
	switch mode {
	case deckEventCommand, deckChallengeCommand, deckNoEventCommand, deckBonusCommand, deckMySekaiCommand:
		return true
	default:
		return false
	}
}

func preserveImplicitMysekaiWorldBloomMetadata(q *deck.AutoQuery) {
	if q == nil {
		return
	}
	if q.WorldBloomCharacterID != nil && *q.WorldBloomCharacterID > 0 {
		q.MetadataWorldBloomCharacterID = drawing.IntPtr(*q.WorldBloomCharacterID)
	}
	q.EventID = nil
	q.EventUnit = ""
	q.EventAttr = ""
	q.WorldBloomEventTurn = nil
	q.WorldBloomFinaleTurn = nil
	q.MetadataWorldBloomFinale = false
	q.WorldBloomCharacterID = nil
	q.WorldBloomCharacterQuery = ""
}

func isTheoreticalDeckQuery(q deck.AutoQuery) bool {
	return !q.UseCurrentDeck && (q.MaxProfile || q.SubMaxProfile)
}
