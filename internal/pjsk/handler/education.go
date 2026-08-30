package handler

import (
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/filteralias"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/education"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func (sekaiHandlers) ChallengeInfoHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "education/challenge",
		Commands: []string{
			"/pjsk challenge info", "/pjsk_challenge_info",
			"/挑战信息", "/挑战详情", "/挑战进度", "/挑战一览", "/每日挑战",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := newSelfQueryParamsMap(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleEducation, "education-challenge", params), nil
		},
	}, executeEducation)
}

func (sekaiHandlers) PowerBonusInfoHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "education/power",
		Commands: []string{
			"/pjsk power bonus info", "/pjsk_power_bonus_info",
			"/加成信息", "/加成详情", "/加成进度", "/加成一览", "/角色加成",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := newSelfQueryParamsMap(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleEducation, "education-power", params), nil
		},
	}, executeEducation)
}

func (sekaiHandlers) AreaItemHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "education/area",
		Commands: []string{
			"/pjsk area item", "/area item",
			"/区域道具", "/区域道具升级", "/区域道具升级材料",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			query, err := buildEducationAreaQuery(ctx.GetArgs(), ctx.originalTriggerCmd)
			if err != nil {
				return nil, err
			}
			params, err := educationAreaParams(ctx, query)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleEducation, educationAreaCommand, params), nil
		},
	}, executeEducation)
}

func educationAreaParams(ctx HarrukiSekaiHandlerContext, query education.AreaItemQuery) (map[string]any, error) {
	params, err := newSelfQueryParamsMap(ctx)
	if err != nil {
		return nil, err
	}
	if query.ShowFull {
		params["show_full"] = true
	}
	if query.Unit != "" {
		params["unit"] = query.Unit
	}
	if query.Cid > 0 {
		params["cid"] = query.Cid
	}
	if query.CharacterQuery != "" {
		params["character_query"] = query.CharacterQuery
	}
	if query.Attr != "" {
		params["attr"] = query.Attr
	}
	if query.Tree {
		params["tree"] = true
	}
	if query.Flower {
		params["flower"] = true
	}
	return params, nil
}

func (sekaiHandlers) BondsHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "education/bonds",
		Commands: []string{
			"/pjsk bonds", "/pjsk bond",
			"/羁绊", "/羁绊等级", "/角色羁绊", "/羁绊信息",
			"/牵绊等级", "/牵绊", "/角色牵绊", "/牵绊信息",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := newSelfQueryParamsMap(ctx)
			if err != nil {
				return nil, err
			}
			if query := strings.TrimSpace(ctx.GetArgs()); query != "" {
				params["character_query"] = query
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleEducation, "education-bonds", params), nil
		},
	}, executeEducation)
}

func (sekaiHandlers) LeaderCountHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "education/leader",
		Commands: []string{
			"/队长统计", "/领队统计", "/角色领队", "/pjsk leader count",
			"/队长次数", "/角色次数", "/队长游玩次数", "/角色游玩次数",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := newSelfQueryParamsMap(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleEducation, "education-leader", params), nil
		},
	}, executeEducation)
}

func educationCharacterMissionUsageError(triggerCmd string) error {
	return onebot11.NewReplayError(
		"参数格式不正确。查看完整用法和任务名列表请发送：%s -help",
		triggerCmd,
	)
}

func (sekaiHandlers) CharacterMissionHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "education/character-mission",
		Commands: []string{
			"/cr任务", "/角色等级任务",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, educationCharacterMissionUsageError(ctx.originalTriggerCmd)
			}

			params, err := newSelfQueryParamsMap(ctx)
			if err != nil {
				return nil, err
			}

			all, remaining := education.ExtractCharacterMissionAllFlag(args)
			query, rest := splitFirstArg(strings.TrimSpace(remaining))
			if strings.TrimSpace(query) == "" {
				return nil, educationCharacterMissionUsageError(ctx.originalTriggerCmd)
			}

			params["character_query"] = query

			if all {
				missionType, unresolved := education.ExtractCharacterMissionType(rest)
				if strings.TrimSpace(missionType) == "" || strings.TrimSpace(unresolved) != "" {
					return nil, educationCharacterMissionUsageError(ctx.originalTriggerCmd)
				}
				params["show_all"] = true
				params["mission_type"] = missionType
			} else if strings.TrimSpace(rest) != "" {
				return nil, educationCharacterMissionUsageError(ctx.originalTriggerCmd)
			}

			return makeCommandRequestWithParams(ctx, parser.ModuleEducation, "education-character-mission", params), nil
		},
	}, executeEducation)
}

