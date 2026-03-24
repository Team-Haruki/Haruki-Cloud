package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
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
	accountdata "haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/drawing"
)

// Execute routes a ResolvedCommand to the corresponding execution controller,
// returning the output payload, its data type, or an error.
// This is the main bridge between the parser output and the render system.
func Execute(ctx context.Context, resolved *parser.ResolvedCommand, app *renderapp.App) ([]byte, CommandResultDataType, error) {
	if resolved == nil {
		return nil, "", fmt.Errorf("bridge: nil resolved command")
	}
	if app == nil {
		return nil, "", fmt.Errorf("bridge: nil render app")
	}

	var (
		data []byte
		err  error
	)
	switch resolved.Module {
	case parser.ModuleCard:
		data, err = executeCard(resolved, app)
	case parser.ModuleEvent:
		data, err = executeEvent(resolved, app)
	case parser.ModuleMusic:
		data, err = executeMusic(resolved, app)
	case parser.ModuleGacha:
		data, err = executeGacha(resolved, app)
	case parser.ModuleDeck:
		data, err = executeDeck(resolved, app)
	case parser.ModuleEducation:
		data, err = executeEducation(resolved, app)
	case parser.ModuleSK:
		data, err = executeSK(ctx, resolved, app)
	case parser.ModuleScore:
		data, err = executeScore(resolved, app)
	case parser.ModuleProfile:
		return executeProfile(ctx, resolved, app)
	case parser.ModuleMysekai:
		data, err = executeMysekai(resolved, app)
	case parser.ModuleStamp:
		data, err = executeStamp(resolved, app)
	case parser.ModuleMisc:
		data, err = executeMisc(resolved, app)
	default:
		return nil, "", fmt.Errorf("bridge: unsupported module %v", resolved.Module)
	}
	if err != nil {
		return nil, "", err
	}
	return data, CommandResultDataTypeImagePNG, nil
}

