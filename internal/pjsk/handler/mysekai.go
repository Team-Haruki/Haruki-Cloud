package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/displaytime"
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
		CommandHandlerBase: CommandHandlerBase{
			Path: "mysekai/resource",
			Commands: []string{
				"/pjsk mysekai res", "/mysekai-resource", "/mysekai资源", "/烤森资源", "/msa",
			},
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
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-resource", params), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiOverviewHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "mysekai/overview",
			Commands: []string{
				"/pjsk mysekai overview", "/mysekai-overview", "/mysekai总览", "/烤森总览", "/msam",
			},
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
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-resource-map", params), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiMapHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "mysekai/map",
			Commands: []string{
				"/pjsk mysekai map", "/mysekai-map", "/mysekai地图", "/烤森地图", "/msm", "/msmap",
			},
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
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-map", params), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiTalkListHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "mysekai/talk-list",
			Commands: []string{
				"/mysekai-talk-list", "/mysekai对话列表", "/烤森对话列表",
			},
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
				return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-list", selfParams), nil
			}
			if _, ok := rendermysekai.ResolveNicknameCharacterID(query); !ok {
				return nil, mysekaiTalkListUsageError(ctx.originalTriggerCmd)
			}
			selfParams["show_id"] = true
			selfParams["show_all_talks"] = showAllTalks
			resolved := makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-talk-list", selfParams)
			resolved.Query = buildMysekaiTalkQuery(unit, query)
			return resolved, nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiFixtureListHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "mysekai/fixture-list",
			Commands: []string{
				"/mysekai-fixture-list", "/mysekai家具列表", "/烤森家具列表",
			},
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
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-list", params), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiFurnitureHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "mysekai/fixture-detail",
			Commands: []string{
				"/pjsk mysekai furniture", "/pjsk mysekai fixture",
				"/msf", "/mysekai 家具", "/家具列表",
			},
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
				return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-list", selfParams), nil
			}

			cleaned = cleanMysekaiArgs(cleaned)
			selfParams["show_id"] = true
			selfParams["obtained_source"] = "fixture"
			if cleaned != "" {
				selfParams["category_query"] = cleaned
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-list", selfParams), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiDoorUpgradeHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "mysekai/door-upgrade",
			Commands: []string{
				"/pjsk mysekai gate", "/mysekai-door-upgrade", "/mysekai大门升级", "/烤森大门升级", "/msg", "/msgate",
			},
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
				resolved := makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-door-upgrade", params)
				resolved.Query = strconv.Itoa(gateID)
				if cleaned != "" {
					resolved.Query = strings.TrimSpace(resolved.Query + " " + cleaned)
				}
				return resolved, nil
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-door-upgrade", params), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiMusicRecordHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "mysekai/music-record",
			Commands: []string{
				"/pjsk mysekai musicrecord", "/mysekai-music-record", "/mysekai唱片", "/烤森唱片", "/mss", "/msr", "/mssong",
			},
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
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-music-record", params), nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiBlueprintHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "mysekai/talk-list",
			Commands: []string{
				"/pjsk mysekai blueprint", "/mysekai blueprint",
				"/msb", "/mysekai 蓝图",
			},
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
				return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-list", selfParams), nil
			}
			if _, ok := rendermysekai.ResolveNicknameCharacterID(query); !ok {
				return nil, mysekaiBlueprintUsageError(ctx.originalTriggerCmd)
			}
			selfParams["show_id"] = true
			selfParams["show_all_talks"] = showAllTalks
			resolved := makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-talk-list", selfParams)
			resolved.Query = buildMysekaiTalkQuery(unit, query)
			return resolved, nil
		},
	}, executeMysekai)
}

