package accountdata

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	pjskschema "haruki-cloud/ent/pjsk/schema"
	"haruki-cloud/internal/i18n"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/chartstyle"
	"haruki-cloud/internal/pjsk/displaytime"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/usererror"
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
	var done func(account i18n.Message) i18n.Message
	switch mode {
	case ProfileModeHideID:
		done = func(account i18n.Message) i18n.Message {
			return i18n.M("profile.visibility.uid_hidden", i18n.Data{"Account": account})
		}
		_, err = update.SetVisible(false).Save(ctx)
	case ProfileModeShowID:
		done = func(account i18n.Message) i18n.Message {
			return i18n.M("profile.visibility.uid_shown", i18n.Data{"Account": account})
		}
		_, err = update.SetVisible(true).Save(ctx)
	case ProfileModeHideSuite:
		done = func(account i18n.Message) i18n.Message {
			return i18n.M("profile.visibility.suite_hidden", i18n.Data{"Account": account})
		}
		_, err = update.SetSuiteVisible(false).Save(ctx)
	case ProfileModeShowSuite:
		done = func(account i18n.Message) i18n.Message {
			return i18n.M("profile.visibility.suite_shown", i18n.Data{"Account": account})
		}
		_, err = update.SetSuiteVisible(true).Save(ctx)
	case ProfileModeHideMySekai:
		done = func(account i18n.Message) i18n.Message {
			return i18n.M("profile.visibility.mysekai_hidden", i18n.Data{"Account": account})
		}
		_, err = update.SetMysekaiVisible(false).Save(ctx)
	case ProfileModeShowMySekai:
		done = func(account i18n.Message) i18n.Message {
			return i18n.M("profile.visibility.mysekai_shown", i18n.Data{"Account": account})
		}
		_, err = update.SetMysekaiVisible(true).Save(ctx)
	default:
		return nil, fmt.Errorf("bridge: unsupported profile visibility mode %q", mode)
	}
	if err != nil {
		return nil, err
	}
	item, err := service.bindingListItemByID(ctx, params.Platform, params.PlatformUserID, binding.ID)
	if err != nil {
		return nil, err
	}
	return []byte(done(bindingItemAccountLabel(*item)).String()), nil
}

func executeProfileVerifyMode(ctx context.Context, service *BindingService, params ProfileSettingsCommandParams, resolve profileBindingResolver) ([]byte, error) {
	if service.fastVerifier == nil {
		return nil, usererror.Misconfigured(errors.New("pjsk: fast verification provider is not configured"))
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
		return []byte(i18n.T("profile.verify.already", i18n.Data{"Account": bindingItemAccountLabel(*item)})), nil
	}
	return []byte(i18n.T("profile.verify.done", i18n.Data{"Account": bindingItemAccountLabel(*item)})), nil
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
		return 0, nil, usererror.Unavailable(i18n.FeatureAccount, fmt.Errorf("read user settings: %w", err))
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
		return nil, usererror.Wrap(usererror.CodeUnavailable, i18n.M("profile.settings.save_failed"), fmt.Errorf("save time zone: %w", err))
	}
	return []byte(i18n.T("profile.timezone.done", i18n.Data{"TimeZone": resolved})), nil
}

func executeProfileArrestDifficultyMode(ctx context.Context, service *BindingService, params ProfileSettingsCommandParams) ([]byte, error) {
	if len(params.DifficultyToggles) == 0 {
		return nil, usererror.Misuse(i18n.M("profile.arrest_difficulty.required"))
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
		return nil, usererror.Wrap(usererror.CodeUnavailable, i18n.M("profile.settings.save_failed"), fmt.Errorf("save arrest difficulties: %w", err))
	}
	return []byte(formatProfileDifficultySummary(settings.PJSKEnabledDifficulties).String()), nil
}

func executeProfileChartStyleMode(ctx context.Context, service *BindingService, params ProfileSettingsCommandParams) ([]byte, error) {
	style := chartstyle.Normalize(params.ChartStyle)
	if style == "" {
		return nil, usererror.Invalid(i18n.M("profile.chart_style.invalid"))
	}
	userID, settings, err := loadProfileUserSettings(ctx, service, params)
	if err != nil {
		return nil, err
	}
	settings.ChartStyle = style
	if err := UpsertUserSettings(ctx, service.pjskDB, userID, settings); err != nil {
		return nil, usererror.Wrap(usererror.CodeUnavailable, i18n.M("profile.settings.save_failed"), fmt.Errorf("save chart style: %w", err))
	}
	return []byte(i18n.T("profile.chart_style.done", i18n.Data{"Style": style})), nil
}

