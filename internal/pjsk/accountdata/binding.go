// Package userdata provides PJSK-specific data access helpers, including
// resolving a user's active game account binding from their haruki_user_id.
package accountdata

import (
	"context"
	"errors"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/userbinding"
	"haruki-cloud/database/pjsk/userdefaultbinding"
	"haruki-cloud/internal/pjsk/drawing"
)

// ErrNoBinding is returned when no PJSK game account is bound for the
// given (haruki_user_id, server) pair.
var ErrNoBinding = errors.New("pjsk: no binding found for user on this server")

// ResolvedBinding holds the result of a successful binding lookup.
type ResolvedBinding struct {
	BindingID      int
	PJSKUserID     string
	Server         string
	Visible        bool
	SuiteVisible   bool
	MySekaiVisible bool
	Verified       bool
	Bg             *drawing.ProfileBgSettings
}

// BindingResolver resolves a (haruki_user_id, server) pair to the user's
// active PJSK game account.
type BindingResolver struct {
	db *pjskdb.Client
}

// NewBindingResolver creates a new BindingResolver.
func NewBindingResolver(db *pjskdb.Client) *BindingResolver {
	return &BindingResolver{db: db}
}

// Resolve returns the active PJSK binding for (harukiUserID, server).
//
// Resolution order:
//  1. user_default_bindings — the user's explicitly chosen default for this server.
//  2. Fallback: first visible binding in user_bindings for this server
//     (for users who bound an account but never set a default).
//
// Returns ErrNoBinding if the user has no binding on the requested server.
func (r *BindingResolver) Resolve(ctx context.Context, harukiUserID int, server string) (*ResolvedBinding, error) {
	// 1. Try the user's default binding.
	defaultBind, err := r.db.UserDefaultBinding.Query().
		Where(
			userdefaultbinding.HarukiUserID(harukiUserID),
			userdefaultbinding.Server(server),
		).
		WithBinding().
		Only(ctx)
	if err == nil {
		b := defaultBind.Edges.Binding
		if b == nil {
			return nil, ErrNoBinding
		}
		return &ResolvedBinding{
			BindingID:      b.ID,
			PJSKUserID:     b.UserID,
			Server:         b.Server,
			Visible:        b.Visible,
			SuiteVisible:   b.SuiteVisible,
			MySekaiVisible: b.MysekaiVisible,
			Verified:       b.Verified,
			Bg:             cloneProfileBGSettings(b.Bg),
		}, nil
	}
	if !pjskdb.IsNotFound(err) {
		return nil, err
	}

	// 2. Fallback: first visible binding for this server.
	b, err := r.db.UserBinding.Query().
		Where(
			userbinding.HarukiUserID(harukiUserID),
			userbinding.Server(server),
			userbinding.Visible(true),
		).
		First(ctx)
	if err != nil {
		if pjskdb.IsNotFound(err) {
			return nil, ErrNoBinding
		}
		return nil, err
	}

	return &ResolvedBinding{
		BindingID:      b.ID,
		PJSKUserID:     b.UserID,
		Server:         b.Server,
		Visible:        b.Visible,
		SuiteVisible:   b.SuiteVisible,
		MySekaiVisible: b.MysekaiVisible,
		Verified:       b.Verified,
		Bg:             cloneProfileBGSettings(b.Bg),
	}, nil
}

func cloneProfileBGSettings(bg *drawing.ProfileBgSettings) *drawing.ProfileBgSettings {
	if bg == nil {
		return nil
	}
	cloned := *bg
	if bg.ImgPath != nil {
		path := *bg.ImgPath
		cloned.ImgPath = &path
	}
	return &cloned
}
