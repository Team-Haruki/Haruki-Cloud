package query

import (
	"context"

	entpjsk "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/alias"
	"haruki-cloud/database/pjsk/groupalias"
	"haruki-cloud/database/pjsk/userbinding"
	"haruki-cloud/database/pjsk/userdefaultbinding"
	"haruki-cloud/database/pjsk/userpreference"
	pjskschema "haruki-cloud/ent/pjsk/schema"
	"haruki-cloud/utils/types"
)

func (c *Client) GetPJSKGlobalAliasToID(ctx context.Context, aliasType string, aliasStr string) (*types.AliasToIDResponse, error) {
	if err := c.requirePJSK(); err != nil {
		return nil, err
	}
	if _, err := types.ParseAliasType(aliasType); err != nil {
		return nil, ErrInvalidAliasType
	}
	if !isValidAlias(aliasStr) {
		return nil, ErrInvalidAlias
	}

	rows, err := c.pjsk.Alias.
		Query().
		Where(
			alias.AliasTypeEQ(aliasType),
			alias.AliasEQ(aliasStr),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrAliasNotFound
	}

	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.AliasTypeID)
	}
	return &types.AliasToIDResponse{MatchIDs: ids}, nil
}

func (c *Client) GetPJSKGlobalAliasesByID(ctx context.Context, aliasType string, aliasTypeID int) (*types.AliasListResponse, error) {
	if err := c.requirePJSK(); err != nil {
		return nil, err
	}
	if _, err := types.ParseAliasType(aliasType); err != nil {
		return nil, ErrInvalidAliasType
	}
	if aliasTypeID < 0 {
		return nil, ErrInvalidUserID
	}

	rows, err := c.pjsk.Alias.
		Query().
		Where(
			alias.AliasTypeEQ(aliasType),
			alias.AliasTypeIDEQ(aliasTypeID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrAliasNotFound
	}

	aliases := make([]string, 0, len(rows))
	for _, row := range rows {
		aliases = append(aliases, row.Alias)
	}
	return &types.AliasListResponse{Aliases: aliases}, nil
}

func (c *Client) GetPJSKGroupAliasToID(ctx context.Context, platform, groupID, aliasType, aliasStr string) (*types.AliasToIDResponse, error) {
	if err := c.requirePJSK(); err != nil {
		return nil, err
	}
	if platform == "" || groupID == "" {
		return nil, ErrInvalidUserID
	}
	if _, err := types.ParseAliasType(aliasType); err != nil {
		return nil, ErrInvalidAliasType
	}
	if !isValidAlias(aliasStr) {
		return nil, ErrInvalidAlias
	}

	rows, err := c.pjsk.GroupAlias.
		Query().
		Where(
			groupalias.PlatformEQ(platform),
			groupalias.GroupIDEQ(groupID),
			groupalias.AliasTypeEQ(aliasType),
			groupalias.AliasEQ(aliasStr),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrAliasNotFound
	}

	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.AliasTypeID)
	}
	return &types.AliasToIDResponse{MatchIDs: ids}, nil
}

func (c *Client) GetPJSKGroupAliasesByID(ctx context.Context, platform, groupID, aliasType string, aliasTypeID int) (*types.AliasListResponse, error) {
	if err := c.requirePJSK(); err != nil {
		return nil, err
	}
	if platform == "" || groupID == "" {
		return nil, ErrInvalidUserID
	}
	if _, err := types.ParseAliasType(aliasType); err != nil {
		return nil, ErrInvalidAliasType
	}
	if aliasTypeID < 0 {
		return nil, ErrInvalidUserID
	}

	rows, err := c.pjsk.GroupAlias.
		Query().
		Where(
			groupalias.PlatformEQ(platform),
			groupalias.GroupIDEQ(groupID),
			groupalias.AliasTypeEQ(aliasType),
			groupalias.AliasTypeIDEQ(aliasTypeID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrAliasNotFound
	}

	aliases := make([]string, 0, len(rows))
	for _, row := range rows {
		aliases = append(aliases, row.Alias)
	}
	return &types.AliasListResponse{Aliases: aliases}, nil
}

func (c *Client) GetPJSKBindings(ctx context.Context, harukiUserID int, server string) (*types.PJSKBindingResponse, error) {
	if err := c.requirePJSK(); err != nil {
		return nil, err
	}
	if harukiUserID <= 0 {
		return nil, ErrInvalidUserID
	}
	if server != "" {
		if _, err := types.ParseBindingServer(server); err != nil {
			return nil, err
		}
	}

	q := c.pjsk.UserBinding.Query().Where(userbinding.HarukiUserIDEQ(harukiUserID))
	if server != "" {
		q = q.Where(userbinding.ServerEQ(server))
	}

	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrBindingNotFound
	}

	bindings := make([]types.PJSKBinding, 0, len(rows))
	for _, row := range rows {
		bindings = append(bindings, types.PJSKBinding{
			ID:           row.ID,
			HarukiUserID: row.HarukiUserID,
			Server:       row.Server,
			UserID:       row.UserID,
			Visible:      row.Visible,
		})
	}

	return &types.PJSKBindingResponse{Bindings: bindings}, nil
}