func splitFirstArg(args string) (string, string) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return "", ""
	}
	if len(fields) == 1 {
		return fields[0], ""
	}
	return fields[0], strings.TrimSpace(strings.Join(fields[1:], " "))
}

var educationAreaUnitAliases = filteralias.UnitMap()

func buildEducationAreaQuery(args string, triggerCmd string) (education.AreaItemQuery, error) {
	args = strings.TrimSpace(args)
	full, args := extractEducationAreaFullFlag(args)
	if args == "" {
		if full {
			return education.AreaItemQuery{}, educationAreaFullUsageError(triggerCmd)
		}
		return education.AreaItemQuery{}, educationAreaUsageError(triggerCmd)
	}

	plant, args := extractEducationAreaFlag(args, "花树", "树花", "植物")
	tree, args := extractEducationAreaFlag(args, "树", "tree")
	flower, args := extractEducationAreaFlag(args, "花", "flower")
	unit, args := extractEducationAreaUnit(args)
	attr, args := extractEducationAreaAttr(args)
	cid, characterQuery, args := extractEducationAreaCharacter(args)

	if args != "" {
		return education.AreaItemQuery{}, educationAreaUsageError(triggerCmd)
	}

	return education.AreaItemQuery{
		ShowFull:       full,
		Unit:           unit,
		Cid:            cid,
		CharacterQuery: characterQuery,
		Attr:           attr,
		Tree:           tree || plant,
		Flower:         flower || plant,
	}, nil
}

func educationAreaUsageError(triggerCmd string) error {
	return onebot11.NewReplayError("请指定要查询的区域道具分类，例如：%s mmj、%s miku、%s 花树。查看完整用法请发送：%s -help",
		triggerCmd, triggerCmd, triggerCmd, triggerCmd)
}

func educationAreaFullUsageError(triggerCmd string) error {
	return onebot11.NewReplayError("full 需要和区域道具分类一起使用，例如：%s mmj full、%s 花树 full。查看完整用法请发送：%s -help",
		triggerCmd, triggerCmd, triggerCmd)
}

func extractEducationAreaFullFlag(args string) (bool, string) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return false, strings.TrimSpace(args)
	}

	full := false
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "full", "全部":
			full = true
		default:
			remaining = append(remaining, field)
		}
	}
	return full, strings.TrimSpace(strings.Join(remaining, " "))
}

func extractEducationAreaCharacter(args string) (int, string, string) {
	args = strings.TrimSpace(args)
	if args == "" {
		return 0, "", ""
	}
	return 0, args, ""
}

func extractEducationAreaFlag(args string, aliases ...string) (bool, string) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return false, strings.TrimSpace(args)
	}

	aliasSet := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		aliasSet[strings.ToLower(strings.TrimSpace(alias))] = struct{}{}
	}

	found := false
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		if _, ok := aliasSet[strings.ToLower(strings.TrimSpace(field))]; ok {
			found = true
			continue
		}
		remaining = append(remaining, field)
	}
	return found, strings.TrimSpace(strings.Join(remaining, " "))
}

func extractEducationAreaUnit(args string) (string, string) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return "", strings.TrimSpace(args)
	}

	remaining := make([]string, 0, len(fields))
	unit := ""
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if resolved, ok := educationAreaUnitAliases[lower]; ok && unit == "" {
			unit = resolved
			continue
		}
		remaining = append(remaining, field)
	}
	return unit, strings.TrimSpace(strings.Join(remaining, " "))
}

func extractEducationAreaAttr(args string) (string, string) {
	ext := parser.NewExtractor(nil)
	res := ext.ExtractAttribute(args)
	if !res.Found {
		return "", strings.TrimSpace(args)
	}
	return res.Value, strings.TrimSpace(res.Remaining)
}

