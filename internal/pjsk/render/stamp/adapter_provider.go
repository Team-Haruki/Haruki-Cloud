package stamp

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// ProviderAdapter bridges provider.MasterDataProvider to stamp.DataSource.
type ProviderAdapter struct {
	p provider.MasterDataProvider
}

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{p: p}
}

func (a *ProviderAdapter) WithContext(ctx context.Context) DataSource {
	if a == nil {
		return nil
	}
	clone := *a
	clone.p = provider.WithContext(a.p, ctx)
	return &clone
}

func (a *ProviderAdapter) DefaultRegion() renderregion.Value { return a.p.Region() }

func (a *ProviderAdapter) GetStamps() ([]masterdata.Stamp, error) {
	return a.p.Stamps().GetAll()
}
