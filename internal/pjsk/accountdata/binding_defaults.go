package accountdata

import (
	"context"
	"fmt"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/userbinding"
	"haruki-cloud/database/pjsk/userdefaultbinding"
	renderregion "haruki-cloud/internal/pjsk/region"
)

// SetDefault sets the default binding for the given scope (global or server-specific).
func (s *BindingService) SetDefault(ctx context.Context, platform, platformUserID, selector, selectorServer, serverScope string) (*DefaultBindingResult, error) {
	return s.updateDefault(ctx, platform, platformUserID, selector, selectorServer, serverScope, false)
}

// ClearDefault clears the default binding. If selector is empty, clears by scope alone.
func (s *BindingService) ClearDefault(ctx context.Context, platform, platformUserID, selector, selectorServer, serverScope string) (*DefaultBindingResult, error) {
	if selector == "" {
		return s.clearDefaultByScope(ctx, platform, platformUserID, serverScope)
	}
	return s.updateDefault(ctx, platform, platformUserID, selector, selectorServer, serverScope, true)
}

// clearDefaultByScope clears the default binding for a scope without requiring
// a specific binding selector. Used when user calls /清除默认绑定 with no arguments.
func (s *BindingService) clearDefaultByScope(ctx context.Context, platform, platformUserID, serverScope string) (*DefaultBindingResult, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return nil, err
	}
	harukiUserID, err := s.identity.ResolveOrCreate(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	scope, scopeLabel, err := normalizeDefaultScope(serverScope)
	if err != nil {
		return nil, err
	}
	existing, err := s.pjskDB.UserDefaultBinding.Query().
		Where(
			userdefaultbinding.HarukiUserID(harukiUserID),
			userdefaultbinding.Server(scope),
		).
		WithBinding(func(q *pjskdb.UserBindingQuery) {
			q.WithGameAccount()
		}).
		Only(ctx)
	if err != nil {
		if pjskdb.IsNotFound(err) {
			return nil, fmt.Errorf("你当前没有设置%s默认绑定", scopeLabel)
		}
		return nil, err
	}
	if err := s.pjskDB.UserDefaultBinding.DeleteOneID(existing.ID).Exec(ctx); err != nil {
		return nil, err
	}
	// Build a BindingListItem for the cleared binding from the current list.
	items, _ := s.List(ctx, platform, platformUserID)
	var target BindingListItem
	for _, item := range items {
		if item.BindingID == existing.BindingID {
			target = item
			break
		}
	}
	return &DefaultBindingResult{
		Scope:   defaultScopeType(scope),
		Server:  scope,
		Binding: target,
	}, nil
}

