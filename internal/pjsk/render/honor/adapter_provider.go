package honor

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

// ProviderAdapter bridges provider.MasterDataProvider to honor.DataSource.
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

func (a *ProviderAdapter) GetHonorByID(id int) (*masterdata.Honor, error) {
	return a.P.Honors().GetByID(a.Context(), id)
}

func (a *ProviderAdapter) GetHonorGroupByID(id int) (*masterdata.HonorGroup, error) {
	return a.P.Honors().GetGroupByID(a.Context(), id)
}

func (a *ProviderAdapter) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	return a.P.Honors().GetBondsHonorByID(a.Context(), id)
}

func (a *ProviderAdapter) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	return a.P.Honors().GetGameCharacterUnitByID(a.Context(), id)
}
