package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"strings"
)

var entertainmentJPRegions = []renderregion.Value{renderregion.JP}

func (sekaiHandlers) EntertainmentLimitHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk entertainment limit", "/pjsk_entertainment_limit",
				"/pjsk娱乐功能上限", "/pel",
			},
			Disabled: true,
		},
		Regions: entertainmentJPRegions,
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return nil, fmt.Errorf("TODO: 设置娱乐次数限制未实现，args=%q", strings.TrimSpace(ctx.GetArgs()))
		},
	}
}

func (sekaiHandlers) EntertainmentLimitCheckHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk entertainment count", "/pjsk_entertainment_count",
				"/pjsk娱乐功能次数", "/pec",
			},
			Disabled: true,
		},
		Regions: entertainmentJPRegions,
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return nil, fmt.Errorf("TODO: 查看娱乐次数未实现，group_id=%s", ctx.GetGroupId())
		},
	}
}

func (sekaiHandlers) GuessCoverHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk guess cover", "/pjsk_guess_cover",
				"/pjsk猜曲封", "/pjsk猜曲绘", "/猜曲绘", "/猜曲封",
			},
			Disabled: true,
		},
		Regions: entertainmentJPRegions,
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return nil, fmt.Errorf("TODO: 猜曲绘未实现，args=%q", strings.TrimSpace(ctx.GetArgs()))
		},
	}
}

func (sekaiHandlers) GuessChartHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk guess chart", "/pjsk_guess_chart",
				"/pjsk猜谱面", "/猜谱面", "/pjsk猜铺面", "/猜铺面",
			},
			Disabled: true,
		},
		Regions: entertainmentJPRegions,
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return nil, fmt.Errorf("TODO: 猜谱面未实现，args=%q", strings.TrimSpace(ctx.GetArgs()))
		},
	}
}

func (sekaiHandlers) GuessCardHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk guess card", "/pjsk_guess_card",
				"/pjsk猜卡面", "/猜卡面", "/pjsk猜卡", "/猜卡",
			},
			Disabled: true,
		},
		Regions: entertainmentJPRegions,
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return nil, fmt.Errorf("TODO: 猜卡面未实现，args=%q", strings.TrimSpace(ctx.GetArgs()))
		},
	}
}

func (sekaiHandlers) GuessMusicHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk guess music", "/pjsk_guess_music",
				"/听歌识曲", "/pjsk听歌识曲", "/猜歌", "/pjsk猜歌", "/猜曲", "/pjsk猜曲",
			},
			Disabled: true,
		},
		Regions: entertainmentJPRegions,
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return nil, fmt.Errorf("TODO: 听歌识曲未实现，args=%q", strings.TrimSpace(ctx.GetArgs()))
		},
	}
}

func (sekaiHandlers) SpinGachaHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/单抽", "/十连", "/10连", "/50连", "/100连", "/150连", "/200连",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			trigger := ctx.GetTriggerCmd()
			args := strings.TrimSpace(ctx.GetArgs())
			return nil, fmt.Errorf("TODO: 模拟抽卡未实现，trigger=%q, args=%q", trigger, args)
		},
	}
}
