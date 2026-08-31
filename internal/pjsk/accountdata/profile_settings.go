package accountdata

import (
	"context"
	"errors"
	"fmt"
	json "haruki-cloud/internal/jsonutil"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	pjskschema "haruki-cloud/ent/pjsk/schema"
	"haruki-cloud/internal/pjsk/chartstyle"
	"haruki-cloud/internal/pjsk/displaytime"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/sekai"
)

const (
	ProfileModeHideID         = "profile-hide-id"
	ProfileModeShowID         = "profile-show-id"
	ProfileModeHideSuite      = "profile-hide-suite"
	ProfileModeShowSuite      = "profile-show-suite"
	ProfileModeHideMySekai    = "profile-hide-mysekai"
	ProfileModeShowMySekai    = "profile-show-mysekai"
	ProfileModeVerify         = "profile-verify"
	ProfileModeVerifyList     = "profile-verify-list"
	ProfileModeSetTimeZone    = "profile-set-timezone"
	ProfileModeSetArrestDiff  = "profile-set-arrest-difficulty"
	ProfileModeSetChartStyle  = "profile-set-chart-style"
	ProfileModeEnableModular  = "profile-enable-modular"
	ProfileModeDisableModular = "profile-disable-modular"
	ProfileModeBGUpload       = "profile-bg-upload"
	ProfileModeBGClear        = "profile-bg-clear"
	ProfileModeBGAdjust       = "profile-bg-adjust"
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
	if err := json.Unmarshal(raw, &params); err != nil {
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
	if profileSettingsModeMutates(mode, params) {
		if err := service.requireWritable(); err != nil {
			return nil, err
		}
	}
	resolveBinding := newProfileBindingResolver(ctx, service, params)
	switch mode {
	case ProfileModeHideID, ProfileModeShowID,
		ProfileModeHideSuite, ProfileModeShowSuite,
		ProfileModeHideMySekai, ProfileModeShowMySekai:
		return executeProfileVisibilityMode(ctx, service, mode, params, resolveBinding)
	case ProfileModeVerify:
		return executeProfileVerifyMode(ctx, service, params, resolveBinding)
	case ProfileModeVerifyList:
		return executeProfileVerifyListMode(ctx, service, params)
	case ProfileModeSetTimeZone:
		return executeProfileTimeZoneMode(ctx, service, params)
	case ProfileModeSetArrestDiff:
		return executeProfileArrestDifficultyMode(ctx, service, params)
	case ProfileModeSetChartStyle:
		return executeProfileChartStyleMode(ctx, service, params)
	case ProfileModeEnableModular, ProfileModeDisableModular:
		return executeProfileModularMode(ctx, service, params, mode == ProfileModeEnableModular)
	case ProfileModeBGUpload, ProfileModeBGClear, ProfileModeBGAdjust:
		return executeProfileBackgroundMode(ctx, service, mode, params, resolveBinding)
	default:
		return nil, fmt.Errorf("bridge: unsupported profile settings mode %q", mode)
	}
}

type profileBindingResolver func() (*pjskdb.UserBinding, error)

func newProfileBindingResolver(ctx context.Context, service *BindingService, params ProfileSettingsCommandParams) profileBindingResolver {
	return func() (*pjskdb.UserBinding, error) {
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
}

func executeProfileVisibilityMode(ctx context.Context, service *BindingService, mode string, params ProfileSettingsCommandParams, resolve profileBindingResolver) ([]byte, error) {
	binding, err := resolve()
	if err != nil {
		return nil, err
	}
	update := service.pjskDB.UserBinding.UpdateOneID(binding.ID)
	action, subject := "", ""
	switch mode {
	case ProfileModeHideID:
		action, subject = "隐藏", "ID信息"
		_, err = update.SetVisible(false).Save(ctx)
	case ProfileModeShowID:
		action, subject = "展示", "ID信息"
		_, err = update.SetVisible(true).Save(ctx)
	case ProfileModeHideSuite:
		action, subject = "隐藏", "抓包信息"
		_, err = update.SetSuiteVisible(false).Save(ctx)
	case ProfileModeShowSuite:
		action, subject = "展示", "抓包信息"
		_, err = update.SetSuiteVisible(true).Save(ctx)
	case ProfileModeHideMySekai:
		action, subject = "隐藏", "烤森抓包信息"
		_, err = update.SetMysekaiVisible(false).Save(ctx)
	case ProfileModeShowMySekai:
		action, subject = "展示", "烤森抓包信息"
		_, err = update.SetMysekaiVisible(true).Save(ctx)
	}
	if err != nil {
		return nil, err
	}
	item, err := service.bindingListItemByID(ctx, params.Platform, params.PlatformUserID, binding.ID)
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("已%s [%s] %s 的%s", action, strings.ToUpper(item.Server), formatBindingUID(*item), subject)), nil
}

func executeProfileVerifyMode(ctx context.Context, service *BindingService, params ProfileSettingsCommandParams, resolve profileBindingResolver) ([]byte, error) {
	if service.fastVerifier == nil {
		return nil, fmt.Errorf("pjsk: fast verification provider is not configured")
	}
	entity, err := resolve()
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
}

func executeProfileVerifyListMode(ctx context.Context, service *BindingService, params ProfileSettingsCommandParams) ([]byte, error) {
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
}

func loadProfileUserSettings(ctx context.Context, service *BindingService, params ProfileSettingsCommandParams) (int, *pjskschema.UserSettings, error) {
	harukiUserID, err := service.identity.ResolveOrCreate(ctx, params.Platform, params.PlatformUserID)
	if err != nil {
		return 0, nil, err
	}
	settings, err := GetUserSettings(ctx, service.pjskDB, harukiUserID)
	if err != nil && !errors.Is(err, ErrUserSettingsNotFound) {
		return 0, nil, fmt.Errorf("读取用户设置失败: %w", err)
	}
	if settings == nil || errors.Is(err, ErrUserSettingsNotFound) {
		settings = newDefaultUserSettings()
	}
	return harukiUserID, settings, nil
}

func executeProfileTimeZoneMode(ctx context.Context, service *BindingService, params ProfileSettingsCommandParams) ([]byte, error) {
	resolved, candidates, err := displaytime.ResolveUserTimeZoneInput(params.TimeZone)
	if err != nil {
		return nil, err
	}
	if len(candidates) > 0 {
		return []byte(formatTimeZoneCandidatesText(params.TimeZone, candidates)), nil
	}
	userID, settings, err := loadProfileUserSettings(ctx, service, params)
	if err != nil {
		return nil, err
	}
	settings.TimeZone = resolved
	if err := UpsertUserSettings(ctx, service.pjskDB, userID, settings); err != nil {
		return nil, fmt.Errorf("保存用户时区失败: %w", err)
	}
	return []byte(fmt.Sprintf("已设置PJSK时区为 %s", resolved)), nil
}

func executeProfileArrestDifficultyMode(ctx context.Context, service *BindingService, params ProfileSettingsCommandParams) ([]byte, error) {
	if len(params.DifficultyToggles) == 0 {
		return nil, fmt.Errorf("请至少指定一个逮捕难度开关")
	}
	userID, settings, err := loadProfileUserSettings(ctx, service, params)
	if err != nil {
		return nil, err
	}
	settings.PJSKEnabledDifficulties, err = applyProfileDifficultyToggles(settings.PJSKEnabledDifficulties, params.DifficultyToggles)
	if err != nil {
		return nil, err
	}
	if err := UpsertUserSettings(ctx, service.pjskDB, userID, settings); err != nil {
		return nil, fmt.Errorf("保存逮捕难度设置失败: %w", err)
	}
	return []byte(fmt.Sprintf("已设置逮捕难度为 %s", formatProfileDifficultySummary(settings.PJSKEnabledDifficulties))), nil
}

func executeProfileChartStyleMode(ctx context.Context, service *BindingService, params ProfileSettingsCommandParams) ([]byte, error) {
	style := chartstyle.Normalize(params.ChartStyle)
	if style == "" {
		return nil, fmt.Errorf("谱面样式只支持 white 或 black")
	}
	userID, settings, err := loadProfileUserSettings(ctx, service, params)
	if err != nil {
		return nil, err
	}
	settings.ChartStyle = style
	if err := UpsertUserSettings(ctx, service.pjskDB, userID, settings); err != nil {
		return nil, fmt.Errorf("保存谱面样式失败: %w", err)
	}
	return []byte(fmt.Sprintf("已设置谱面样式为 %s", style)), nil
}

func executeProfileModularMode(ctx context.Context, service *BindingService, params ProfileSettingsCommandParams, enabled bool) ([]byte, error) {
	userID, settings, err := loadProfileUserSettings(ctx, service, params)
	if err != nil {
		return nil, err
	}
	settings.ModularProfileEnabled = enabled
	if err := UpsertUserSettings(ctx, service.pjskDB, userID, settings); err != nil {
		return nil, fmt.Errorf("保存模块个人信息设置失败: %w", err)
	}
	if enabled {
		return []byte("已开启模块个人信息，之后 /个人信息 将使用模块布局"), nil
	}
	return []byte("已关闭模块个人信息，之后 /个人信息 将使用经典布局"), nil
}

func executeProfileBackgroundMode(ctx context.Context, service *BindingService, mode string, params ProfileSettingsCommandParams, resolve profileBindingResolver) ([]byte, error) {
	binding, err := resolve()
	if err != nil {
		return nil, err
	}
	switch mode {
	case ProfileModeBGUpload:
		item, err := service.setBindingProfileBG(ctx, params.Platform, params.PlatformUserID, binding, params.ImageURL)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已更新%s服个人信息背景", strings.ToUpper(item.Server))), nil
	case ProfileModeBGClear:
		item, err := service.clearBindingProfileBG(ctx, params.Platform, params.PlatformUserID, binding)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已清空%s服个人信息背景", strings.ToUpper(item.Server))), nil
	case ProfileModeBGAdjust:
		if params.Blur == nil && params.Alpha == nil && params.Vertical == nil {
			item, err := service.bindingListItemByID(ctx, params.Platform, params.PlatformUserID, binding.ID)
			if err != nil {
				return nil, err
			}
			return []byte(formatProfileBGSettingsText(*item)), nil
		}
		item, err := service.adjustBindingProfileBG(ctx, params.Platform, params.PlatformUserID, binding, params.Blur, params.Alpha, params.Vertical)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已更新%s服个人信息背景设置", strings.ToUpper(item.Server))), nil
	default:
		return nil, fmt.Errorf("bridge: unsupported profile background mode %q", mode)
	}
}

func profileSettingsModeMutates(mode string, params ProfileSettingsCommandParams) bool {
	switch mode {
	case ProfileModeHideID,
		ProfileModeShowID,
		ProfileModeHideSuite,
		ProfileModeShowSuite,
		ProfileModeHideMySekai,
		ProfileModeShowMySekai,
		ProfileModeSetTimeZone,
		ProfileModeSetArrestDiff,
		ProfileModeSetChartStyle,
		ProfileModeEnableModular,
		ProfileModeDisableModular,
		ProfileModeBGUpload,
		ProfileModeBGClear:
		return true
	case ProfileModeBGAdjust:
		return params.Blur != nil || params.Alpha != nil || params.Vertical != nil
	default:
		return false
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
		lines = append(lines, formatVerifyListItem(item, i+1, server != ""))
	}
	return strings.Join(lines, "\n")
}

func formatVerifyListItem(item BindingListItem, globalIndex int, useServerIndex bool) string {
	status := "❌"
	if item.Verified {
		status = "✅"
	}
	displayIndex := globalIndex
	if useServerIndex {
		displayIndex = item.Index
	}
	line := fmt.Sprintf("u%d [%s] %s %s", displayIndex, strings.ToUpper(item.Server), formatBindingUID(item), status)
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
	return line
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
