package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	accountdata "haruki-cloud/internal/pjsk/userdata"
	"strings"
)

func (sekaiHandlers) ProfileBindHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk bind", "/pjsk id",
				"/绑定", "/pjsk 绑定", "/绑定列表",
			},
			Path: "profile/bind",
		},
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if isProfileBindingListCommand(ctx.GetTriggerCmd()) {
				if args != "" {
					return nil, fmt.Errorf("使用方式:\n%s", ctx.originalTriggerCmd)
				}
				return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBindList, newProfileBindingParams(ctx, "", "")), nil
			}
			if args == "" {
				return nil, fmt.Errorf("使用方式:\n%s 账号ID", ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBind, newProfileBindingParams(ctx, args, "")), nil
		},
	}
}

func newProfileBindingParams(ctx SekaiHandlerContext, selector, scope string) accountdata.ProfileBindingCommandParams {
	return accountdata.ProfileBindingCommandParams{
		Platform:       ctx.GetPlatform(),
		PlatformUserID: ctx.GetUserId(),
		Selector:       strings.TrimSpace(selector),
		Scope:          strings.TrimSpace(scope),
	}
}

func newProfileSettingsParams(ctx SekaiHandlerContext) accountdata.ProfileSettingsCommandParams {
	return accountdata.ProfileSettingsCommandParams{
		Platform:       ctx.GetPlatform(),
		PlatformUserID: ctx.GetUserId(),
		Server:         ctx.Region().String(),
	}
}

func extractFirstImageURL(ctx SekaiHandlerContext) string {
	for _, segment := range ctx.GetMessage() {
		if segment.Type != "image" {
			continue
		}
		if imageURL := strings.TrimSpace(segment.Data["url"]); imageURL != "" {
			return imageURL
		}
		if fileURL := strings.TrimSpace(segment.Data["file"]); strings.HasPrefix(strings.ToLower(fileURL), "http://") || strings.HasPrefix(strings.ToLower(fileURL), "https://") {
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
				return params, fmt.Errorf("使用方式:\n调整个人信息背景 [横屏|竖屏] [模糊 0~10] [透明 0~100]")
			}
			value, err := parseProfileBGInt(tokens[i+1], 0, 10)
			if err != nil {
				return params, err
			}
			params.Blur = &value
			i++
		case "透明", "alpha":
			if i+1 >= len(tokens) {
				return params, fmt.Errorf("使用方式:\n调整个人信息背景 [横屏|竖屏] [模糊 0~10] [透明 0~100]")
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

func isProfileBindingListCommand(triggerCmd string) bool {
	switch strings.TrimSpace(strings.ToLower(triggerCmd)) {
	case "/绑定列表":
		return true
	default:
		return false
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
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, fmt.Errorf("使用方式:\n%s 账号ID\n或 %s u1", ctx.originalTriggerCmd, ctx.originalTriggerCmd)
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
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, fmt.Errorf("使用方式:\n%s 账号ID\n%s u1", ctx.originalTriggerCmd, ctx.originalTriggerCmd)
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
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, fmt.Errorf("使用方式:\n%s 账号ID\n%s u1", ctx.originalTriggerCmd, ctx.originalTriggerCmd)
			}

			scope := ""
			if ctx.HasExplicitRegion() {
				scope = ctx.Region().String()
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeDefaultClear, newProfileBindingParams(ctx, args, scope)), nil
		},
	}
}

func (sekaiHandlers) ProfileSwapBindHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk swap bind", "/pjsk交换绑定",
				"/交换绑定", "/绑定交换", "/交换账号", "/交换账号顺序",
			},
			Disabled: true,
		},
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			parts := strings.Fields(strings.TrimSpace(ctx.GetArgs()))
			if len(parts) != 2 {
				return nil, fmt.Errorf("使用方式:\n%s u1 u2", ctx.originalTriggerCmd)
			}
			// TODO: 迁移 swap_player_bind_id(ctx, qid, index1, index2)
			return nil, fmt.Errorf("TODO: 交换绑定未实现，parts=%v", parts)
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
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, fmt.Errorf("使用方式:\n%s", ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeHideSuite, newProfileSettingsParams(ctx)), nil
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
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, fmt.Errorf("使用方式:\n%s", ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeShowSuite, newProfileSettingsParams(ctx)), nil
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
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, fmt.Errorf("使用方式:\n%s", ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeHideID, newProfileSettingsParams(ctx)), nil
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
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, fmt.Errorf("使用方式:\n%s", ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeShowID, newProfileSettingsParams(ctx)), nil
		},
	}
}

func (sekaiHandlers) ProfileInfoHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk profile", "/个人信息", "/名片", "/pjsk 个人信息", "/pjsk 名片",
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

func (sekaiHandlers) ProfileCheckServiceHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk check service", "/pcs", "/pjsk检查服务状态",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			// TODO: 迁移 get_service_status 逻辑
			return nil, fmt.Errorf("TODO: profile服务状态检查未实现")
		},
	}
}

func (sekaiHandlers) ProfileDataModeHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk data mode", "/pjsk抓包模式", "/pjsk抓包获取模式", "/抓包模式",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			// TODO: 迁移 data_modes 查询/切换逻辑
			return nil, fmt.Errorf("TODO: 抓包模式查询/设置未实现，args=%q", args)
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
			p, err := resolveUserQueryParams(ctx)
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
			Commands: []string{"/msd"},
			Path:     "profile/check-data-mysekai",
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			p, err := resolveUserQueryParams(ctx)
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
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, fmt.Errorf("使用方式:\n%s", ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeVerify, newProfileSettingsParams(ctx)), nil
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
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, fmt.Errorf("使用方式:\n%s", ctx.originalTriggerCmd)
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
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, fmt.Errorf("使用方式:\n%s [图片]", ctx.originalTriggerCmd)
			}
			imageURL := extractFirstImageURL(ctx)
			if imageURL == "" {
				return nil, fmt.Errorf("请在命令中附带一张个人信息背景图片")
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
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, fmt.Errorf("使用方式:\n%s", ctx.originalTriggerCmd)
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
		ParseUIDArg: boolPtr(false),
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

func (sekaiHandlers) ProfileBindHistoryHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk bind history", "/pjsk bind his", "/绑定历史", "/绑定记录",
			},
			Priority: 1,
			Disabled: true,
		},
		// TODO: refer 中这里是 CmdHandler（非 SekaiCmdHandler）
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			// TODO: 迁移 superuser 校验 + bind_history 查询逻辑
			return nil, fmt.Errorf("TODO: 绑定历史查询未实现，args=%q", args)
		},
	}
}
