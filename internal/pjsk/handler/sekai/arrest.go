package sekai

import (
	"strings"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/utils/logger"
)

// UserQueryParams holds the resolved identity context for commands that query
// another user's data (arrest, registration time, etc.).
type UserQueryParams struct {
	Mode           string `json:"mode"`               // "self", "at_user", "uid"
	Platform       string `json:"platform"`           // caller's IM platform
	PlatformUserID string `json:"platform_user_id"`   // caller's platform UID (self mode)
	AtUserID       string `json:"at_user_id"`         // @-mentioned platform UID (at_user mode)
	PJSKUserID     string `json:"pjsk_user_id"`       // direct game UID (uid mode)
	Selector       string `json:"selector,omitempty"` // u[i] binding selector (self mode only)
}

// isBindingSelector returns true if the value is a u[i] binding selector (e.g. "u1", "u2").
func isBindingSelector(value string) bool {
	if len(value) < 2 {
		return false
	}
	if value[0] != 'u' && value[0] != 'U' {
		return false
	}
	for _, r := range value[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
	case isBindingSelector(uidArg):
		p.Mode = "self"
		p.Selector = uidArg
	case strings.HasPrefix(uidArg, "@"):
		p.Mode = "at_user"
		p.AtUserID = uidArg[1:] // strip "@"
	case isDigits(uidArg):
		p.Mode = "uid"
		p.PJSKUserID = uidArg
	default:
		return p, onebot11.NewReplayError("无效的参数：%q\n使用方式：%s [@用户 | 游戏ID | u序号]", uidArg, ctx.originalTriggerCmd)
	}
	return p, nil
}

// resolveSelfOnlyQueryParams is like resolveUserQueryParams but restricts to
// self-mode only (with optional u[i] selector). Used by commands that should
// not support @mention or direct UID queries (e.g. sud, msd).
func resolveSelfOnlyQueryParams(ctx SekaiHandlerContext) (UserQueryParams, error) {
	p := UserQueryParams{
		Platform:       ctx.GetPlatform(),
		PlatformUserID: ctx.GetUserId(),
		Mode:           "self",
	}
	uidArg := ctx.UIDArg()
	if uidArg == "" {
		return p, nil
	}
	if isBindingSelector(uidArg) {
		p.Selector = uidArg
		return p, nil
	}
	return p, onebot11.NewReplayError("此命令仅支持查询自己的数据\n使用方式：%s [u序号]", ctx.originalTriggerCmd)
}

func (sekaiHandlers) ArrestHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		ParseUIDArg: common.BoolPtr(true),
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/逮捕", "/pjsk逮捕", "/pjsk arrest",
			},
			Path: "arrest",
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
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
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			p, err := resolveSelfOnlyQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleRegTime, "reg-time", p), nil
		},
	}
}
