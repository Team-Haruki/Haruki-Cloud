package sekai

import (
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
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
			return makeResolvedCmd(ctx, parser.ModuleEducation, "education-area"), nil
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
