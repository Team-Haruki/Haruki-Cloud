package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"

	"golang.org/x/sync/errgroup"
)

func mysekaiBlueprintUsageError(trigger string) error {
	return onebot11.NewReplayError(
		"使用方式:\n%s\n%s 角色名\n查看家具详情请使用：/msf 家具ID",
		trigger,
		trigger,
	)
}

func mysekaiTalkListUsageError(trigger string) error {
	return onebot11.NewReplayError(
		"使用方式:\n%s\n%s 角色名\n查看家具详情请使用：/msf 家具ID",
		trigger,
		trigger,
	)
}

func mysekaiHousingSKUsageError(trigger string) error {
	return onebot11.NewReplayError(
		"使用方式:\n%s\n%s 1-5\n一次最多查询5个排名",
		trigger,
		trigger,
	)
}

func applyMysekaiStaticFixtureListParams(params map[string]any, onlyCraftable bool) {
	params["show_id"] = true
	params["only_craftable"] = onlyCraftable
	params["show_profile"] = false
	params["show_progress"] = false
	params["show_obtained"] = false
}

func (sekaiHandlers) MysekaiResourceHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "mysekai/resource",
		Commands: []string{
			"/pjsk mysekai res", "/mysekai-resource", "/mysekai资源", "/烤森资源", "/msa",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			params := map[string]any{}
			if strings.Contains(strings.ToLower(args), "all") {
				params["show_harvested"] = true
			}
			params["check_time"] = !hasMysekaiForceFlag(args)
			if err := embedSelfQuery(params, ctx); err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiResourceCommand, params), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiOverviewHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "mysekai/overview",
		Commands: []string{
			"/pjsk mysekai overview", "/mysekai-overview", "/mysekai总览", "/烤森总览", "/msam",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			params := map[string]any{}
			if strings.Contains(strings.ToLower(args), "all") {
				params["show_harvested"] = true
			}
			params["check_time"] = !hasMysekaiForceFlag(args)
			mapIDs, parseErr := parseMysekaiMapIDs(args)
			if parseErr != nil {
				return nil, parseErr
			}
			if len(mapIDs) > 0 {
				params["map_ids"] = mapIDs
			}
			if err := embedSelfQuery(params, ctx); err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiResourceMapCommand, params), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiMapHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "mysekai/map",
		Commands: []string{
			"/pjsk mysekai map", "/mysekai-map", "/mysekai地图", "/烤森地图", "/msm", "/msmap",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			params := map[string]any{}
			if strings.Contains(strings.ToLower(args), "all") {
				params["show_harvested"] = true
			}
			params["check_time"] = !hasMysekaiForceFlag(args)
			mapIDs, parseErr := parseMysekaiMapIDs(args)
			if parseErr != nil {
				return nil, parseErr
			}
			if len(mapIDs) > 0 {
				params["map_ids"] = mapIDs
			}
			if err := embedSelfQuery(params, ctx); err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiMapCommand, params), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiTalkListHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "mysekai/talk-list",
		Commands: []string{
			"/mysekai-talk-list", "/mysekai对话列表", "/烤森对话列表",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			selfParams := map[string]any{}
			if err := embedSelfQuery(selfParams, ctx); err != nil {
				return nil, err
			}

			args := strings.TrimSpace(ctx.GetArgs())
			if ids := parseMysekaiFixtureIDs(args); len(ids) > 0 {
				return nil, mysekaiTalkListUsageError(ctx.originalTriggerCmd)
			}
			query, unit, showAllTalks := parseMysekaiBlueprintArgs(args)
			if query == "" {
				selfParams["show_id"] = true
				selfParams["only_craftable"] = true
				selfParams["obtained_source"] = "blueprint"
				return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiFixtureListCommand, selfParams), nil
			}
			if _, ok := rendermysekai.ResolveNicknameCharacterID(query); !ok {
				return nil, mysekaiTalkListUsageError(ctx.originalTriggerCmd)
			}
			selfParams["show_id"] = true
			selfParams["show_all_talks"] = showAllTalks
			resolved := makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiTalkListCommand, selfParams)
			resolved.Query = buildMysekaiTalkQuery(unit, query)
			return resolved, nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiFixtureListHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "mysekai/fixture-list",
		Commands: []string{
			"/mysekai-fixture-list", "/mysekai家具列表", "/烤森家具列表",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			fullList, cleanedArgs := extractMysekaiFullOnlyFlag(args)
			args = cleanedArgs
			showID := !strings.Contains(strings.ToLower(args), "noid")
			onlyCraftable := false
			if strings.Contains(strings.ToLower(args), "craft") {
				onlyCraftable = true
			}
			showProfile := false
			showProgress := false
			showObtained := false
			params := map[string]any{
				"show_id":        showID,
				"only_craftable": onlyCraftable,
				"show_profile":   showProfile,
				"show_progress":  showProgress,
				"show_obtained":  showObtained,
			}
			if fullList {
				params["only_craftable"] = false
			}
			if err := embedSelfQuery(params, ctx); err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiFixtureListCommand, params), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiFurnitureHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "mysekai/fixture-detail",
		Commands: []string{
			"/pjsk mysekai furniture", "/pjsk mysekai fixture",
			"/msf", "/mysekai 家具", "/家具列表",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())

			selfParams := map[string]any{}
			if err := embedSelfQuery(selfParams, ctx); err != nil {
				return nil, err
			}

			if ids := parseMysekaiFixtureIDs(args); len(ids) > 0 {
				resolved := makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-detail", selfParams)
				resolved.Query = strings.Join(strings.Fields(args), " ")
				return resolved, nil
			}

			fullList, cleaned := extractMysekaiFullFlag(args)
			if fullList {
				applyMysekaiStaticFixtureListParams(selfParams, false)
				if cleaned != "" {
					selfParams["category_query"] = cleaned
				}
				return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiFixtureListCommand, selfParams), nil
			}

			cleaned = cleanMysekaiArgs(cleaned)
			selfParams["show_id"] = true
			selfParams["obtained_source"] = "fixture"
			if cleaned != "" {
				selfParams["category_query"] = cleaned
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiFixtureListCommand, selfParams), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiDoorUpgradeHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "mysekai/door-upgrade",
		Commands: []string{
			"/pjsk mysekai gate", "/mysekai-door-upgrade", "/mysekai大门升级", "/烤森大门升级", "/msg", "/msgate",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			showFull, cleanedArgs := extractMysekaiFullOnlyFlag(args)
			args = cleanedArgs
			showAll, cleanedArgs := extractMysekaiAllFlag(args)
			args = cleanedArgs
			ctx.SetArgs(args)
			params := map[string]any{}
			if err := embedSelfQuery(params, ctx); err != nil {
				return nil, err
			}
			if showFull {
				params["show_full"] = true
			}
			if showAll {
				params["show_all"] = true
			}
			if gateID, cleaned := extractMysekaiGateID(args); gateID != 0 {
				resolved := makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiDoorUpgradeCommand, params)
				resolved.Query = strconv.Itoa(gateID)
				if cleaned != "" {
					resolved.Query = strings.TrimSpace(resolved.Query + " " + cleaned)
				}
				return resolved, nil
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiDoorUpgradeCommand, params), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiMusicRecordHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "mysekai/music-record",
		Commands: []string{
			"/pjsk mysekai musicrecord", "/mysekai-music-record", "/mysekai唱片", "/烤森唱片", "/mss", "/msr", "/mssong",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			params := map[string]any{}
			if err := embedSelfQuery(params, ctx); err != nil {
				return nil, err
			}
			showID := strings.Contains(strings.ToLower(args), "id")
			if showID {
				cleaned := strings.TrimSpace(strings.ReplaceAll(strings.ToLower(args), "id", ""))
				ctx.SetArgs(cleaned)
				params["show_id"] = true
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiMusicRecordCommand, params), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiBlueprintHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "mysekai/talk-list",
		Commands: []string{
			"/pjsk mysekai blueprint", "/mysekai blueprint",
			"/msb", "/mysekai 蓝图",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			selfParams := map[string]any{}
			if err := embedSelfQuery(selfParams, ctx); err != nil {
				return nil, err
			}

			args := strings.TrimSpace(ctx.GetArgs())
			if ids := parseMysekaiFixtureIDs(args); len(ids) > 0 {
				return nil, mysekaiBlueprintUsageError(ctx.originalTriggerCmd)
			}
			query, unit, showAllTalks := parseMysekaiBlueprintArgs(args)
			if query == "" {
				selfParams["show_id"] = true
				selfParams["only_craftable"] = true
				selfParams["obtained_source"] = "blueprint"
				return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiFixtureListCommand, selfParams), nil
			}
			if _, ok := rendermysekai.ResolveNicknameCharacterID(query); !ok {
				return nil, mysekaiBlueprintUsageError(ctx.originalTriggerCmd)
			}
			selfParams["show_id"] = true
			selfParams["show_all_talks"] = showAllTalks
			resolved := makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiTalkListCommand, selfParams)
			resolved.Query = buildMysekaiTalkQuery(unit, query)
			return resolved, nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiHousingSKHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "mysekai/housing-sk",
		Commands: []string{
			"/百景sk", "/百景SK", "/烤森百景sk", "/烤森百景SK",
			"/mysekai-housing-sk", "/mshsk", "/bjsk",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			query, err := parseMysekaiHousingSKArgs(strings.TrimSpace(ctx.GetArgs()))
			if err != nil {
				return nil, onebot11.NewReplayError("%v\n\n%s", err, mysekaiHousingSKUsageError(ctx.originalTriggerCmd))
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-housing-sk", query), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiPhotoHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "mysekai/photo",
		Commands: []string{
			"/pjsk mysekai photo", "/pjsk mysekai picture",
			"/msp", "/mysekai 照片",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			seq, err := strconv.Atoi(args)
			if err != nil || seq == 0 {
				return nil, fmt.Errorf("请输入正确的照片编号（从1或-1开始）")
			}
			params := map[string]any{
				"seq": seq,
			}
			if err := embedSelfQuery(params, ctx); err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiPhotoCommand, params), nil
		},
	}, executeMysekai)
}

