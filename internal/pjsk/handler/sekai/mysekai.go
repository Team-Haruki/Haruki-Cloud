package sekai

import (
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
)

func (sekaiHandlers) MysekaiResourceHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/resource",
			Commands: []string{
				"/pjsk mysekai res", "/mysekai-resource", "/mysekai资源", "/烤森资源", "/msa",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			params := map[string]any{}
			if strings.Contains(strings.ToLower(args), "all") {
				params["show_harvested"] = true
			}
			if !strings.Contains(strings.ToLower(args), "force") {
				params["check_time"] = true
			} else {
				params["check_time"] = false
			}
			if err := embedSelfQuery(params, ctx); err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-resource", params), nil
		},
	}
}

func (sekaiHandlers) MysekaiOverviewHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/overview",
			Commands: []string{
				"/pjsk mysekai overview", "/mysekai-overview", "/mysekai总览", "/烤森总览", "/msam",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			params := map[string]any{}
			if strings.Contains(strings.ToLower(args), "all") {
				params["show_harvested"] = true
			}
			if !strings.Contains(strings.ToLower(args), "force") {
				params["check_time"] = true
			} else {
				params["check_time"] = false
			}
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
			return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-resource-map", params), nil
		},
	}
}

func (sekaiHandlers) MysekaiMapHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/map",
			Commands: []string{
				"/pjsk mysekai map", "/mysekai-map", "/mysekai地图", "/烤森地图", "/msm", "/msmap",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			params := map[string]any{}
			if strings.Contains(strings.ToLower(args), "all") {
				params["show_harvested"] = true
			}
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
			return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-map", params), nil
		},
	}
}

func (sekaiHandlers) MysekaiTalkListHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/talk-list",
			Commands: []string{
				"/mysekai-talk-list", "/mysekai对话列表", "/烤森对话列表",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			showAllTalks := strings.Contains(strings.ToLower(args), "all")
			cleaned := cleanMysekaiArgs(args)
			params := map[string]any{
				"show_id":        true,
				"show_all_talks": showAllTalks,
			}
			if err := embedSelfQuery(params, ctx); err != nil {
				return nil, err
			}
			resolved := makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-talk-list", params)
			resolved.Query = cleaned
			return resolved, nil
		},
	}
}

func (sekaiHandlers) MysekaiFixtureListHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/fixture-list",
			Commands: []string{
				"/mysekai-fixture-list", "/mysekai家具列表", "/烤森家具列表",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			showID := !strings.Contains(strings.ToLower(args), "noid")
			onlyCraftable := false
			if strings.Contains(strings.ToLower(args), "craft") {
				onlyCraftable = true
			}
			params := map[string]any{
				"show_id":        showID,
				"only_craftable": onlyCraftable,
			}
			if err := embedSelfQuery(params, ctx); err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-list", params), nil
		},
	}
}

func (sekaiHandlers) MysekaiFurnitureHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/fixture-detail",
			Commands: []string{
				"/pjsk mysekai furniture", "/pjsk mysekai fixture",
				"/msf", "/mysekai 家具", "/家具列表",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			args := strings.TrimSpace(ctx.GetArgs())

			selfParams := map[string]any{}
			if err := embedSelfQuery(selfParams, ctx); err != nil {
				return nil, err
			}

			if ids := parseMysekaiFixtureIDs(args); len(ids) > 0 {
				resolved := makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-detail", selfParams)
				resolved.Query = strings.Join(strings.Fields(args), " ")
				return resolved, nil
			}

			showAllTalks := strings.Contains(strings.ToLower(args), "all")
			cleaned := cleanMysekaiArgs(args)
			if cleaned == "" {
				selfParams["show_id"] = true
				selfParams["only_craftable"] = false
				return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-list", selfParams), nil
			}

			selfParams["show_id"] = true
			selfParams["show_all_talks"] = showAllTalks
			resolved := makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-talk-list", selfParams)
			resolved.Query = cleaned
			return resolved, nil
		},
	}
}

func (sekaiHandlers) MysekaiDoorUpgradeHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/door-upgrade",
			Commands: []string{
				"/pjsk mysekai gate", "/mysekai-door-upgrade", "/mysekai大门升级", "/烤森大门升级", "/msg", "/msgate",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			showAll, cleanedArgs := extractMysekaiAllFlag(args)
			args = cleanedArgs
			ctx.SetArgs(args)
			params := map[string]any{}
			if err := embedSelfQuery(params, ctx); err != nil {
				return nil, err
			}
			if showAll {
				params["show_all"] = true
			}
			if gateID, cleaned := extractMysekaiGateID(args); gateID != 0 {
				resolved := makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-door-upgrade", params)
				resolved.Query = strconv.Itoa(gateID)
				if cleaned != "" {
					resolved.Query = strings.TrimSpace(resolved.Query + " " + cleaned)
				}
				return resolved, nil
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-door-upgrade", params), nil
		},
	}
}

func (sekaiHandlers) MysekaiMusicRecordHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/music-record",
			Commands: []string{
				"/pjsk mysekai musicrecord", "/mysekai-music-record", "/mysekai唱片", "/烤森唱片", "/mss", "/msr", "/mssong",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
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
			return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-music-record", params), nil
		},
	}
}

func (sekaiHandlers) MysekaiBlueprintHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/blueprint",
			Commands: []string{
				"/pjsk mysekai blueprint", "/mysekai blueprint",
				"/msb", "/mysekai 蓝图",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			selfParams := map[string]any{}
			if err := embedSelfQuery(selfParams, ctx); err != nil {
				return nil, err
			}

			args := strings.TrimSpace(ctx.GetArgs())
			query, unit, showAllTalks := parseMysekaiBlueprintArgs(args)
			if query == "" {
				selfParams["show_id"] = true
				selfParams["only_craftable"] = true
				return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-list", selfParams), nil
			}
			if _, ok := rendermysekai.ResolveNicknameCharacterID(query); !ok {
				selfParams["show_id"] = true
				selfParams["only_craftable"] = true
				return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-list", selfParams), nil
			}
			selfParams["show_id"] = true
			selfParams["show_all_talks"] = showAllTalks
			resolved := makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-talk-list", selfParams)
			resolved.Query = buildMysekaiTalkQuery(unit, query)
			return resolved, nil
		},
	}
}

func (sekaiHandlers) MysekaiPhotoHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/photo",
			Commands: []string{
				"/pjsk mysekai photo", "/pjsk mysekai picture",
				"/msp", "/mysekai 照片",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
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
			return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-photo", params), nil
		},
	}
}
