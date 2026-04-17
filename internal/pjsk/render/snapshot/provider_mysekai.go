package snapshot

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
)

type MySekaiPayloadProvider interface {
	Resolve(ctx context.Context, selector Selector, preferGlobalDefault bool) ([]byte, error)
}

type ToolboxMySekaiPayloadProvider struct {
	bindings bindingLookup
	client   privateDataClient
}

func NewToolboxMySekaiPayloadProvider(bindings bindingLookup, client privateDataClient) *ToolboxMySekaiPayloadProvider {
	return &ToolboxMySekaiPayloadProvider{
		bindings: bindings,
		client:   client,
	}
}

func (p *ToolboxMySekaiPayloadProvider) Resolve(ctx context.Context, selector Selector, preferGlobalDefault bool) ([]byte, error) {
	if p == nil || p.bindings == nil || p.client == nil {
		return nil, ErrProviderUnavailable
	}

	platform := strings.TrimSpace(selector.IMPlatform)
	imUserID := strings.TrimSpace(selector.IMUserID)
	if platform == "" || imUserID == "" {
		return nil, fmt.Errorf("snapshot: mysekai selector is incomplete")
	}

	region := renderregion.WithDefault(selector.Region)
	binding, err := resolveMySekaiPayloadBinding(ctx, p.bindings, platform, imUserID, region, selector.PJSKUserID, preferGlobalDefault)
	if err != nil {
		return nil, err
	}

	uid, err := strconv.ParseInt(binding.PJSKUserID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("snapshot: invalid bound pjsk user id %q: %w", binding.PJSKUserID, err)
	}

	payload, err := p.client.GetMySekaiData(binding.Server, uid, platform, imUserID)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, ErrSnapshotUnavailable
	}
	return slices.Clone(payload), nil
}

type FallbackMySekaiPayloadProvider struct {
	providers []MySekaiPayloadProvider
}

func NewFallbackMySekaiPayloadProvider(providers ...MySekaiPayloadProvider) *FallbackMySekaiPayloadProvider {
	available := make([]MySekaiPayloadProvider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			available = append(available, provider)
		}
	}
	return &FallbackMySekaiPayloadProvider{providers: available}
}

func (p *FallbackMySekaiPayloadProvider) Resolve(ctx context.Context, selector Selector, preferGlobalDefault bool) ([]byte, error) {
	if p == nil || len(p.providers) == 0 {
		return nil, ErrProviderUnavailable
	}

	var lastErr error
	for _, provider := range p.providers {
		payload, err := provider.Resolve(ctx, selector, preferGlobalDefault)
		if err == nil && len(payload) > 0 {
			return payload, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrSnapshotUnavailable
}

func resolveMySekaiPayloadBinding(
	ctx context.Context,
	bindings bindingLookup,
	platform, imUserID string,
	region renderregion.Value,
	pjskUserID string,
	preferGlobalDefault bool,
) (*accountdata.ResolvedBinding, error) {
	if bindings == nil {
		return nil, ErrProviderUnavailable
	}
	if strings.TrimSpace(pjskUserID) != "" {
		return resolveExplicitMySekaiPayloadBinding(ctx, bindings, platform, imUserID, region, pjskUserID)
	}

	regionStr := region.String()
	if preferGlobalDefault {
		_, binding, err := bindings.ResolveUserBinding(ctx, platform, imUserID, accountdata.GlobalDefaultBindingScope)
		if err == nil && mySekaiPayloadBindingAllowed(binding) {
			return binding, nil
		}
	}

	_, binding, err := bindings.ResolveUserBinding(ctx, platform, imUserID, regionStr)
	if err != nil {
		return nil, err
	}
	if !mySekaiPayloadBindingAllowed(binding) {
		return nil, fmt.Errorf("snapshot: binding does not expose mysekai payload")
	}
	return binding, nil
}

func resolveExplicitMySekaiPayloadBinding(
	ctx context.Context,
	bindings bindingLookup,
	platform, imUserID string,
	region renderregion.Value,
	pjskUserID string,
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
		cp := item
		match = &cp
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
	if !mySekaiPayloadBindingAllowed(binding) {
		return nil, fmt.Errorf("snapshot: binding %s does not expose mysekai payload", pjskUserID)
	}
	return binding, nil
}

func mySekaiPayloadBindingAllowed(binding *accountdata.ResolvedBinding) bool {
	return binding != nil && binding.MySekaiVisible
}