func (sekaiHandlers) MysekaiHousingSKHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "mysekai/housing-sk",
			Commands: []string{
				"/百景sk", "/百景SK", "/烤森百景sk", "/烤森百景SK",
				"/mysekai-housing-sk", "/mshsk", "/bjsk",
			},
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
		CommandHandlerBase: CommandHandlerBase{
			Path: "mysekai/photo",
			Commands: []string{
				"/pjsk mysekai photo", "/pjsk mysekai picture",
				"/msp", "/mysekai 照片",
			},
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
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, "mysekai-photo", params), nil
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
		lower := strings.ToLower(clean)
		switch {
		case strings.HasPrefix(lower, "id="), strings.HasPrefix(lower, "housing_id="):
			value := clean[strings.Index(clean, "=")+1:]
			id, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || id <= 0 {
				return query, fmt.Errorf("请输入正确的百景 housing_id")
			}
			query.HousingID = id
		case strings.HasPrefix(lower, "sample="), strings.HasPrefix(lower, "samples="), strings.HasPrefix(lower, "count="):
			value := clean[strings.Index(clean, "=")+1:]
			count, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || count <= 0 {
				return query, fmt.Errorf("请输入正确的刷新次数")
			}
			query.SampleCount = count
		case strings.HasPrefix(lower, "interval="), strings.HasPrefix(lower, "interval_ms="):
			value := clean[strings.Index(clean, "=")+1:]
			interval, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return query, fmt.Errorf("请输入正确的刷新间隔")
			}
			query.SampleIntervalMillis = interval
		default:
			rankTokens = append(rankTokens, clean)
		}
	}
	if len(rankTokens) > 1 && query.HousingID == 0 && isPositiveIntegerToken(rankTokens[0]) && isMysekaiHousingRankRangeToken(rankTokens[1]) {
		id, _ := strconv.Atoi(rankTokens[0])
		query.HousingID = id
		rankTokens = rankTokens[1:]
	}
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
	part = strings.TrimSpace(part)
	if part == "" {
		return nil, nil
	}
	normalized := strings.NewReplacer(
		"～", "-",
		"~", "-",
		"－", "-",
		"—", "-",
		"–", "-",
		"..", "-",
		"到", "-",
		"至", "-",
	).Replace(part)
	if strings.Count(normalized, "-") == 1 && !strings.HasPrefix(normalized, "-") && !strings.HasSuffix(normalized, "-") {
		parts := strings.Split(normalized, "-")
		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || start <= 0 {
			return nil, fmt.Errorf("请输入正确的百景排名")
		}
		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || end <= 0 {
			return nil, fmt.Errorf("请输入正确的百景排名")
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
	rank, err := strconv.Atoi(normalized)
	if err != nil || rank <= 0 {
		return nil, fmt.Errorf("请输入正确的百景排名")
	}
	return []int{rank}, nil
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
	case "mysekai-resource", "mysekai-resource-map", "mysekai-map":
		return true
	default:
		return false
	}
}

