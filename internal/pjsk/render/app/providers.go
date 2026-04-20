package app

import (
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
)

func (a *App) ProviderForRegion(region renderregion.Value) provider.MasterDataProvider {
	if a == nil {
		return nil
	}

	resolved := renderregion.Normalize(region.String())
	if resolved.IsZero() {
		if configured := renderregion.WithDefault(a.Config.DefaultRegion); !configured.IsZero() {
			resolved = configured
		}
	}

	if len(a.Providers) > 0 {
		if !resolved.IsZero() {
			if src, ok := a.Providers[resolved]; ok && src != nil {
				return src
			}
		}
		if configured := renderregion.WithDefault(a.Config.DefaultRegion); !configured.IsZero() {
			if src, ok := a.Providers[configured]; ok && src != nil {
				return src
			}
		}
		for _, src := range a.Providers {
			if src != nil {
				return src
			}
		}
	}

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
