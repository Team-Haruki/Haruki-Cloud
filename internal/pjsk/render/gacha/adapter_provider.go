package gacha

import (
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// ProviderAdapter bridges provider.MasterDataProvider to gacha.DataSource.
type ProviderAdapter struct {
	p provider.MasterDataProvider
}

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{p: p}
}

func (a *ProviderAdapter) DefaultRegion() renderregion.Value { return a.p.Region() }

func (a *ProviderAdapter) GetGachaByID(id int) (*masterdata.Gacha, error) {
	return a.p.Gachas().GetByID(id)
}

func (a *ProviderAdapter) GetGachas() []*masterdata.Gacha {
	return a.p.Gachas().GetAll()
}

func (a *ProviderAdapter) GetCardByID(id int) (*masterdata.Card, error) {
	return a.p.Cards().GetByID(id)
}
