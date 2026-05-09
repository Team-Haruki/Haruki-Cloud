package handler

import (
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/filteralias"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/education"
	"strings"
)

func (sekaiHandlers) ChallengeInfoHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "education/challenge",
			Commands: []string{
				"/pjsk challenge info", "/pjsk_challenge_info",
				"/挑战信息", "/挑战详情", "/挑战进度", "/挑战一览", "/每日挑战",
			},
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
		CommandHandlerBase: CommandHandlerBase{
			Path: "education/power",
			Commands: []string{
				"/pjsk power bonus info", "/pjsk_power_bonus_info",
				"/加成信息", "/加成详情", "/加成进度", "/加成一览", "/角色加成",
			},
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
		CommandHandlerBase: CommandHandlerBase{
			Path: "education/area",
			Commands: []string{
				"/pjsk area item", "/area item",
				"/区域道具", "/区域道具升级", "/区域道具升级材料",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			query, err := buildEducationAreaQuery(ctx.GetArgs(), ctx.originalTriggerCmd)
			if err != nil {
				return nil, err
			}
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
			return makeCommandRequestWithParams(ctx, parser.ModuleEducation, "education-area", params), nil
		},
	}, executeEducation)
}

func (sekaiHandlers) BondsHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "education/bonds",
			Commands: []string{
				"/pjsk bonds", "/pjsk bond",
				"/羁绊", "/羁绊等级", "/角色羁绊", "/羁绊信息",
				"/牵绊等级", "/牵绊", "/角色牵绊", "/牵绊信息",
			},
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
		CommandHandlerBase: CommandHandlerBase{
			Path: "education/leader",
			Commands: []string{
				"/队长统计", "/领队统计", "/角色领队", "/pjsk leader count",
				"/队长次数", "/角色次数", "/队长游玩次数", "/角色游玩次数",
			},
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
		"使用方式:\n%s 角色名\n%s 角色名 all 任务名",
		triggerCmd,
		triggerCmd,
	)
}

