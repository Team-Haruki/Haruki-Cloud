package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"strconv"
	"strings"
)

var mysekaiMapIndexToID = map[int]int{
	1: 5,
	2: 6,
	3: 7,
	4: 8,
}

func (sekaiHandlers) MysekaiResourceHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/resource",
			Commands: []string{
				"/pjsk mysekai res", "/mysekai-resource", "/mysekai资源", "/烤森资源", "/msa",
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

func (sekaiHandlers) MysekaiMapHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "mysekai/map",
			Commands: []string{
				"/pjsk mysekai map", "/mysekai-map", "/mysekai地图", "/烤森地图", "/msm", "/msmap",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
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
			if len(params) > 0 {
				return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-map", params), nil
			}
			return makeResolvedCmd(ctx, parser.ModuleMysekai, "mysekai-map"), nil
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
				"/pjsk mysekai musicrecord", "/mysekai-music-record", "/mysekai唱片", "/烤森唱片", "/mss", "/msr", "/mssong",
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

func parseMysekaiMapIDs(args string) ([]int, error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return nil, nil
	}
	result := make([]int, 0, len(fields))
	seen := make(map[int]struct{}, len(fields))
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || lower == "all" {
			continue
		}
		if !isASCIIInt(lower) {
			continue
		}

		// 支持紧凑写法，例如 "13" -> [1, 3]
		if len(lower) > 1 {
			splittable := true
			for _, ch := range lower {
				if ch < '1' || ch > '4' {
					splittable = false
					break
				}
			}
			if splittable {
				for _, ch := range lower {
					index := int(ch - '0')
					mapID := mysekaiMapIndexToID[index]
					if _, ok := seen[mapID]; ok {
						continue
					}
					seen[mapID] = struct{}{}
					result = append(result, mapID)
				}
				continue
			}
		}

		index, _ := strconv.Atoi(lower)
		mapID, ok := mysekaiMapIndexToID[index]
		if !ok {
			return nil, fmt.Errorf("地图编号仅支持 1-4（对应地图ID 5-8）")
		}
		if _, ok := seen[mapID]; ok {
			continue
		}
		seen[mapID] = struct{}{}
		result = append(result, mapID)
	}
	return result, nil
}

func isASCIIInt(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
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
			Path: "mysekai/blueprint",
			Commands: []string{
				"/pjsk mysekai blueprint", "/mysekai blueprint",
				"/msb", "/mysekai 蓝图",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			showAllTalks := strings.Contains(strings.ToLower(args), "all")
			cleaned := cleanMysekaiArgs(args)
			if cleaned == "" {
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
