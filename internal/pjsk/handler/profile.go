package handler

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/profile"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func (sekaiHandlers) ProfileBindHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk bind", "/pjsk id",
				"/绑定", "/pjsk 绑定",
			},
			Path: "profile/bind",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s 账号ID", ctx.originalTriggerCmd)
			}
			if rerouted, handled, err := tryRerouteProfileBindCommand(ctx, args); handled {
				return rerouted, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBind, newProfileBindingParams(ctx, args, "")), nil
		},
	}, executeProfile)
}

func tryRerouteProfileBindCommand(ctx HarrukiSekaiHandlerContext, args string) (*CommandRequest, bool, error) {
	tokens := strings.Fields(strings.TrimSpace(args))
	if len(tokens) == 0 {
		return nil, false, nil
	}

	switch strings.ToLower(strings.TrimSpace(tokens[0])) {
	case "列表", "list":
		if len(tokens) != 1 {
			return nil, true, onebot11.NewReplayError("使用方式:\n%s", buildProfileBindDerivedTrigger(ctx, "list"))
		}
		params := newProfileBindingParams(ctx, "", "")
		if !ctx.HasExplicitRegion() {
			params.Server = ""
		}
		return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBindList, params), true, nil
	case "交换", "swap":
		if len(tokens) != 3 {
			return nil, true, onebot11.NewReplayError("使用方式:\n%s u1 u2", buildProfileBindDerivedTrigger(ctx, "swap"))
		}
		params := newProfileBindingParams(ctx, tokens[1], "")
		params.SelectorOther = tokens[2]
		if !ctx.HasExplicitRegion() {
			params.Server = ""
		}
		return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBindSwap, params), true, nil
	default:
		return nil, false, nil
	}
}

func buildProfileBindDerivedTrigger(ctx HarrukiSekaiHandlerContext, mode string) string {
	switch mode {
	case "list":
		if ctx.HasExplicitRegion() {
			return fmt.Sprintf("/%s绑定列表", ctx.Region().String())
		}
		return "/绑定列表"
	case "swap":
		if ctx.HasExplicitRegion() {
			return fmt.Sprintf("/%s绑定交换", ctx.Region().String())
		}
		return "/绑定交换"
	default:
		return ctx.originalTriggerCmd
	}
}

func (sekaiHandlers) ProfileBindListHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/绑定列表", "/pjsk bind list", "/pjsk绑定列表",
			},
			Path: "profile/bind/list",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, onebot11.NewReplayError("使用方式:\n%s", ctx.originalTriggerCmd)
			}
			params := newProfileBindingParams(ctx, "", "")
			if !ctx.HasExplicitRegion() {
				params.Server = ""
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBindList, params), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileBindSwapHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/绑定交换", "/pjsk bind swap", "/pjsk绑定交换",
			},
			Path: "profile/bind/swap",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.Fields(strings.TrimSpace(ctx.GetArgs()))
			if len(args) != 2 {
				return nil, onebot11.NewReplayError("使用方式:\n%s u1 u2", ctx.originalTriggerCmd)
			}

			params := newProfileBindingParams(ctx, args[0], "")
			params.SelectorOther = args[1]
			if !ctx.HasExplicitRegion() {
				params.Server = ""
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBindSwap, params), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileUnbindHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk unbind", "/pjsk解绑", "/解绑", "/取消绑定",
			},
			Path: "profile/unbind",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
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
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeUnbind, params), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileSetMainHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/pjsk set main", "/pjsk主账号", "/设置主账号", "/主账号",
				"/设置默认绑定", "/默认绑定",
			},
			Path: "profile/default",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
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
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeDefaultSet, params), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileClearDefaultBindingHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/取消默认绑定", "/清除默认绑定", "/取消主账号", "/清除主账号",
			},
			Path: "profile/default/clear",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())

			scope := ""
			if ctx.HasExplicitRegion() {
				scope = ctx.Region().String()
			}
			params := newProfileBindingParams(ctx, args, scope)
			params.Server = scope // selector searches all bindings when no explicit region
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeDefaultClear, params), nil
		},
	}, executeProfile)
}

