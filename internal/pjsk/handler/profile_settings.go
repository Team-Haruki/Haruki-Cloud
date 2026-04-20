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

var profileTimeZoneBaseCommands = []string{
	"/pjsk时区", "/pjsktimezone", "/pjsktz",
}

func profileTimeZoneCommands() []string {
	commands := append([]string(nil), profileTimeZoneBaseCommands...)
	seen := make(map[string]struct{}, len(commands))
	for _, command := range commands {
		seen[command] = struct{}{}
	}

	for _, prefix := range []string{"/pjsktimezone", "/pjsktz"} {
		for _, alias := range displaytime.KnownTimeZoneAliases() {
			command := prefix + alias
			if _, ok := seen[command]; ok {
				continue
			}
			seen[command] = struct{}{}
			commands = append(commands, command)
		}
	}
	return commands
}

func extractProfileTimeZoneArg(ctx HarrukiSekaiHandlerContext) string {
	if args := strings.TrimSpace(ctx.GetArgs()); args != "" {
		return args
	}

	triggerCmd := strings.TrimSpace(ctx.GetTriggerCmd())
	for _, prefix := range profileTimeZoneBaseCommands {
		if len(triggerCmd) <= len(prefix) {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(triggerCmd), strings.ToLower(prefix)) {
			continue
		}
		if suffix := strings.TrimSpace(triggerCmd[len(prefix):]); suffix != "" {
			return suffix
		}
	}
	return ""
}

func parseProfileDifficultyToken(raw string) sekaiapi.MusicDifficultyType {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "easy":
		return sekaiapi.MusicDifficultyEasy
	case "normal":
		return sekaiapi.MusicDifficultyNormal
	case "hard", "hd":
		return sekaiapi.MusicDifficultyHard
	case "expert", "ex":
		return sekaiapi.MusicDifficultyExpert
	case "master":
		return sekaiapi.MusicDifficultyMaster
	case "append", "apd":
		return sekaiapi.MusicDifficultyAppend
	default:
		return ""
	}
}

func parseProfileDifficultyState(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "开启", "开", "on", "enable", "enabled", "true", "1":
		return true, true
	case "关闭", "关", "off", "disable", "disabled", "false", "0":
		return false, true
	default:
		return false, false
	}
}

func parseProfileDifficultyCompactToggle(raw string) (accountdata.ProfileDifficultyToggle, bool) {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if lower == "" {
		return accountdata.ProfileDifficultyToggle{}, false
	}

	for _, suffix := range []string{"开启", "关闭", "开", "关"} {
		if !strings.HasSuffix(lower, suffix) {
			continue
		}
		diff := parseProfileDifficultyToken(strings.TrimSpace(strings.TrimSuffix(lower, suffix)))
		if diff == "" {
			return accountdata.ProfileDifficultyToggle{}, false
		}
		enabled, _ := parseProfileDifficultyState(suffix)
		return accountdata.ProfileDifficultyToggle{Difficulty: diff, Enabled: enabled}, true
	}

	return accountdata.ProfileDifficultyToggle{}, false
}

func parseProfileDifficultyToggles(raw string) ([]accountdata.ProfileDifficultyToggle, error) {
	normalized := strings.NewReplacer("，", " ", ",", " ", "、", " ", "\n", " ", "\t", " ").Replace(strings.TrimSpace(raw))
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty")
	}

	toggles := make([]accountdata.ProfileDifficultyToggle, 0, len(fields))
	for i := 0; i < len(fields); i++ {
		if toggle, ok := parseProfileDifficultyCompactToggle(fields[i]); ok {
			toggles = append(toggles, toggle)
			continue
		}

		if i+1 >= len(fields) {
			return nil, fmt.Errorf("invalid token %q", fields[i])
		}
		diff := parseProfileDifficultyToken(fields[i])
		enabled, ok := parseProfileDifficultyState(fields[i+1])
		if diff == "" || !ok {
			return nil, fmt.Errorf("invalid token %q", fields[i])
		}
		toggles = append(toggles, accountdata.ProfileDifficultyToggle{
			Difficulty: diff,
			Enabled:    enabled,
		})
		i++
	}
	return toggles, nil
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
			Commands: profileTimeZoneCommands(),
			Path:     "profile/timezone",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := extractProfileTimeZoneArg(ctx)
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

func (sekaiHandlers) ProfileArrestDifficultyHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/逮捕难度", "/pjsk逮捕难度", "/pjsk arrest difficulty",
			},
			Path: "profile/arrest-difficulty",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, onebot11.NewReplayError(
					"使用方式:\n%s easy关闭 normal关闭 hard关闭 expert关闭 master开启 append开启",
					ctx.originalTriggerCmd,
				)
			}

			toggles, err := parseProfileDifficultyToggles(args)
			if err != nil {
				return nil, onebot11.NewReplayError(
					"无效的逮捕难度参数\n使用方式:\n%s easy关闭 normal关闭 hard关闭 expert关闭 master开启 append开启",
					ctx.originalTriggerCmd,
				)
			}

			params := newProfileSettingsParams(ctx)
			params.DifficultyToggles = toggles
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeSetArrestDiff, params), nil
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
