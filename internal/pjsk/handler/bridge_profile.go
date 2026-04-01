package handler

import (
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/render/profile"
	accountdata "haruki-cloud/internal/pjsk/userdata"
	sekaiutils "haruki-cloud/utils/sekai"
)

func executeProfile(rc *RequestContext) (onebot11.Message, error) {
	switch rc.Cmd.Mode {
	case accountdata.ProfileModeRender:
		var p userQueryParams
		mergeParams(rc.Cmd.Params, &p)

		region := regionWithDefault(rc.Cmd.Region)

		target, err := resolveGameTarget(rc.Ctx, p, region, rc.Cmd.RegionExplicit, rc.App)
		if err != nil {
			return nil, err
		}

		resp, err := sekaiutils.GetSekaiAPIClient().GetUserProfile(region, target.PJSKUserID)
		if err != nil {
			return nil, fmt.Errorf("获取玩家信息失败：%w", err)
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

		// Fetch player frames from the suite snapshot (best-effort; nil = no frame rendered).
		var framesJSON []byte
		if p.Mode == "self" && hasUsableSuiteData(target.Binding) {
			if platform, platformUserID := platformCredentials(p); platform != "" {
				if uid, convErr := strconv.ParseInt(target.PJSKUserID, 10, 64); convErr == nil {
					framesJSON, _ = sekaiutils.GetToolboxClient().GetPrivateDataValue(
						region, sekaiutils.ToolboxDataTypeSuite, uid, platform, platformUserID, "userPlayerFrames")
				}
			}
		}

		q := profile.Query{
			Region:     rc.Cmd.Region,
			Visible:    target.Visible,
			BgSettings: target.BgSettings,
		}
		data, err := rc.App.Profiles.RenderProfileFromAPI(q, resp, framesJSON)
		if err != nil {
			return nil, err
		}
		return imageMessage(data, rc.App, BotModulePJSK)
	case accountdata.ProfileModeBind, accountdata.ProfileModeBindList, accountdata.ProfileModeUnbind, accountdata.ProfileModeDefaultSet, accountdata.ProfileModeDefaultClear:
		if rc.App.Bindings == nil {
			return nil, fmt.Errorf("绑定服务未就绪，请稍后再试")
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
		accountdata.ProfileModeBGUpload, accountdata.ProfileModeBGClear, accountdata.ProfileModeBGAdjust:
		if rc.App.Bindings == nil {
			return nil, fmt.Errorf("绑定服务未就绪，请稍后再试")
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
	default:
		return nil, unsupportedModeError("profile", rc.Cmd.Mode)
	}
}

// maskPJSKUID masks the middle digits of a PJSK user ID when visible is false.
// Shows first 3 and last 3 digits with asterisks in between.
func maskPJSKUID(uid string, visible bool) string {
	if visible || len(uid) <= 6 {
		return uid
	}
	return uid[:3] + strings.Repeat("*", len(uid)-6) + uid[len(uid)-3:]
}
