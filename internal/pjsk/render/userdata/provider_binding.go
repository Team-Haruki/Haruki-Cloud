package userdata

import (
	"context"
	"fmt"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/render/region"
	accountdata "haruki-cloud/internal/pjsk/userdata"
)

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
	if strings.TrimSpace(pjskUserID) != "" {
		return resolveExplicitSnapshotBinding(ctx, bindings, platform, imUserID, region, pjskUserID, opts)
	}

	regionStr := region.String()
	if opts.PreferGlobalDefault {
		_, binding, err := bindings.ResolveUserBinding(ctx, platform, imUserID, accountdata.GlobalDefaultBindingScope)
		if err == nil && bindingAllowed(binding, opts) {
			return binding, nil
		}
	}

	_, binding, err := bindings.ResolveUserBinding(ctx, platform, imUserID, regionStr)
	if err != nil {
		return nil, err
	}
	if !bindingAllowed(binding, opts) {
		if opts.NeedMySekai {
			return nil, fmt.Errorf("userdata: binding does not expose mysekai snapshot")
		}
		return nil, fmt.Errorf("userdata: binding does not expose suite snapshot")
	}
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
