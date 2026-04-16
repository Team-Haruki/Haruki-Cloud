package userdata

import (
	"context"
	"fmt"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/utils/logger"
)

var bindingDebugLogger = logger.NewLoggerFromGlobal("PJSKBinding")

func resolveSnapshotBinding(
	ctx context.Context,
	bindings bindingLookup,
	platform, imUserID string,
	region renderregion.Value,
	pjskUserID string,
	opts ResolveOptions,
) (*accountdata.ResolvedBinding, error) {
	if bindings == nil {
		return nil, ErrProviderUnavailable
	}
	bindingDebugLogger.Debugf("snapshot binding resolve start: platform=%s user=%s region=%s pjsk_user=%s prefer_global_default=%t need_mysekai=%t",
		strings.TrimSpace(platform), maskBindingDebugID(imUserID), region.String(), maskBindingDebugID(pjskUserID), opts.PreferGlobalDefault, opts.NeedMySekai)
	if strings.TrimSpace(pjskUserID) != "" {
		return resolveExplicitSnapshotBinding(ctx, bindings, platform, imUserID, region, pjskUserID, opts)
	}

	regionStr := region.String()
	if opts.PreferGlobalDefault {
		_, binding, err := bindings.ResolveUserBinding(ctx, platform, imUserID, accountdata.GlobalDefaultBindingScope)
		bindingDebugLogger.Debugf("snapshot binding global-default result: platform=%s user=%s binding=%s err=%v",
			strings.TrimSpace(platform), maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), err)
		if err == nil && bindingAllowed(binding, opts) {
			bindingDebugLogger.Debugf("snapshot binding selected global-default: platform=%s user=%s binding=%s",
				strings.TrimSpace(platform), maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding))
			return binding, nil
		}
	}

	_, binding, err := bindings.ResolveUserBinding(ctx, platform, imUserID, regionStr)
	bindingDebugLogger.Debugf("snapshot binding region result: platform=%s user=%s region=%s binding=%s err=%v",
		strings.TrimSpace(platform), maskBindingDebugID(imUserID), strings.TrimSpace(regionStr), formatSnapshotBindingDebug(binding), err)
	if err != nil {
		return nil, err
	}
	if !bindingAllowed(binding, opts) {
		if opts.NeedMySekai {
			return nil, fmt.Errorf("userdata: binding does not expose mysekai snapshot")
		}
		return nil, fmt.Errorf("userdata: binding does not expose suite snapshot")
	}
	bindingDebugLogger.Debugf("snapshot binding selected region binding: platform=%s user=%s region=%s binding=%s",
		strings.TrimSpace(platform), maskBindingDebugID(imUserID), strings.TrimSpace(regionStr), formatSnapshotBindingDebug(binding))
	return binding, nil
}

func resolveExplicitSnapshotBinding(
	ctx context.Context,
	bindings bindingLookup,
	platform, imUserID string,
	region renderregion.Value,
	pjskUserID string,
	opts ResolveOptions,
) (*accountdata.ResolvedBinding, error) {
	items, err := bindings.List(ctx, platform, imUserID)
	if err != nil {
		return nil, err
	}

	var match *accountdata.BindingListItem
	for i := range items {
		item := items[i]
		if strings.TrimSpace(item.UserID) != strings.TrimSpace(pjskUserID) {
			continue
		}
		if !region.IsZero() && !strings.EqualFold(item.Server, region.String()) {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("userdata: multiple bindings match pjsk user id %s; region is required", pjskUserID)
		}
		copy := item
		match = &copy
	}
	if match == nil {
		return nil, fmt.Errorf("userdata: binding %s not found", pjskUserID)
	}

	binding := &accountdata.ResolvedBinding{
		BindingID:      match.BindingID,
		PJSKUserID:     match.UserID,
		Server:         match.Server,
		Visible:        match.Visible,
		SuiteVisible:   match.SuiteVisible,
		MySekaiVisible: match.MySekaiVisible,
		Verified:       match.Verified,
		Bg:             match.Bg,
	}
	if !bindingAllowed(binding, opts) {
		if opts.NeedMySekai {
			return nil, fmt.Errorf("userdata: binding %s does not expose mysekai snapshot", pjskUserID)
		}
		return nil, fmt.Errorf("userdata: binding %s does not expose suite snapshot", pjskUserID)
	}
	bindingDebugLogger.Debugf("snapshot binding selected explicit binding: platform=%s user=%s region=%s binding=%s",
		strings.TrimSpace(platform), maskBindingDebugID(imUserID), region.String(), formatSnapshotBindingDebug(binding))
	return binding, nil
}

func bindingAllowed(binding *accountdata.ResolvedBinding, opts ResolveOptions) bool {
	if binding == nil || !binding.SuiteVisible {
		return false
	}
	if opts.NeedMySekai && !binding.MySekaiVisible {
		return false
	}
	return true
}

func formatSnapshotBindingDebug(binding *accountdata.ResolvedBinding) string {
	if binding == nil {
		return "<nil>"
	}
	return fmt.Sprintf("{binding_id=%d server=%s pjsk_user=%s visible=%t suite_visible=%t mysekai_visible=%t verified=%t}",
		binding.BindingID, strings.TrimSpace(binding.Server), maskBindingDebugID(binding.PJSKUserID), binding.Visible, binding.SuiteVisible, binding.MySekaiVisible, binding.Verified)
}

func maskBindingDebugID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 6 {
		return value
	}
	return value[:3] + "***" + value[len(value)-3:]
}
