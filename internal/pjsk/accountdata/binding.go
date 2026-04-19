// Package accountdata provides PJSK-specific data access helpers, including
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

// bindingResolver resolves a (haruki_user_id, server) pair to the user's
// active PJSK game account.
type bindingResolver struct {
	db *pjskdb.Client
}

// newBindingResolver creates a new bindingResolver.
func newBindingResolver(db *pjskdb.Client) *bindingResolver {
	return &bindingResolver{db: db}
}

// Resolve returns the active PJSK binding for (harukiUserID, server).
//
// Resolution order:
//  1. user_default_bindings — the user's explicitly chosen default for this server.
//  2. Fallback: first visible binding in user_bindings for this server
//     (for users who bound an account but never set a default).
//
// Returns ErrNoBinding if the user has no binding on the requested server.
func (r *bindingResolver) Resolve(ctx context.Context, harukiUserID int, server string) (*ResolvedBinding, error) {
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
		bg, err := loadProfileBackground(ctx, r.db, b.Server, b.UserID)
		if err != nil {
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
			Bg:             cloneProfileBGSettings(bg),
		}, nil
	}
	if !pjskdb.IsNotFound(err) {
		return nil, err
	}

	// 2. Fallback: first visible binding for this server.
	bindings, err := r.db.UserBinding.Query().
		Where(
			userbinding.HarukiUserID(harukiUserID),
			userbinding.Server(server),
			userbinding.Visible(true),
		).
		All(ctx)
	if err != nil {
		if pjskdb.IsNotFound(err) {
			return nil, ErrNoBinding
		}
		return nil, err
	}
	if len(bindings) == 0 {
		return nil, ErrNoBinding
	}
	items := buildBindingList(bindings, nil)
	targetBindingID := items[0].BindingID
	var b *pjskdb.UserBinding
	for _, binding := range bindings {
		if binding.ID == targetBindingID {
			b = binding
			break
		}
	}
	if b == nil {
		return nil, ErrNoBinding
	}
	bg, err := loadProfileBackground(ctx, r.db, b.Server, b.UserID)
	if err != nil {
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
		Bg:             cloneProfileBGSettings(bg),
	}, nil
}

func cloneProfileBGSettings(bg *drawing.ProfileBgSettings) *drawing.ProfileBgSettings {
	if bg == nil {
		return nil
	}
	cloned := *bg
	if bg.ImgPath != nil {
		cloned.ImgPath = new(*bg.ImgPath)
	}
	return &cloned
}