func executeCard(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	switch r.Mode {
	case "card-detail":
		q := card.Query{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Cards.RenderCardDetail(q)
	case "card-list":
		q := card.ListRequest{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Cards.RenderCardList(q)
	case "card-box":
		queries := []card.Query{{Query: r.Query, Region: r.Region}}
		return app.Cards.RenderCardBox(queries)
	default:
		return nil, fmt.Errorf("bridge: unsupported card mode %q", r.Mode)
	}
}

func executeEvent(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	region := renderregion.Value(r.Region)
	switch r.Mode {
	case "event-detail":
		q := event.DetailQuery{Region: region}
		mergeParams(r.Params, &q)
		return app.Events.RenderEventDetail(q)
	case "event-list":
		q := event.ListQuery{Region: region}
		mergeParams(r.Params, &q)
		return app.Events.RenderEventList(q)
	case "event-record":
		req := drawing.EventRecordRequest{}
		mergeParams(r.Params, &req)
		return app.Events.RenderEventRecord(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported event mode %q", r.Mode)
	}
}

func executeMusic(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	switch r.Mode {
	case "music-detail":
		q := music.Query{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Music.RenderMusicDetail(q)
	case "music-list":
		q := music.ListQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Music.RenderMusicList(q)
	case "music-chart":
		q := music.ChartQuery{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Music.RenderMusicChart(q)
	case "music-progress":
		q := music.ProgressQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Music.RenderMusicProgress(q)
	case "music-rewards":
		q := music.RewardsBasicQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Music.RenderMusicRewardsBasic(q)
	default:
		return nil, fmt.Errorf("bridge: unsupported music mode %q", r.Mode)
	}
}

func executeGacha(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	region := renderregion.Value(r.Region)
	switch r.Mode {
	case "gacha":
		q := gacha.ListQuery{Region: region}
		mergeParams(r.Params, &q)
		return app.Gachas.RenderGachaList(q)
	default:
		return nil, fmt.Errorf("bridge: unsupported gacha mode %q", r.Mode)
	}
}

func executeDeck(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
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
	default:
		return nil, fmt.Errorf("bridge: unsupported deck mode %q", r.Mode)
	}
	q := deck.AutoQuery{Region: r.Region, RecommendType: recommendType}
	mergeParams(r.Params, &q)
	return app.Decks.RenderAutoRecommend(q)
}

func executeEducation(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	region := renderregion.Value(r.Region)
	switch r.Mode {
	case "education-challenge":
		q := education.ChallengeLiveQuery{Region: region}
		mergeParams(r.Params, &q)
		return app.Edu.RenderChallengeLiveDetails(q)
	case "education-power":
		req := drawing.PowerBonusDetailRequest{}
		mergeParams(r.Params, &req)
		return app.Edu.RenderPowerBonusDetail(req)
	case "education-area":
		req := drawing.AreaItemUpgradeMaterialsRequest{}
		mergeParams(r.Params, &req)
		return app.Edu.RenderAreaItemUpgradeMaterials(req)
	case "education-bonds":
		req := drawing.BondsRequest{}
		mergeParams(r.Params, &req)
		return app.Edu.RenderBonds(req)
	case "education-leader":
		req := drawing.LeaderCountRequest{}
		mergeParams(r.Params, &req)
		return app.Edu.RenderLeaderCount(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported education mode %q", r.Mode)
	}
}

func executeSK(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	switch r.Mode {
	case "sk-line":
		if trackerReq, ok := trackerRankQueryFromParams(r); ok {
			if err := resolveTrackerTargetUser(ctx, app, &trackerReq); err != nil {
				return nil, err
			}
			payload, err := app.SK.BuildLineRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			return app.SK.RenderLine(*payload)
		}
		req := sk.LineRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderLine(req)
	case "sk-query":
		if trackerReq, ok := trackerRankQueryFromParams(r); ok {
			if err := resolveTrackerTargetUser(ctx, app, &trackerReq); err != nil {
				return nil, err
			}
			payload, err := app.SK.BuildQueryRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			return app.SK.RenderQuery(*payload)
		}
		req := drawing.SKRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderQuery(req)
	case "sk-check-room":
		if trackerReq, ok := trackerRankQueryFromParams(r); ok {
			if err := resolveTrackerTargetUser(ctx, app, &trackerReq); err != nil {
				return nil, err
			}
			payload, err := app.SK.BuildCheckRoomRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			return app.SK.RenderCheckRoom(*payload)
		}
		req := drawing.CFRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderCheckRoom(req)
	case "sk-speed":
		if trackerReq, ok := trackerRankQueryFromParams(r); ok {
			if err := resolveTrackerTargetUser(ctx, app, &trackerReq); err != nil {
				return nil, err
			}
			payload, err := app.SK.BuildSpeedRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			return app.SK.RenderSpeed(*payload)
		}
		req := drawing.SpeedRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderSpeed(req)
	case "sk-player-trace":
		req := drawing.PlayerTraceRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderPlayerTrace(req)
	case "sk-rank-trace":
		if trackerReq, ok := trackerRankQueryFromParams(r); ok {
			if err := resolveTrackerTargetUser(ctx, app, &trackerReq); err != nil {
				return nil, err
			}
			payload, err := app.SK.BuildRankTraceRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			return app.SK.RenderRankTrace(*payload)
		}
		req := drawing.RankTraceRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderRankTrace(req)
	case "sk-winrate":
		req := drawing.WinRateRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderWinRate(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported sk mode %q", r.Mode)
	}
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
		return fmt.Errorf("暂不支持@用户查询：绑定服务未就绪，请改用游戏UID")
	}

	bindings, err := app.Bindings.List(ctx, targetPlatform, targetUserID)
	if err != nil {
		return fmt.Errorf("无法解析@用户 %s 的绑定: %w", targetUserID, err)
	}

	selected, ok := pickTrackerBindingByRegion(bindings, req.Region)
	if !ok {
		return fmt.Errorf("@用户 %s 在 %s 服没有可用绑定", targetUserID, strings.ToUpper(strings.TrimSpace(req.Region)))
	}

	uid, parseErr := strconv.ParseInt(strings.TrimSpace(selected.UserID), 10, 64)
	if parseErr != nil || uid <= 0 {
		return fmt.Errorf("@用户 %s 的绑定UID无效: %s", targetUserID, selected.UserID)
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

func executeScore(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	switch r.Mode {
	case "score-control":
		req := drawing.ScoreControlRequest{}
		mergeParams(r.Params, &req)
		return app.Score.RenderScoreControl(req)
	case "score-custom-room":
		req := drawing.CustomRoomScoreRequest{}
		mergeParams(r.Params, &req)
		return app.Score.RenderCustomRoomScore(req)
	case "score-music-meta":
		var req []drawing.MusicMetaRequest
		if r.Params != nil {
			if err := json.Unmarshal(r.Params, &req); err != nil {
				return nil, fmt.Errorf("bridge: unmarshal music-meta params: %w", err)
			}
		}
		return app.Score.RenderMusicMeta(req)
	case "score-music-board":
		req := drawing.MusicBoardRequest{}
		mergeParams(r.Params, &req)
		return app.Score.RenderMusicBoard(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported score mode %q", r.Mode)
	}
}

func executeProfile(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) ([]byte, CommandResultDataType, error) {
	switch r.Mode {
	case ProfileModeRender:
		q := profile.Query{Region: r.Region}
		mergeParams(r.Params, &q)
		data, err := app.Profiles.RenderProfile(q)
		if err != nil {
			return nil, "", err
		}
		return data, CommandResultDataTypeImagePNG, nil
	case accountdata.ProfileModeBind, accountdata.ProfileModeBindList, accountdata.ProfileModeUnbind, accountdata.ProfileModeDefaultSet, accountdata.ProfileModeDefaultClear:
		params, err := accountdata.DecodeProfileBindingParams(r.Params)
		if err != nil {
			return nil, "", err
		}
		data, err := accountdata.ExecuteProfileBindingCommand(ctx, app.Bindings, r.Mode, params)
		if err != nil {
			return nil, "", err
		}
		return data, CommandResultDataTypeText, nil
	default:
		return nil, "", fmt.Errorf("bridge: unsupported profile mode %q", r.Mode)
	}
}

func executeMysekai(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	switch r.Mode {
	case "mysekai-resource":
		q := mysekai.ResourceQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.MySekai.RenderResource(q)
	case "mysekai-fixture-list":
		q := mysekai.FixtureListQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.MySekai.RenderFixtureList(q)
	case "mysekai-fixture-detail":
		q := mysekai.FixtureDetailQuery{Region: r.Region, Query: r.Query}
		mergeParams(r.Params, &q)
		return app.MySekai.RenderFixtureDetail(q)
	case "mysekai-door-upgrade":
		q := mysekai.DoorUpgradeQuery{Region: r.Region, Query: r.Query}
		mergeParams(r.Params, &q)
		return app.MySekai.RenderDoorUpgrade(q)
	case "mysekai-music-record":
		q := mysekai.MusicRecordQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.MySekai.RenderMusicRecord(q)
	case "mysekai-talk-list":
		q := mysekai.TalkListQuery{Region: r.Region, Query: r.Query}
		mergeParams(r.Params, &q)
		return app.MySekai.RenderTalkList(q)
	default:
		return nil, fmt.Errorf("bridge: unsupported mysekai mode %q", r.Mode)
	}
}

func executeStamp(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	region := renderregion.Value(r.Region)
	switch r.Mode {
	case "stamp-list":
		q := stamp.ListQuery{Region: region}
		mergeParams(r.Params, &q)
		return app.Stamps.RenderStampList(q)
	default:
		return nil, fmt.Errorf("bridge: unsupported stamp mode %q", r.Mode)
	}
}

func executeMisc(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	switch r.Mode {
	case "misc-birthday":
		req := drawing.CharaBirthdayRequest{}
		mergeParams(r.Params, &req)
		return app.Misc.RenderCharaBirthday(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported misc mode %q", r.Mode)
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