func (c *Client) GetPJSKDefaultBinding(ctx context.Context, harukiUserID int, server string) (*types.PJSKBindingResponse, error) {
	if err := c.requirePJSK(); err != nil {
		return nil, err
	}
	if harukiUserID <= 0 {
		return nil, ErrInvalidUserID
	}
	if server == "" {
		server = "default"
	}
	if _, err := types.ParseDefaultBindingServer(server); err != nil {
		return nil, err
	}

	row, err := c.pjsk.UserDefaultBinding.
		Query().
		Where(
			userdefaultbinding.HarukiUserIDEQ(harukiUserID),
			userdefaultbinding.ServerEQ(server),
		).
		WithBinding().
		First(ctx)
	if err != nil {
		if entpjsk.IsNotFound(err) {
			return nil, ErrBindingNotFound
		}
		return nil, err
	}
	if row.Edges.Binding == nil {
		return nil, ErrBindingNotFound
	}

	binding := row.Edges.Binding
	return &types.PJSKBindingResponse{
		Binding: &types.PJSKBinding{
			ID:           binding.ID,
			HarukiUserID: binding.HarukiUserID,
			Server:       binding.Server,
			UserID:       binding.UserID,
			Visible:      binding.Visible,
		},
	}, nil
}

// GetPJSKSettings returns the UserSettings JSONB for the given haruki user.
// Returns ErrPreferenceNotFound when no settings row exists yet.
func (c *Client) GetPJSKSettings(ctx context.Context, harukiUserID int) (*pjskschema.UserSettings, error) {
	if err := c.requirePJSK(); err != nil {
		return nil, err
	}
	if harukiUserID <= 0 {
		return nil, ErrInvalidUserID
	}

	row, err := c.pjsk.UserPreference.
		Query().
		Where(userpreference.HarukiUserIDEQ(harukiUserID)).
		Only(ctx)
	if err != nil {
		if entpjsk.IsNotFound(err) {
			return nil, ErrPreferenceNotFound
		}
		return nil, err
	}
	if row.Settings == nil {
		return &pjskschema.UserSettings{}, nil
	}
	return row.Settings, nil
}

// UpsertPJSKSettings writes settings for a user, creating the preference row
// if it does not yet exist.
func (c *Client) UpsertPJSKSettings(ctx context.Context, harukiUserID int, settings *pjskschema.UserSettings) error {
	if err := c.requirePJSK(); err != nil {
		return err
	}
	if harukiUserID <= 0 {
		return ErrInvalidUserID
	}
	if settings == nil {
		return nil
	}

	existing, err := c.pjsk.UserPreference.
		Query().
		Where(userpreference.HarukiUserIDEQ(harukiUserID)).
		Only(ctx)
	if err != nil {
		if !entpjsk.IsNotFound(err) {
			return err
		}
		// Create
		_, err = c.pjsk.UserPreference.
			Create().
			SetHarukiUserID(harukiUserID).
			SetSettings(settings).
			Save(ctx)
		return err
	}
	// Update
	_, err = c.pjsk.UserPreference.
		UpdateOneID(existing.ID).
		SetSettings(settings).
		Save(ctx)
	return err
}

// IncrNoncompliantBGCount increments the user-level BG noncompliance counter by 1
// and returns the new count. Returns an error only on DB failure; missing rows are
// treated as count=0.
func (c *Client) IncrNoncompliantBGCount(ctx context.Context, harukiUserID int) (int, error) {
	if err := c.requirePJSK(); err != nil {
		return 0, err
	}
	if harukiUserID <= 0 {
		return 0, ErrInvalidUserID
	}

	settings := &pjskschema.UserSettings{}
	existing, err := c.GetPJSKSettings(ctx, harukiUserID)
	if err == nil {
		settings = existing
	}

	settings.NoncompliantBGCount++
	newCount := settings.NoncompliantBGCount
	if upsertErr := c.UpsertPJSKSettings(ctx, harukiUserID, settings); upsertErr != nil {
		return newCount, upsertErr
	}
	return newCount, nil
}
