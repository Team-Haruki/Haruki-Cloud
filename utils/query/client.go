package query

import (
	"errors"

	chunithmMainDB "haruki-cloud/database/chunithm/maindb"
	chunithmMusicDB "haruki-cloud/database/chunithm/music"
	pjskDB "haruki-cloud/database/pjsk"
	usersDB "haruki-cloud/database/users"
)

var (
	ErrInvalidAlias       = errors.New("invalid alias")
	ErrInvalidAliasType   = errors.New("invalid alias type")
	ErrInvalidMusicID     = errors.New("invalid music_id")
	ErrInvalidUserID      = errors.New("invalid user_id")
	ErrAliasNotFound      = errors.New("alias not found")
	ErrMusicNotFound      = errors.New("music not found")
	ErrBindingNotFound    = errors.New("binding not found")
	ErrPreferenceNotFound = errors.New("preference not found")
	ErrUserNotFound       = errors.New("user not found")

	ErrChunithmNotConfigured = errors.New("chunithm client is not configured")
	ErrPJSKNotConfigured     = errors.New("pjsk client is not configured")
	ErrUsersNotConfigured    = errors.New("users client is not configured")
)

type Client struct {
	chunithmMain  *chunithmMainDB.Client
	chunithmMusic *chunithmMusicDB.Client
	pjsk          *pjskDB.Client
	users         *usersDB.Client
}

func NewClient(
	chunithmMain *chunithmMainDB.Client,
	chunithmMusic *chunithmMusicDB.Client,
	pjsk *pjskDB.Client,
	users *usersDB.Client,
) *Client {
	return &Client{
		chunithmMain:  chunithmMain,
		chunithmMusic: chunithmMusic,
		pjsk:          pjsk,
		users:         users,
	}
}

func (c *Client) requireChunithm() error {
	if c.chunithmMain == nil || c.chunithmMusic == nil {
		return ErrChunithmNotConfigured
	}
	return nil
}

func (c *Client) requirePJSK() error {
	if c.pjsk == nil {
		return ErrPJSKNotConfigured
	}
	return nil
}

func (c *Client) requireUsers() error {
	if c.users == nil {
		return ErrUsersNotConfigured
	}
	return nil
}
