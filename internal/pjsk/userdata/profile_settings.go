package userdata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/render/region"
)

const (
	ProfileModeHideID      = "profile-hide-id"
	ProfileModeShowID      = "profile-show-id"
	ProfileModeHideSuite   = "profile-hide-suite"
	ProfileModeShowSuite   = "profile-show-suite"
	ProfileModeHideMySekai = "profile-hide-mysekai"
	ProfileModeShowMySekai = "profile-show-mysekai"
	ProfileModeVerify      = "profile-verify"
	ProfileModeVerifyList  = "profile-verify-list"
	ProfileModeBGUpload    = "profile-bg-upload"
	ProfileModeBGClear     = "profile-bg-clear"
	ProfileModeBGAdjust    = "profile-bg-adjust"
)

type ProfileSettingsCommandParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	Server         string `json:"server"`
	ImageURL       string `json:"image_url,omitempty"`
	Blur           *int   `json:"blur,omitempty"`
	Alpha          *int   `json:"alpha,omitempty"`
	Vertical       *bool  `json:"vertical,omitempty"`
}

func DecodeProfileSettingsParams(raw json.RawMessage) (ProfileSettingsCommandParams, error) {
	var params ProfileSettingsCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing profile settings params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal profile settings params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	params.Server = strings.TrimSpace(strings.ToLower(params.Server))
	params.ImageURL = strings.TrimSpace(params.ImageURL)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing profile settings identity context")
	}
	normalized := renderregion.Normalize(params.Server)
	if normalized.IsZero() {
		return params, fmt.Errorf("bridge: invalid profile settings server %q", params.Server)
	}
	params.Server = normalized.String()
	return params, nil
}

func ExecuteProfileSettingsCommand(ctx context.Context, service *BindingService, mode string, params ProfileSettingsCommandParams) ([]byte, error) {
	if service == nil || !service.IsReady() {
		return nil, ErrBindingServiceUnavailable
	}

	switch mode {
	case ProfileModeHideID:
		item, err := service.SetBindingVisible(ctx, params.Platform, params.PlatformUserID, params.Server, false)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已隐藏%s服ID信息", strings.ToUpper(item.Server))), nil
	case ProfileModeShowID:
		item, err := service.SetBindingVisible(ctx, params.Platform, params.PlatformUserID, params.Server, true)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已展示%s服ID信息", strings.ToUpper(item.Server))), nil
	case ProfileModeHideSuite:
		item, err := service.SetBindingSuiteVisible(ctx, params.Platform, params.PlatformUserID, params.Server, false)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已隐藏%s服抓包信息", strings.ToUpper(item.Server))), nil
	case ProfileModeShowSuite:
		item, err := service.SetBindingSuiteVisible(ctx, params.Platform, params.PlatformUserID, params.Server, true)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已展示%s服抓包信息", strings.ToUpper(item.Server))), nil
	case ProfileModeHideMySekai:
		item, err := service.SetBindingMySekaiVisible(ctx, params.Platform, params.PlatformUserID, params.Server, false)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已隐藏%s服烤森抓包信息", strings.ToUpper(item.Server))), nil
	case ProfileModeShowMySekai:
		item, err := service.SetBindingMySekaiVisible(ctx, params.Platform, params.PlatformUserID, params.Server, true)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已展示%s服烤森抓包信息", strings.ToUpper(item.Server))), nil
	case ProfileModeVerify:
		item, alreadyVerified, err := service.VerifyCurrentBinding(ctx, params.Platform, params.PlatformUserID, params.Server)
		if err != nil {
			return nil, err
		}
		if alreadyVerified {
			return []byte(fmt.Sprintf("当前%s服绑定账号已经验证过", strings.ToUpper(item.Server))), nil
		}
		return []byte(fmt.Sprintf("已验证%s服账号 %s", strings.ToUpper(item.Server), formatBindingUID(*item))), nil
	case ProfileModeVerifyList:
		items, err := service.ListVerifiedBindings(ctx, params.Platform, params.PlatformUserID, params.Server)
		if err != nil {
			return nil, err
		}
		return []byte(formatVerifiedBindingListText(params.Server, items)), nil
	case ProfileModeBGUpload:
		item, err := service.SetCurrentBindingProfileBG(ctx, params.Platform, params.PlatformUserID, params.Server, params.ImageURL)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已更新%s服个人信息背景", strings.ToUpper(item.Server))), nil
	case ProfileModeBGClear:
		item, err := service.ClearCurrentBindingProfileBG(ctx, params.Platform, params.PlatformUserID, params.Server)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已清空%s服个人信息背景", strings.ToUpper(item.Server))), nil
	case ProfileModeBGAdjust:
		if params.Blur == nil && params.Alpha == nil && params.Vertical == nil {
			binding, err := service.currentBindingEntity(ctx, params.Platform, params.PlatformUserID, params.Server)
			if err != nil {
				return nil, err
			}
			item, err := service.bindingListItemByID(ctx, params.Platform, params.PlatformUserID, binding.ID)
			if err != nil {
				return nil, err
			}
			return []byte(formatProfileBGSettingsText(*item)), nil
		}
		item, err := service.AdjustCurrentBindingProfileBG(ctx, params.Platform, params.PlatformUserID, params.Server, params.Blur, params.Alpha, params.Vertical)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已更新%s服个人信息背景设置", strings.ToUpper(item.Server))), nil
	default:
		return nil, fmt.Errorf("bridge: unsupported profile settings mode %q", mode)
	}
}

func formatVerifiedBindingListText(server string, items []BindingListItem) string {
	server = strings.ToUpper(strings.TrimSpace(server))
	if len(items) == 0 {
		return fmt.Sprintf("你还没有验证过任何%s服游戏ID", server)
	}

	lines := []string{fmt.Sprintf("你验证过的%s服游戏ID:", server)}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("u%d %s", item.Index, formatBindingUID(item)))
	}
	return strings.Join(lines, "\n")
}

func formatProfileBGSettingsText(item BindingListItem) string {
	server := strings.ToUpper(strings.TrimSpace(item.Server))
	if item.Bg == nil || item.Bg.ImgPath == nil || strings.TrimSpace(*item.Bg.ImgPath) == "" {
		return fmt.Sprintf("当前%s服还没有自定义个人信息背景", server)
	}

	lines := []string{
		fmt.Sprintf("当前%s服个人信息背景设置:", server),
		fmt.Sprintf("ID: %s", formatBindingUID(item)),
	}
	if item.Bg.Vertical {
		lines = append(lines, "方向: 竖屏")
	} else {
		lines = append(lines, "方向: 横屏")
	}
	lines = append(lines,
		fmt.Sprintf("模糊度: %d", item.Bg.Blur),
		fmt.Sprintf("透明度: %d", item.Bg.Alpha),
	)
	return strings.Join(lines, "\n")
}