func executeProfileModularMode(ctx context.Context, service *BindingService, params ProfileSettingsCommandParams, enabled bool) ([]byte, error) {
	userID, settings, err := loadProfileUserSettings(ctx, service, params)
	if err != nil {
		return nil, err
	}
	settings.ModularProfileEnabled = enabled
	if err := UpsertUserSettings(ctx, service.pjskDB, userID, settings); err != nil {
		return nil, usererror.Wrap(usererror.CodeUnavailable, i18n.M("profile.settings.save_failed"), fmt.Errorf("save modular profile setting: %w", err))
	}
	if enabled {
		return []byte(i18n.T("profile.modular.enabled")), nil
	}
	return []byte(i18n.T("profile.modular.disabled")), nil
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
		return []byte(i18n.T("profile.bg.uploaded", i18n.Data{"Account": bindingItemAccountLabel(*item)})), nil
	case ProfileModeBGClear:
		item, err := service.clearBindingProfileBG(ctx, params.Platform, params.PlatformUserID, binding)
		if err != nil {
			return nil, err
		}
		return []byte(i18n.T("profile.bg.cleared", i18n.Data{"Account": bindingItemAccountLabel(*item)})), nil
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
		return []byte(i18n.T("profile.bg.adjusted", i18n.Data{"Account": bindingItemAccountLabel(*item)})), nil
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

// bindingItemAccountLabel is the "[日服(JP)] 123***789" label of a binding.
func bindingItemAccountLabel(item BindingListItem) i18n.Message {
	return i18n.AccountLabel(item.Server, item.UserID, item.Visible)
}

func formatVerifyListText(items []BindingListItem, server string) string {
	if len(items) == 0 {
		if server != "" {
			return i18n.T("binding.none_in_region", i18n.Data{"Region": i18n.RegionLabel(server)})
		}
		return i18n.T("binding.none")
	}

	var lines []string
	if server != "" {
		lines = []string{i18n.T("profile.verify_list.header_region", i18n.Data{"Region": i18n.RegionLabel(server)})}
	} else {
		lines = []string{i18n.T("profile.verify_list.header")}
	}

	for i, item := range items {
		lines = append(lines, formatVerifyListItem(item, i+1, server != "").String())
	}
	return strings.Join(lines, "\n")
}

func formatVerifyListItem(item BindingListItem, globalIndex int, useServerIndex bool) i18n.Message {
	status := i18n.M("profile.verify_list.unverified")
	if item.Verified {
		status = i18n.M("profile.verify_list.verified")
	}
	displayIndex := globalIndex
	if useServerIndex {
		displayIndex = item.Index
	}
	account := bindingItemAccountLabel(item)
	region := i18n.RegionLabel(item.Server)
	var defaults i18n.Message
	switch {
	case item.IsGlobalDefault && item.IsServerDefault:
		defaults = i18n.M("profile.verify_list.default_both", i18n.Data{"Region": region})
	case item.IsGlobalDefault:
		defaults = i18n.M("profile.verify_list.default_global")
	case item.IsServerDefault:
		defaults = i18n.M("profile.verify_list.default_region", i18n.Data{"Region": region})
	default:
		return i18n.M("profile.verify_list.item", i18n.Data{"Index": displayIndex, "Account": account, "Status": status})
	}
	return i18n.M("profile.verify_list.item_default", i18n.Data{"Index": displayIndex, "Account": account, "Status": status, "Default": defaults})
}

func formatProfileBGSettingsText(item BindingListItem) string {
	if item.Bg == nil || item.Bg.ImgPath == nil || strings.TrimSpace(*item.Bg.ImgPath) == "" {
		return i18n.T("profile.bg.none", i18n.Data{"Region": i18n.RegionLabel(item.Server)})
	}
	orientation := i18n.M("profile.bg.orientation.horizontal")
	if item.Bg.Vertical {
		orientation = i18n.M("profile.bg.orientation.vertical")
	}
	return i18n.T("profile.bg.settings", i18n.Data{
		"Account":     bindingItemAccountLabel(item),
		"Orientation": orientation,
		"Blur":        item.Bg.Blur,
		"Alpha":       item.Bg.Alpha,
	})
}

func formatTimeZoneCandidatesText(raw string, candidates []string) string {
	const maxCandidates = 20

	lines := []string{
		i18n.T("profile.timezone.ambiguous", i18n.Data{"Query": i18n.EchoQuery(raw)}),
	}

	limit := min(len(candidates), maxCandidates)
	lines = append(lines, candidates[:limit]...)
	if len(candidates) > limit {
		lines = append(lines, i18n.T("profile.timezone.ambiguous_more", i18n.Data{"Count": len(candidates) - limit}))
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
			return nil, usererror.BadParam(string(toggle.Difficulty), i18n.M("profile.arrest_difficulty.unknown"))
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

// formatProfileDifficultySummary is the reply after the arrest difficulties
// changed: the enabled difficulties in game order.
func formatProfileDifficultySummary(enabled []sekai.MusicDifficultyType) i18n.Message {
	enabledSet := make(map[sekai.MusicDifficultyType]bool, len(enabled))
	for _, diff := range enabled {
		normalized := normalizeProfileDifficulty(diff)
		if normalized != "" {
			enabledSet[normalized] = true
		}
	}

	labels := make([]string, 0, len(sekai.AllMusicDifficulties))
	for _, diff := range sekai.AllMusicDifficulties {
		if enabledSet[diff] {
			labels = append(labels, i18n.DifficultyLabel(string(diff)).String())
		}
	}
	if len(labels) == 0 {
		return i18n.M("profile.arrest_difficulty.done_none")
	}
	return i18n.M("profile.arrest_difficulty.done", i18n.Data{"Difficulties": strings.Join(labels, "、")})
}

func newDefaultUserSettings() *pjskschema.UserSettings {
	return &pjskschema.UserSettings{
		PJSKEnabledDifficulties: []sekai.MusicDifficultyType{
			sekai.MusicDifficultyExpert,
			sekai.MusicDifficultyMaster,
		},
	}
}
