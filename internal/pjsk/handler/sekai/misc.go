package sekai

import (
	"errors"
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"strings"
)

func (sekaiHandlers) MiscBirthdayHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "misc/birthday",
			Commands: []string{
				"/pjsk chara birthday", "/角色生日", "/生日", "/查生日",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleMisc, "misc-birthday"), nil
		},
	}
}

func (sekaiHandlers) ProfileHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "profile",
			Commands: []string{
				"/个人中心", "/profile",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			p, err := resolveUserQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, handler.ProfileModeRender, p), nil
		},
	}
}

func (sekaiHandlers) HelpHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/help", "/帮助",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleHelp, "help"), nil
		},
	}
}

func (sekaiHandlers) UpdateHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk update", "/pjsk refresh", "/pjsk更新",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return nil, fmt.Errorf("TODO: 更新查询未实现，region=%s", string(ctx.region))
		},
	}
}

func (sekaiHandlers) NgWordHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk ng", "/pjsk ngword", "/pjsk ng word",
				"/pjsk屏蔽词", "/pjsk屏蔽", "/pjsk敏感", "/pjsk敏感词",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			text := strings.TrimSpace(ctx.GetArgs())
			if text == "" {
				return nil, errors.New("请输入要查询的文本")
			}
			return nil, fmt.Errorf("TODO: 屏蔽词检测未实现，text=%q", text)
		},
	}
}

func (sekaiHandlers) UploadHelpHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/抓包帮助", "/抓包", "/pjsk upload help",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return nil, errors.New("TODO: 抓包帮助未实现")
		},
	}
}

func (sekaiHandlers) ExtractCardHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/提取卡牌",
			},
			Disabled: true,
		},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return nil, errors.New("TODO: 提取卡牌未实现")
		},
	}
}

func (sekaiHandlers) HeyiweiHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjskb30", "/pjskdetail", "/b30", "/b39", "/pjskb39", "/pjsk b30", "/pjsk b39", "/pjsk detail",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return "何意味", nil
		},
	}
}