type concurrentMessageJob func(context.Context) (onebot11.Message, error)

func isStaticMySekaiFixtureListQuery(q rendermysekai.FixtureListQuery) bool {
	showProfile := true
	if q.ShowProfile != nil {
		showProfile = *q.ShowProfile
	}
	showProgress := true
	if q.ShowProgress != nil {
		showProgress = *q.ShowProgress
	}
	showObtained := true
	if q.ShowObtained != nil {
		showObtained = *q.ShowObtained
	}
	return !showProfile && !showProgress && !showObtained
}

func isStaticMySekaiDoorUpgradeQuery(q rendermysekai.DoorUpgradeQuery) bool {
	return q.ShowFull != nil && *q.ShowFull
}

func shouldCheckMysekaiExpiry(params []byte) bool {
	var payload struct {
		CheckTime *bool `json:"check_time,omitempty"`
	}
	mergeParams(params, &payload)
	if payload.CheckTime == nil {
		return true
	}
	return *payload.CheckTime
}

func hasMysekaiForceFlag(args string) bool {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(args)))
	for _, field := range fields {
		if field == "force" {
			return true
		}
	}
	return false
}

func parseMysekaiHousingSKArgs(args string) (rendermysekai.HousingCompetitionLineQuery, error) {
	query := rendermysekai.HousingCompetitionLineQuery{}
	fields := strings.Fields(strings.TrimSpace(args))
	rankTokens := make([]string, 0, len(fields))
	for _, field := range fields {
		clean := strings.Trim(strings.TrimSpace(field), ",，")
		if clean == "" {
			continue
		}
		handled, err := applyMysekaiHousingOption(&query, clean)
		if err != nil {
			return query, err
		}
		if !handled {
			rankTokens = append(rankTokens, clean)
		}
	}
	rankTokens = inferMysekaiHousingID(&query, rankTokens)
	if len(rankTokens) > 0 {
		ranks, err := parseMysekaiHousingRankTokens(rankTokens)
		if err != nil {
			return query, err
		}
		query.Ranks = ranks
	}
	ranks, err := rendermysekai.NormalizeHousingCompetitionRanks(query.Ranks)
	if err != nil {
		return query, err
	}
	query.Ranks = ranks
	return query, nil
}

