package sekai

import (
	"fmt"
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s", ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBindList, newProfileBindingParams(ctx, "", "")), nil
		},
	}
}

func newProfileBindingParams(ctx SekaiHandlerContext, selector, scope string) accountdata.ProfileBindingCommandParams {
	return accountdata.ProfileBindingCommandParams{
		Platform:       ctx.GetPlatform(),
		PlatformUserID: ctx.GetUserId(),
		Selector:       strings.TrimSpace(selector),
		Server:         ctx.Region().String(),
		Scope:          strings.TrimSpace(scope),
	}
}

func newProfileSettingsParams(ctx SekaiHandlerContext, selector ...string) accountdata.ProfileSettingsCommandParams {
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
func resolveSettingsSelector(ctx SekaiHandlerContext) (string, error) {
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

func extractFirstImageURL(ctx SekaiHandlerContext) string {
	for _, segment := range ctx.GetMessage() {
		if segment.Type != "image" {
			continue
		}
		imgData, ok := segment.Data.(onebot11.ImageData)
		if !ok {
			continue
		}

		if imageURL := strings.TrimSpace(imgData.Url); imageURL != "" {
			return imageURL
		}
		if fileURL := strings.TrimSpace(imgData.File); strings.HasPrefix(strings.ToLower(fileURL), "http://") || strings.HasPrefix(strings.ToLower(fileURL), "https://") {
			return fileURL
		}
	}
	return ""
}

func parseProfileBGAdjustArgs(args string) (accountdata.ProfileSettingsCommandParams, error) {
	params := accountdata.ProfileSettingsCommandParams{}
	args = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(args, "度", ""), "%", ""))
	if args == "" {
		return params, nil
	}

	tokens := strings.Fields(args)
	for i := 0; i < len(tokens); i++ {
		token := strings.TrimSpace(tokens[i])
		switch strings.ToLower(token) {
		case "横屏", "横向", "横版":
			v := false
			params.Vertical = &v
		case "竖屏", "竖向", "竖版", "纵向":
			v := true
			params.Vertical = &v
		case "模糊", "blur":
			if i+1 >= len(tokens) {
				return params, onebot11.NewReplayError("使用方式:\n调整个人信息背景 [横屏|竖屏] [模糊 0~10] [透明 0~100]")
			}
			value, err := parseProfileBGInt(tokens[i+1], 0, 10)
			if err != nil {
				return params, err
			}
			params.Blur = &value
			i++
		case "透明", "alpha":
			if i+1 >= len(tokens) {
				return params, onebot11.NewReplayError("使用方式:\n调整个人信息背景 [横屏|竖屏] [模糊 0~10] [透明 0~100]")
			}
			value, err := parseProfileBGInt(tokens[i+1], 0, 100)
			if err != nil {
				return params, err
			}
			params.Alpha = &value
			i++
		default:
			if strings.HasPrefix(token, "模糊") {
				value, err := parseProfileBGInt(strings.TrimPrefix(token, "模糊"), 0, 10)
				if err != nil {
					return params, err
				}
				params.Blur = &value
				continue
			}
			if strings.HasPrefix(token, "透明") {
				value, err := parseProfileBGInt(strings.TrimPrefix(token, "透明"), 0, 100)
				if err != nil {
					return params, err
				}
				params.Alpha = &value
				continue
			}
			return params, fmt.Errorf("无法识别的个人信息背景参数: %s", token)
		}
	}
	return params, nil
}

func parseProfileBGInt(raw string, minValue, maxValue int) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("请提供正确的数值")
	}
	n := 0
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("请提供正确的数值")
		}
		n = n*10 + int(ch-'0')
	}
	if n < minValue || n > maxValue {
		return 0, fmt.Errorf("数值超出范围，需要在 %d~%d 之间", minValue, maxValue)
	}
	return n, nil
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s 账号ID\n或 %s u1", ctx.originalTriggerCmd, ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeUnbind, newProfileBindingParams(ctx, args, "")), nil
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s 账号ID\n%s u1", ctx.originalTriggerCmd, ctx.originalTriggerCmd)
			}

			scope := ""
			if ctx.HasExplicitRegion() {
				scope = ctx.Region().String()
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeDefaultSet, newProfileBindingParams(ctx, args, scope)), nil
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())

			scope := ""
			if ctx.HasExplicitRegion() {
				scope = ctx.Region().String()
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeDefaultClear, newProfileBindingParams(ctx, args, scope)), nil
		},
	}
}

