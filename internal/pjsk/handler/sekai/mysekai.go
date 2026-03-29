package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	"strconv"
	"strings"
)

var mysekaiMapIndexToID = map[int]int{
	1: 5,
	2: 7,
	3: 6,
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
			if err := embedSelfQuery(params, ctx); err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-resource", params), nil
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
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
			resolved := makeResolvedCmdWithParams(ctx, parser.ModuleMysekai, "mysekai-fixture-list", params)
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			params := map[string]any{}
			if err := embedSelfQuery(params, ctx); err != nil {
				return nil, err
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
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

var mysekaiBlueprintUnitAliases = map[string]string{
	"l/n":                    "light_sound",
	"ln":                     "light_sound",
	"leoneed":                "light_sound",
	"light_sound":            "light_sound",
	"lightsound":             "light_sound",
	"light_sound_club":       "light_sound",
	"leo/need":               "light_sound",
	"mmj":                    "idol",
	"moremorejump":           "idol",
	"more_more_jump":         "idol",
	"idol":                   "idol",
	"vbs":                    "street",
	"vividbadsquad":          "street",
	"vivid_bad_squad":        "street",
	"street":                 "street",
	"ws":                     "theme_park",
	"wxs":                    "theme_park",
	"wonderlands":            "theme_park",
	"wonderlandsxshowtime":   "theme_park",
	"wonderlands_x_showtime": "theme_park",
	"theme_park":             "theme_park",
	"themepark":              "theme_park",
	"25":                     "school_refusal",
	"25h":                    "school_refusal",
	"25ji":                   "school_refusal",
	"niigo":                  "school_refusal",
	"nightcord":              "school_refusal",
	"school_refusal":         "school_refusal",
	"schoolrefusal":          "school_refusal",
	"25_ji_night_cord_de":    "school_refusal",
}

var mysekaiBlueprintDiscardedUnitTokens = map[string]struct{}{
	"vs":            {},
	"piapro":        {},
	"virtualsinger": {},
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

func parseMysekaiBlueprintArgs(args string) (string, string, bool) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return "", "", false
	}

	showAllTalks := false
	unit := ""
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		switch lower {
		case "", "id":
			continue
		case "all":
			showAllTalks = true
			continue
		}
		if resolved, ok := mysekaiBlueprintUnitAliases[lower]; ok && unit == "" {
			unit = resolved
			continue
		}
		if _, ok := mysekaiBlueprintDiscardedUnitTokens[lower]; ok {
			continue
		}
		remaining = append(remaining, field)
	}
	return strings.TrimSpace(strings.Join(remaining, " ")), unit, showAllTalks
}

func buildMysekaiTalkQuery(unit, query string) string {
	query = strings.TrimSpace(query)
	unit = strings.TrimSpace(unit)
	if query == "" {
		return ""
	}
	if unit == "" {
		return query
	}
	return unit + " " + query
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
