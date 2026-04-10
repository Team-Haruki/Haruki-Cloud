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
	P   MasterDataProvider
	Ctx context.Context
}

func NewProviderAdapterBase(p MasterDataProvider) ProviderAdapterBase {
	return ProviderAdapterBase{P: p, Ctx: context.Background()}
}

func (b *ProviderAdapterBase) DefaultRegion() renderregion.Value {
	return b.P.Region()
}

// Context returns the request context bound to this adapter, falling back
// to context.Background() when no context has been set.
func (b *ProviderAdapterBase) Context() context.Context {
	if b == nil || b.Ctx == nil {
		return context.Background()
	}
	return b.Ctx
}

// CloneWithContext returns a new base bound to ctx. All sub-provider
// interfaces accept ctx directly, so no provider wrapping is needed.
func (b *ProviderAdapterBase) CloneWithContext(ctx context.Context) ProviderAdapterBase {
	if ctx == nil {
		ctx = context.Background()
	}
	return ProviderAdapterBase{P: b.P, Ctx: ctx}
}
