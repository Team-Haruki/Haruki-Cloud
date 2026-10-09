package accountdata

import (
	"context"
	"fmt"
	"haruki-cloud/internal/i18n"
	json "haruki-cloud/internal/jsonutil"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
)

const (
	ProfileModeRender       = "profile"
	ProfileModeBind         = "profile-bind"
	ProfileModeBindList     = "profile-bind-list"
	ProfileModeBindSwap     = "profile-bind-swap"
	ProfileModeUnbind       = "profile-unbind"
	ProfileModeDefaultSet   = "profile-default-set"
	ProfileModeDefaultClear = "profile-default-clear"
	ProfileModeQueryUID     = "profile-query-uid"
)

type ProfileBindingCommandParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	Selector       string `json:"selector,omitempty"`
	SelectorOther  string `json:"selector_other,omitempty"`
	Server         string `json:"server,omitempty"`
	Scope          string `json:"scope,omitempty"`
}

func DecodeProfileBindingParams(raw json.RawMessage) (ProfileBindingCommandParams, error) {
	var params ProfileBindingCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing profile binding params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal profile binding params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	params.Selector = strings.TrimSpace(params.Selector)
	params.SelectorOther = strings.TrimSpace(params.SelectorOther)
	params.Server = strings.TrimSpace(strings.ToLower(params.Server))
	params.Scope = strings.TrimSpace(params.Scope)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing binding identity context")
	}
	if params.Server != "" {
		normalized := renderregion.Normalize(params.Server)
		if normalized.IsZero() {
			return params, fmt.Errorf("bridge: invalid binding server %q", params.Server)
		}
		params.Server = normalized.String()
	}
	return params, nil
}

func ExecuteProfileBindingCommand(ctx context.Context, service *BindingService, mode string, params ProfileBindingCommandParams) ([]byte, error) {
	if service == nil || !service.IsReady() {
		return nil, ErrBindingServiceUnavailable
	}

	switch mode {
	case ProfileModeBind:
		result, err := service.Bind(ctx, params.Platform, params.PlatformUserID, params.Selector)
		if err != nil {
			return nil, err
		}
		return []byte(formatBindResultText(result)), nil
	case ProfileModeBindList:
		items, err := service.List(ctx, params.Platform, params.PlatformUserID)
		if err != nil {
			return nil, err
		}
		if params.Server != "" {
			items = filterBindingsByServer(items, params.Server)
		}
		return []byte(formatBindingListText(items, params.Server)), nil
	case ProfileModeBindSwap:
		items, err := service.Swap(ctx, params.Platform, params.PlatformUserID, params.Selector, params.SelectorOther, params.Server)
		if err != nil {
			return nil, err
		}
		if params.Server != "" {
			items = filterBindingsByServer(items, params.Server)
		}
		return []byte(formatBindingSwapResultText(params.Selector, params.SelectorOther, params.Server, items)), nil
	case ProfileModeUnbind:
		result, err := service.Unbind(ctx, params.Platform, params.PlatformUserID, params.Selector, params.Server)
		if err != nil {
			return nil, err
		}
		return []byte(formatUnbindResultText(result)), nil
	case ProfileModeDefaultSet:
		result, err := service.SetDefault(ctx, params.Platform, params.PlatformUserID, params.Selector, params.Server, params.Scope)
		if err != nil {
			return nil, err
		}
		return []byte(formatDefaultBindingSetText(result)), nil
	case ProfileModeDefaultClear:
		result, err := service.ClearDefault(ctx, params.Platform, params.PlatformUserID, params.Selector, params.Server, params.Scope)
		if err != nil {
			return nil, err
		}
		return []byte(formatDefaultBindingClearedText(result)), nil
	case ProfileModeQueryUID:
		result, err := service.ResolveOwnBindingForUIDQuery(ctx, params.Platform, params.PlatformUserID, params.Selector, params.Server)
		if err != nil {
			return nil, err
		}
		return []byte(result.UserID), nil
	default:
		return nil, fmt.Errorf("bridge: unsupported profile binding mode %q", mode)
	}
}

func formatBindingListText(items []BindingListItem, server string) string {
	if len(items) == 0 {
		if server != "" {
			return i18n.T("binding.none_in_region", i18n.Data{"Region": i18n.RegionLabel(server)})
		}
		return i18n.T("binding.none")
	}

	lines := make([]string, 0, len(items)+1)
	if server != "" {
		lines = append(lines, i18n.T("account.list.header_region", i18n.Data{"Region": i18n.RegionLabel(server)}))
	} else {
		lines = append(lines, i18n.T("account.list.header"))
	}
	for i, item := range items {
		displayIdx := i + 1
		if server != "" {
			displayIdx = item.Index
		}
		if marks := bindingDefaultMarks(item); marks != "" {
			lines = append(lines, i18n.T("account.list.item_marked", i18n.Data{"Index": displayIdx, "Account": bindingAccountLabel(item), "Marks": marks}))
			continue
		}
		lines = append(lines, i18n.T("account.list.item", i18n.Data{"Index": displayIdx, "Account": bindingAccountLabel(item)}))
	}
	return strings.Join(lines, "\n")
}