func (sekaiHandlers) CharacterMissionHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "education/character-mission",
			Commands: []string{
				"/cr任务", "/角色等级任务",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, educationCharacterMissionUsageError(ctx.originalTriggerCmd)
			}
			lower := strings.ToLower(args)
			if lower == "help" || args == "帮助" {
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
		return education.AreaItemQuery{}, onebot11.NewReplayError(
			"使用方式:\n%s 团名\n%s 角色名\n%s 属性\n%s 树\n%s 花\n%s full",
			triggerCmd, triggerCmd, triggerCmd, triggerCmd, triggerCmd, triggerCmd,
		)
	}

	tree, args := extractEducationAreaFlag(args, "树", "tree")
	flower, args := extractEducationAreaFlag(args, "花", "flower")
	unit, args := extractEducationAreaUnit(args)
	attr, args := extractEducationAreaAttr(args)
	cid, characterQuery, args := extractEducationAreaCharacter(args)

	if args != "" {
		return education.AreaItemQuery{}, onebot11.NewReplayError(
			"使用方式:\n%s 团名\n%s 角色名\n%s 属性\n%s 树\n%s 花\n%s full",
			triggerCmd, triggerCmd, triggerCmd, triggerCmd, triggerCmd, triggerCmd,
		)
	}

	return education.AreaItemQuery{
		ShowFull:       full,
		Unit:           unit,
		Cid:            cid,
		CharacterQuery: characterQuery,
		Attr:           attr,
		Tree:           tree,
		Flower:         flower,
	}, nil
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
	var data []byte
	region := rc.Region
	regionStr := rc.RegionStr

	if rc.Cmd.Mode == "education-area" {
		query := education.AreaItemQuery{Region: region}
		mergeParams(rc.Cmd.Params, &query)
		if query.Region.IsZero() {
			query.Region = region
		}
		if query.ShowFull {
			if query.Cid <= 0 && strings.TrimSpace(query.CharacterQuery) != "" {
				query.Cid, err = resolveEducationAreaCharacterID(rc.Ctx, rc.App, region, query.CharacterQuery)
				if err != nil {
					return nil, err
				}
			}
			builtReq, buildErr := eduCtrl.BuildAreaItemUpgradeMaterialsRequestFull(query)
			if buildErr != nil {
				return nil, buildErr
			}
			data, err = eduCtrl.RenderAreaItemUpgradeMaterials(*builtReq)
			if err != nil {
				return nil, err
			}
			return rc.ImageMessage(data)
		}
	}

	binding, suiteSnapshot, suiteErr := rc.requireVisibleSuiteSnapshot()
	if suiteErr != nil {
		return nil, suiteErr
	}
	publicDetailedProfile, _ := resolveCommandDisplayProfiles(rc, suiteSnapshot)

	platform := rc.Platform
	platformUserID := rc.PlatformUserID
	var suitePJSKUserID string
	var suitePlatform, suitePlatformUserID string
	if binding != nil {
		suitePJSKUserID = binding.PJSKUserID
		suitePlatform = platform
		suitePlatformUserID = platformUserID
	}

	switch rc.Cmd.Mode {
	case "education-challenge":
		q := education.ChallengeLiveQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		if q.Region.IsZero() {
			q.Region = region
		}
		q.Profile = publicDetailedProfile
		if suiteSnapshot != nil {
			q.Snapshot = suiteSnapshot
		}
		data, err = eduCtrl.RenderChallengeLiveDetails(q)

	case "education-bonds":
		query := education.BondsQuery{Region: region}
		mergeParams(rc.Cmd.Params, &query)
		if query.Region.IsZero() {
			query.Region = region
		}
		if query.Cid <= 0 && strings.TrimSpace(query.CharacterQuery) != "" {
			query.Cid, err = resolveEducationBondsCharacterID(rc.Ctx, rc.App, region, query.CharacterQuery)
			if err != nil {
				return nil, err
			}
		}

		req := drawing.BondsRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if len(req.Bonds) == 0 && suiteSnapshot != nil {
			query.Profile = publicDetailedProfile
			query.Snapshot = suiteSnapshot
			bondsReq, buildErr := eduCtrl.BuildBondsRequestFromSnapshot(query)
			if buildErr == nil {
				req = *bondsReq
			}
		}
		data, err = eduCtrl.RenderBonds(req)

	case "education-leader":
		req := drawing.LeaderCountRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if len(req.LeaderCounts) == 0 && suiteSnapshot != nil {
			leaderReq, buildErr := eduCtrl.BuildLeaderCountRequestFromSnapshot(education.LeaderCountQuery{
				Region:   region,
				Profile:  publicDetailedProfile,
				Snapshot: suiteSnapshot,
			})
			if buildErr == nil {
				req = *leaderReq
			}
		}
		data, err = eduCtrl.RenderLeaderCount(req)

	case "education-character-mission":
		query := education.CharacterMissionQuery{Region: region}
		mergeParams(rc.Cmd.Params, &query)
		if query.Region.IsZero() {
			query.Region = region
		}
		if query.Cid <= 0 && strings.TrimSpace(query.CharacterQuery) != "" {
			query.Cid, err = resolveEducationBondsCharacterID(rc.Ctx, rc.App, region, query.CharacterQuery)
			if err != nil {
				return nil, err
			}
		}
		query.Profile = publicDetailedProfile
		query.Snapshot = suiteSnapshot
		if query.ShowAll {
			req, buildErr := eduCtrl.BuildCharacterMissionAllRequestFromSnapshot(query)
			if buildErr != nil {
				return nil, buildErr
			}
			data, err = eduCtrl.RenderCharacterMissionAll(*req)
		} else {
			req, buildErr := eduCtrl.BuildCharacterMissionOverviewRequestFromSnapshot(query)
			if buildErr != nil {
				return nil, buildErr
			}
			data, err = eduCtrl.RenderCharacterMissionOverview(*req)
		}

	case "education-power":
		req := drawing.PowerBonusDetailRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if len(req.CharaBonuses) == 0 && len(req.UnitBonuses) == 0 && len(req.AttrBonuses) == 0 {
			snapshot := suiteSnapshot
			if binding != nil && hasUsableMySekaiData(binding) {
				if fullSnapshot := resolveTargetSnapshot(rc.Ctx, rc.App, regionStr, suitePlatform, suitePlatformUserID, suitePJSKUserID, true); fullSnapshot != nil {
					snapshot = fullSnapshot
				}
			}
			if snapshot != nil {
				builtReq, buildErr := eduCtrl.BuildPowerBonusDetailRequestFromSnapshot(education.PowerBonusQuery{
					Region:   region,
					Profile:  publicDetailedProfile,
					Snapshot: snapshot,
				})
				if buildErr != nil {
					return nil, buildErr
				}
				req = *builtReq
			}
		}
		data, err = eduCtrl.RenderPowerBonusDetail(req)

	case "education-area":
		query := education.AreaItemQuery{Region: region}
		mergeParams(rc.Cmd.Params, &query)
		if query.Region.IsZero() {
			query.Region = region
		}
		if query.Cid <= 0 && strings.TrimSpace(query.CharacterQuery) != "" {
			query.Cid, err = resolveEducationAreaCharacterID(rc.Ctx, rc.App, region, query.CharacterQuery)
			if err != nil {
				return nil, err
			}
		}
		query.Profile = publicDetailedProfile
		if suiteSnapshot != nil {
			query.Snapshot = suiteSnapshot
			builtReq, buildErr := eduCtrl.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(query)
			if buildErr != nil {
				return nil, buildErr
			}
			data, err = eduCtrl.RenderAreaItemUpgradeMaterials(*builtReq)
			break
		}
		data, err = eduCtrl.RenderAreaItemUpgradeMaterials(drawing.AreaItemUpgradeMaterialsRequest{})

	default:
		return nil, unsupportedModeError("education", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}
