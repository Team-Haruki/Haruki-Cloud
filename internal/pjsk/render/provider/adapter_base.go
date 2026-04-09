package provider

import (
	"context"

	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// ProviderAdapterBase provides common adapter scaffolding embedded by each
// render module's ProviderAdapter.  It exposes DefaultRegion and a
// CloneWithContext helper so individual adapters only need a thin
// WithContext wrapper that returns their module-specific DataSource.
type ProviderAdapterBase struct {
	P MasterDataProvider
}

func NewProviderAdapterBase(p MasterDataProvider) ProviderAdapterBase {
	return ProviderAdapterBase{P: p}
}

func (b *ProviderAdapterBase) DefaultRegion() renderregion.Value {
	return b.P.Region()
}

// CloneWithContext returns a new base with the provider wrapped for ctx.
func (b *ProviderAdapterBase) CloneWithContext(ctx context.Context) ProviderAdapterBase {
	return ProviderAdapterBase{P: WithContext(b.P, ctx)}
}
