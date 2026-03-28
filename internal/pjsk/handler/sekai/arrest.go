package sekai

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/utils/logger"
)

// UserQueryParams holds the resolved identity context for commands that query
// another user's data (arrest, registration time, etc.).
type UserQueryParams struct {
	Mode           string `json:"mode"`             // "self", "at_user", "uid"
	Platform       string `json:"platform"`         // caller's IM platform
	PlatformUserID string `json:"platform_user_id"` // caller's platform UID (self mode)
	AtUserID       string `json:"at_user_id"`       // @-mentioned platform UID (at_user mode)
	PJSKUserID     string `json:"pjsk_user_id"`     // direct game UID (uid mode)
}

func resolveUserQueryParams(ctx SekaiHandlerContext) (UserQueryParams, error) {
	p := UserQueryParams{
		Platform:       ctx.GetPlatform(),
		PlatformUserID: ctx.GetUserId(),
	}
	uidArg := ctx.UIDArg()
	switch {
	case uidArg == "":
		p.Mode = "self"
	case strings.HasPrefix(uidArg, "@"):
		p.Mode = "at_user"
		p.AtUserID = uidArg[1:] // strip "@"
	case isDigits(uidArg):
		p.Mode = "uid"
		p.PJSKUserID = uidArg
	default:
		return p, fmt.Errorf("无效的参数：%q\n使用方式：%s [@用户 | 游戏ID]", uidArg, ctx.originalTriggerCmd)
	}
	return p, nil
}

func (sekaiHandlers) ArrestHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		ParseUIDArg: boolPtr(true),
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/逮捕", "/pjsk逮捕", "/pjsk arrest",
			},
			Path: "arrest",
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			logger.Infof("uidArg: %s, event: %+v", ctx.UIDArg(), ctx.GetEvent().Message)
			p, err := resolveUserQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleArrest, "arrest", p), nil
		},
	}
}

func (sekaiHandlers) RegTimeHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/注册时间", "/pjsk reg time", "/pjsk 注册时间", "/查时间",
			},
			Path: "profile/reg-time",
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			p, err := resolveUserQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleRegTime, "reg-time", p), nil
		},
	}
}
