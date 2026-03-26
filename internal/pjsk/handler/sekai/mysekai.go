package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"strconv"
	"strings"
)

func (sekaiHandlers) MysekaiResourceHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/resource",
			Commands: []string{
				"/pjsk mysekai res", "/mysekai-resource", "/mysekai资源", "/烤森资源", "/msr", "/msmap", "/msa",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
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
			if len(params) > 0 {
				return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-resource", params), nil
			}
			return makeResolvedCmd(ctx, parser.ModuleMysekai, "mysekai-resource"), nil
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			showAllTalks := strings.Contains(strings.ToLower(args), "all")
			cleaned := cleanMysekaiArgs(args)
			resolved := makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-talk-list", map[string]any{
				"show_id":        true,
				"show_all_talks": showAllTalks,
			})
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			showID := !strings.Contains(strings.ToLower(args), "noid")
			onlyCraftable := false
			if strings.Contains(strings.ToLower(args), "craft") {
				onlyCraftable = true
			}
			resolved := makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-list", map[string]any{
				"show_id":        showID,
				"only_craftable": onlyCraftable,
			})
			return resolved, nil
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if ids := parseMysekaiFixtureIDs(args); len(ids) > 0 {
				resolved := makeResolvedCmd(ctx, parser.ModuleMysekai, "mysekai-fixture-detail")
				resolved.Query = strings.Join(strings.Fields(args), " ")
				return resolved, nil
			}

			showAllTalks := strings.Contains(strings.ToLower(args), "all")
			cleaned := cleanMysekaiArgs(args)
			if cleaned == "" {
				return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-list", map[string]any{
					"show_id":        true,
					"only_craftable": false,
				}), nil
			}

			resolved := makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-talk-list", map[string]any{
				"show_id":        true,
				"show_all_talks": showAllTalks,
			})
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if gateID, cleaned := extractMysekaiGateID(args); gateID != 0 {
				resolved := makeResolvedCmd(ctx, parser.ModuleMysekai, "mysekai-door-upgrade")
				resolved.Query = strconv.Itoa(gateID)
				if cleaned != "" {
					resolved.Query = strings.TrimSpace(resolved.Query + " " + cleaned)
				}
				return resolved, nil
			}
			return makeResolvedCmd(ctx, parser.ModuleMysekai, "mysekai-door-upgrade"), nil
		},
	}
}

func (sekaiHandlers) MysekaiMusicRecordHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/music-record",
			Commands: []string{
				"/pjsk mysekai musicrecord", "/mysekai-music-record", "/mysekai唱片", "/烤森唱片", "/msm", "/mss", "/mssong",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			showID := strings.Contains(strings.ToLower(args), "id")
			if showID {
				cleaned := strings.TrimSpace(strings.ReplaceAll(strings.ToLower(args), "id", ""))
				ctx.SetArgs(cleaned)
				return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-music-record", map[string]bool{
					"show_id": true,
				}), nil
			}
			return makeResolvedCmd(ctx, parser.ModuleMysekai, "mysekai-music-record"), nil
		},
	}
}

func parseMysekaiFixtureIDs(args string) []int {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return nil
	}
	ids := make([]int, 0, len(fields))
	for _, field := range fields {
		value, err := strconv.Atoi(field)
		if err != nil || value <= 0 {
			return nil
		}
		ids = append(ids, value)
	}
	return ids
}

func cleanMysekaiArgs(args string) string {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return ""
	}
	unitTokens := map[string]struct{}{
		"ln": {}, "mmj": {}, "vbs": {}, "ws": {}, "wxs": {}, "25": {}, "25h": {}, "25ji": {}, "niigo": {}, "vs": {}, "piapro": {},
	}
	var kept []string
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || lower == "all" || lower == "id" {
			continue
		}
		if _, ok := unitTokens[lower]; ok {
			continue
		}
		kept = append(kept, field)
	}
	return strings.TrimSpace(strings.Join(kept, " "))
}

func extractMysekaiGateID(args string) (int, string) {
	lower := strings.ToLower(strings.TrimSpace(args))
	unitMap := map[string]int{
		"light_sound":    1,
		"ln":             1,
		"idol":           2,
		"mmj":            2,
		"street":         3,
		"vbs":            3,
		"theme_park":     4,
		"ws":             4,
		"wxs":            4,
		"school_refusal": 5,
		"25":             5,
		"25h":            5,
		"25ji":           5,
		"niigo":          5,
	}
	for token, gateID := range unitMap {
		if strings.Contains(lower, token) {
			cleaned := strings.TrimSpace(strings.ReplaceAll(lower, token, ""))
			return gateID, cleaned
		}
	}
	return 0, strings.TrimSpace(args)
}

func (sekaiHandlers) MysekaiBlueprintHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk mysekai blueprint", "/mysekai blueprint",
				"/msb", "/mysekai 蓝图",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			showAllTalks := strings.Contains(strings.ToLower(args), "all")
			cid, cleaned := resolveNicknameArg(args)
			if cid == 0 {
				return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-list", map[string]any{
					"show_id":        true,
					"only_craftable": true,
				}), nil
			}
			resolved := makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-talk-list", map[string]any{
				"show_id":        true,
				"show_all_talks": showAllTalks,
			})
			resolved.Query = cleaned
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			seq, err := strconv.Atoi(args)
			if err != nil || seq == 0 {
				return nil, fmt.Errorf("请输入正确的照片编号（从1或-1开始）")
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-photo", map[string]any{
				"seq": seq,
			}), nil
		},
	}
}

func (sekaiHandlers) CheckMysekaiDataHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk check mysekai data",
				"/pjsk烤森抓包数据", "/pjsk烤森抓包", "/烤森抓包", "/烤森抓包数据",
				"/msd",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return nil, fmt.Errorf("TODO: 烤森抓包状态未实现，user_id=%s", ctx.GetUserId())
		},
	}
}
