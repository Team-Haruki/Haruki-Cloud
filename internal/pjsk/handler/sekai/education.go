package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/education"
	"strings"
)

func (sekaiHandlers) ChallengeInfoHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "education/challenge",
			Commands: []string{
				"/pjsk challenge info", "/pjsk_challenge_info",
				"/挑战信息", "/挑战详情", "/挑战进度", "/挑战一览", "/每日挑战",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleEducation, "education-challenge"), nil
		},
	}
}

func (sekaiHandlers) PowerBonusInfoHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "education/power",
			Commands: []string{
				"/pjsk power bonus info", "/pjsk_power_bonus_info",
				"/加成信息", "/加成详情", "/加成进度", "/加成一览", "/角色加成",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleEducation, "education-power"), nil
		},
	}
}

func (sekaiHandlers) AreaItemHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "education/area",
			Commands: []string{
				"/pjsk area item", "/area item",
				"/区域道具", "/区域道具升级", "/区域道具升级材料",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params, err := buildEducationAreaQuery(ctx.GetArgs(), ctx.originalTriggerCmd)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleEducation, "education-area", params), nil
		},
	}
}

func (sekaiHandlers) BondsHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "education/bonds",
			Commands: []string{
				"/pjsk bonds", "/pjsk bond",
				"/羁绊", "/羁绊等级", "/角色羁绊", "/羁绊信息",
				"/牵绊等级", "/牵绊", "/角色牵绊", "/牵绊信息",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleEducation, "education-bonds"), nil
		},
	}
}

func (sekaiHandlers) LeaderCountHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "education/leader",
			Commands: []string{
				"/队长统计", "/领队统计", "/角色领队", "/pjsk leader count",
				"/队长次数", "/角色次数", "/队长游玩次数", "/角色游玩次数",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleEducation, "education-leader"), nil
		},
	}
}

var educationAreaUnitAliases = map[string]string{
	"l/n":                  "light_sound",
	"ln":                   "light_sound",
	"leoneed":              "light_sound",
	"light_sound":          "light_sound",
	"lightsound":           "light_sound",
	"leo/need":             "light_sound",
	"mmj":                  "idol",
	"moremorejump":         "idol",
	"idol":                 "idol",
	"vbs":                  "street",
	"vividbadsquad":        "street",
	"street":               "street",
	"ws":                   "theme_park",
	"wxs":                  "theme_park",
	"wonderlands":          "theme_park",
	"wonderlandsxshowtime": "theme_park",
	"theme_park":           "theme_park",
	"themepark":            "theme_park",
	"25":                   "school_refusal",
	"25h":                  "school_refusal",
	"25ji":                 "school_refusal",
	"niigo":                "school_refusal",
	"nightcord":            "school_refusal",
	"school_refusal":       "school_refusal",
	"schoolrefusal":        "school_refusal",
	"vs":                   "piapro",
	"piapro":               "piapro",
	"virtualsinger":        "piapro",
}

func buildEducationAreaQuery(args string, triggerCmd string) (education.AreaItemQuery, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return education.AreaItemQuery{}, nil
	}

	tree, args := extractEducationAreaFlag(args, "树", "tree")
	flower, args := extractEducationAreaFlag(args, "花", "flower")
	unit, args := extractEducationAreaUnit(args)
	attr, args := extractEducationAreaAttr(args)
	cid, args := resolveNicknameArg(args)
	args = strings.TrimSpace(args)

	if args != "" {
		return education.AreaItemQuery{}, fmt.Errorf(
			"使用方式:\n%s 团名\n%s 角色名\n%s 属性\n%s 树\n%s 花",
			triggerCmd, triggerCmd, triggerCmd, triggerCmd, triggerCmd,
		)
	}

	return education.AreaItemQuery{
		Unit:   unit,
		Cid:    cid,
		Attr:   attr,
		Tree:   tree,
		Flower: flower,
	}, nil
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
	ext := parser.NewExtractor(currentNicknames)
	res := ext.ExtractAttribute(args)
	if !res.Found {
		return "", strings.TrimSpace(args)
	}
	return res.Value, strings.TrimSpace(res.Remaining)
}
