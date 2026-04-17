package sekai

import (
	"strings"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/common"
)

func newProfileBindingParams(ctx HarrukiSekaiHandlerContext, selector, scope string) accountdata.ProfileBindingCommandParams {
	return accountdata.ProfileBindingCommandParams{
		Platform:       ctx.GetPlatform(),
		PlatformUserID: ctx.GetUserId(),
		Selector:       strings.TrimSpace(selector),
		Server:         ctx.Region().String(),
		Scope:          strings.TrimSpace(scope),
	}
}

func newProfileSettingsParams(ctx HarrukiSekaiHandlerContext, selector ...string) accountdata.ProfileSettingsCommandParams {
	params := accountdata.ProfileSettingsCommandParams{
		Platform:       ctx.GetPlatform(),
		PlatformUserID: ctx.GetUserId(),
		Server:         ctx.Region().String(),
		RegionExplicit: ctx.HasExplicitRegion(),
	}
	if len(selector) > 0 && selector[0] != "" {
		params.Selector = selector[0]
	}
	return params
}

// resolveSettingsSelector extracts a u[i] binding selector from the handler context.
// Returns empty string when no selector is specified (use default binding for region).
// Returns error if args are present but not a valid u[i] selector.
func resolveSettingsSelector(ctx HarrukiSekaiHandlerContext) (string, error) {
	args := strings.TrimSpace(ctx.GetArgs())
	if args != "" {
		return "", onebot11.NewReplayError("使用方式:\n%s [u序号]", ctx.originalTriggerCmd)
	}
	uidArg := ctx.UIDArg()
	if uidArg == "" {
		return "", nil
	}
	if isBindingSelector(uidArg) {
		return uidArg, nil
	}
	return "", onebot11.NewReplayError("此设置仅支持操作自己的账号\n使用方式：%s [u序号]", ctx.originalTriggerCmd)
}

func (sekaiHandlers) ProfileHideSuiteHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk hide suite", "/pjsk隐藏抓包", "/隐藏抓包",
			},
			Path: "profile/suite/hide",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeHideSuite, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileShowSuiteHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk show suite", "/pjsk显示抓包", "/pjsk展示抓包", "/展示抓包",
			},
			Path: "profile/suite/show",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeShowSuite, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileHideMySekaiHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk hide mysekai", "/pjsk隐藏烤森抓包", "/隐藏烤森抓包",
			},
			Path: "profile/mysekai/hide",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeHideMySekai, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileShowMySekaiHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk show mysekai", "/pjsk显示烤森抓包", "/pjsk展示烤森抓包", "/展示烤森抓包",
			},
			Path: "profile/mysekai/show",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeShowMySekai, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileHideIDHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk hide id", "/pjsk隐藏id", "/pjsk隐藏ID", "/隐藏id", "/隐藏ID",
			},
			Path: "profile/visibility/hide",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeHideID, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileShowIDHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk show id", "/pjsk显示id", "/pjsk显示ID", "/pjsk展示id", "/pjsk展示ID",
				"/展示id", "/展示ID", "/显示id", "/显示ID",
			},
			Path: "profile/visibility/show",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeShowID, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileCheckDataHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk check data", "/pjsk抓包", "/pjsk抓包状态", "/pjsk抓包数据", "/pjsk抓包查询",
				"/抓包数据", "/抓包状态", "/抓包信息", "/sud",
			},
			Path: "profile/check-data",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			p, err := resolveSelfOnlyQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleCheckData, "suite", p), nil
		},
	}
}

func (sekaiHandlers) MsdHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/msd",
				"/pjsk check mysekai data",
				"/pjsk烤森抓包数据", "/pjsk烤森抓包", "/烤森抓包", "/烤森抓包数据",
			},
			Path: "profile/check-data-mysekai",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			p, err := resolveSelfOnlyQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleCheckData, "mysekai", p), nil
		},
	}
}

func (sekaiHandlers) ProfileVerifyHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk verify", "/pjsk验证",
			},
			Path: "profile/verify",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeVerify, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileVerifyListHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk verify list", "/pjsk验证列表", "/pjsk验证状态",
			},
			Path: "profile/verify/list",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s", ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeVerifyList, newProfileSettingsParams(ctx)), nil
		},
	}
}
