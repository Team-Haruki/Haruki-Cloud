package handler

import (
	"errors"
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	"haruki-cloud/utils/usererror"
)

// The standalone info panel: the profile card that tops other renders, on its
// own as a transparent PNG, built from the data source the user picks.
const (
	profileModeInfoPanel       = "profile-info-panel"
	mySekaiInfoPanelCommand    = "mysekai-info-panel"
	mySekaiInfoPanelAllCommand = "mysekai-info-panel-all"
)

const infoPanelHelp = `使用方式:
/信息面板 su
/信息面板 ms
/信息面板 all

su / suite：使用 Suite 数据；ms / mysekai：使用 MySekai 数据；all：同时显示 Suite 与 MySekai 数据。可加 u序号 选择自己的绑定账号。`

func (sekaiHandlers) ProfileInfoPanelHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "profile/info-panel",
		Commands: []string{
			"/信息面板", "/pjsk info panel", "/info-panel",
		},
		Helper: infoPanelHelp,
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			self, err := resolveSelfOnlyQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			mode, module, err := parseInfoPanelSource(ctx.GetArgs(), ctx.originalTriggerCmd)
			if err != nil {
				return nil, err
			}
			params := userQueryParams{
				Mode:           self.Mode,
				Platform:       self.Platform,
				PlatformUserID: self.PlatformUserID,
				Selector:       self.Selector,
			}
			return makeCommandRequestWithParams(ctx, module, mode, params), nil
		},
	}, executeInfoPanel)
}

// parseInfoPanelSource maps the one argument to its mode; MySekai (and the
// combined "all") runs as a MySekai command so the MySekai module switch and
// region gate apply to it, exactly as for the other mixed Suite+MySekai renders.
func parseInfoPanelSource(args, trigger string) (string, parser.TargetModule, error) {
	switch strings.ToLower(strings.TrimSpace(args)) {
	case "su", "suite":
		return profileModeInfoPanel, parser.ModuleProfile, nil
	case "ms", "mysekai":
		return mySekaiInfoPanelCommand, parser.ModuleMysekai, nil
	case "all":
		return mySekaiInfoPanelAllCommand, parser.ModuleMysekai, nil
	default:
		return "", parser.ModuleProfile, usererror.Unrecognized()
	}
}

func executeInfoPanel(rc *RequestContext) (onebot11.Message, error) {
	if rc == nil || rc.Cmd == nil {
		return nil, unsupportedModeError("profile", "")
	}
	switch rc.Cmd.Mode {
	case profileModeInfoPanel:
		return executeSuiteInfoPanel(rc)
	case mySekaiInfoPanelCommand, mySekaiInfoPanelAllCommand:
		return executeMysekai(rc)
	default:
		return nil, unsupportedModeError("profile", rc.Cmd.Mode)
	}
}

// executeSuiteInfoPanel renders the card suite-backed commands show: the public
// profile (censored name, current leader) with the Suite source and frame.
func executeSuiteInfoPanel(rc *RequestContext) (onebot11.Message, error) {
	if rc.App == nil || rc.App.Drawing == nil {
		return nil, usererror.Misconfigured(errors.New("drawing service unavailable"))
	}
	rc.warmSuiteAndPublicProfile(false)
	binding, snap, err := rc.requireVisibleSuiteSnapshot()
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, suiteDataNotFoundError(binding)
	}
	_, card := resolveCommandDisplayProfiles(rc, snap)
	if card == nil {
		return nil, suiteDataNotFoundError(binding)
	}
	data, err := rc.App.Drawing.WithContext(rc.Ctx).GenerateInfoPanelImage(card)
	if err != nil {
		return nil, err
	}
	return rc.RenderedImageMessage(data)
}

func executeMysekaiInfoPanel(rc *RequestContext, renderCtx mySekaiRenderContext) (onebot11.Message, error) {
	data, err := renderCtx.Controller.RenderInfoPanelImage(rendermysekai.InfoPanelQuery{
		Region:  renderCtx.Region,
		Profile: renderCtx.Profile,
	})
	return mysekaiRenderedImageResult(rc, data, err)
}

// executeMysekaiInfoPanelAll renders the combined panel the mixed MySekai
// renders (shop, talk list) carry: Suite and MySekai sources with MySekai Lv.
// The MySekai gates (module switch, CN gate, MySekai data) already ran in
// executeMysekai; the Suite half still needs Suite visibility, with the same
// reply /信息面板 su gives.
func executeMysekaiInfoPanelAll(rc *RequestContext, renderCtx mySekaiRenderContext) (onebot11.Message, error) {
	if binding, _ := rc.GetBinding(); binding != nil && !hasUsableSuiteData(binding) {
		return nil, suiteDataNotFoundError(binding)
	}
	data, err := renderCtx.Controller.RenderInfoPanelImage(rendermysekai.InfoPanelQuery{
		Region:       renderCtx.Region,
		Profile:      renderCtx.Profile,
		IncludeSuite: true,
	})
	return mysekaiRenderedImageResult(rc, data, err)
}