// bindingDefaultMarks lists the default scopes an account holds, for the
// binding and verification lists; empty when it is no default.
func bindingDefaultMarks(item BindingListItem) string {
	marks := make([]string, 0, 2)
	if item.IsGlobalDefault {
		marks = append(marks, i18n.T("account.mark.global_default"))
	}
	if item.IsServerDefault {
		marks = append(marks, i18n.T("account.mark.region_default", i18n.Data{"Region": i18n.RegionLabel(item.Server)}))
	}
	return strings.Join(marks, "、")
}

func formatBindResultText(result *BindResult) string {
	if result == nil {
		return i18n.T("account.bind.done_plain")
	}

	lines := []string{i18n.T("account.bind.done", i18n.Data{
		"Account": i18n.AccountLabel(result.Server, result.UserID, false),
		"Name":    result.UserName,
	})}
	if result.AlreadyBound {
		lines = append(lines, i18n.T("account.bind.note_already_bound"))
	}
	if result.SetGlobalDefault {
		lines = append(lines, i18n.T("account.bind.note_global_default"))
	}
	if result.SetServerDefault {
		lines = append(lines, i18n.T("account.bind.note_region_default", i18n.Data{"Region": i18n.RegionLabel(result.Server)}))
	}
	if result.MultipleServerMatch {
		lines = append(lines, i18n.T("account.bind.note_multiple_regions"))
	}
	return strings.Join(lines, "\n")
}

func formatUnbindResultText(result *UnbindResult) string {
	if result == nil {
		return i18n.T("account.unbind.done_plain")
	}

	lines := []string{i18n.T("account.unbind.done", i18n.Data{"Account": bindingAccountLabel(result.Removed)})}
	if result.ReassignedGlobal != nil {
		lines = append(lines, i18n.T("account.unbind.reassigned_global", i18n.Data{"Account": bindingAccountLabel(*result.ReassignedGlobal)}))
	}
	if result.ReassignedServer != nil {
		lines = append(lines, i18n.T("account.unbind.reassigned_region", i18n.Data{
			"Region":  i18n.RegionLabel(result.ReassignedServer.Server),
			"Account": bindingAccountLabel(*result.ReassignedServer),
		}))
	}
	return strings.Join(lines, "\n")
}

func formatDefaultBindingSetText(result *DefaultBindingResult) string {
	switch {
	case result == nil:
		return i18n.T("account.default.set_plain")
	case result.Scope == DefaultScopeServer:
		return i18n.T("account.default.set_region", i18n.Data{"Region": i18n.RegionLabel(result.Server), "Account": bindingAccountLabel(result.Binding)})
	default:
		return i18n.T("account.default.set_global", i18n.Data{"Account": bindingAccountLabel(result.Binding)})
	}
}

func formatDefaultBindingClearedText(result *DefaultBindingResult) string {
	switch {
	case result == nil:
		return i18n.T("account.default.cleared_plain")
	case result.Scope == DefaultScopeServer:
		return i18n.T("account.default.cleared_region", i18n.Data{"Region": i18n.RegionLabel(result.Server), "Account": bindingAccountLabel(result.Binding)})
	default:
		return i18n.T("account.default.cleared_global", i18n.Data{"Account": bindingAccountLabel(result.Binding)})
	}
}

func formatBindingSwapResultText(left, right, server string, items []BindingListItem) string {
	list := formatBindingListText(items, server)
	if server != "" {
		return i18n.T("account.swap.done_region", i18n.Data{
			"Region": i18n.RegionLabel(server),
			"Left":   strings.TrimSpace(left),
			"Right":  strings.TrimSpace(right),
			"List":   list,
		})
	}
	return i18n.T("account.swap.done", i18n.Data{"Left": strings.TrimSpace(left), "Right": strings.TrimSpace(right), "List": list})
}

// bindingAccountLabel is the "[日服(JP)] 123***789" label of a bound
// account, with the UID masked unless the user made it visible.
func bindingAccountLabel(item BindingListItem) i18n.Message {
	return i18n.AccountLabel(item.Server, item.UserID, item.Visibility.UID)
}
