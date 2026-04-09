package profile

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

// ProviderAdapter bridges provider.MasterDataProvider to profile.DataSource.
type ProviderAdapter struct {
	provider.ProviderAdapterBase
}

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{ProviderAdapterBase: provider.NewProviderAdapterBase(p)}
}

func (a *ProviderAdapter) WithContext(ctx context.Context) DataSource {
	if a == nil {
		return nil
	}
	return &ProviderAdapter{ProviderAdapterBase: a.CloneWithContext(ctx)}
}

// honor.DataSource methods

func (a *ProviderAdapter) GetHonorByID(id int) (*masterdata.Honor, error) {
	return a.P.Honors().GetByID(id)
}

func (a *ProviderAdapter) GetHonorGroupByID(id int) (*masterdata.HonorGroup, error) {
	return a.P.Honors().GetGroupByID(id)
}

func (a *ProviderAdapter) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	return a.P.Honors().GetBondsHonorByID(id)
}

func (a *ProviderAdapter) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	return a.P.Honors().GetGameCharacterUnitByID(id)
}

// profile-specific methods

func (a *ProviderAdapter) GetPlayerFrameByID(id int) (*masterdata.PlayerFrame, error) {
	return a.P.PlayerFrames().GetByID(id)
}

func (a *ProviderAdapter) GetPlayerFrameGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	return a.P.PlayerFrames().GetGroupByID(id)
}

func (a *ProviderAdapter) GetCardByID(id int) (*masterdata.Card, error) {
	return a.P.Cards().GetByID(id)
}

func (a *ProviderAdapter) GetEventIDByHonorID(honorID int) int {
	return a.P.Honors().GetEventIDByHonorID(honorID)
}
