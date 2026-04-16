package accountdata

import (
	"context"
	"fmt"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/utils/query"
)

func (s *BindingService) setBindingProfileBG(ctx context.Context, platform, platformUserID string, binding *pjskdb.UserBinding, imageURL string) (*BindingListItem, error) {
	if s == nil || s.bgStorage == nil {
		return nil, fmt.Errorf("pjsk: profile background storage is not configured")
	}
	if binding == nil {
		return nil, fmt.Errorf("未找到要设置背景的绑定账号")
	}
	if !binding.Verified {
		return nil, fmt.Errorf("当前%s服绑定账号尚未验证，无法设置个人信息背景", strings.ToUpper(binding.Server))
	}

	// User-level BG ban check: read from UserSettings (haruki_user_id granularity).
	queryClient := query.NewClient(nil, nil, s.pjskDB, nil)
	userSettings, _ := queryClient.GetPJSKSettings(ctx, binding.HarukiUserID)
	currentCount := 0
	if userSettings != nil {
		currentCount = userSettings.NoncompliantBGCount
	}
	if currentCount >= 3 {
		return nil, fmt.Errorf("已达到背景图片违规上传上限（%d/3），背景上传功能已被禁用", currentCount)
	}

	// Image content moderation
	if s.censor != nil {
		if !s.censor.CensorImage(ctx, binding.HarukiUserID, imageURL) {
			newCount, _ := queryClient.IncrNoncompliantBGCount(ctx, binding.HarukiUserID)
			if newCount >= 3 {
				return nil, fmt.Errorf("背景图片内容审核未通过，背景上传功能已被禁用（违规次数已达 3/3）")
			}
			return nil, fmt.Errorf("背景图片内容审核未通过，请更换图片（违规次数：%d/3）", newCount)
		}
	}

	oldBg, err := loadProfileBackground(ctx, s.pjskDB, binding.Server, binding.UserID)
	if err != nil {
		return nil, err
	}
	settings, err := s.bgStorage.SaveProfileBackground(ctx, binding.Server, binding.UserID, imageURL)
	if err != nil {
		return nil, err
	}
	if err := upsertProfileBackground(ctx, s.pjskDB, binding.Server, binding.UserID, settings); err != nil {
		return nil, err
	}
	// Remove the old background file after the DB record is updated.
	if oldBg != nil && !sameProfileBGPath(oldBg, settings) {
		_ = s.bgStorage.DeleteProfileBackground(ctx, oldBg)
	}
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

func (s *BindingService) clearBindingProfileBG(ctx context.Context, platform, platformUserID string, binding *pjskdb.UserBinding) (*BindingListItem, error) {
	if binding == nil {
		return nil, fmt.Errorf("未找到要清除背景的绑定账号")
	}
	if !binding.Verified {
		return nil, fmt.Errorf("当前%s服绑定账号尚未验证，无法清除个人信息背景", strings.ToUpper(binding.Server))
	}
	settings, err := loadProfileBackground(ctx, s.pjskDB, binding.Server, binding.UserID)
	if err != nil {
		return nil, err
	}
	if s.bgStorage != nil {
		if err := s.bgStorage.DeleteProfileBackground(ctx, settings); err != nil {
			return nil, err
		}
	}
	if err := deleteProfileBackground(ctx, s.pjskDB, binding.Server, binding.UserID); err != nil {
		return nil, err
	}
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

func (s *BindingService) adjustBindingProfileBG(ctx context.Context, platform, platformUserID string, binding *pjskdb.UserBinding, blur, alpha *int, vertical *bool) (*BindingListItem, error) {
	if binding == nil {
		return nil, fmt.Errorf("未找到要调整背景的绑定账号")
	}
	if !binding.Verified {
		return nil, fmt.Errorf("当前%s服绑定账号尚未验证，无法调整个人信息背景", strings.ToUpper(binding.Server))
	}
	currentBg, err := loadProfileBackground(ctx, s.pjskDB, binding.Server, binding.UserID)
	if err != nil {
		return nil, err
	}
	if currentBg == nil || currentBg.ImgPath == nil || strings.TrimSpace(*currentBg.ImgPath) == "" {
		return nil, fmt.Errorf("当前%s服还没有自定义个人信息背景", strings.ToUpper(binding.Server))
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

	if err := upsertProfileBackground(ctx, s.pjskDB, binding.Server, binding.UserID, settings); err != nil {
		return nil, err
	}
	return s.bindingListItemByID(ctx, platform, platformUserID, binding.ID)
}

// SetBindingVisible sets the visibility flag for the current binding.
func (s *BindingService) SetBindingVisible(ctx context.Context, platform, platformUserID, server string, visible bool) (*BindingListItem, error) {
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
	records, err := s.fastVerifier.GetToolboxUserFastVerificationGameAccountBindings(platform, platformUserID)
	if err != nil {
		return nil, false, err
	}
	matched := false
	for _, record := range records {
		if strings.EqualFold(strings.TrimSpace(record.Server), binding.Server) &&
			strings.TrimSpace(record.GameUserID) == binding.UserID {
			matched = true
			break
		}
	}
	if !matched {
		return nil, false, fmt.Errorf("当前%s服绑定账号未出现在快速验证列表中", strings.ToUpper(binding.Server))
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
		return nil, false, fmt.Errorf("pjsk: fast verification provider is not configured")
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
		return nil, fmt.Errorf("请提供区服")
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
