package app

import (
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
)

func (a *App) ProviderForRegion(region renderregion.Value) provider.MasterDataProvider {
	if a == nil {
		return nil
	}

	resolved := a.resolveProviderRegion(region)
	if src := firstConfiguredProvider(a.Providers, resolved, renderregion.WithDefault(a.Config.DefaultRegion)); src != nil {
		return src
	}
	return a.legacyProviderForRegion(resolved)
}

func (a *App) resolveProviderRegion(region renderregion.Value) renderregion.Value {
	resolved := renderregion.Normalize(region.String())
	if resolved.IsZero() {
		return renderregion.WithDefault(a.Config.DefaultRegion)
	}
	return resolved
}

func firstConfiguredProvider(providers map[renderregion.Value]provider.MasterDataProvider, preferred ...renderregion.Value) provider.MasterDataProvider {
	for _, region := range preferred {
		if !region.IsZero() && providers[region] != nil {
			return providers[region]
		}
	}
	for _, src := range providers {
		if src != nil {
			return src
		}
	}
	return nil
}

func (a *App) legacyProviderForRegion(resolved renderregion.Value) provider.MasterDataProvider {
	if a.Provider == nil {
		return nil
	}
	if resolved.IsZero() {
		return a.Provider
	}
	if renderregion.WithDefault(a.Provider.Region()) == resolved {
		return a.Provider
	}
	return nil
}