func executeEducation(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App == nil || rc.App.Edu == nil {
		return nil, unsupportedModeError("education", rc.Cmd.Mode)
	}
	eduCtrl := rc.App.Edu.WithContext(rc.Ctx)
	if data, handled, renderErr := renderFullEducationArea(rc, eduCtrl); handled {
		if renderErr != nil {
			return nil, renderErr
		}
		return rc.ImageMessage(data)
	}

	binding, suiteSnapshot, suiteErr := rc.requireVisibleSuiteSnapshot()
	if suiteErr != nil {
		return nil, suiteErr
	}
	publicDetailedProfile, _ := resolveCommandDisplayProfiles(rc, suiteSnapshot)
	execution := &educationExecution{
		rc:         rc,
		controller: eduCtrl,
		snapshot:   suiteSnapshot,
		profile:    publicDetailedProfile,
	}
	if binding != nil {
		execution.pjskUserID = binding.PJSKUserID
		execution.platform = rc.Platform
		execution.platformUserID = rc.PlatformUserID
		execution.hasMySekaiData = hasUsableMySekaiData(binding)
	}
	data, err := execution.render()
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

func renderFullEducationArea(rc *RequestContext, controller *education.Controller) ([]byte, bool, error) {
	if rc.Cmd.Mode != educationAreaCommand {
		return nil, false, nil
	}
	query := education.AreaItemQuery{Region: rc.Region}
	mergeParams(rc.Cmd.Params, &query)
	setDefaultEducationRegion(&query.Region, rc.Region)
	if !query.ShowFull {
		return nil, false, nil
	}
	if err := resolveEducationAreaQueryCharacter(rc, &query); err != nil {
		return nil, true, err
	}
	request, err := controller.BuildAreaItemUpgradeMaterialsRequestFull(query)
	if err != nil {
		return nil, true, err
	}
	data, err := controller.RenderAreaItemUpgradeMaterials(*request)
	return data, true, err
}

type educationExecution struct {
	rc             *RequestContext
	controller     *education.Controller
	snapshot       snapshot.Snapshot
	profile        *drawing.DetailedProfileCardRequest
	pjskUserID     string
	platform       string
	platformUserID string
	hasMySekaiData bool
}

func (e *educationExecution) render() ([]byte, error) {
	switch e.rc.Cmd.Mode {
	case "education-challenge":
		return e.renderChallenge()
	case "education-bonds":
		return e.renderBonds()
	case "education-leader":
		return e.renderLeader()
	case "education-character-mission":
		return e.renderCharacterMission()
	case "education-power":
		return e.renderPower()
	case educationAreaCommand:
		return e.renderArea()
	default:
		return nil, unsupportedModeError("education", e.rc.Cmd.Mode)
	}
}

func (e *educationExecution) renderChallenge() ([]byte, error) {
	query := education.ChallengeLiveQuery{Region: e.rc.Region}
	mergeParams(e.rc.Cmd.Params, &query)
	setDefaultEducationRegion(&query.Region, e.rc.Region)
	query.Profile = e.profile
	query.Snapshot = e.snapshot
	return e.controller.RenderChallengeLiveDetails(query)
}

func (e *educationExecution) renderBonds() ([]byte, error) {
	query := education.BondsQuery{Region: e.rc.Region}
	mergeParams(e.rc.Cmd.Params, &query)
	setDefaultEducationRegion(&query.Region, e.rc.Region)
	if err := resolveEducationBondsQueryCharacter(e.rc, &query); err != nil {
		return nil, err
	}
	request := drawing.BondsRequest{}
	mergeParams(e.rc.Cmd.Params, &request)
	if len(request.Bonds) == 0 && e.snapshot != nil {
		query.Profile = e.profile
		query.Snapshot = e.snapshot
		if built, err := e.controller.BuildBondsRequestFromSnapshot(query); err == nil {
			request = *built
		}
	}
	return e.controller.RenderBonds(request)
}

func (e *educationExecution) renderLeader() ([]byte, error) {
	request := drawing.LeaderCountRequest{}
	mergeParams(e.rc.Cmd.Params, &request)
	if len(request.LeaderCounts) == 0 && e.snapshot != nil {
		if built, err := e.controller.BuildLeaderCountRequestFromSnapshot(education.LeaderCountQuery{
			Region:   e.rc.Region,
			Profile:  e.profile,
			Snapshot: e.snapshot,
		}); err == nil {
			request = *built
		}
	}
	return e.controller.RenderLeaderCount(request)
}

func (e *educationExecution) renderCharacterMission() ([]byte, error) {
	query := education.CharacterMissionQuery{Region: e.rc.Region}
	mergeParams(e.rc.Cmd.Params, &query)
	setDefaultEducationRegion(&query.Region, e.rc.Region)
	if err := resolveEducationMissionQueryCharacter(e.rc, &query); err != nil {
		return nil, err
	}
	query.Profile = e.profile
	query.Snapshot = e.snapshot
	if query.ShowAll {
		request, err := e.controller.BuildCharacterMissionAllRequestFromSnapshot(query)
		if err != nil {
			return nil, err
		}
		return e.controller.RenderCharacterMissionAll(*request)
	}
	request, err := e.controller.BuildCharacterMissionOverviewRequestFromSnapshot(query)
	if err != nil {
		return nil, err
	}
	return e.controller.RenderCharacterMissionOverview(*request)
}

func (e *educationExecution) renderPower() ([]byte, error) {
	request := drawing.PowerBonusDetailRequest{}
	mergeParams(e.rc.Cmd.Params, &request)
	if powerRequestPopulated(request) {
		return e.controller.RenderPowerBonusDetail(request)
	}
	snap := e.powerSnapshot()
	if snap == nil {
		return e.controller.RenderPowerBonusDetail(request)
	}
	built, err := e.controller.BuildPowerBonusDetailRequestFromSnapshot(education.PowerBonusQuery{
		Region:   e.rc.Region,
		Profile:  e.profile,
		Snapshot: snap,
	})
	if err != nil {
		return nil, err
	}
	return e.controller.RenderPowerBonusDetail(*built)
}

func powerRequestPopulated(request drawing.PowerBonusDetailRequest) bool {
	return len(request.CharaBonuses) > 0 || len(request.UnitBonuses) > 0 || len(request.AttrBonuses) > 0
}

func (e *educationExecution) powerSnapshot() snapshot.Snapshot {
	if !e.hasMySekaiData {
		return e.snapshot
	}
	full := resolveTargetSnapshot(e.rc.Ctx, e.rc.App, e.rc.RegionStr, e.platform, e.platformUserID, e.pjskUserID, true)
	if full != nil {
		return full
	}
	return e.snapshot
}

func (e *educationExecution) renderArea() ([]byte, error) {
	query := education.AreaItemQuery{Region: e.rc.Region}
	mergeParams(e.rc.Cmd.Params, &query)
	setDefaultEducationRegion(&query.Region, e.rc.Region)
	if err := resolveEducationAreaQueryCharacter(e.rc, &query); err != nil {
		return nil, err
	}
	query.Profile = e.profile
	if e.snapshot == nil {
		return e.controller.RenderAreaItemUpgradeMaterials(drawing.AreaItemUpgradeMaterialsRequest{})
	}
	query.Snapshot = e.snapshot
	request, err := e.controller.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(query)
	if err != nil {
		return nil, err
	}
	return e.controller.RenderAreaItemUpgradeMaterials(*request)
}

func setDefaultEducationRegion(region *renderregion.Value, fallback renderregion.Value) {
	if region.IsZero() {
		*region = fallback
	}
}

func resolveEducationAreaQueryCharacter(rc *RequestContext, query *education.AreaItemQuery) error {
	if query.Cid > 0 || strings.TrimSpace(query.CharacterQuery) == "" {
		return nil
	}
	characterID, err := resolveEducationAreaCharacterID(rc.Ctx, rc.App, rc.Region, query.CharacterQuery)
	if err != nil {
		return err
	}
	query.Cid = characterID
	return nil
}

func resolveEducationBondsQueryCharacter(rc *RequestContext, query *education.BondsQuery) error {
	if query.Cid > 0 || strings.TrimSpace(query.CharacterQuery) == "" {
		return nil
	}
	characterID, err := resolveEducationBondsCharacterID(rc.Ctx, rc.App, rc.Region, query.CharacterQuery)
	if err != nil {
		return err
	}
	query.Cid = characterID
	return nil
}

func resolveEducationMissionQueryCharacter(rc *RequestContext, query *education.CharacterMissionQuery) error {
	if query.Cid > 0 || strings.TrimSpace(query.CharacterQuery) == "" {
		return nil
	}
	characterID, err := resolveEducationBondsCharacterID(rc.Ctx, rc.App, rc.Region, query.CharacterQuery)
	if err != nil {
		return err
	}
	query.Cid = characterID
	return nil
}
