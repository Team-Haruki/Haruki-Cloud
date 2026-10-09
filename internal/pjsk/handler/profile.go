package handler

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"haruki-cloud/internal/core/upstreamerr"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/profile"
	"haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/logger"
	"haruki-cloud/utils/usererror"
)

func (sekaiHandlers) ProfileBindHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Commands: []string{
			"/pjsk bind", "/pjsk id",
			"/绑定", "/pjsk 绑定",
		},
		Path:        "profile/bind",
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, usererror.Misuse(i18n.M("binding.bind.uid_required"))
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
			return nil, true, usererror.Misuse(i18n.M("common.no_args"))
		}
		params := newProfileBindingParams(ctx, "", "")
		if !ctx.HasExplicitRegion() {
			params.Server = ""
		}
		return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBindList, params), true, nil
	case "交换", "swap":
		if len(tokens) != 3 {
			return nil, true, usererror.Misuse(i18n.M("binding.swap.selectors_required"))
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
		Commands: []string{
			"/绑定列表", "/pjsk bind list", "/pjsk绑定列表",
		},
		Path:        "profile/bind/list",
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, usererror.Misuse(i18n.M("common.no_args"))
			}
			params := newProfileBindingParams(ctx, "", "")
			if !ctx.HasExplicitRegion() {
				params.Server = ""
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeBindList, params), nil
		},
	}, executeProfile)
}

func (sekaiHandlers) ProfileUIDHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Commands: []string{
			"/查uid", "/uid",
		},
		Path:        "profile/uid",
		ParseUIDArg: common.BoolPtr(true),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, profileUIDUsageError()
			}
			selector := strings.TrimSpace(ctx.UIDArg())
			if strings.HasPrefix(selector, "@") {
				return nil, usererror.Forbidden(i18n.M("binding.uid.self_only"))
			}
			if selector != "" && !isBindingSelector(selector) {
				return nil, profileUIDUsageError()
			}

			params := newProfileBindingParams(ctx, selector, "")
			if !ctx.HasExplicitRegion() {
				params.Server = ""
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeQueryUID, params), nil
		},
	}, executeProfile)
}

func profileUIDUsageError() error {
	return usererror.Unrecognized()
}

func (sekaiHandlers) ProfileBindSwapHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Commands: []string{
			"/绑定交换", "/pjsk bind swap", "/pjsk绑定交换",
		},
		Path:        "profile/bind/swap",
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.Fields(strings.TrimSpace(ctx.GetArgs()))
			if len(args) != 2 {
				return nil, usererror.Misuse(i18n.M("binding.swap.selectors_required"))
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
		Commands: []string{
			"/pjsk unbind", "/pjsk解绑", "/解绑", "/取消绑定",
		},
		Path:        "profile/unbind",
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, usererror.Misuse(i18n.M("binding.selector_required"))
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
		Commands: []string{
			"/pjsk set main", "/pjsk主账号", "/设置主账号", "/主账号",
			"/设置默认绑定", "/默认绑定",
		},
		Path:        "profile/default",
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, usererror.Misuse(i18n.M("binding.selector_required"))
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
		Commands: []string{
			"/取消默认绑定", "/清除默认绑定", "/取消主账号", "/清除主账号",
		},
		Path:        "profile/default/clear",
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
	case profileModeCustomProfileCard:
		return executeProfileCustomProfileCard(rc)
	case accountdata.ProfileModeBind, accountdata.ProfileModeBindList, accountdata.ProfileModeBindSwap, accountdata.ProfileModeUnbind, accountdata.ProfileModeDefaultSet, accountdata.ProfileModeDefaultClear, accountdata.ProfileModeQueryUID:
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
		accountdata.ProfileModeEnableModular,
		accountdata.ProfileModeDisableModular,
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
				text = i18n.T("profile.bg.adjusted_any")
			}
			profileLogger.WarnContext(rc.Ctx, "profile preview render failed after a background change",
				"error", usererror.RedactForLog(usererror.LogText(renderErr), usererror.DefaultLogMessageLimit))
			return onebot11.Message{onebot11.Text(text + "\n" + i18n.T("profile.bg.preview_failed"))}, nil
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

