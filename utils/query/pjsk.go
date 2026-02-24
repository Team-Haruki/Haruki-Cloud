package query

import (
	"context"

	entpjsk "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/alias"
	"haruki-cloud/database/pjsk/groupalias"
	"haruki-cloud/database/pjsk/userbinding"
	"haruki-cloud/database/pjsk/userdefaultbinding"
	"haruki-cloud/database/pjsk/userpreference"
	"haruki-cloud/utils"
	"haruki-cloud/utils/types"
)

func (c *Client) GetPJSKGlobalAliasToID(ctx context.Context, aliasType string, aliasStr string) (*types.AliasToIDResponse, error) {
	if err := c.requirePJSK(); err != nil {
		return nil, err
	}
	if _, err := utils.ParseAliasType(aliasType); err != nil {
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
	if _, err := utils.ParseAliasType(aliasType); err != nil {
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
	if _, err := utils.ParseAliasType(aliasType); err != nil {
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
	if _, err := utils.ParseAliasType(aliasType); err != nil {
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
		if _, err := utils.ParseBindingServer(server); err != nil {
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
	if _, err := utils.ParseDefaultBindingServer(server); err != nil {
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

func (c *Client) GetPJSKPreferences(ctx context.Context, harukiUserID int) (*types.PJSKPreferenceResponse, error) {
	if err := c.requirePJSK(); err != nil {
		return nil, err
	}
	if harukiUserID <= 0 {
		return nil, ErrInvalidUserID
	}

	rows, err := c.pjsk.UserPreference.
		Query().
		Where(userpreference.HarukiUserIDEQ(harukiUserID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrPreferenceNotFound
	}

	options := make([]types.PJSKPreference, 0, len(rows))
	for _, row := range rows {
		options = append(options, types.PJSKPreference{
			Option: row.Option,
			Value:  row.Value,
		})
	}
	return &types.PJSKPreferenceResponse{Options: options}, nil
}

func (c *Client) GetPJSKPreference(ctx context.Context, harukiUserID int, option string) (*types.PJSKPreferenceResponse, error) {
	if err := c.requirePJSK(); err != nil {
		return nil, err
	}
	if harukiUserID <= 0 {
		return nil, ErrInvalidUserID
	}
	if option == "" {
		return nil, ErrPreferenceNotFound
	}

	row, err := c.pjsk.UserPreference.
		Query().
		Where(
			userpreference.HarukiUserIDEQ(harukiUserID),
			userpreference.OptionEQ(option),
		).
		First(ctx)
	if err != nil {
		if entpjsk.IsNotFound(err) {
			return nil, ErrPreferenceNotFound
		}
		return nil, err
	}

	respOption := &types.PJSKPreference{
		Option: row.Option,
		Value:  row.Value,
	}
	return &types.PJSKPreferenceResponse{Option: respOption}, nil
}