func applyMysekaiHousingOption(query *rendermysekai.HousingCompetitionLineQuery, field string) (bool, error) {
	key, value, ok := strings.Cut(field, "=")
	if !ok {
		return false, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "id", "housing_id":
		if err != nil || parsed <= 0 {
			return true, fmt.Errorf("请输入正确的百景 housing_id")
		}
		query.HousingID = parsed
	case "sample", "samples", "count":
		if err != nil || parsed <= 0 {
			return true, fmt.Errorf("请输入正确的刷新次数")
		}
		query.SampleCount = parsed
	case "interval", "interval_ms":
		if err != nil {
			return true, fmt.Errorf("请输入正确的刷新间隔")
		}
		query.SampleIntervalMillis = parsed
	default:
		return false, nil
	}
	return true, nil
}

func inferMysekaiHousingID(query *rendermysekai.HousingCompetitionLineQuery, rankTokens []string) []string {
	if len(rankTokens) <= 1 || query.HousingID != 0 {
		return rankTokens
	}
	if !isPositiveIntegerToken(rankTokens[0]) || !isMysekaiHousingRankRangeToken(rankTokens[1]) {
		return rankTokens
	}
	query.HousingID, _ = strconv.Atoi(rankTokens[0])
	return rankTokens[1:]
}

