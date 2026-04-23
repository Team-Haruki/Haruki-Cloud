package accountdata

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/userbinding"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func buildBindingList(bindings []*pjskdb.UserBinding, defaults []*pjskdb.UserDefaultBinding) []BindingListItem {
	return buildBindingListWithBackgrounds(bindings, defaults)
}

func buildBindingListWithBackgrounds(bindings []*pjskdb.UserBinding, defaults []*pjskdb.UserDefaultBinding) []BindingListItem {
	if len(bindings) == 0 {
		return nil
	}

	globalDefaultID := 0
	serverDefaultByServer := make(map[string]int, len(defaults))
	for _, item := range defaults {
		if item.Server == GlobalDefaultBindingScope {
			globalDefaultID = item.BindingID
			continue
		}
		serverDefaultByServer[item.Server] = item.BindingID
	}

	list := make([]BindingListItem, 0, len(bindings))
	for _, binding := range bindings {
		server := bindingServer(binding)
		userID := bindingUserID(binding)
		list = append(list, BindingListItem{
			BindingID:       binding.ID,
			DisplayOrder:    effectiveBindingDisplayOrder(binding),
			Server:          server,
			UserID:          userID,
			Visible:         binding.Visible,
			SuiteVisible:    binding.SuiteVisible,
			MySekaiVisible:  binding.MysekaiVisible,
			Verified:        binding.Verified,
			Bg:              resolveBindingProfileBG(binding),
			IsGlobalDefault: binding.ID == globalDefaultID,
			IsServerDefault: binding.ID == serverDefaultByServer[server],
		})
	}

	slices.SortFunc(list, func(a, b BindingListItem) int {
		if cmp := effectiveBindingListDisplayOrder(a) - effectiveBindingListDisplayOrder(b); cmp != 0 {
			return cmp
		}
		if a.Server < b.Server {
			return -1
		}
		if a.Server > b.Server {
			return 1
		}
		return a.BindingID - b.BindingID
	})

	serverIndex := make(map[string]int, len(list))
	for i := range list {
		serverIndex[list[i].Server]++
		list[i].Index = serverIndex[list[i].Server]
	}
	return list
}

func selectBinding(items []BindingListItem, selector, server string) (BindingListItem, error) {
	if len(items) == 0 {
		return BindingListItem{}, fmt.Errorf("你还没有绑定任何PJSK账号")
	}
	selector = normalizeUID(selector)
	if selector == "" {
		return BindingListItem{}, fmt.Errorf("请提供账号ID或u序号")
	}

	lower := strings.ToLower(selector)
	if strings.HasPrefix(lower, "u") {
		index, err := strconv.Atoi(strings.TrimSpace(lower[1:]))
		if err != nil || index <= 0 {
			return BindingListItem{}, fmt.Errorf("请提供正确的u序号，例如 u1")
		}

		scopedItems := items
		if normalizedServer := normalizeSelectorServer(server); normalizedServer != "" {
			scopedItems = filterBindingsByServer(items, normalizedServer)
			if len(scopedItems) == 0 {
				return BindingListItem{}, fmt.Errorf("你还没有绑定任何%s服账号", strings.ToUpper(normalizedServer))
			}
			if index > len(scopedItems) {
				return BindingListItem{}, fmt.Errorf("指定的%s服账号序号超出范围，目前仅绑定了%d个账号", strings.ToUpper(normalizedServer), len(scopedItems))
			}
			return scopedItems[index-1], nil
		}

		if index > len(scopedItems) {
			return BindingListItem{}, fmt.Errorf("指定的账号序号超出范围，目前仅绑定了%d个账号", len(scopedItems))
		}
		return scopedItems[index-1], nil
	}

	var matched []BindingListItem
	for _, item := range items {
		if item.UserID == selector {
			matched = append(matched, item)
		}
	}
	switch len(matched) {
	case 0:
		return BindingListItem{}, fmt.Errorf("未找到绑定的账号ID %s", selector)
	case 1:
		return matched[0], nil
	default:
		return BindingListItem{}, fmt.Errorf("账号ID %s 在多个区服都已绑定，请改用 u序号 操作", selector)
	}
}