// updateDefault handles both setting and clearing default bindings.
func (s *BindingService) updateDefault(ctx context.Context, platform, platformUserID, selector, selectorServer, serverScope string, clear bool) (*DefaultBindingResult, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return nil, err
	}
	harukiUserID, err := s.identity.ResolveOrCreate(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}

	bindings, err := s.pjskDB.UserBinding.Query().
		Where(userbinding.HarukiUserID(harukiUserID)).
		WithGameAccount().
		All(ctx)
	if err != nil {
		return nil, err
	}
	defaults, err := s.pjskDB.UserDefaultBinding.Query().
		Where(userdefaultbinding.HarukiUserID(harukiUserID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := buildBindingList(bindings, defaults)
	target, err := selectBinding(items, selector, selectorServer)
	if err != nil {
		return nil, err
	}

	scope, scopeLabel, err := normalizeDefaultScope(serverScope)
	if err != nil {
		return nil, err
	}
	if scope != GlobalDefaultBindingScope && target.Server != scope {
		return nil, fmt.Errorf("所选账号不属于%s服", strings.ToUpper(scope))
	}

	existing, err := s.pjskDB.UserDefaultBinding.Query().
		Where(
			userdefaultbinding.HarukiUserID(harukiUserID),
			userdefaultbinding.Server(scope),
		).
		Only(ctx)
	if err != nil && !pjskdb.IsNotFound(err) {
		return nil, err
	}

	if clear {
		if pjskdb.IsNotFound(err) {
			return nil, fmt.Errorf("你当前没有设置%s默认绑定", scopeLabel)
		}
		if existing.BindingID != target.BindingID {
			return nil, fmt.Errorf("所选账号不是你当前的%s默认绑定", scopeLabel)
		}
		if err := s.pjskDB.UserDefaultBinding.DeleteOneID(existing.ID).Exec(ctx); err != nil {
			return nil, err
		}
		return &DefaultBindingResult{
			Scope:   defaultScopeType(scope),
			Server:  scope,
			Binding: target,
		}, nil
	}

	tx, err := s.pjskDB.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err := upsertDefaultBindingTx(ctx, tx, harukiUserID, scope, target.BindingID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil

	if scope == GlobalDefaultBindingScope {
		target.IsGlobalDefault = true
	} else {
		target.IsServerDefault = true
	}
	return &DefaultBindingResult{
		Scope:   defaultScopeType(scope),
		Server:  scope,
		Binding: target,
	}, nil
}

// ensureDefaultBindingTx creates a default binding only if one doesn't already exist for the scope.
// Returns true if a new default was created, false if one already existed.
func ensureDefaultBindingTx(ctx context.Context, tx *pjskdb.Tx, harukiUserID int, scope string, bindingID int) (bool, error) {
	_, err := tx.UserDefaultBinding.Query().
		Where(
			userdefaultbinding.HarukiUserID(harukiUserID),
			userdefaultbinding.Server(scope),
		).
		Only(ctx)
	if err == nil {
		return false, nil
	}
	if !pjskdb.IsNotFound(err) {
		return false, err
	}
	if _, err := tx.UserDefaultBinding.Create().
		SetHarukiUserID(harukiUserID).
		SetServer(scope).
		SetBindingID(bindingID).
		Save(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// upsertDefaultBindingTx creates or updates a default binding for the given scope.
func upsertDefaultBindingTx(ctx context.Context, tx *pjskdb.Tx, harukiUserID int, scope string, bindingID int) (*pjskdb.UserDefaultBinding, error) {
	existing, err := tx.UserDefaultBinding.Query().
		Where(
			userdefaultbinding.HarukiUserID(harukiUserID),
			userdefaultbinding.Server(scope),
		).
		Only(ctx)
	if err == nil {
		return tx.UserDefaultBinding.UpdateOneID(existing.ID).
			SetBindingID(bindingID).
			Save(ctx)
	}
	if !pjskdb.IsNotFound(err) {
		return nil, err
	}
	return tx.UserDefaultBinding.Create().
		SetHarukiUserID(harukiUserID).
		SetServer(scope).
		SetBindingID(bindingID).
		Save(ctx)
}

// hasDefaultScope checks if a default binding exists for the given scope.
func hasDefaultScope(items []*pjskdb.UserDefaultBinding, scope string) bool {
	for _, item := range items {
		if item.Server == scope {
			return true
		}
	}
	return false
}

// normalizeDefaultScope normalizes a scope string into a canonical form.
// Returns the normalized scope, a human-readable label, and any error.
func normalizeDefaultScope(scope string) (string, string, error) {
	scope = strings.TrimSpace(strings.ToLower(scope))
	if scope == "" || scope == GlobalDefaultBindingScope {
		return GlobalDefaultBindingScope, "全局", nil
	}
	normalized := renderregion.Normalize(scope)
	if normalized.IsZero() {
		return "", "", fmt.Errorf("不支持的区服: %s", scope)
	}
	return normalized.String(), strings.ToUpper(normalized.String()), nil
}

// defaultScopeType returns the DefaultScope type for a given scope string.
func defaultScopeType(scope string) DefaultScope {
	if scope == GlobalDefaultBindingScope {
		return DefaultScopeGlobal
	}
	return DefaultScopeServer
}