func parseMysekaiHousingRankTokens(tokens []string) ([]int, error) {
	var ranks []int
	for _, token := range tokens {
		for _, part := range splitMysekaiHousingRankToken(token) {
			parsed, err := parseMysekaiHousingRankPart(part)
			if err != nil {
				return nil, err
			}
			ranks = append(ranks, parsed...)
			if len(ranks) > rendermysekai.MaxHousingCompetitionRankCount {
				return nil, fmt.Errorf("一次最多查询%d个百景排名", rendermysekai.MaxHousingCompetitionRankCount)
			}
		}
	}
	return ranks, nil
}

func splitMysekaiHousingRankToken(token string) []string {
	return strings.FieldsFunc(token, func(r rune) bool {
		return r == ',' || r == '，' || r == '、'
	})
}

func parseMysekaiHousingRankPart(part string) ([]int, error) {
	normalized := normalizeMysekaiHousingRankPart(part)
	if normalized == "" {
		return nil, nil
	}
	if isMysekaiHousingRankInterval(normalized) {
		return parseMysekaiHousingRankInterval(normalized)
	}
	rank, err := parsePositiveMysekaiHousingRank(normalized)
	if err != nil {
		return nil, err
	}
	return []int{rank}, nil
}

func normalizeMysekaiHousingRankPart(part string) string {
	return strings.NewReplacer(
		"～", "-",
		"~", "-",
		"－", "-",
		"—", "-",
		"–", "-",
		"..", "-",
		"到", "-",
		"至", "-",
	).Replace(strings.TrimSpace(part))
}

func isMysekaiHousingRankInterval(part string) bool {
	return strings.Count(part, "-") == 1 && !strings.HasPrefix(part, "-") && !strings.HasSuffix(part, "-")
}

func parseMysekaiHousingRankInterval(interval string) ([]int, error) {
	parts := strings.Split(interval, "-")
	start, err := parsePositiveMysekaiHousingRank(parts[0])
	if err != nil {
		return nil, err
	}
	end, err := parsePositiveMysekaiHousingRank(parts[1])
	if err != nil {
		return nil, err
	}
	if end < start {
		start, end = end, start
	}
	if end-start+1 > rendermysekai.MaxHousingCompetitionRankCount {
		return nil, fmt.Errorf("一次最多查询%d个百景排名", rendermysekai.MaxHousingCompetitionRankCount)
	}
	out := make([]int, 0, end-start+1)
	for rank := start; rank <= end; rank++ {
		out = append(out, rank)
	}
	return out, nil
}