func (sekaiHandlers) ProfileHideSuiteHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk hide suite", "/pjsk隐藏抓包", "/隐藏抓包",
			},
			Path: "profile/suite/hide",
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeHideSuite, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileShowSuiteHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk show suite", "/pjsk显示抓包", "/pjsk展示抓包", "/展示抓包",
			},
			Path: "profile/suite/show",
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeShowSuite, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileHideMySekaiHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk hide mysekai", "/pjsk隐藏烤森抓包", "/隐藏烤森抓包",
			},
			Path: "profile/mysekai/hide",
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeHideMySekai, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileShowMySekaiHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk show mysekai", "/pjsk显示烤森抓包", "/pjsk展示烤森抓包", "/展示烤森抓包",
			},
			Path: "profile/mysekai/show",
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeShowMySekai, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileHideIDHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk hide id", "/pjsk隐藏id", "/pjsk隐藏ID", "/隐藏id", "/隐藏ID",
			},
			Path: "profile/visibility/hide",
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeHideID, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileShowIDHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk show id", "/pjsk显示id", "/pjsk显示ID", "/pjsk展示id", "/pjsk展示ID",
				"/展示id", "/展示ID", "/显示id", "/显示ID",
			},
			Path: "profile/visibility/show",
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeShowID, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileCheckDataHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk check data", "/pjsk抓包", "/pjsk抓包状态", "/pjsk抓包数据", "/pjsk抓包查询",
				"/抓包数据", "/抓包状态", "/抓包信息", "/sud",
			},
			Path: "profile/check-data",
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			p, err := resolveSelfOnlyQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleCheckData, "suite", p), nil
		},
	}
}

func (sekaiHandlers) MsdHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/msd",
				"/pjsk check mysekai data",
				"/pjsk烤森抓包数据", "/pjsk烤森抓包", "/烤森抓包", "/烤森抓包数据",
			},
			Path: "profile/check-data-mysekai",
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			p, err := resolveSelfOnlyQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleCheckData, "mysekai", p), nil
		},
	}
}

func (sekaiHandlers) ProfileVerifyHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk verify", "/pjsk验证",
			},
			Path: "profile/verify",
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeVerify, newProfileSettingsParams(ctx, selector)), nil
		},
	}
}

func (sekaiHandlers) ProfileVerifyListHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk verify list", "/pjsk验证列表", "/pjsk验证状态",
			},
			Path: "profile/verify/list",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s", ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeVerifyList, newProfileSettingsParams(ctx)), nil
		},
	}
}

func (sekaiHandlers) ProfileUploadBGHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk upload profile bg", "/pjsk upload profile background",
				"/上传个人信息背景", "/上传个人信息图片", "/上传个人背景", "/上传个人信息",
			},
			Path: "profile/bg/upload",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s [图片]", ctx.originalTriggerCmd)
			}
			imageURL := extractFirstImageURL(ctx)
			if imageURL == "" {
				return nil, onebot11.NewReplayError("请在命令中附带一张个人信息背景图片")
			}
			params := newProfileSettingsParams(ctx)
			params.ImageURL = imageURL
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBGUpload, params), nil
		},
	}
}

func (sekaiHandlers) ProfileClearBGHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk clear profile bg", "/pjsk clear profile background",
				"/清空个人信息背景", "/清除个人信息背景", "/清空个人信息图片", "/清除个人信息图片",
			},
			Path: "profile/bg/clear",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s", ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBGClear, newProfileSettingsParams(ctx)), nil
		},
	}
}

func (sekaiHandlers) ProfileAdjustBGHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk adjust profile", "/pjsk adjust profile bg", "/pjsk adjust profile background",
				"/调整个人信息背景", "/调整个人信息", "/设置个人信息", "/设置个人信息背景",
			},
			Path: "profile/bg/adjust",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params := newProfileSettingsParams(ctx)
			adjustParams, err := parseProfileBGAdjustArgs(ctx.GetArgs())
			if err != nil {
				return nil, err
			}
			params.Blur = adjustParams.Blur
			params.Alpha = adjustParams.Alpha
			params.Vertical = adjustParams.Vertical
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBGAdjust, params), nil
		},
	}
}
