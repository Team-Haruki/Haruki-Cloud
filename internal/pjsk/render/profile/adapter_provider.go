package profile

import (
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// ProviderAdapter bridges provider.MasterDataProvider to profile.Source.
type ProviderAdapter struct {
	p provider.MasterDataProvider
}

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{p: p}
}

// honor.DataSource methods

func (a *ProviderAdapter) DefaultRegion() renderregion.Value { return a.p.Region() }

func (a *ProviderAdapter) GetHonorByID(id int) (*masterdata.Honor, error) {
	return a.p.Honors().GetByID(id)
}

func (a *ProviderAdapter) GetHonorGroupByID(id int) (*masterdata.HonorGroup, error) {
	return a.p.Honors().GetGroupByID(id)
}

func (a *ProviderAdapter) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	return a.p.Honors().GetBondsHonorByID(id)
}

func (a *ProviderAdapter) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	return a.p.Honors().GetGameCharacterUnitByID(id)
}

// profile-specific methods

func (a *ProviderAdapter) GetPlayerFrameByID(id int) (*masterdata.PlayerFrame, error) {
	return a.p.PlayerFrames().GetByID(id)
}

func (a *ProviderAdapter) GetPlayerFrameGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	return a.p.PlayerFrames().GetGroupByID(id)
}

func (a *ProviderAdapter) GetCardByID(id int) (*masterdata.Card, error) {
	return a.p.Cards().GetByID(id)
}

func (a *ProviderAdapter) GetEventIDByHonorID(honorID int) int {
	return a.p.Honors().GetEventIDByHonorID(honorID)
}
