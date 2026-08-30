package snapshot

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
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
	bindingDebugLogger.DebugContext(ctx, "snapshot binding resolution started",
		"platform", strings.TrimSpace(platform),
		"region", region.String(),
		"explicit_pjsk_user", strings.TrimSpace(pjskUserID) != "",
		"prefer_global_default", opts.PreferGlobalDefault,
		"need_mysekai", opts.NeedMySekai,
	)
	if strings.TrimSpace(pjskUserID) != "" {
		return resolveExplicitSnapshotBinding(ctx, bindings, platform, imUserID, region, pjskUserID, opts)
	}

	regionStr := region.String()
	if opts.PreferGlobalDefault {
		_, binding, err := bindings.ResolveUserBinding(ctx, platform, imUserID, accountdata.GlobalDefaultBindingScope)
		bindingDebugLogger.DebugContext(ctx, "snapshot binding candidate resolved",
			"scope", "global_default",
			"outcome", bindingResolutionOutcome(binding, err, opts),
			"error_type", snapshotBindingErrorType(err),
		)
		if err == nil && bindingAllowed(binding, opts) {
			bindingDebugLogger.DebugContext(ctx, snapshotBindingSelectedLog, "scope", "global_default")
			return binding, nil
		}
	}

	_, binding, err := bindings.ResolveUserBinding(ctx, platform, imUserID, regionStr)
	bindingDebugLogger.DebugContext(ctx, "snapshot binding candidate resolved",
		"scope", "region",
		"region", strings.TrimSpace(regionStr),
		"outcome", bindingResolutionOutcome(binding, err, opts),
		"error_type", snapshotBindingErrorType(err),
	)
	if err != nil {
		return nil, err
	}
	if !bindingAllowed(binding, opts) {
		if opts.NeedMySekai {
			return nil, fmt.Errorf("snapshot: binding does not expose mysekai snapshot")
		}
		return nil, fmt.Errorf("snapshot: binding does not expose suite snapshot")
	}
	bindingDebugLogger.DebugContext(ctx, snapshotBindingSelectedLog, "scope", "region", "region", strings.TrimSpace(regionStr))
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
			return nil, fmt.Errorf("snapshot: multiple bindings match pjsk user id %s; region is required", pjskUserID)
		}
		match = new(item)
	}
	if match == nil {
		return nil, fmt.Errorf("snapshot: binding %s not found", pjskUserID)
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
			return nil, fmt.Errorf("snapshot: binding %s does not expose mysekai snapshot", pjskUserID)
		}
		return nil, fmt.Errorf("snapshot: binding %s does not expose suite snapshot", pjskUserID)
	}
	bindingDebugLogger.DebugContext(ctx, snapshotBindingSelectedLog, "scope", "explicit", "region", region.String())
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

func bindingResolutionOutcome(binding *accountdata.ResolvedBinding, err error, opts ResolveOptions) string {
	if err != nil {
		return "error"
	}
	if bindingAllowed(binding, opts) {
		return "eligible"
	}
	return "ineligible"
}

func snapshotBindingErrorType(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}
