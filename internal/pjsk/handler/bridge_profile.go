package handler

import (
	"fmt"
	"log/slog"
	"time"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/render/profile"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func executeProfile(rc *RequestContext) (onebot11.Message, error) {
	profileCtrl := rc.App.Profiles.WithContext(rc.Ctx)
	switch rc.Cmd.Mode {
	case accountdata.ProfileModeRender:
		var p userQueryParams
		mergeParams(rc.Cmd.Params, &p)

		region := regionWithDefault(rc.Cmd.Region)

		target, err := resolveGameTarget(rc.Ctx, p, region, rc.Cmd.RegionExplicit, rc.App)
		if err != nil {
			return nil, err
		}
		region = resolvedTargetRegion(region, target)

		resp, err := rc.App.SekaiAPI.GetUserProfile(region, target.PJSKUserID)
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
			return nil, err
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
			return nil, err
		}
		slog.Info("profile image message prepared",
			"region", region,
			"target_user_id", target.PJSKUserID,
			"segments", len(message),
			"duration_ms", time.Since(messageStart).Milliseconds(),
		)
		return message, nil
	case accountdata.ProfileModeBind, accountdata.ProfileModeBindList, accountdata.ProfileModeUnbind, accountdata.ProfileModeDefaultSet, accountdata.ProfileModeDefaultClear:
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
		accountdata.ProfileModeBGUpload, accountdata.ProfileModeBGClear, accountdata.ProfileModeBGAdjust:
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
	default:
		return nil, unsupportedModeError("profile", rc.Cmd.Mode)
	}
}
