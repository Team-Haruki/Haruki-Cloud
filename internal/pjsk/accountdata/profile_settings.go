package accountdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	sonic "github.com/bytedance/sonic"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	pjskschema "haruki-cloud/ent/pjsk/schema"
	"haruki-cloud/internal/pjsk/chartstyle"
	"haruki-cloud/internal/pjsk/displaytime"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/sekai"
)

const (
	ProfileModeHideID        = "profile-hide-id"
	ProfileModeShowID        = "profile-show-id"
	ProfileModeHideSuite     = "profile-hide-suite"
	ProfileModeShowSuite     = "profile-show-suite"
	ProfileModeHideMySekai   = "profile-hide-mysekai"
	ProfileModeShowMySekai   = "profile-show-mysekai"
	ProfileModeVerify        = "profile-verify"
	ProfileModeVerifyList    = "profile-verify-list"
	ProfileModeSetTimeZone   = "profile-set-timezone"
	ProfileModeSetArrestDiff = "profile-set-arrest-difficulty"
	ProfileModeSetChartStyle = "profile-set-chart-style"
	ProfileModeBGUpload      = "profile-bg-upload"
	ProfileModeBGClear       = "profile-bg-clear"
	ProfileModeBGAdjust      = "profile-bg-adjust"
)

type ProfileDifficultyToggle struct {
	Difficulty sekai.MusicDifficultyType `json:"difficulty"`
	Enabled    bool                      `json:"enabled"`
}

type ProfileSettingsCommandParams struct {
	Platform          string                    `json:"platform"`
	PlatformUserID    string                    `json:"platform_user_id"`
	Server            string                    `json:"server"`
	RegionExplicit    bool                      `json:"region_explicit,omitempty"`
	Selector          string                    `json:"selector,omitempty"`
	TimeZone          string                    `json:"time_zone,omitempty"`
	DifficultyToggles []ProfileDifficultyToggle `json:"difficulty_toggles,omitempty"`
	ChartStyle        string                    `json:"chart_style,omitempty"`
	ImageURL          string                    `json:"image_url,omitempty"`
	Blur              *int                      `json:"blur,omitempty"`
	Alpha             *int                      `json:"alpha,omitempty"`
	Vertical          *bool                     `json:"vertical,omitempty"`
}

