package sekai

import (
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func (sekaiHandlers) SKLineHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/line",
			Commands: []string{
				"/sk-line", "/sk线", "/榜线", "/pjsk sk line", "/pjsk board line", "/skl",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, err := buildSKTrackerParams(ctx, true, true, false)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-line", params), nil
		},
	}
}

func (sekaiHandlers) SKQueryHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{Path: "sk/query", Commands: []string{
			"/sk-query", "/sk查询", "/sk查分", "/pjsk sk board", "/pjsk board",
		},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, err := buildSKTrackerParams(ctx, false, true, true)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-query", params), nil
		},
	}
}

func (sekaiHandlers) SKSpeedHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/speed",
			Commands: []string{
				"/pjsk sk speed", "/pjsk board speed", "/时速", "/sks", "/skv", "/sk时速",
				"/sk-speed", "/sk时速", "/时速线", "/pjsk sk speed", "/pjsk board speed", "/sks", "/skv", "/sktime",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			applySKSpeedDefaults(params, "h", 60*60)
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-speed", params), nil
		},
	}
}

func (sekaiHandlers) SKCheckRoomHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/check-room",
			Commands: []string{
				"/sk-check-room", "/sk查房", "/查房", "/cf", "/pjsk查房", "/csb", "/冲水板", "/pjsk冲水板",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-check-room", params), nil
		},
	}
}

func (sekaiHandlers) SKPlayerTraceHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/player-trace",
			Commands: []string{
				"/sk-player-trace", "/sk玩家轨迹", "/玩家轨迹", "/ptr", "/pjsk玩家追踪", "/pjsk ptr",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, err := buildSKPlayerTraceParams(ctx)
			if err != nil {
				return nil, err
			}
			if len(params) == 0 {
				return makeResolvedCmd(ctx, parser.ModuleSK, "sk-player-trace"), nil
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-player-trace", params), nil
		},
	}
}

func (sekaiHandlers) SKRankTraceHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/rank-trace",
			Commands: []string{
				"/sk-rank-trace", "/sk档线轨迹", "/档线轨迹", "/rtr", "/skt", "/sklt", "/sktl", "/pjsk追踪", "/pjsk sk追踪",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-rank-trace", params), nil
		},
	}
}

func (sekaiHandlers) WinratePredictHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{Path: "sk/winrate", Commands: []string{
			"/pjsk winrate predict", "/胜率预测", "/5v5预测", "/胜率", "/5v5胜率", "/预测胜率", "/预测5v5",
		}},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			return makeResolvedCmd(ctx, parser.ModuleSK, "sk-winrate"), nil
		},
	}
}

func (sekaiHandlers) SKDailySpeedHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/daily-speed",
			Commands: []string{
				"/pjsk sk daily speed", "/pjsk board daily speed", "/日速", "/skds", "/skdv", "/sk日速",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			applySKSpeedDefaults(params, "d", 24*60*60)
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-daily-speed", params), nil
		},
	}
}

func (sekaiHandlers) SKPredictHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/predict",
			Commands: []string{
				"/pjsk sk predict", "/pjsk board predict", "/sk预测", "/榜线预测", "/skp",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-predict", params), nil
		},
	}
}

func (sekaiHandlers) SKBoardHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/query",
			Commands: []string{
				"/pjsk sk board", "/pjsk board", "/sk",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, err := buildSKTrackerParams(ctx, false, true, true)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-query", params), nil
		},
	}
}

func (sekaiHandlers) CSBHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/check-room",
			Commands: []string{
				"/csb", "/查水表", "/pjsk查水表", "/停车时间",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-check-room", params), nil
		},
	}
}
