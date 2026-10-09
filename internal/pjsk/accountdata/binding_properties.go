package accountdata

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/usererror"
)

func unverifiedBindingProfileBGError(binding *pjskdb.UserBinding) error {
	server := strings.ToLower(strings.TrimSpace(bindingServer(binding)))
	return usererror.Setup(i18n.M("profile.bg.unverified", i18n.Data{
		"Region":      i18n.RegionLabel(server),
		"Command":     "/" + server + "pjsk verify",
		"ToolboxLink": i18n.M("binding.toolbox_link"),
	}))
}

func (s *BindingService) setBindingProfileBG(ctx context.Context, platform, platformUserID string, binding *pjskdb.UserBinding, imageURL string) (*BindingListItem, error) {
	if s == nil || s.bgStorage == nil {
		return nil, usererror.Misconfigured(errors.New("pjsk: profile background storage is not configured"))
	}
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("profile.bg.binding_missing"))
	}
	if !binding.Verified {
		return nil, unverifiedBindingProfileBGError(binding)
	}

	if err := s.validateProfileBGUpload(ctx, binding, imageURL); err != nil {
		return nil, err
	}

	server := bindingServer(binding)
	userID := bindingUserID(binding)
	gameAccountID := bindingGameAccountID(binding)

	oldBg, revision, err := loadProfileBackgroundRevision(ctx, s.pjskDB, gameAccountID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, profileBGMutationTimeout)
	defer cancel()
	var intentID int
	uploadedSettings, err := s.bgStorage.SaveProfileBackgroundTracked(ctx, server, userID, imageURL, func(settings *drawing.ProfileBgSettings) error {
		var reserveErr error
		intentID, reserveErr = s.reserveProfileBGUpload(ctx, gameAccountID, settings)
		return reserveErr
	})
	if err != nil {
		return nil, err
	}
	if intentID == 0 {
		return nil, fmt.Errorf("profile background storage did not reserve an upload")
	}
	settings := mergeUploadedProfileBGSettings(oldBg, uploadedSettings)
	cleanupID, err := s.commitProfileBackground(ctx, gameAccountID, revision, oldBg, settings, intentID)
	if err != nil {
		return nil, err
	}
	s.tryProfileBGCleanup(ctx, cleanupID)
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

func (s *BindingService) validateProfileBGUpload(
	ctx context.Context,
	binding *pjskdb.UserBinding,
	imageURL string,
) error {
	userSettings, err := GetUserSettings(ctx, s.pjskDB, binding.HarukiUserID)
	if err != nil && !errors.Is(err, ErrUserSettingsNotFound) {
		return usererror.Unavailable(i18n.FeatureAccount, fmt.Errorf("read user settings: %w", err))
	}
	currentCount := 0
	if userSettings != nil {
		currentCount = userSettings.NoncompliantBGCount
	}
	if currentCount >= 3 {
		return usererror.Forbidden(i18n.M("profile.bg.upload_disabled", i18n.Data{"Count": currentCount, "Max": 3}))
	}
	if s.censor == nil || s.censor.CensorImage(ctx, binding.HarukiUserID, imageURL) {
		return nil
	}
	newCount, err := IncrNoncompliantBGCount(ctx, s.pjskDB, binding.HarukiUserID)
	if err != nil {
		return usererror.Wrap(usererror.CodeInput, i18n.M("profile.bg.rejected", i18n.Data{"Count": currentCount + 1, "Max": 3}), fmt.Errorf("update noncompliant background count: %w", err))
	}
	if newCount >= 3 {
		return usererror.Forbidden(i18n.M("profile.bg.rejected_disabled", i18n.Data{"Max": 3}))
	}
	return usererror.Invalid(i18n.M("profile.bg.rejected", i18n.Data{"Count": newCount, "Max": 3}))
}

func (s *BindingService) clearBindingProfileBG(ctx context.Context, platform, platformUserID string, binding *pjskdb.UserBinding) (*BindingListItem, error) {
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("profile.bg.binding_missing"))
	}
	if !binding.Verified {
		return nil, unverifiedBindingProfileBGError(binding)
	}
	gameAccountID := bindingGameAccountID(binding)
	settings, revision, err := loadProfileBackgroundRevision(ctx, s.pjskDB, gameAccountID)
	if err != nil {
		return nil, err
	}
	cleanupID, err := s.commitProfileBackground(ctx, gameAccountID, revision, settings, clearProfileBGImagePath(settings), 0)
	if err != nil {
		return nil, err
	}
	s.tryProfileBGCleanup(ctx, cleanupID)
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

func (s *BindingService) adjustBindingProfileBG(ctx context.Context, platform, platformUserID string, binding *pjskdb.UserBinding, blur, alpha *int, vertical *bool) (*BindingListItem, error) {
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("profile.bg.binding_missing"))
	}
	if !binding.Verified {
		return nil, unverifiedBindingProfileBGError(binding)
	}
	gameAccountID := bindingGameAccountID(binding)
	currentBg, revision, err := loadProfileBackgroundRevision(ctx, s.pjskDB, gameAccountID)
	if err != nil {
		return nil, err
	}
	if currentBg == nil || currentBg.ImgPath == nil || strings.TrimSpace(*currentBg.ImgPath) == "" {
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("profile.bg.none", i18n.Data{"Region": i18n.RegionLabel(bindingServer(binding))}))
	}

	settings := cloneProfileBGSettings(currentBg)
	if blur != nil {
		settings.Blur = *blur
	}
	if alpha != nil {
		settings.Alpha = *alpha
	}
	if vertical != nil {
		settings.Vertical = *vertical
	}

	if _, err := s.commitProfileBackground(ctx, gameAccountID, revision, currentBg, settings, 0); err != nil {
		return nil, err
	}
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

