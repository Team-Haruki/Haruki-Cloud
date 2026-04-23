package accountdata

import (
	"context"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/gameaccount"
	"haruki-cloud/internal/pjsk/drawing"
)

func bindingGameAccount(binding *pjskdb.UserBinding) *pjskdb.GameAccount {
	if binding == nil {
		return nil
	}
	return binding.Edges.GameAccount
}

func bindingGameAccountID(binding *pjskdb.UserBinding) int {
	if binding == nil {
		return 0
	}
	if binding.GameAccountID != nil {
		return *binding.GameAccountID
	}
	if account := bindingGameAccount(binding); account != nil {
		return account.ID
	}
	return 0
}

func bindingServer(binding *pjskdb.UserBinding) string {
	account := bindingGameAccount(binding)
	if account == nil {
		return ""
	}
	return strings.TrimSpace(strings.ToLower(account.Server))
}

func bindingUserID(binding *pjskdb.UserBinding) string {
	account := bindingGameAccount(binding)
	if account == nil {
		return ""
	}
	return strings.TrimSpace(account.UserID)
}

func resolveBindingProfileBG(binding *pjskdb.UserBinding) *drawing.ProfileBgSettings {
	account := bindingGameAccount(binding)
	if account == nil || !hasCustomProfileBGImage(account.Bg) {
		return nil
	}
	return cloneProfileBGSettings(account.Bg)
}

func hasCustomProfileBGImage(bg *drawing.ProfileBgSettings) bool {
	return bg != nil && bg.ImgPath != nil && strings.TrimSpace(*bg.ImgPath) != ""
}

func sameProfileBGPath(left, right *drawing.ProfileBgSettings) bool {
	if !hasCustomProfileBGImage(left) || !hasCustomProfileBGImage(right) {
		return false
	}
	return strings.TrimSpace(*left.ImgPath) == strings.TrimSpace(*right.ImgPath)
}

func mergeUploadedProfileBGSettings(current, uploaded *drawing.ProfileBgSettings) *drawing.ProfileBgSettings {
	if current == nil {
		return cloneProfileBGSettings(uploaded)
	}
	merged := cloneProfileBGSettings(current)
	if uploaded != nil {
		if uploaded.ImgPath != nil {
			path := strings.TrimSpace(*uploaded.ImgPath)
			merged.ImgPath = &path
		}
		// A newly uploaded image should always refresh the auto-detected orientation,
		// matching lunabot's behavior instead of inheriting the previous image layout.
		merged.Vertical = uploaded.Vertical
	}
	return merged
}

func clearProfileBGImagePath(current *drawing.ProfileBgSettings) *drawing.ProfileBgSettings {
	if current == nil {
		return nil
	}
	cleared := cloneProfileBGSettings(current)
	cleared.ImgPath = nil
	return cleared
}

func loadProfileBackground(ctx context.Context, db *pjskdb.Client, gameAccountID int) (*drawing.ProfileBgSettings, error) {
	if db == nil || gameAccountID <= 0 {
		return nil, nil
	}
	row, err := db.GameAccount.Query().
		Where(gameaccount.IDEQ(gameAccountID)).
		Only(ctx)
	if err != nil {
		if pjskdb.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return cloneProfileBGSettings(row.Bg), nil
}

func upsertProfileBackground(ctx context.Context, db *pjskdb.Client, gameAccountID int, settings *drawing.ProfileBgSettings) error {
	if db == nil || gameAccountID <= 0 || settings == nil {
		return nil
	}
	_, err := db.GameAccount.UpdateOneID(gameAccountID).
		SetBg(cloneProfileBGSettings(settings)).
		Save(ctx)
	return err
}

func deleteProfileBackground(ctx context.Context, db *pjskdb.Client, gameAccountID int) error {
	if db == nil || gameAccountID <= 0 {
		return nil
	}
	_, err := db.GameAccount.UpdateOneID(gameAccountID).
		ClearBg().
		Save(ctx)
	if pjskdb.IsNotFound(err) {
		return nil
	}
	return err
}