func DecodeProfileSettingsParams(raw json.RawMessage) (ProfileSettingsCommandParams, error) {
	var params ProfileSettingsCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing profile settings params")
	}
	if err := sonic.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal profile settings params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	params.Server = strings.TrimSpace(strings.ToLower(params.Server))
	params.TimeZone = strings.TrimSpace(params.TimeZone)
	params.ChartStyle = strings.TrimSpace(strings.ToLower(params.ChartStyle))
	params.ImageURL = strings.TrimSpace(params.ImageURL)
	for i := range params.DifficultyToggles {
		params.DifficultyToggles[i].Difficulty = normalizeProfileDifficulty(params.DifficultyToggles[i].Difficulty)
	}
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
			selectorServer := ""
			if params.RegionExplicit {
				selectorServer = params.Server
			}
			return service.currentBindingEntityBySelector(ctx, params.Platform, params.PlatformUserID, selectorServer, params.Selector)
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
		server := ""
		if params.RegionExplicit {
			server = params.Server
			items = filterBindingsByServer(items, server)
		}
		return []byte(formatVerifyListText(items, server)), nil
	case ProfileModeSetTimeZone:
		resolvedTimeZone, candidates, err := displaytime.ResolveUserTimeZoneInput(params.TimeZone)
		if err != nil {
			return nil, err
		}
		if len(candidates) > 0 {
			return []byte(formatTimeZoneCandidatesText(params.TimeZone, candidates)), nil
		}

		harukiUserID, err := service.identity.ResolveOrCreate(ctx, params.Platform, params.PlatformUserID)
		if err != nil {
			return nil, err
		}

		settings, err := GetUserSettings(ctx, service.pjskDB, harukiUserID)
		if err != nil {
			if !errors.Is(err, ErrUserSettingsNotFound) {
				return nil, fmt.Errorf("读取用户设置失败: %w", err)
			}
			settings = newDefaultUserSettings()
		}
		if settings == nil {
			settings = newDefaultUserSettings()
		}
		settings.TimeZone = resolvedTimeZone
		if err := UpsertUserSettings(ctx, service.pjskDB, harukiUserID, settings); err != nil {
			return nil, fmt.Errorf("保存用户时区失败: %w", err)
		}
		return []byte(fmt.Sprintf("已设置PJSK时区为 %s", resolvedTimeZone)), nil
	case ProfileModeSetArrestDiff:
		if len(params.DifficultyToggles) == 0 {
			return nil, fmt.Errorf("请至少指定一个逮捕难度开关")
		}

		harukiUserID, err := service.identity.ResolveOrCreate(ctx, params.Platform, params.PlatformUserID)
		if err != nil {
			return nil, err
		}

		settings, err := GetUserSettings(ctx, service.pjskDB, harukiUserID)
		if err != nil {
			if !errors.Is(err, ErrUserSettingsNotFound) {
				return nil, fmt.Errorf("读取用户设置失败: %w", err)
			}
			settings = newDefaultUserSettings()
		}
		if settings == nil {
			settings = newDefaultUserSettings()
		}

		nextDiffs, err := applyProfileDifficultyToggles(settings.PJSKEnabledDifficulties, params.DifficultyToggles)
		if err != nil {
			return nil, err
		}
		settings.PJSKEnabledDifficulties = nextDiffs
		if err := UpsertUserSettings(ctx, service.pjskDB, harukiUserID, settings); err != nil {
			return nil, fmt.Errorf("保存逮捕难度设置失败: %w", err)
		}
		return []byte(fmt.Sprintf("已设置逮捕难度为 %s", formatProfileDifficultySummary(settings.PJSKEnabledDifficulties))), nil
	case ProfileModeSetChartStyle:
		resolvedChartStyle := chartstyle.Normalize(params.ChartStyle)
		if resolvedChartStyle == "" {
			return nil, fmt.Errorf("谱面样式只支持 white 或 black")
		}

		harukiUserID, err := service.identity.ResolveOrCreate(ctx, params.Platform, params.PlatformUserID)
		if err != nil {
			return nil, err
		}

		settings, err := GetUserSettings(ctx, service.pjskDB, harukiUserID)
		if err != nil {
			if !errors.Is(err, ErrUserSettingsNotFound) {
				return nil, fmt.Errorf("读取用户设置失败: %w", err)
			}
			settings = newDefaultUserSettings()
		}
		if settings == nil {
			settings = newDefaultUserSettings()
		}
		settings.ChartStyle = resolvedChartStyle
		if err := UpsertUserSettings(ctx, service.pjskDB, harukiUserID, settings); err != nil {
			return nil, fmt.Errorf("保存谱面样式失败: %w", err)
		}
		return []byte(fmt.Sprintf("已设置谱面样式为 %s", resolvedChartStyle)), nil
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

