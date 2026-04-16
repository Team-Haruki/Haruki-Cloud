package accountdata

import (
	"context"
	"errors"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/userpreference"
	pjskschema "haruki-cloud/ent/pjsk/schema"
)

// ErrUserSettingsNotFound is returned when no UserSettings row exists for the user.
var ErrUserSettingsNotFound = errors.New("pjsk: user settings not found")

// GetUserSettings returns the UserSettings JSONB for the given haruki user.
// Returns ErrUserSettingsNotFound when no row exists yet.
func GetUserSettings(ctx context.Context, db *pjskdb.Client, harukiUserID int) (*pjskschema.UserSettings, error) {
	if db == nil {
		return nil, errors.New("pjsk: client is not configured")
	}
	if harukiUserID <= 0 {
		return nil, errors.New("pjsk: invalid haruki_user_id")
	}

	row, err := db.UserPreference.
		Query().
		Where(userpreference.HarukiUserIDEQ(harukiUserID)).
		Only(ctx)
	if err != nil {
		if pjskdb.IsNotFound(err) {
			return nil, ErrUserSettingsNotFound
		}
		return nil, err
	}
	if row.Settings == nil {
		return &pjskschema.UserSettings{}, nil
	}
	return row.Settings, nil
}

// UpsertUserSettings writes settings for a user, creating the preference row
// if it does not yet exist.
func UpsertUserSettings(ctx context.Context, db *pjskdb.Client, harukiUserID int, settings *pjskschema.UserSettings) error {
	if db == nil {
		return errors.New("pjsk: client is not configured")
	}
	if harukiUserID <= 0 {
		return errors.New("pjsk: invalid haruki_user_id")
	}
	if settings == nil {
		return nil
	}

	existing, err := db.UserPreference.
		Query().
		Where(userpreference.HarukiUserIDEQ(harukiUserID)).
		Only(ctx)
	if err != nil {
		if !pjskdb.IsNotFound(err) {
			return err
		}
		_, err = db.UserPreference.
			Create().
			SetHarukiUserID(harukiUserID).
			SetSettings(settings).
			Save(ctx)
		return err
	}
	_, err = db.UserPreference.
		UpdateOneID(existing.ID).
		SetSettings(settings).
		Save(ctx)
	return err
}

// IncrNoncompliantBGCount increments the user-level BG noncompliance counter by 1
// and returns the new count. Missing rows are treated as count=0.
func IncrNoncompliantBGCount(ctx context.Context, db *pjskdb.Client, harukiUserID int) (int, error) {
	if db == nil {
		return 0, errors.New("pjsk: client is not configured")
	}
	if harukiUserID <= 0 {
		return 0, errors.New("pjsk: invalid haruki_user_id")
	}

	settings := &pjskschema.UserSettings{}
	if existing, err := GetUserSettings(ctx, db, harukiUserID); err == nil && existing != nil {
		settings = existing
	}

	settings.NoncompliantBGCount++
	newCount := settings.NoncompliantBGCount
	if upsertErr := UpsertUserSettings(ctx, db, harukiUserID, settings); upsertErr != nil {
		return newCount, upsertErr
	}
	return newCount, nil
}
