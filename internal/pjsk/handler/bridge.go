package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/musicalias"
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
		message, err = executeProfileWithCache(ctx, resolved, app)
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
	return onebot11.Message{onebot11.Image(url)}, nil
}

// executeProfileWithCache wraps executeProfile so image results go through the
// image cache layer.
func executeProfileWithCache(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) (onebot11.Message, error) {
	data, dataType, err := executeProfile(ctx, r, app)
	if err != nil {
		return nil, err
	}
	switch dataType {
	case CommandResultDataTypeImagePNG:
		return imageMessage(data, app, BotModulePJSK)
	case CommandResultDataTypeText:
		return onebot11.Message{onebot11.Text(string(data))}, nil
	default:
		return nil, fmt.Errorf("bridge: unsupported profile result type %q", dataType)
	}
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
		queries := []card.Query{{Query: r.Query, Region: r.Region, DetailedProfile: publicDetailedProfile}}
		data, err = app.Cards.RenderCardBox(queries)
	default:
		return nil, fmt.Errorf("bridge: unsupported card mode %q", r.Mode)
	}
	if err != nil {
		return
	}
	url, err = app.ImageCache.StoreAndGetURL(data, BotModulePJSK)
	return onebot11.Message{onebot11.Image(url)}, err
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
		req := drawing.EventRecordRequest{}
		mergeParams(r.Params, &req)
		data, err = app.Events.RenderEventRecord(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported event mode %q", r.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

func executeMusic(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
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
			lines = append(lines, fmt.Sprintf("【%d】%s - %s %d", item.Music.ID, item.Music.Title, strings.ToUpper(item.Difficulty), item.PlayLevel))
		}
		return onebot11.Message{onebot11.Text(strings.Join(lines, "\n"))}, nil
	case "music-cover":
		q := music.Query{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		result, resolveErr := app.Music.ResolveMusicCover(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		image, imageErr := musicAssetMessage(result.JacketPath, app)
		if imageErr != nil {
			return nil, imageErr
		}
		text := fmt.Sprintf("【%d】%s", result.Music.ID, result.Music.Title)
		return append(image, onebot11.Text(text)), nil
	case "music-bpm":
		q := music.Query{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		result, resolveErr := app.Music.ResolveMusicBPM(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		image, imageErr := musicAssetMessage(result.JacketPath, app)
		if imageErr != nil {
			return nil, imageErr
		}
		textLines := []string{
			fmt.Sprintf("【%d】%s", result.Music.ID, result.Music.Title),
			fmt.Sprintf("主 BPM: %s", formatMusicBPM(result.MainBPM)),
			fmt.Sprintf("BPM 变化: %s", formatBPMEvents(result.Events)),
			fmt.Sprintf("谱面来源: %s", strings.ToUpper(result.Difficulty)),
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
		return nil, fmt.Errorf("歌曲别名服务未就绪，请稍后再试")
	}
	data, err := musicalias.ExecuteCommand(ctx, app.Aliases, r.Mode, r.Params)
	if err != nil {
		return nil, err
	}
	return onebot11.Message{onebot11.Text(string(data))}, nil
}

func musicAssetMessage(path string, app *renderapp.App) (onebot11.Message, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("music asset path is empty")
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return onebot11.Message{onebot11.Image(path)}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

func formatBPMEvents(events []music.BPMEvent) string {
	if len(events) == 0 {
		return "无数据"
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
	if r == nil || app == nil || app.Profiles == nil {
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
	target, err := resolveGameTarget(context.Background(), queryParams, region, app)
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
		target, err := resolveGameTarget(context.Background(), queryParams, region, app)
		if err == nil && target.Binding != nil {
			if !hasUsableSuiteData(target.Binding) {
				reason = "当前已关闭 Suite 抓包数据，以下为基于公开信息的估算结果。"
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
					reason = "Suite 抓包数据可用，但奖励明细构建失败，以下为基于公开信息的估算结果。"
				} else {
					reason = "当前无法读取 Suite 抓包奖励明细，以下为基于公开信息的估算结果。"
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
	default:
		return nil, fmt.Errorf("bridge: unsupported deck mode %q", r.Mode)
	}
	q := deck.AutoQuery{Region: r.Region, RecommendType: recommendType}
	mergeParams(r.Params, &q)
	if detail, _ := buildPublicMusicProfiles(r, app); detail != nil {
		q.Profile = detail
	}
	data, err = app.Decks.RenderAutoRecommend(q)
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

func executeEducation(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	var data []byte
	region := renderregion.Value(r.Region)
	publicDetailedProfile, _ := buildPublicMusicProfiles(r, app)
	switch r.Mode {
	case "education-challenge":
		q := education.ChallengeLiveQuery{Region: region}
		mergeParams(r.Params, &q)
		q.Profile = publicDetailedProfile
		data, err = app.Edu.RenderChallengeLiveDetails(q)
	case "education-power":
		req := drawing.PowerBonusDetailRequest{}
		mergeParams(r.Params, &req)
		data, err = app.Edu.RenderPowerBonusDetail(req)
	case "education-area":
		req := drawing.AreaItemUpgradeMaterialsRequest{}
		mergeParams(r.Params, &req)
		data, err = app.Edu.RenderAreaItemUpgradeMaterials(req)
	case "education-bonds":
		req := drawing.BondsRequest{}
		mergeParams(r.Params, &req)
		data, err = app.Edu.RenderBonds(req)
	case "education-leader":
		req := drawing.LeaderCountRequest{}
		mergeParams(r.Params, &req)
		data, err = app.Edu.RenderLeaderCount(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported education mode %q", r.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

func executeSK(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	var data []byte
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
			data, err = app.SK.RenderLine(*payload)
			break
		}
		req := sk.LineRequest{}
		mergeParams(r.Params, &req)
		data, err = app.SK.RenderLine(req)
	case "sk-query":
		if trackerReq, ok := trackerRankQueryFromParams(r); ok {
			if err := resolveTrackerTargetUser(ctx, app, &trackerReq); err != nil {
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
			if err := resolveTrackerTargetUser(ctx, app, &trackerReq); err != nil {
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
			if err := resolveTrackerTargetUser(ctx, app, &trackerReq); err != nil {
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
		req := drawing.PlayerTraceRequest{}
		mergeParams(r.Params, &req)
		data, err = app.SK.RenderPlayerTrace(req)
	case "sk-rank-trace":
		if trackerReq, ok := trackerRankQueryFromParams(r); ok {
			if err := resolveTrackerTargetUser(ctx, app, &trackerReq); err != nil {
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

func executeScore(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	var data []byte
	switch r.Mode {
	case "score-control":
		req := drawing.ScoreControlRequest{}
		mergeParams(r.Params, &req)
		data, err = app.Score.RenderScoreControl(req)
	case "score-custom-room":
		req := drawing.CustomRoomScoreRequest{}
		mergeParams(r.Params, &req)
		data, err = app.Score.RenderCustomRoomScore(req)
	case "score-music-meta":
		var req []drawing.MusicMetaRequest
		if r.Params != nil {
			if err := json.Unmarshal(r.Params, &req); err != nil {
				return nil, fmt.Errorf("bridge: unmarshal music-meta params: %w", err)
			}
		}
		data, err = app.Score.RenderMusicMeta(req)
	case "score-music-board":
		req := drawing.MusicBoardRequest{}
		mergeParams(r.Params, &req)
		data, err = app.Score.RenderMusicBoard(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported score mode %q", r.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

func executeProfile(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) ([]byte, CommandResultDataType, error) {
	switch r.Mode {
	case ProfileModeRender:
		var p userQueryParams
		mergeParams(r.Params, &p)

		region := r.Region
		if region == "" {
			region = string(renderregion.JP)
		}

		target, err := resolveGameTarget(ctx, p, region, app)
		if err != nil {
			return nil, "", err
		}

		resp, err := sekaiutils.GetSekaiAPIClient().GetUserProfile(region, target.PJSKUserID)
		if err != nil {
			return nil, "", fmt.Errorf("获取玩家信息失败：%w", err)
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
			return nil, "", err
		}
		return data, CommandResultDataTypeImagePNG, nil
	case accountdata.ProfileModeBind, accountdata.ProfileModeBindList, accountdata.ProfileModeUnbind, accountdata.ProfileModeDefaultSet, accountdata.ProfileModeDefaultClear:
		if app.Bindings == nil {
			return nil, "", fmt.Errorf("绑定服务未就绪，请稍后再试")
		}
		params, err := accountdata.DecodeProfileBindingParams(r.Params)
		if err != nil {
			return nil, "", err
		}
		data, err := accountdata.ExecuteProfileBindingCommand(ctx, app.Bindings, r.Mode, params)
		if err != nil {
			return nil, "", err
		}
		return data, CommandResultDataTypeText, nil
	case accountdata.ProfileModeHideID, accountdata.ProfileModeShowID,
		accountdata.ProfileModeHideSuite, accountdata.ProfileModeShowSuite,
		accountdata.ProfileModeHideMySekai, accountdata.ProfileModeShowMySekai,
		accountdata.ProfileModeVerify, accountdata.ProfileModeVerifyList,
		accountdata.ProfileModeBGUpload, accountdata.ProfileModeBGClear, accountdata.ProfileModeBGAdjust:
		if app.Bindings == nil {
			return nil, "", fmt.Errorf("绑定服务未就绪，请稍后再试")
		}
		params, err := accountdata.DecodeProfileSettingsParams(r.Params)
		if err != nil {
			return nil, "", err
		}
		data, err := accountdata.ExecuteProfileSettingsCommand(ctx, app.Bindings, r.Mode, params)
		if err != nil {
			return nil, "", err
		}
		return data, CommandResultDataTypeText, nil
	default:
		return nil, "", fmt.Errorf("bridge: unsupported profile mode %q", r.Mode)
	}
}

func executeMysekai(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	var data []byte
	_, publicProfileCard := buildPublicMusicProfiles(r, app)
	switch r.Mode {
	case "mysekai-resource":
		q := mysekai.ResourceQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		q.Profile = publicProfileCard
		data, err = app.MySekai.RenderResource(q)
	case "mysekai-fixture-list":
		q := mysekai.FixtureListQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		q.Profile = publicProfileCard
		data, err = app.MySekai.RenderFixtureList(q)
	case "mysekai-fixture-detail":
		q := mysekai.FixtureDetailQuery{Region: r.Region, Query: r.Query}
		mergeParams(r.Params, &q)
		data, err = app.MySekai.RenderFixtureDetail(q)
	case "mysekai-door-upgrade":
		q := mysekai.DoorUpgradeQuery{Region: r.Region, Query: r.Query}
		mergeParams(r.Params, &q)
		q.Profile = publicProfileCard
		data, err = app.MySekai.RenderDoorUpgrade(q)
	case "mysekai-music-record":
		q := mysekai.MusicRecordQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		q.Profile = publicProfileCard
		data, err = app.MySekai.RenderMusicRecord(q)
	case "mysekai-talk-list":
		q := mysekai.TalkListQuery{Region: r.Region, Query: r.Query}
		mergeParams(r.Params, &q)
		q.Profile = publicProfileCard
		data, err = app.MySekai.RenderTalkList(q)
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
	var data []byte
	region := renderregion.Value(r.Region)
	switch r.Mode {
	case "stamp-list":
		q := stamp.ListQuery{Region: region}
		mergeParams(r.Params, &q)
		data, err = app.Stamps.RenderStampList(q)
	default:
		return nil, fmt.Errorf("bridge: unsupported stamp mode %q", r.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

func executeMisc(r *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	var data []byte
	switch r.Mode {
	case "misc-birthday":
		req := drawing.CharaBirthdayRequest{}
		mergeParams(r.Params, &req)
		data, err = app.Misc.RenderCharaBirthday(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported misc mode %q", r.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, app, BotModulePJSK)
}

// userQueryParams mirrors sekai.UserQueryParams for bridge-side decoding.
type userQueryParams struct {
	Mode           string `json:"mode"`
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	AtUserID       string `json:"at_user_id"`
	PJSKUserID     string `json:"pjsk_user_id"`
}

type resolvedGameTarget struct {
	HarukiUserID int
	PJSKUserID   string
	Visible      bool
	BgSettings   *drawing.ProfileBgSettings
	Binding      *accountdata.ResolvedBinding
}

func resolveGameTarget(ctx context.Context, p userQueryParams, region string, app *renderapp.App) (resolvedGameTarget, error) {
	switch p.Mode {
	case "self":
		hid, binding, err := app.Bindings.ResolveUserBinding(ctx, p.Platform, p.PlatformUserID, region)
		if err != nil {
			return resolvedGameTarget{}, fmt.Errorf("未找到绑定账号：%w", err)
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
			return resolvedGameTarget{}, fmt.Errorf("未找到该用户的绑定账号：%w", err)
		}
		if !binding.Visible {
			return resolvedGameTarget{}, fmt.Errorf("该用户已隐藏个人信息")
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
		return resolvedGameTarget{}, fmt.Errorf("未知的查询模式：%q", p.Mode)
	}
}

// resolveGameUID resolves a game UID from userQueryParams using the identity and
// binding layers in the renderapp. For "at_user" mode the caller's visibility
// is checked; if the target has hidden their profile an error is returned.
//
// Returns (harukiUserID, pjskUserID, visible, error).
// harukiUserID is 0 and visible is true when the mode is "uid".
func resolveGameUID(ctx context.Context, p userQueryParams, region string, app *renderapp.App) (int, string, bool, error) {
	target, err := resolveGameTarget(ctx, p, region, app)
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

	harukiUserID, pjskUserID, _, err := resolveGameUID(ctx, p, region, app)
	if err != nil {
		return nil, err
	}
	resp, err := sekaiutils.GetSekaiAPIClient().GetUserProfile(region, pjskUserID)
	if err != nil {
		return nil, fmt.Errorf("获取玩家信息失败：%w", err)
	}

	// Load the caller's enabled difficulties for self-mode; default for others.
	enabledDiffs := defaultEnabledDiffs()
	if p.Mode == "self" && harukiUserID > 0 && app.PJSK != nil {
		if settings, sErr := query.NewClient(nil, nil, app.PJSK, nil).GetPJSKSettings(ctx, harukiUserID); sErr == nil && settings != nil {
			if len(settings.PJSKEnabledDifficulties) > 0 {
				enabledDiffs = settings.PJSKEnabledDifficulties
			}
		}
	}

	text := formatArrestText(resp, enabledDiffs)
	return onebot11.Message{onebot11.Text(text)}, nil
}

func defaultEnabledDiffs() []sekaiutils.MusicDifficultyType {
	return []sekaiutils.MusicDifficultyType{
		sekaiutils.MusicDifficultyMaster,
		sekaiutils.MusicDifficultyExpert,
	}
}

func formatArrestText(resp *sekaiutils.GetAnotherProfileResponse, diffs []sekaiutils.MusicDifficultyType) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("逮捕: %s (UID: %d) Lv.%d\n",
		resp.User.Name, resp.User.UserID, resp.User.Rank))

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
		sb.WriteString(fmt.Sprintf("[%s] 谱面:%d FC:%d AP:%d\n",
			diff, c.LiveClear, c.FullCombo, c.AllPerfect))
	}

	if resp.UserChallengeLiveSoloResult.HighScore > 0 {
		sb.WriteString(fmt.Sprintf("挑战Live(角色#%d): %s分",
			resp.UserChallengeLiveSoloResult.CharacterID,
			formatInt(resp.UserChallengeLiveSoloResult.HighScore)))
	}

	return strings.TrimRight(sb.String(), "\n")
}

// formatInt formats an integer with comma separators (e.g. 3011947 → "3,011,947").
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

	_, pjskUserID, _, err := resolveGameUID(ctx, p, region, app)
	if err != nil {
		return nil, err
	}

	ts, err := calcRegistrationTime(pjskUserID, region)
	if err != nil {
		return nil, err
	}

	regTime := time.Unix(ts, 0).UTC()
	duration := time.Since(regTime)
	days := int(math.Floor(duration.Hours() / 24))

	text := fmt.Sprintf("UID %s 的注册时间\n%s UTC\n（约 %d 天前）",
		pjskUserID,
		regTime.Format("2006-01-02 15:04:05"),
		days)
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
	switch r.Mode {
	case "mysekai":
		if p.Mode != "self" {
			return nil, fmt.Errorf("MySekai抓包相关内容仅支持查询自己的数据")
		}
		_, binding, err := app.Bindings.ResolveUserBinding(ctx, p.Platform, p.PlatformUserID, region)
		if err != nil {
			return nil, fmt.Errorf("未找到绑定账号：%w", err)
		}
		if !hasUsableMySekaiData(binding) {
			return nil, fmt.Errorf("当前账号没有可用的 MySekai 抓包数据")
		}
		uid, err = strconv.ParseInt(binding.PJSKUserID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("无效的账号ID：%w", err)
		}
		platform = p.Platform
		platformUserID = p.PlatformUserID
		dataType = sekaiutils.ToolboxDataTypeMySekai
		label = "MySekai"
	default:
		if p.Mode != "self" {
			return nil, fmt.Errorf("Suite抓包相关内容仅支持查询自己的数据")
		}
		_, binding, err := app.Bindings.ResolveUserBinding(ctx, p.Platform, p.PlatformUserID, region)
		if err != nil {
			return nil, fmt.Errorf("未找到绑定账号：%w", err)
		}
		if !hasUsableSuiteData(binding) {
			return nil, fmt.Errorf("当前账号没有可用的 Suite 抓包数据")
		}
		uid, err = strconv.ParseInt(binding.PJSKUserID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("无效的账号ID：%w", err)
		}
		platform = p.Platform
		platformUserID = p.PlatformUserID
		dataType = sekaiutils.ToolboxDataTypeSuite
		label = "套件"
	}

	raw, err := sekaiutils.GetToolboxClient().GetUploadTime(region, dataType, uid, platform, platformUserID)
	if err != nil {
		return nil, fmt.Errorf("获取%s更新时间失败：%w", label, err)
	}

	ts, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("解析更新时间失败：%w", err)
	}

	uploadTime := time.Unix(ts, 0).UTC()
	duration := time.Since(uploadTime)
	days := int(math.Floor(duration.Hours() / 24))

	text := fmt.Sprintf("%s 数据更新时间\n%s UTC\n（约 %d 天前）", label, uploadTime.Format("2006-01-02 15:04:05"), days)
	return onebot11.Message{onebot11.Text(text)}, nil
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
			return 0, fmt.Errorf("账号ID格式不正确")
		}
		n, err := strconv.ParseInt(userID[:len(userID)-3], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的账号ID：%w", err)
		}
		return 1600218000 + int64(float64(n)/(1024*4096)), nil
	case "tw", "kr", "cn":
		n, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的账号ID：%w", err)
		}
		return int64(float64(n) / (1024 * 1024 * 4096)), nil
	default:
		return 0, fmt.Errorf("不支持的服务器：%s", server)
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