func formatVerifyListText(items []BindingListItem, server string) string {
	if len(items) == 0 {
		if server != "" {
			return fmt.Sprintf("你还没有绑定任何%s服PJSK账号", strings.ToUpper(server))
		}
		return "你还没有绑定任何PJSK账号"
	}

	var lines []string
	if server != "" {
		lines = []string{fmt.Sprintf("已绑定%s服账号验证状态（u序号按该区服编号）:", strings.ToUpper(server))}
	} else {
		lines = []string{"已绑定账号验证状态（u序号全局编号）:"}
	}

	for i, item := range items {
		status := "❌"
		if item.Verified {
			status = "✅"
		}
		displayIdx := i + 1
		if server != "" {
			displayIdx = item.Index
		}
		line := fmt.Sprintf("u%d [%s] %s %s", displayIdx, strings.ToUpper(item.Server), formatBindingUID(item), status)
		marks := make([]string, 0, 2)
		if item.IsGlobalDefault {
			marks = append(marks, "全局默认")
		}
		if item.IsServerDefault {
			marks = append(marks, strings.ToUpper(item.Server)+"服默认")
		}
		if len(marks) > 0 {
			line += " (" + strings.Join(marks, " / ") + ")"
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

func formatTimeZoneCandidatesText(raw string, candidates []string) string {
	const maxCandidates = 20

	lines := []string{
		fmt.Sprintf("偏移量 %q 匹配到多个时区，请使用时区名重新设置：", strings.TrimSpace(raw)),
	}

	limit := len(candidates)
	if limit > maxCandidates {
		limit = maxCandidates
	}
	for i := 0; i < limit; i++ {
		lines = append(lines, candidates[i])
	}
	if len(candidates) > limit {
		lines = append(lines, fmt.Sprintf("... 以及另外 %d 个候选", len(candidates)-limit))
	}
	return strings.Join(lines, "\n")
}

func normalizeProfileDifficulty(value sekai.MusicDifficultyType) sekai.MusicDifficultyType {
	switch strings.ToLower(strings.TrimSpace(string(value))) {
	case string(sekai.MusicDifficultyEasy):
		return sekai.MusicDifficultyEasy
	case string(sekai.MusicDifficultyNormal):
		return sekai.MusicDifficultyNormal
	case string(sekai.MusicDifficultyHard):
		return sekai.MusicDifficultyHard
	case string(sekai.MusicDifficultyExpert):
		return sekai.MusicDifficultyExpert
	case string(sekai.MusicDifficultyMaster):
		return sekai.MusicDifficultyMaster
	case string(sekai.MusicDifficultyAppend):
		return sekai.MusicDifficultyAppend
	default:
		return ""
	}
}

func applyProfileDifficultyToggles(current []sekai.MusicDifficultyType, toggles []ProfileDifficultyToggle) ([]sekai.MusicDifficultyType, error) {
	enabled := make(map[sekai.MusicDifficultyType]bool, len(sekai.AllMusicDifficulties))
	for _, diff := range current {
		normalized := normalizeProfileDifficulty(diff)
		if normalized != "" {
			enabled[normalized] = true
		}
	}
	for _, toggle := range toggles {
		diff := normalizeProfileDifficulty(toggle.Difficulty)
		if diff == "" {
			return nil, fmt.Errorf("不支持的难度: %q", toggle.Difficulty)
		}
		enabled[diff] = toggle.Enabled
	}

	result := make([]sekai.MusicDifficultyType, 0, len(sekai.AllMusicDifficulties))
	for _, diff := range sekai.AllMusicDifficulties {
		if enabled[diff] {
			result = append(result, diff)
		}
	}
	return result, nil
}

func formatProfileDifficultySummary(enabled []sekai.MusicDifficultyType) string {
	enabledSet := make(map[sekai.MusicDifficultyType]bool, len(enabled))
	for _, diff := range enabled {
		normalized := normalizeProfileDifficulty(diff)
		if normalized != "" {
			enabledSet[normalized] = true
		}
	}

	parts := make([]string, 0, len(sekai.AllMusicDifficulties))
	for _, diff := range sekai.AllMusicDifficulties {
		status := "关闭"
		if enabledSet[diff] {
			status = "开启"
		}
		parts = append(parts, fmt.Sprintf("%s%s", diff, status))
	}
	return strings.Join(parts, " ")
}

func newDefaultUserSettings() *pjskschema.UserSettings {
	return &pjskschema.UserSettings{
		PJSKEnabledDifficulties: []sekai.MusicDifficultyType{
			sekai.MusicDifficultyExpert,
			sekai.MusicDifficultyMaster,
		},
	}
}
