package handler

import (
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/common"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
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
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk hide suite", "/pjsk隐藏抓包", "/隐藏抓包",
			},
			Path: "profile/suite/hide",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeHideSuite, newProfileSettingsParams(ctx, selector)), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileShowSuiteHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk show suite", "/pjsk显示抓包", "/pjsk展示抓包", "/展示抓包",
			},
			Path: "profile/suite/show",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeShowSuite, newProfileSettingsParams(ctx, selector)), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileHideMySekaiHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk hide mysekai", "/pjsk隐藏烤森抓包", "/隐藏烤森抓包",
			},
			Path: "profile/mysekai/hide",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeHideMySekai, newProfileSettingsParams(ctx, selector)), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileShowMySekaiHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk show mysekai", "/pjsk显示烤森抓包", "/pjsk展示烤森抓包", "/展示烤森抓包",
			},
			Path: "profile/mysekai/show",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeShowMySekai, newProfileSettingsParams(ctx, selector)), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileHideIDHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk hide id", "/pjsk隐藏id", "/pjsk隐藏ID", "/隐藏id", "/隐藏ID",
			},
			Path: "profile/visibility/hide",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeHideID, newProfileSettingsParams(ctx, selector)), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileShowIDHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk show id", "/pjsk显示id", "/pjsk显示ID", "/pjsk展示id", "/pjsk展示ID",
				"/展示id", "/展示ID", "/显示id", "/显示ID",
			},
			Path: "profile/visibility/show",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeShowID, newProfileSettingsParams(ctx, selector)), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileTimeZoneHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk时区", "/pjsktimezone", "/pjsktz",
			},
			Path: "profile/timezone",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, onebot11.NewReplayError(
					"使用方式:\n%s <时区名|偏移量>\n示例:\n%s Asia/Shanghai\n%s +8\n%s +09:00\n%s +28800",
					ctx.originalTriggerCmd,
					ctx.originalTriggerCmd,
					ctx.originalTriggerCmd,
					ctx.originalTriggerCmd,
					ctx.originalTriggerCmd,
				)
			}
			params := newProfileSettingsParams(ctx)
			params.TimeZone = args
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeSetTimeZone, params), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileChartStyleHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk chart style",
				"/谱面样式", "/谱面底色", "/设置谱面样式", "/设置谱面底色",
			},
			Path: "profile/chart-style",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, onebot11.NewReplayError(
					"使用方式:\n%s <white|black>",
					ctx.originalTriggerCmd,
				)
			}
			params := newProfileSettingsParams(ctx)
			params.ChartStyle = args
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeSetChartStyle, params), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileCheckDataHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk check data", "/pjsk抓包", "/pjsk抓包状态", "/pjsk抓包数据", "/pjsk抓包查询",
				"/抓包数据", "/抓包状态", "/抓包信息", "/sud",
			},
			Path: "profile/check-data",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			p, err := resolveSelfOnlyQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleCheckData, "suite", p), nil
		},
	}, executeCheckData)
}

func (sekaiHandlers) MsdHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/msd",
				"/pjsk check mysekai data",
				"/pjsk烤森抓包数据", "/pjsk烤森抓包", "/烤森抓包", "/烤森抓包数据",
			},
			Path: "profile/check-data-mysekai",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			p, err := resolveSelfOnlyQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleCheckData, "mysekai", p), nil
		},
	}, executeCheckData)
}

func (sekaiHandlers) ProfileVerifyHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk verify", "/pjsk验证",
			},
			Path: "profile/verify",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			selector, err := resolveSettingsSelector(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeVerify, newProfileSettingsParams(ctx, selector)), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileVerifyListHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk verify list", "/pjsk验证列表", "/pjsk验证状态",
			},
			Path: "profile/verify/list",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s", ctx.originalTriggerCmd)
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeVerifyList, newProfileSettingsParams(ctx)), nil
		},
	}, executeProfile)
}

func executeCheckData(rc *RequestContext) (onebot11.Message, error) {
	var p userQueryParams
	mergeParams(rc.Cmd.Params, &p)

	region := regionWithDefault(rc.Cmd.Region)

	var dataType sekaiapi.ToolboxDataType
	var label string
	var uid int64
	var platform string
	var platformUserID string
	var pjskUID string
	var bindingVisible bool
	var resolvedHarukiID int
	var bindingServer string

	resolveBinding := func(requireSuite, requireMySekai bool) (*accountdata.ResolvedBinding, int, error) {
		hid, binding, err := resolveBindingWithFallback(
			rc.Ctx, rc.App.Bindings, p.Platform, p.PlatformUserID, region,
			rc.Cmd.RegionExplicit,
			bindingResolutionOptions{
				RequireSuite:   requireSuite,
				RequireMySekai: requireMySekai,
				Selector:       p.Selector,
			},
		)
		if err != nil {
			return nil, 0, normalizeBindingLookupError(err, "解析绑定账号失败")
		}
		return binding, hid, nil
	}

	switch rc.Cmd.Mode {
	case "mysekai":
		if p.Mode != "self" {
			return nil, fmt.Errorf("MySekai抓包相关内容仅支持查询自己的数据")
		}
		binding, hid, err := resolveBinding(false, true)
		if err != nil {
			return nil, err
		}
		if !hasUsableMySekaiData(binding) {
			return nil, newMySekaiDataNotFoundReplayError()
		}
		uid, err = strconv.ParseInt(binding.PJSKUserID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("无效的账号ID：%w", err)
		}
		platform = p.Platform
		platformUserID = p.PlatformUserID
		dataType = sekaiapi.ToolboxDataTypeMySekai
		label = "MySekai"
		pjskUID = binding.PJSKUserID
		bindingVisible = binding.Visible
		resolvedHarukiID = hid
		bindingServer = binding.Server
	default:
		if p.Mode != "self" {
			return nil, fmt.Errorf("suite抓包相关内容仅支持查询自己的数据")
		}
		binding, hid, err := resolveBinding(true, false)
		if err != nil {
			return nil, err
		}
		if !hasUsableSuiteData(binding) {
			return nil, newSuiteDataNotFoundReplayError()
		}
		uid, err = strconv.ParseInt(binding.PJSKUserID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("无效的账号ID：%w", err)
		}
		platform = p.Platform
		platformUserID = p.PlatformUserID
		dataType = sekaiapi.ToolboxDataTypeSuite
		label = "Suite"
		pjskUID = binding.PJSKUserID
		bindingVisible = binding.Visible
		resolvedHarukiID = hid
		bindingServer = binding.Server
	}

	if bindingServer == "" {
		bindingServer = region
	}

	raw, err := rc.App.Toolbox.GetUploadTime(bindingServer, dataType, uid, platform, platformUserID)
	if err != nil {
		return nil, fmt.Errorf("获取%s更新时间失败：%w", label, err)
	}

	ts, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("解析更新时间失败：%w", err)
	}

	timeZone := resolveHarukiUserTimeZone(rc.Ctx, rc.App, resolvedHarukiID)
	uploadTime := displaytime.TimeFromUnixSeconds(ts, timeZone)
	relDur := displaytime.FormatRelativeDuration(displaytime.Now(timeZone).Sub(displaytime.TimeFromUnixSeconds(ts, timeZone)))
	maskedUID := maskPJSKUID(pjskUID, bindingVisible)

	text := fmt.Sprintf("UID %s 的%s数据更新时间:\n%s (%s) (%s)",
		maskedUID, label, displaytime.FormatTime(uploadTime, "2006-01-02 15:04:05"), timeZone, relDur)
	return onebot11.Message{onebot11.Text(text)}, nil
}