func executeProfile(rc *RequestContext) (onebot11.Message, error) {
	switch rc.Cmd.Mode {
	case accountdata.ProfileModeRender:
		var p userQueryParams
		mergeParams(rc.Cmd.Params, &p)
		_, message, err := renderProfileMessageForQuery(rc, p, rc.Cmd.Region, rc.Cmd.RegionExplicit)
		return message, err
	case profileModeCustomProfileCardThumbnail:
		return executeProfileCustomProfileCardThumbnail(rc)
	case accountdata.ProfileModeBind, accountdata.ProfileModeBindList, accountdata.ProfileModeBindSwap, accountdata.ProfileModeUnbind, accountdata.ProfileModeDefaultSet, accountdata.ProfileModeDefaultClear:
		if rc.App.Bindings == nil {
			return nil, accountdata.ErrBindingServiceUnavailable
		}
		params, err := accountdata.DecodeProfileBindingParams(rc.Cmd.Params)
		if err != nil {
			return nil, err
		}
		data, err := accountdata.ExecuteProfileBindingCommand(rc.Ctx, rc.App.Bindings, rc.Cmd.Mode, params)
		if err != nil {
			return nil, err
		}
		return onebot11.Message{onebot11.Text(string(data))}, nil
	case accountdata.ProfileModeHideID, accountdata.ProfileModeShowID,
		accountdata.ProfileModeHideSuite, accountdata.ProfileModeShowSuite,
		accountdata.ProfileModeHideMySekai, accountdata.ProfileModeShowMySekai,
		accountdata.ProfileModeVerify, accountdata.ProfileModeVerifyList,
		accountdata.ProfileModeSetTimeZone,
		accountdata.ProfileModeSetArrestDiff,
		accountdata.ProfileModeSetChartStyle,
		accountdata.ProfileModeBGUpload, accountdata.ProfileModeBGClear:
		if rc.App.Bindings == nil {
			return nil, accountdata.ErrBindingServiceUnavailable
		}
		params, err := accountdata.DecodeProfileSettingsParams(rc.Cmd.Params)
		if err != nil {
			return nil, err
		}
		data, err := accountdata.ExecuteProfileSettingsCommand(rc.Ctx, rc.App.Bindings, rc.Cmd.Mode, params)
		if err != nil {
			return nil, err
		}
		return onebot11.Message{onebot11.Text(string(data))}, nil
	case accountdata.ProfileModeBGAdjust:
		if rc.App.Bindings == nil {
			return nil, accountdata.ErrBindingServiceUnavailable
		}
		params, err := accountdata.DecodeProfileSettingsParams(rc.Cmd.Params)
		if err != nil {
			return nil, err
		}
		data, err := accountdata.ExecuteProfileSettingsCommand(rc.Ctx, rc.App.Bindings, rc.Cmd.Mode, params)
		if err != nil {
			return nil, err
		}
		if params.Blur == nil && params.Alpha == nil && params.Vertical == nil {
			return onebot11.Message{onebot11.Text(string(data))}, nil
		}

		query := userQueryParams{
			Mode:           "self",
			Platform:       params.Platform,
			PlatformUserID: params.PlatformUserID,
			Selector:       params.Selector,
		}
		_, image, renderErr := renderProfileMessageForQuery(rc, query, params.Server, params.RegionExplicit)
		if renderErr != nil {
			text := strings.TrimSpace(string(data))
			if text == "" {
				text = "已更新个人信息背景设置"
			}
			return onebot11.Message{onebot11.Text(text + "\n个人信息预览渲染失败: " + renderErr.Error())}, nil
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			return image, nil
		}
		return append(image, onebot11.Text("\n"+text)), nil
	default:
		return nil, unsupportedModeError("profile", rc.Cmd.Mode)
	}
}

func renderProfileMessageForQuery(rc *RequestContext, p userQueryParams, region string, regionExplicit bool) (ResolvedGameTarget, onebot11.Message, error) {
	var zeroTarget ResolvedGameTarget
	if rc == nil || rc.App == nil || rc.App.Profiles == nil || rc.App.SekaiAPI == nil {
		return zeroTarget, nil, fmt.Errorf("profile service unavailable")
	}

	profileCtrl := rc.App.Profiles.WithContext(rc.Ctx)
	region = regionWithDefault(region)

	target, err := resolveGameTarget(rc.Ctx, p, region, regionExplicit, rc.App)
	if err != nil {
		return zeroTarget, nil, err
	}
	region = resolvedTargetRegion(region, target)

	resp, err := fetchCachedSekaiUserProfile(rc.Ctx, rc.App, region, target.PJSKUserID)
	if err != nil {
		return zeroTarget, nil, fmt.Errorf("获取玩家信息失败：%w", err)
	}

	if rc.App.Censor != nil {
		harukiID := target.HarukiUserID
		if !rc.App.Censor.CensorName(rc.Ctx, harukiID, target.PJSKUserID, resp.User.Name, region) {
			resp.User.Name = ""
		}
		if !rc.App.Censor.CensorShortBio(rc.Ctx, harukiID, target.PJSKUserID, resp.UserProfile.Word, region) {
			resp.UserProfile.Word = ""
		}
	}

	var profileSnapshot snapshot.Snapshot
	if p.Mode == "self" && hasUsableSuiteData(target.Binding) {
		if platform, platformUserID := platformCredentials(p); platform != "" {
			profileSnapshot = resolveTargetSnapshot(rc.Ctx, rc.App, region, platform, platformUserID, target.PJSKUserID, false)
		}
	}

	q := profile.Query{
		Region:           region,
		Visible:          target.Visible,
		BgSettings:       target.BgSettings,
		VerticalOverride: p.ProfileVertical,
	}
	renderStart := time.Now()
	data, err := profileCtrl.RenderProfileFromAPIWithSnapshot(q, resp, profileSnapshot)
	if err != nil {
		return zeroTarget, nil, err
	}
	slog.Info("profile render completed",
		"region", region,
		"target_user_id", target.PJSKUserID,
		"image_bytes", len(data),
		"duration_ms", time.Since(renderStart).Milliseconds(),
	)

	messageStart := time.Now()
	message, err := imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
	if err != nil {
		return zeroTarget, nil, err
	}
	slog.Info("profile image message prepared",
		"region", region,
		"target_user_id", target.PJSKUserID,
		"segments", len(message),
		"duration_ms", time.Since(messageStart).Milliseconds(),
	)
	return target, message, nil
}
