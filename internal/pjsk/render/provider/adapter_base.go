package provider

import (
	"context"

	renderregion "haruki-cloud/internal/pjsk/region"
)

// PjskProviderAdapterBase provides common adapter scaffolding embedded by each
// render module's ProviderAdapter.  It exposes DefaultRegion and a
// CloneWithContext helper so individual adapters only need a thin
// WithContext wrapper that returns their module-specific DataSource.
type PjskProviderAdapterBase struct {
	P   MasterDataProvider
	Ctx context.Context
}

func NewProviderAdapterBase(p MasterDataProvider) PjskProviderAdapterBase {
	return PjskProviderAdapterBase{P: p, Ctx: context.TODO()}
}

func (b *PjskProviderAdapterBase) DefaultRegion() renderregion.Value {
	return b.P.Region()
}

// Context returns the request context bound to this adapter, falling back
// to context.Background() when no context has been set.
func (b *PjskProviderAdapterBase) Context() context.Context {
	if b == nil || b.Ctx == nil {
		return context.TODO()
	}
	return b.Ctx
}

// CloneWithContext returns a new base bound to ctx. All sub-provider
// interfaces accept ctx directly, so no provider wrapping is needed.
func (b *PjskProviderAdapterBase) CloneWithContext(ctx context.Context) PjskProviderAdapterBase {
	if ctx == nil {
		ctx = context.TODO()
	}
	return PjskProviderAdapterBase{P: b.P, Ctx: ctx}
}