func parsePositiveMysekaiHousingRank(value string) (int, error) {
	rank, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || rank <= 0 {
		return 0, fmt.Errorf("请输入正确的百景排名")
	}
	return rank, nil
}

func isPositiveIntegerToken(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isMysekaiHousingRankRangeToken(token string) bool {
	return strings.ContainsAny(token, "-~～－—–") || strings.Contains(token, "到") || strings.Contains(token, "至") || strings.Contains(token, "..")
}

func shouldEnforceMysekaiExpiry(mode string) bool {
	switch mode {
	case mySekaiResourceCommand, mySekaiResourceMapCommand, mySekaiMapCommand:
		return true
	default:
		return false
	}
}

func mysekaiRenderContextOptionsForMode(mode string) mySekaiRenderContextOptions {
	switch mode {
	case mySekaiMapCommand, mySekaiPhotoCommand:
		return mySekaiRenderContextOptions{
			NeedProfile:          false,
			PreferMySekaiPayload: true,
			MySekaiPayloadOnly:   true,
		}
	case mySekaiResourceCommand, mySekaiResourceMapCommand, mySekaiFixtureListCommand,
		mySekaiMusicRecordCommand:
		return mySekaiRenderContextOptions{
			NeedProfile:          true,
			PreferMySekaiPayload: true,
			MySekaiPayloadOnly:   true,
		}
	case mySekaiDoorUpgradeCommand:
		return mySekaiRenderContextOptions{
			NeedProfile:       true,
			SuiteOnlySnapshot: true,
		}
	default:
		return mySekaiRenderContextOptions{NeedProfile: true}
	}
}

func buildMysekaiExpiredReplayError(rc *RequestContext, harukiUserID int, status rendermysekai.SnapshotStatus) error {
	lastUpdated := "未知"
	timeZone := resolveHarukiUserTimeZone(rc.Ctx, rc.App, harukiUserID)
	if harukiUserID <= 0 {
		timeZone = resolveRequesterHarukiUserTimeZone(rc.Ctx, rc.App, rc.Platform, rc.PlatformUserID)
	}
	if !status.LastUpdatedAt.IsZero() {
		loc, _ := displaytime.LoadLocation(timeZone)
		lastUpdated = displaytime.FormatTime(status.LastUpdatedAt.In(loc), "2006-01-02 15:04:05")
	}
	return onebot11.NewReplayError(
		"您的mysekai数据已过期\n上次更新时间: %s\n如果需要查看新的，请重新上传\n如果确定需要看目前数据，请在指令上加force参数",
		lastUpdated,
	)
}

func mysekaiNoRemainingMaterialMessage(region string) onebot11.Message {
	label := strings.ToUpper(strings.TrimSpace(regionWithDefault(region)))
	return onebot11.Message{onebot11.Text(fmt.Sprintf("当前%s服账号已无剩余可获取材料", label))}
}

func mysekaiMapHasRemainingMaterials(renderCtx mySekaiRenderContext, params []byte) (bool, error) {
	q := rendermysekai.MapQuery{Region: renderCtx.Region}
	mergeParams(params, &q)
	showHarvested := q.ShowHarvested != nil && *q.ShowHarvested
	if showHarvested {
		return true, nil
	}
	return renderCtx.Controller.HasRemainingHarvestResources(q)
}

func executeConcurrentMessages(ctx context.Context, jobs ...concurrentMessageJob) (onebot11.Message, error) {
	if len(jobs) == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.TODO()
	}

	group, groupCtx := errgroup.WithContext(ctx)
	messages := make([]onebot11.Message, len(jobs))
	for i, job := range jobs {
		i, job := i, job
		group.Go(func() error {
			message, err := job(groupCtx)
			if err != nil {
				return err
			}
			messages[i] = message
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	combined := make(onebot11.Message, 0, len(messages))
	for _, message := range messages {
		combined = append(combined, message...)
	}
	return combined, nil
}

func executeMysekai(rc *RequestContext) (message onebot11.Message, err error) {
	defer func() {
		mode := ""
		if rc != nil && rc.Cmd != nil {
			mode = rc.Cmd.Mode
		}
		err = normalizeMySekaiUserFacingError(err, mode)
	}()

	if rc.App == nil || rc.App.MySekai == nil {
		return nil, fmt.Errorf("mysekai service unavailable: mysekai controller is not configured")
	}

	if !isMySekaiRegionAllowedForMode(rc.Cmd, regionWithDefault(rc.Cmd.Region)) {
		return rejectCNMySekai(rc)
	}

	regionStr := rc.RegionStr
	if rc.Cmd.Mode == "mysekai-housing-sk" {
		return executeMysekaiHousingSK(rc, regionWithDefault(regionStr))
	}

	if staticMessage, handled, staticErr := executeStaticMysekaiMode(rc, regionStr); handled {
		return staticMessage, staticErr
	}

	p := mysekaiUserQueryParams(rc)
	renderCtx, err := resolveMySekaiRenderContextWithOptions(rc.Ctx, rc.App, p, regionStr, rc.Cmd.RegionExplicit, mysekaiRenderContextOptionsForMode(rc.Cmd.Mode))
	if err != nil {
		return nil, err
	}
	if !isMySekaiRegionAllowed(rc.Cmd, renderCtx.Region) {
		return rejectCNMySekai(rc)
	}

	if err := validateMysekaiSnapshotExpiry(rc, renderCtx); err != nil {
		return nil, err
	}
	return executeResolvedMysekaiMode(rc, renderCtx)
}

func mysekaiUserQueryParams(rc *RequestContext) userQueryParams {
	var params userQueryParams
	mergeParams(rc.Cmd.Params, &params)
	if params.Mode == "" {
		params.Mode = "self"
		params.Platform = rc.Platform
		params.PlatformUserID = rc.PlatformUserID
	}
	return params
}

func executeStaticMysekaiMode(rc *RequestContext, region string) (onebot11.Message, bool, error) {
	switch rc.Cmd.Mode {
	case mySekaiFixtureListCommand:
		query := rendermysekai.FixtureListQuery{Region: region}
		mergeParams(rc.Cmd.Params, &query)
		if !isStaticMySekaiFixtureListQuery(query) {
			return nil, false, nil
		}
		query.Region = defaultMysekaiQueryRegion(query.Region, region)
		data, err := rc.App.MySekai.WithContext(rc.Ctx).RenderFixtureList(query)
		message, err := mysekaiImageResult(rc, data, err)
		return message, true, err
	case "mysekai-fixture-detail":
		query := rendermysekai.FixtureDetailQuery{Region: region, Query: rc.Cmd.Query}
		mergeParams(rc.Cmd.Params, &query)
		query.Region = defaultMysekaiQueryRegion(query.Region, region)
		data, err := rc.App.MySekai.WithContext(rc.Ctx).RenderFixtureDetail(query)
		message, err := mysekaiImageResult(rc, data, err)
		return message, true, err
	case mySekaiDoorUpgradeCommand:
		query := rendermysekai.DoorUpgradeQuery{Region: region, Query: rc.Cmd.Query}
		mergeParams(rc.Cmd.Params, &query)
		if !isStaticMySekaiDoorUpgradeQuery(query) {
			return nil, false, nil
		}
		query.Region = defaultMysekaiQueryRegion(query.Region, region)
		data, err := rc.App.MySekai.WithContext(rc.Ctx).RenderDoorUpgrade(query)
		message, err := mysekaiImageResult(rc, data, err)
		return message, true, err
	default:
		return nil, false, nil
	}
}

func defaultMysekaiQueryRegion(queryRegion, fallback string) string {
	if strings.TrimSpace(queryRegion) == "" {
		return regionWithDefault(fallback)
	}
	return queryRegion
}

func validateMysekaiSnapshotExpiry(rc *RequestContext, renderCtx mySekaiRenderContext) error {
	if !shouldEnforceMysekaiExpiry(rc.Cmd.Mode) || !shouldCheckMysekaiExpiry(rc.Cmd.Params) {
		return nil
	}
	status, err := renderCtx.Controller.SnapshotStatus(renderCtx.Region, time.Now())
	if err != nil {
		return err
	}
	if status.Expired {
		return buildMysekaiExpiredReplayError(rc, renderCtx.HarukiUserID, status)
	}
	return nil
}

func executeResolvedMysekaiMode(rc *RequestContext, renderCtx mySekaiRenderContext) (onebot11.Message, error) {
	switch rc.Cmd.Mode {
	case mySekaiResourceCommand:
		return executeMysekaiResource(rc, renderCtx)
	case mySekaiResourceMapCommand:
		return executeMysekaiResourceMap(rc, renderCtx)
	case mySekaiMapCommand:
		return executeMysekaiMap(rc, renderCtx)
	case mySekaiFixtureListCommand:
		query := rendermysekai.FixtureListQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &query)
		query.Profile = renderCtx.Profile
		data, err := renderCtx.Controller.RenderFixtureList(query)
		return mysekaiImageResult(rc, data, err)
	case mySekaiDoorUpgradeCommand:
		query := rendermysekai.DoorUpgradeQuery{Region: renderCtx.Region, Query: rc.Cmd.Query}
		mergeParams(rc.Cmd.Params, &query)
		query.Profile = renderCtx.Profile
		data, err := renderCtx.Controller.RenderDoorUpgrade(query)
		return mysekaiImageResult(rc, data, err)
	case mySekaiMusicRecordCommand:
		query := rendermysekai.MusicRecordQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &query)
		query.Profile = renderCtx.Profile
		data, err := renderCtx.Controller.RenderMusicRecord(query)
		return mysekaiImageResult(rc, data, err)
	case mySekaiPhotoCommand:
		return executeMysekaiPhoto(rc, renderCtx)
	case mySekaiTalkListCommand:
		query := rendermysekai.TalkListQuery{Region: renderCtx.Region, Query: rc.Cmd.Query}
		mergeParams(rc.Cmd.Params, &query)
		query.Profile = renderCtx.Profile
		data, err := renderCtx.Controller.RenderTalkList(query)
		return mysekaiImageResult(rc, data, err)
	default:
		return nil, unsupportedModeError("mysekai", rc.Cmd.Mode)
	}
}

func executeMysekaiResource(rc *RequestContext, renderCtx mySekaiRenderContext) (onebot11.Message, error) {
	hasRemaining, err := mysekaiMapHasRemainingMaterials(renderCtx, rc.Cmd.Params)
	if err != nil {
		return nil, err
	}
	if !hasRemaining {
		return mysekaiNoRemainingMaterialMessage(renderCtx.Region), nil
	}
	query := rendermysekai.ResourceQuery{Region: renderCtx.Region}
	mergeParams(rc.Cmd.Params, &query)
	query.Profile = renderCtx.Profile
	data, err := renderCtx.Controller.RenderResource(query)
	return mysekaiImageResult(rc, data, err)
}

func executeMysekaiResourceMap(rc *RequestContext, renderCtx mySekaiRenderContext) (onebot11.Message, error) {
	resourceQuery := rendermysekai.ResourceQuery{Region: renderCtx.Region}
	mergeParams(rc.Cmd.Params, &resourceQuery)
	resourceQuery.Profile = renderCtx.Profile
	mapQuery := rendermysekai.MapQuery{Region: renderCtx.Region}
	mergeParams(rc.Cmd.Params, &mapQuery)
	mapPayload, err := buildMysekaiMapPayload(rc.Ctx, renderCtx.Controller, mapQuery)
	if err != nil {
		return nil, err
	}
	if mysekaiMapHasNoRemainingResources(mapQuery, mapPayload) {
		return mysekaiNoRemainingMaterialMessage(renderCtx.Region), nil
	}
	return executeConcurrentMessages(
		rc.Ctx,
		mysekaiResourceMessageJob(rc, renderCtx, resourceQuery),
		mysekaiMapMessageJob(rc, renderCtx, mapPayload),
	)
}

func buildMysekaiMapPayload(ctx context.Context, controller *rendermysekai.Controller, query rendermysekai.MapQuery) (*drawing.MysekaiMsrMapRequest, error) {
	finishBuild := measurePayloadBuild(ctx)
	payload, err := controller.BuildMapRequest(query)
	finishBuild()
	return payload, err
}

func mysekaiMapHasNoRemainingResources(query rendermysekai.MapQuery, payload *drawing.MysekaiMsrMapRequest) bool {
	showHarvested := query.ShowHarvested != nil && *query.ShowHarvested
	return !showHarvested && !rendermysekai.MapRequestHasRemainingHarvestResources(payload)
}

func mysekaiResourceMessageJob(rc *RequestContext, renderCtx mySekaiRenderContext, query rendermysekai.ResourceQuery) concurrentMessageJob {
	return func(ctx context.Context) (onebot11.Message, error) {
		data, err := renderCtx.Controller.WithContext(ctx).RenderResource(query)
		return mysekaiImageResultWithContext(ctx, rc, data, err)
	}
}

func mysekaiMapMessageJob(rc *RequestContext, renderCtx mySekaiRenderContext, payload *drawing.MysekaiMsrMapRequest) concurrentMessageJob {
	return func(ctx context.Context) (onebot11.Message, error) {
		data, err := renderCtx.Controller.WithContext(ctx).RenderMapRequest(payload)
		return mysekaiImageResultWithContext(ctx, rc, data, err)
	}
}

func executeMysekaiMap(rc *RequestContext, renderCtx mySekaiRenderContext) (onebot11.Message, error) {
	query := rendermysekai.MapQuery{Region: renderCtx.Region}
	mergeParams(rc.Cmd.Params, &query)
	mapPayload, err := buildMysekaiMapPayload(rc.Ctx, renderCtx.Controller, query)
	if err != nil {
		return nil, err
	}
	if mysekaiMapHasNoRemainingResources(query, mapPayload) {
		return mysekaiNoRemainingMaterialMessage(renderCtx.Region), nil
	}
	data, err := renderCtx.Controller.RenderMapRequest(mapPayload)
	message, err := mysekaiImageResult(rc, data, err)
	if err != nil {
		return nil, err
	}
	return append(message, onebot11.At(rc.PlatformUserID)), nil
}

func executeMysekaiPhoto(rc *RequestContext, renderCtx mySekaiRenderContext) (onebot11.Message, error) {
	query := rendermysekai.PhotoQuery{Region: renderCtx.Region}
	mergeParams(rc.Cmd.Params, &query)
	result, err := renderCtx.Controller.ResolvePhoto(query)
	if err != nil {
		return nil, err
	}
	data, err := rc.App.SekaiAPI.WithContext(rc.Ctx).GetMySekaiImage(result.Region, result.ImagePath)
	if err != nil {
		return nil, fmt.Errorf("获取 MySekai 照片失败：%w", err)
	}
	image, err := imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
	if err != nil {
		return nil, err
	}
	return append(image, onebot11.Text(fmt.Sprintf("拍摄时间: %s", mysekaiPhotoTime(rc, renderCtx.HarukiUserID, result.ObtainedAt)))), nil
}

func mysekaiPhotoTime(rc *RequestContext, harukiUserID int, obtainedAt time.Time) string {
	if obtainedAt.IsZero() {
		return "未知"
	}
	timeZone := resolveHarukiUserTimeZone(rc.Ctx, rc.App, harukiUserID)
	loc, _ := displaytime.LoadLocation(timeZone)
	return displaytime.FormatTime(obtainedAt.In(loc), "2006-01-02 15:04")
}

func mysekaiImageResult(rc *RequestContext, data []byte, err error) (onebot11.Message, error) {
	return mysekaiImageResultWithContext(rc.Ctx, rc, data, err)
}

func mysekaiImageResultWithContext(ctx context.Context, rc *RequestContext, data []byte, err error) (onebot11.Message, error) {
	if err != nil {
		return nil, err
	}
	return imageMessage(ctx, data, rc.App, BotModulePJSK)
}
