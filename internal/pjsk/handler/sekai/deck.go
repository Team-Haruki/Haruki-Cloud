package sekai

import (
	"fmt"
	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"strconv"
	"strings"
)

func buildDeckParamsWithSelfQuery(ctx SekaiHandlerContext, mode string) (deckAutoQueryParams, UserQueryParams, error) {
	params, err := buildDeckQueryParams(ctx, mode)
	if err != nil {
		return deckAutoQueryParams{}, UserQueryParams{}, err
	}
	query, err := resolveSelfOnlyQueryParams(ctx)
	if err != nil {
		return deckAutoQueryParams{}, UserQueryParams{}, err
	}
	params.Selector = query.Selector
	return params, query, nil
}

func (sekaiHandlers) EventDeckHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "deck/event",
			Commands: []string{
				"/pjsk event card", "/pjsk event deck", "/pjsk deck",
				"/活动组卡", "/活动组队", "/活动卡组", "/活动配队",
				"/组卡", "/组队", "/配队",
				"/指定属性组卡", "/指定属性组队", "/指定属性卡组", "/指定属性配队",
				"/模拟组卡", "/模拟配队", "/模拟组队", "/模拟卡组",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, _, err := buildDeckParamsWithSelfQuery(ctx, "deck-event")
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleDeck, "deck-event", params), nil
		},
	}
}

func (sekaiHandlers) ChallengeDeckHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "deck/challenge",
			Commands: []string{
				"/pjsk challenge card", "/pjsk challenge deck",
				"/挑战组卡", "/挑战组队", "/挑战卡组", "/挑战配队",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, _, err := buildDeckParamsWithSelfQuery(ctx, "deck-challenge")
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleDeck, "deck-challenge", params), nil
		},
	}
}

func (sekaiHandlers) NoEventDeckHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "deck/no-event",
			Commands: []string{
				"/pjsk no event deck", "/pjsk best deck",
				"/长草组卡", "/长草组队", "/长草卡组", "/长草配队",
				"/最强卡组", "/最强组卡", "/最强组队", "/最强配队",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, _, err := buildDeckParamsWithSelfQuery(ctx, "deck-no-event")
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleDeck, "deck-no-event", params), nil
		},
	}
}

func (sekaiHandlers) BonusDeckHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "deck/bonus",
			Commands: []string{
				"/pjsk bonus deck", "/pjsk bonus card",
				"/加成组卡", "/加成组队", "/加成卡组", "/加成配队",
				"/控分组卡", "/控分组队", "/控分卡组", "/控分配队",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, _, err := buildDeckParamsWithSelfQuery(ctx, "deck-bonus")
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleDeck, "deck-bonus", params), nil
		},
	}
}

func (sekaiHandlers) MysekaiDeckHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "deck/mysekai",
			Commands: []string{
				"/mysekai deck", "/pjsk mysekai deck",
				"/烤森组卡", "/烤森组队", "/烤森卡组", "/烤森配队",
				"/ms组卡", "/ms组队", "/ms卡组", "/ms配队",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, p, err := buildDeckParamsWithSelfQuery(ctx, "deck-mysekai")
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleDeck, "deck-mysekai", mysekaiDeckCombinedParams{
				Deck:  params,
				Query: p,
			}), nil
		},
	}
}

func (sekaiHandlers) ScoreUpHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "deck/score-up",
			Commands: []string{
				"/实效", "/倍率", "/时效", "/pjsk score up",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			parts := strings.Fields(strings.TrimSpace(ctx.GetArgs()))
			if len(parts) != 5 {
				return nil, onebot11.NewReplayError("使用方式: %s 队长技能 技能2 技能3 技能4 技能5\n例: %s 160 160 150 150 150", ctx.GetTriggerCmd(), ctx.GetTriggerCmd())
			}

			values := make([]float64, 0, 5)
			for _, p := range parts {
				v, err := strconv.ParseFloat(p, 64)
				if err != nil || v < 0 {
					return nil, onebot11.NewReplayError("使用方式: %s 队长技能 技能2 技能3 技能4 技能5\n例: %s 160 160 150 150 150", ctx.GetTriggerCmd(), ctx.GetTriggerCmd())
				}
				values = append(values, v)
			}

			leader := values[0]
			others := values[1] + values[2] + values[3] + values[4]
			internalValue := values[0] + values[1] + values[2] + values[3] + values[4]
			scoreUp := leader + others*0.2
			multiplier := scoreUp/100.0 + 1.0
			return makeResolvedCmdWithParams(
				ctx,
				parser.ModuleDeck,
				"deck-score-up",
				fmt.Sprintf(
					"队长技能加成: %.4g%%\n内部值: %.4g\n实效: %.4g%%\n倍率: %.4g",
					leader, internalValue, scoreUp, multiplier,
				)), nil
		},
	}
}
