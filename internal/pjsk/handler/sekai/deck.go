package sekai

import (
	"errors"
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"strconv"
	"strings"
)

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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleDeck, "deck-event"), nil
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleDeck, "deck-challenge"), nil
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleDeck, "deck-no-event"), nil
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleDeck, "deck-bonus"), nil
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleDeck, "deck-mysekai"), nil
		},
	}
}

func (sekaiHandlers) ScoreUpHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/实效", "/倍率", "/时效", "/pjsk score up",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			parts := strings.Fields(strings.TrimSpace(ctx.GetArgs()))
			if len(parts) != 5 {
				return nil, fmt.Errorf("使用方式: %s 100 100 100 100 100", ctx.GetTriggerCmd())
			}

			values := make([]float64, 0, 5)
			for _, p := range parts {
				v, err := strconv.ParseFloat(p, 64)
				if err != nil {
					return nil, fmt.Errorf("使用方式: %s 100 100 100 100 100", ctx.GetTriggerCmd())
				}
				values = append(values, v)
			}

			res := values[0] + (values[1]+values[2]+values[3]+values[4])/5.0
			if res < 0 {
				return nil, errors.New("实效计算结果异常")
			}
			return fmt.Sprintf("实效: %.1f%%", res), nil
		},
	}
}