func filterBindingsByServer(items []BindingListItem, server string) []BindingListItem {
	if server == "" {
		return slices.Clone(items)
	}

	filtered := make([]BindingListItem, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Server), server) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func normalizeSelectorServer(server string) string {
	server = strings.TrimSpace(strings.ToLower(server))
	if server == "" || server == GlobalDefaultBindingScope {
		return ""
	}
	normalized := renderregion.Normalize(server)
	if normalized.IsZero() {
		return ""
	}
	return normalized.String()
}

func normalizeUID(value string) string {
	return strings.TrimSpace(value)
}

func isNumericUID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (s *BindingService) currentBindingEntity(ctx context.Context, platform, platformUserID, server string) (*pjskdb.UserBinding, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return nil, err
	}
	_, resolved, err := s.ResolveUserBinding(ctx, platform, platformUserID, server)
	if err != nil {
		return nil, err
	}
	return s.pjskDB.UserBinding.Query().
		Where(userbinding.ID(resolved.BindingID)).
		WithGameAccount().
		Only(ctx)
}

func (s *BindingService) bindingListItemByID(ctx context.Context, platform, platformUserID string, bindingID int) (*BindingListItem, error) {
	items, err := s.List(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.BindingID == bindingID {
			return new(item), nil
		}
	}
	return nil, fmt.Errorf("未找到绑定记录 %d", bindingID)
}

// ResolveUserBinding resolves a platform user's active PJSK binding for the given
// server by combining identity resolution and binding resolution.
// Returns the haruki user ID and the resolved binding.
func (s *BindingService) ResolveUserBinding(ctx context.Context, platform, platformUserID, server string) (int, *ResolvedBinding, error) {
	harukiUserID, err := s.identity.ResolveOrCreate(ctx, platform, platformUserID)
	if err != nil {
		return 0, nil, err
	}
	resolver := newBindingResolver(s.pjskDB)
	binding, err := resolver.Resolve(ctx, harukiUserID, server)
	if err != nil {
		return harukiUserID, nil, err
	}
	return harukiUserID, binding, nil
}

// ResolveUserBindingBySelector resolves a binding using a u[i] selector (e.g. "u1", "u2")
// or a raw game UID. For u[i], the selector is scoped to the given server when provided.
func (s *BindingService) ResolveUserBindingBySelector(ctx context.Context, platform, platformUserID, server, selector string) (int, *ResolvedBinding, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return 0, nil, err
	}
	harukiUserID, err := s.identity.ResolveOrCreate(ctx, platform, platformUserID)
	if err != nil {
		return 0, nil, err
	}
	items, err := s.List(ctx, platform, platformUserID)
	if err != nil {
		return harukiUserID, nil, err
	}
	item, err := selectBinding(items, selector, server)
	if err != nil {
		return harukiUserID, nil, err
	}
	return harukiUserID, &ResolvedBinding{
		BindingID:      item.BindingID,
		PJSKUserID:     item.UserID,
		Server:         item.Server,
		Visible:        item.Visible,
		SuiteVisible:   item.SuiteVisible,
		MySekaiVisible: item.MySekaiVisible,
		Verified:       item.Verified,
		Bg:             item.Bg,
	}, nil
}

// currentBindingEntityBySelector resolves a binding entity by u[i] selector.
func (s *BindingService) currentBindingEntityBySelector(ctx context.Context, platform, platformUserID, server, selector string) (*pjskdb.UserBinding, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return nil, err
	}
	_, resolved, err := s.ResolveUserBindingBySelector(ctx, platform, platformUserID, server, selector)
	if err != nil {
		return nil, err
	}
	return s.pjskDB.UserBinding.Query().
		Where(userbinding.ID(resolved.BindingID)).
		WithGameAccount().
		Only(ctx)
}
