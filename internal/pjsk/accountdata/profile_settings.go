package accountdata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	renderregion "haruki-cloud/internal/pjsk/region"
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
	RegionExplicit bool   `json:"region_explicit,omitempty"`
	Selector       string `json:"selector,omitempty"`
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

	// When a u[i] selector is provided, resolve it to a specific binding entity
	// instead of using server-based lookup. This supports users with multiple
	// bindings on the same server.
	// When no selector and no explicit region prefix, try global default binding
	// first so the user's "default" account is targeted, not a server-specific one.
	resolveBinding := func() (*pjskdb.UserBinding, error) {
		if params.Selector != "" {
			return service.currentBindingEntityBySelector(ctx, params.Platform, params.PlatformUserID, params.Server, params.Selector)
		}
		if !params.RegionExplicit {
			entity, err := service.currentBindingEntity(ctx, params.Platform, params.PlatformUserID, GlobalDefaultBindingScope)
			if err == nil {
				return entity, nil
			}
		}
		return service.currentBindingEntity(ctx, params.Platform, params.PlatformUserID, params.Server)
	}

	switch mode {
	case ProfileModeHideID:
		binding, err := resolveBinding()
		if err != nil {
			return nil, err
		}
		if _, err := service.pjskDB.UserBinding.UpdateOneID(binding.ID).SetVisible(false).Save(ctx); err != nil {
			return nil, err
		}
		item, err := service.bindingListItemByID(ctx, params.Platform, params.PlatformUserID, binding.ID)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已隐藏 [%s] %s 的ID信息", strings.ToUpper(item.Server), formatBindingUID(*item))), nil
	case ProfileModeShowID:
		binding, err := resolveBinding()
		if err != nil {
			return nil, err
		}
		if _, err := service.pjskDB.UserBinding.UpdateOneID(binding.ID).SetVisible(true).Save(ctx); err != nil {
			return nil, err
		}
		item, err := service.bindingListItemByID(ctx, params.Platform, params.PlatformUserID, binding.ID)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已展示 [%s] %s 的ID信息", strings.ToUpper(item.Server), formatBindingUID(*item))), nil
	case ProfileModeHideSuite:
		binding, err := resolveBinding()
		if err != nil {
			return nil, err
		}
		if _, err := service.pjskDB.UserBinding.UpdateOneID(binding.ID).SetSuiteVisible(false).Save(ctx); err != nil {
			return nil, err
		}
		item, err := service.bindingListItemByID(ctx, params.Platform, params.PlatformUserID, binding.ID)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已隐藏 [%s] %s 的抓包信息", strings.ToUpper(item.Server), formatBindingUID(*item))), nil
	case ProfileModeShowSuite:
		binding, err := resolveBinding()
		if err != nil {
			return nil, err
		}
		if _, err := service.pjskDB.UserBinding.UpdateOneID(binding.ID).SetSuiteVisible(true).Save(ctx); err != nil {
			return nil, err
		}
		item, err := service.bindingListItemByID(ctx, params.Platform, params.PlatformUserID, binding.ID)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已展示 [%s] %s 的抓包信息", strings.ToUpper(item.Server), formatBindingUID(*item))), nil
	case ProfileModeHideMySekai:
		binding, err := resolveBinding()
		if err != nil {
			return nil, err
		}
		if _, err := service.pjskDB.UserBinding.UpdateOneID(binding.ID).SetMysekaiVisible(false).Save(ctx); err != nil {
			return nil, err
		}
		item, err := service.bindingListItemByID(ctx, params.Platform, params.PlatformUserID, binding.ID)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已隐藏 [%s] %s 的烤森抓包信息", strings.ToUpper(item.Server), formatBindingUID(*item))), nil
	case ProfileModeShowMySekai:
		binding, err := resolveBinding()
		if err != nil {
			return nil, err
		}
		if _, err := service.pjskDB.UserBinding.UpdateOneID(binding.ID).SetMysekaiVisible(true).Save(ctx); err != nil {
			return nil, err
		}
		item, err := service.bindingListItemByID(ctx, params.Platform, params.PlatformUserID, binding.ID)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已展示 [%s] %s 的烤森抓包信息", strings.ToUpper(item.Server), formatBindingUID(*item))), nil
	case ProfileModeVerify:
		if service.fastVerifier == nil {
			return nil, fmt.Errorf("pjsk: fast verification provider is not configured")
		}
		entity, err := resolveBinding()
		if err != nil {
			return nil, err
		}
		item, alreadyVerified, err := service.verifyBindingEntity(ctx, params.Platform, params.PlatformUserID, entity)
		if err != nil {
			return nil, err
		}
		if alreadyVerified {
			return []byte(fmt.Sprintf("当前%s服绑定账号已经验证过", strings.ToUpper(item.Server))), nil
		}
		return []byte(fmt.Sprintf("已验证%s服账号 %s", strings.ToUpper(item.Server), formatBindingUID(*item))), nil
	case ProfileModeVerifyList:
		items, err := service.List(ctx, params.Platform, params.PlatformUserID)
		if err != nil {
			return nil, err
		}
		return []byte(formatVerifyListText(items)), nil
	case ProfileModeBGUpload:
		binding, err := resolveBinding()
		if err != nil {
			return nil, err
		}
		item, err := service.setBindingProfileBG(ctx, params.Platform, params.PlatformUserID, binding, params.ImageURL)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已更新%s服个人信息背景", strings.ToUpper(item.Server))), nil
	case ProfileModeBGClear:
		binding, err := resolveBinding()
		if err != nil {
			return nil, err
		}
		item, err := service.clearBindingProfileBG(ctx, params.Platform, params.PlatformUserID, binding)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已清空%s服个人信息背景", strings.ToUpper(item.Server))), nil
	case ProfileModeBGAdjust:
		if params.Blur == nil && params.Alpha == nil && params.Vertical == nil {
			binding, err := resolveBinding()
			if err != nil {
				return nil, err
			}
			item, err := service.bindingListItemByID(ctx, params.Platform, params.PlatformUserID, binding.ID)
			if err != nil {
				return nil, err
			}
			return []byte(formatProfileBGSettingsText(*item)), nil
		}
		binding, err := resolveBinding()
		if err != nil {
			return nil, err
		}
		item, err := service.adjustBindingProfileBG(ctx, params.Platform, params.PlatformUserID, binding, params.Blur, params.Alpha, params.Vertical)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已更新%s服个人信息背景设置", strings.ToUpper(item.Server))), nil
	default:
		return nil, fmt.Errorf("bridge: unsupported profile settings mode %q", mode)
	}
}

func formatVerifyListText(items []BindingListItem) string {
	if len(items) == 0 {
		return "你还没有绑定任何PJSK账号"
	}
	lines := []string{"已绑定账号验证状态（u序号按区服分别编号）:"}
	for _, item := range items {
		status := "❌"
		if item.Verified {
			status = "✅"
		}
		line := fmt.Sprintf("u%d [%s] %s %s", item.Index, strings.ToUpper(item.Server), formatBindingUID(item), status)
		marks := make([]string, 0, 2)
		if item.IsGlobalDefault {
			marks = append(marks, "全局默认")
		}
		if item.IsServerDefault {
			marks = append(marks, strings.ToUpper(item.Server)+"服默认")
		}
		if len(marks) > 0 {
			line += " (" + strings.Join(marks, "/") + ")"
		}
		lines = append(lines, line)
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