func mysekaiRenderContextOptionsForMode(mode string) mySekaiRenderContextOptions {
	switch mode {
	case "mysekai-map", "mysekai-photo":
		return mySekaiRenderContextOptions{
			NeedProfile:          false,
			PreferMySekaiPayload: true,
			MySekaiPayloadOnly:   true,
		}
	case "mysekai-resource", "mysekai-resource-map", "mysekai-fixture-list",
		"mysekai-music-record":
		return mySekaiRenderContextOptions{
			NeedProfile:          true,
			PreferMySekaiPayload: true,
			MySekaiPayloadOnly:   true,
		}
	case "mysekai-door-upgrade":
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
		return mySekaiRegionUnavailableMessage(), nil
	}

	regionStr := rc.RegionStr
	if rc.Cmd.Mode == "mysekai-housing-sk" {
		return executeMysekaiHousingSK(rc, regionWithDefault(regionStr))
	}

	var p userQueryParams
	mergeParams(rc.Cmd.Params, &p)
	if p.Mode == "" {
		p.Mode = "self"
		p.Platform = rc.Platform
		p.PlatformUserID = rc.PlatformUserID
	}

	staticFixtureListQuery := rendermysekai.FixtureListQuery{Region: regionStr}
	if rc.Cmd.Mode == "mysekai-fixture-list" {
		mergeParams(rc.Cmd.Params, &staticFixtureListQuery)
		if isStaticMySekaiFixtureListQuery(staticFixtureListQuery) {
			if strings.TrimSpace(staticFixtureListQuery.Region) == "" {
				staticFixtureListQuery.Region = regionWithDefault(regionStr)
			}
			data, err := rc.App.MySekai.WithContext(rc.Ctx).RenderFixtureList(staticFixtureListQuery)
			if err != nil {
				return nil, err
			}
			return imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
		}
	}
	if rc.Cmd.Mode == "mysekai-fixture-detail" {
		q := rendermysekai.FixtureDetailQuery{Region: regionStr, Query: rc.Cmd.Query}
		mergeParams(rc.Cmd.Params, &q)
		if strings.TrimSpace(q.Region) == "" {
			q.Region = regionWithDefault(regionStr)
		}
		data, err := rc.App.MySekai.WithContext(rc.Ctx).RenderFixtureDetail(q)
		if err != nil {
			return nil, err
		}
		return imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
	}
	staticDoorUpgradeQuery := rendermysekai.DoorUpgradeQuery{Region: regionStr, Query: rc.Cmd.Query}
	if rc.Cmd.Mode == "mysekai-door-upgrade" {
		mergeParams(rc.Cmd.Params, &staticDoorUpgradeQuery)
		if isStaticMySekaiDoorUpgradeQuery(staticDoorUpgradeQuery) {
			if strings.TrimSpace(staticDoorUpgradeQuery.Region) == "" {
				staticDoorUpgradeQuery.Region = regionWithDefault(regionStr)
			}
			data, err := rc.App.MySekai.WithContext(rc.Ctx).RenderDoorUpgrade(staticDoorUpgradeQuery)
			if err != nil {
				return nil, err
			}
			return imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
		}
	}

	renderCtx, err := resolveMySekaiRenderContextWithOptions(rc.Ctx, rc.App, p, regionStr, rc.Cmd.RegionExplicit, mysekaiRenderContextOptionsForMode(rc.Cmd.Mode))
	if err != nil {
		return nil, err
	}
	if !isMySekaiRegionAllowed(rc.Cmd, renderCtx.Region) {
		return mySekaiRegionUnavailableMessage(), nil
	}

	if shouldEnforceMysekaiExpiry(rc.Cmd.Mode) && shouldCheckMysekaiExpiry(rc.Cmd.Params) {
		status, statusErr := renderCtx.Controller.SnapshotStatus(renderCtx.Region, time.Now())
		if statusErr != nil {
			return nil, statusErr
		}
		if status.Expired {
			return nil, buildMysekaiExpiredReplayError(rc, renderCtx.HarukiUserID, status)
		}
	}

	var data []byte
	switch rc.Cmd.Mode {
	case "mysekai-resource":
		hasRemaining, remainingErr := mysekaiMapHasRemainingMaterials(renderCtx, rc.Cmd.Params)
		if remainingErr != nil {
			return nil, remainingErr
		}
		if !hasRemaining {
			return mysekaiNoRemainingMaterialMessage(renderCtx.Region), nil
		}
		q := rendermysekai.ResourceQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = renderCtx.Profile
		data, err = renderCtx.Controller.RenderResource(q)
	case "mysekai-resource-map":
		resourceQuery := rendermysekai.ResourceQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &resourceQuery)
		resourceQuery.Profile = renderCtx.Profile
		mapQuery := rendermysekai.MapQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &mapQuery)
		finishBuild := measurePayloadBuild(rc.Ctx)
		mapPayload, mapBuildErr := renderCtx.Controller.BuildMapRequest(mapQuery)
		finishBuild()
		if mapBuildErr != nil {
			return nil, mapBuildErr
		}
		showHarvested := mapQuery.ShowHarvested != nil && *mapQuery.ShowHarvested
		if !showHarvested && !rendermysekai.MapRequestHasRemainingHarvestResources(mapPayload) {
			return mysekaiNoRemainingMaterialMessage(renderCtx.Region), nil
		}
		message, runErr := executeConcurrentMessages(
			rc.Ctx,
			func(ctx context.Context) (onebot11.Message, error) {
				resourceData, resourceErr := renderCtx.Controller.WithContext(ctx).RenderResource(resourceQuery)
				if resourceErr != nil {
					return nil, resourceErr
				}
				return imageMessage(ctx, resourceData, rc.App, BotModulePJSK)
			},
			func(ctx context.Context) (onebot11.Message, error) {
				mapData, mapErr := renderCtx.Controller.WithContext(ctx).RenderMapRequest(mapPayload)
				if mapErr != nil {
					return nil, mapErr
				}
				return imageMessage(ctx, mapData, rc.App, BotModulePJSK)
			},
		)
		if runErr != nil {
			return nil, runErr
		}
		return message, nil
	case "mysekai-map":
		q := rendermysekai.MapQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &q)
		finishBuild := measurePayloadBuild(rc.Ctx)
		mapPayload, mapBuildErr := renderCtx.Controller.BuildMapRequest(q)
		finishBuild()
		if mapBuildErr != nil {
			return nil, mapBuildErr
		}
		showHarvested := q.ShowHarvested != nil && *q.ShowHarvested
		if !showHarvested && !rendermysekai.MapRequestHasRemainingHarvestResources(mapPayload) {
			return mysekaiNoRemainingMaterialMessage(renderCtx.Region), nil
		}
		data, err = renderCtx.Controller.RenderMapRequest(mapPayload)
		if err != nil {
			return nil, err
		}
		replayMessage, err := imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
		if err != nil {
			return nil, err
		}

		replayMessage = append(replayMessage, onebot11.At(rc.PlatformUserID))
		return replayMessage, nil
	case "mysekai-fixture-list":
		q := rendermysekai.FixtureListQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = renderCtx.Profile
		data, err = renderCtx.Controller.RenderFixtureList(q)
	case "mysekai-door-upgrade":
		q := rendermysekai.DoorUpgradeQuery{Region: renderCtx.Region, Query: rc.Cmd.Query}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = renderCtx.Profile
		data, err = renderCtx.Controller.RenderDoorUpgrade(q)
	case "mysekai-music-record":
		q := rendermysekai.MusicRecordQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = renderCtx.Profile
		data, err = renderCtx.Controller.RenderMusicRecord(q)
	case "mysekai-photo":
		q := rendermysekai.PhotoQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &q)
		result, resolveErr := renderCtx.Controller.ResolvePhoto(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		data, err = rc.App.SekaiAPI.WithContext(rc.Ctx).GetMySekaiImage(result.Region, result.ImagePath)
		if err != nil {
			return nil, fmt.Errorf("获取 MySekai 照片失败：%w", err)
		}
		image, imageErr := imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
		if imageErr != nil {
			return nil, imageErr
		}
		photoTime := "未知"
		if !result.ObtainedAt.IsZero() {
			timeZone := resolveHarukiUserTimeZone(rc.Ctx, rc.App, renderCtx.HarukiUserID)
			loc, _ := displaytime.LoadLocation(timeZone)
			photoTime = displaytime.FormatTime(result.ObtainedAt.In(loc), "2006-01-02 15:04")
		}
		return append(image, onebot11.Text(fmt.Sprintf("拍摄时间: %s", photoTime))), nil
	case "mysekai-talk-list":
		q := rendermysekai.TalkListQuery{Region: renderCtx.Region, Query: rc.Cmd.Query}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = renderCtx.Profile
		data, err = renderCtx.Controller.RenderTalkList(q)
	default:
		return nil, unsupportedModeError("mysekai", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	message, err = imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
	if err != nil {
		return nil, err
	}
	return message, nil
}
