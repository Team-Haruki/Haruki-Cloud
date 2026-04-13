package sekai

import (
	"strings"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/common"
	accountdata "haruki-cloud/internal/pjsk/userdata"
)

func (sekaiHandlers) ProfileBindHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk bind", "/pjsk id",
				"/绑定", "/pjsk 绑定",
			},
			Path: "profile/bind",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s 账号ID", ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBind, newProfileBindingParams(ctx, args, "")), nil
		},
	}
}

func (sekaiHandlers) ProfileBindListHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/绑定列表", "/pjsk bind list", "/pjsk绑定列表",
			},
			Path: "profile/bind/list",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s", ctx.originalTriggerCmd)
			}
			params := newProfileBindingParams(ctx, "", "")
			if !ctx.HasExplicitRegion() {
				params.Server = ""
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBindList, params), nil
		},
	}
}

func (sekaiHandlers) ProfileUnbindHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk unbind", "/pjsk解绑", "/解绑", "/取消绑定",
			},
			Path: "profile/unbind",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s 账号ID\n或 %s u1", ctx.originalTriggerCmd, ctx.originalTriggerCmd)
			}
			params := newProfileBindingParams(ctx, args, "")
			scope := ""
			if ctx.HasExplicitRegion() {
				scope = ctx.Region().String()
			}
			params.Server = scope
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeUnbind, params), nil
		},
	}
}

func (sekaiHandlers) ProfileSetMainHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk set main", "/pjsk主账号", "/设置主账号", "/主账号",
				"/设置默认绑定", "/默认绑定",
			},
			Path: "profile/default",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s 账号ID\n%s u1", ctx.originalTriggerCmd, ctx.originalTriggerCmd)
			}

			scope := ""
			if ctx.HasExplicitRegion() {
				scope = ctx.Region().String()
			}
			params := newProfileBindingParams(ctx, args, scope)
			params.Server = scope // selector searches all bindings when no explicit region
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeDefaultSet, params), nil
		},
	}
}

func (sekaiHandlers) ProfileClearDefaultBindingHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/取消默认绑定", "/清除默认绑定", "/取消主账号", "/清除主账号",
			},
			Path: "profile/default/clear",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			args := strings.TrimSpace(ctx.GetArgs())

			scope := ""
			if ctx.HasExplicitRegion() {
				scope = ctx.Region().String()
			}
			params := newProfileBindingParams(ctx, args, scope)
			params.Server = scope // selector searches all bindings when no explicit region
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeDefaultClear, params), nil
		},
	}
}