// SetBindingVisible sets the visibility flag for the current binding.
func (s *BindingService) SetBindingVisible(ctx context.Context, platform, platformUserID, server string, visible bool) (*BindingListItem, error) {
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	if _, err := s.pjskDB.UserBinding.UpdateOneID(binding.ID).
		SetVisible(visible).
		Save(ctx); err != nil {
		return nil, err
	}
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

// SetBindingSuiteVisible sets the suite visibility flag for the current binding.
func (s *BindingService) SetBindingSuiteVisible(ctx context.Context, platform, platformUserID, server string, suiteVisible bool) (*BindingListItem, error) {
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	if _, err := s.pjskDB.UserBinding.UpdateOneID(binding.ID).
		SetSuiteVisible(suiteVisible).
		Save(ctx); err != nil {
		return nil, err
	}
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

// SetBindingMySekaiVisible sets the MySekai visibility flag for the current binding.
func (s *BindingService) SetBindingMySekaiVisible(ctx context.Context, platform, platformUserID, server string, mySekaiVisible bool) (*BindingListItem, error) {
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	if _, err := s.pjskDB.UserBinding.UpdateOneID(binding.ID).
		SetMysekaiVisible(mySekaiVisible).
		Save(ctx); err != nil {
		return nil, err
	}
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

// verifyBindingEntity performs the actual verification check on an already-resolved
// binding entity. Called by VerifyCurrentBinding and ExecuteProfileSettingsCommand.
func (s *BindingService) verifyBindingEntity(ctx context.Context, platform, platformUserID string, binding *pjskdb.UserBinding) (*BindingListItem, bool, error) {
	if binding.Verified {
		item, itemErr := s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
		return item, true, itemErr
	}
	if err := s.requireWritable(); err != nil {
		return nil, false, err
	}
	var records []sekaiapi.UserGameBinding
	var err error
	if contextual, ok := s.fastVerifier.(contextFastVerificationProvider); ok {
		records, err = contextual.GetToolboxUserFastVerificationGameAccountBindingsContext(ctx, platform, platformUserID)
	} else {
		records, err = s.fastVerifier.GetToolboxUserFastVerificationGameAccountBindings(platform, platformUserID)
	}
	if err != nil {
		return nil, false, err
	}
	matched := false
	for _, record := range records {
		if strings.EqualFold(strings.TrimSpace(record.Server), bindingServer(binding)) &&
			strings.TrimSpace(record.GameUserID) == bindingUserID(binding) {
			matched = true
			break
		}
	}
	if !matched {
		return nil, false, usererror.Setup(i18n.M("binding.verify.not_listed", i18n.Data{"Region": i18n.RegionLabel(bindingServer(binding))}))
	}
	if _, err := s.pjskDB.UserBinding.UpdateOneID(binding.ID).
		SetVerified(true).
		Save(ctx); err != nil {
		return nil, false, err
	}
	item, err := s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
	return item, false, err
}

// VerifyCurrentBinding verifies the current binding using fast verification.
func (s *BindingService) VerifyCurrentBinding(ctx context.Context, platform, platformUserID, server string) (*BindingListItem, bool, error) {
	if s == nil || s.fastVerifier == nil {
		return nil, false, usererror.Misconfigured(errors.New("pjsk: fast verification provider is not configured"))
	}
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, false, err
	}
	return s.verifyBindingEntity(ctx, platform, platformUserID, binding)
}

// ListVerifiedBindings returns all verified bindings for a server.
func (s *BindingService) ListVerifiedBindings(ctx context.Context, platform, platformUserID, server string) ([]BindingListItem, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return nil, err
	}
	server = strings.TrimSpace(strings.ToLower(server))
	if server == "" {
		return nil, usererror.Misuse(i18n.M("binding.region_required"))
	}
	items, err := s.List(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	var verified []BindingListItem
	for _, item := range items {
		if !strings.EqualFold(item.Server, server) || !item.Verified {
			continue
		}
		verified = append(verified, item)
	}
	return verified, nil
}

// SetCurrentBindingProfileBG sets the profile background for the current binding.
func (s *BindingService) SetCurrentBindingProfileBG(ctx context.Context, platform, platformUserID, server, imageURL string) (*BindingListItem, error) {
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	return s.setBindingProfileBG(ctx, platform, platformUserID, binding, imageURL)
}

// ClearCurrentBindingProfileBG clears the profile background for the current binding.
func (s *BindingService) ClearCurrentBindingProfileBG(ctx context.Context, platform, platformUserID, server string) (*BindingListItem, error) {
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	return s.clearBindingProfileBG(ctx, platform, platformUserID, binding)
}

// AdjustCurrentBindingProfileBG adjusts the profile background settings for the current binding.
func (s *BindingService) AdjustCurrentBindingProfileBG(ctx context.Context, platform, platformUserID, server string, blur, alpha *int, vertical *bool) (*BindingListItem, error) {
	binding, err := s.currentBindingEntity(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	return s.adjustBindingProfileBG(ctx, platform, platformUserID, binding, blur, alpha, vertical)
}
