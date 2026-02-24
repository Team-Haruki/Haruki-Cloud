package query

import (
	"context"

	entusers "haruki-cloud/database/users"
	"haruki-cloud/database/users/user"
	"haruki-cloud/utils/types"
)

func (c *Client) GetUserByPlatform(ctx context.Context, platform, platformUserID string) (*types.UserInfo, error) {
	if err := c.requireUsers(); err != nil {
		return nil, err
	}
	if platform == "" || platformUserID == "" {
		return nil, ErrInvalidUserID
	}

	row, err := c.users.User.
		Query().
		Where(
			user.PlatformEQ(platform),
			user.UserIDEQ(platformUserID),
		).
		First(ctx)
	if err != nil {
		if entusers.IsNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	resp := toUserInfo(row)
	return &resp, nil
}

func (c *Client) GetUserByID(ctx context.Context, harukiUserID int) (*types.UserInfo, error) {
	if err := c.requireUsers(); err != nil {
		return nil, err
	}
	if harukiUserID <= 0 {
		return nil, ErrInvalidUserID
	}

	row, err := c.users.User.Get(ctx, harukiUserID)
	if err != nil {
		if entusers.IsNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	resp := toUserInfo(row)
	return &resp, nil
}

func toUserInfo(u *entusers.User) types.UserInfo {
	return types.UserInfo{
		ID:                     u.ID,
		Platform:               u.Platform,
		UserID:                 u.UserID,
		BanState:               u.BanState,
		BanReason:              u.BanReason,
		PjskBanState:           u.PjskBanState,
		PjskBanReason:          u.PjskBanReason,
		ChunithmBanState:       u.ChunithmBanState,
		ChunithmBanReason:      u.ChunithmBanReason,
		PjskMainBanState:       u.PjskMainBanState,
		PjskMainBanReason:      u.PjskMainBanReason,
		PjskRankingBanState:    u.PjskRankingBanState,
		PjskRankingBanReason:   u.PjskRankingBanReason,
		PjskAliasBanState:      u.PjskAliasBanState,
		PjskAliasBanReason:     u.PjskAliasBanReason,
		PjskMysekaiBanState:    u.PjskMysekaiBanState,
		PjskMysekaiBanReason:   u.PjskMysekaiBanReason,
		ChunithmMainBanState:   u.ChunithmMainBanState,
		ChunithmMainBanReason:  u.ChunithmMainBanReason,
		ChunithmAliasBanState:  u.ChunithmAliasBanState,
		ChunithmAliasBanReason: u.ChunithmAliasBanReason,
	}
}