var profileLogger = logger.NewLoggerFromGlobal("PJSKProfile")

// playerProfileFetchError is the reply for a failed player profile fetch:
// typed errors pass, upstream failures are classified.
func playerProfileFetchError(err error) error {
	if isUserFacingError(err) {
		return err
	}
	if typed := upstreamerr.UserError(err); typed != nil {
		return typed
	}
	return usererror.Wrap(usererror.CodeUnavailable, i18n.M("profile.fetch_failed"), err)
}

func renderProfileMessageForQuery(rc *RequestContext, p userQueryParams, region string, regionExplicit bool) (ResolvedGameTarget, onebot11.Message, error) {
	var zeroTarget ResolvedGameTarget
	if rc == nil || rc.App == nil || rc.App.Profiles == nil || rc.App.SekaiAPI == nil {
		return zeroTarget, nil, usererror.Misconfigured(errors.New("profile service unavailable"))
	}

	profileCtrl := rc.App.Profiles.WithContext(rc.Ctx)
	region = regionWithDefault(region)

	target, err := resolveGameTarget(rc.Ctx, p, region, regionExplicit, rc.App)
	if err != nil {
		return zeroTarget, nil, err
	}
	region = resolvedTargetRegion(region, target)

	resp, profileSnapshot, err := fetchProfileAndTargetSnapshot(rc, p, target, region)
	if err != nil {
		return zeroTarget, nil, playerProfileFetchError(err)
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

	q := profile.Query{
		Region:           region,
		Visible:          target.Visible,
		BgSettings:       target.BgSettings,
		VerticalOverride: p.ProfileVertical,
	}
	var data drawing.ImageResult
	if isRequesterModularProfileEnabled(rc, p) {
		data, err = profileCtrl.RenderModularProfileFromAPIWithSnapshotImage(q, resp, profileSnapshot)
	} else {
		data, err = profileCtrl.RenderProfileFromAPIWithSnapshotImage(q, resp, profileSnapshot)
	}
	if err != nil {
		return zeroTarget, nil, err
	}
	message, err := rc.RenderedImageMessage(data)
	if err != nil {
		return zeroTarget, nil, err
	}
	return target, message, nil
}

// fetchProfileAndTargetSnapshot fetches the SekaiAPI profile and, for a self
// query with a visible suite, the target's suite snapshot concurrently. The
// profile error stays fatal and the snapshot stays optional; the snapshot is
// never fetched when the profile is not wanted (other modes, hidden suite).
func fetchProfileAndTargetSnapshot(rc *RequestContext, p userQueryParams, target ResolvedGameTarget, region string) (*sekaiapi.GetAnotherProfileResponse, snapshot.Snapshot, error) {
	var (
		platform, platformUserID string
		profileSnapshot          snapshot.Snapshot
	)
	if p.Mode == "self" && hasUsableSuiteData(target.Binding) {
		platform, platformUserID = platformCredentials(p)
	}
	var wg sync.WaitGroup
	if platform != "" {
		wg.Go(func() {
			profileSnapshot = resolveTargetSnapshot(rc.Ctx, rc.App, region, platform, platformUserID, target.PJSKUserID, false)
		})
	}
	resp, err := fetchCachedSekaiUserProfile(rc.Ctx, rc.App, region, target.PJSKUserID)
	wg.Wait()
	if err != nil {
		return nil, nil, err
	}
	return resp, profileSnapshot, nil
}

func isRequesterModularProfileEnabled(rc *RequestContext, p userQueryParams) bool {
	if rc == nil || rc.App == nil || rc.App.Bindings == nil {
		return false
	}
	platform := strings.TrimSpace(p.Platform)
	platformUserID := strings.TrimSpace(p.PlatformUserID)
	if platform == "" {
		platform = rc.Platform
	}
	if platformUserID == "" {
		platformUserID = rc.PlatformUserID
	}
	if platform == "" || platformUserID == "" {
		return false
	}
	settings, _, err := rc.App.Bindings.GetUserSettings(rc.Ctx, platform, platformUserID)
	return err == nil && settings != nil && settings.ModularProfileEnabled
}
