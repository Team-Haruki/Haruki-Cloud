package costume

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{PjskProviderAdapterBase: provider.NewProviderAdapterBase(p)}
}

type ProviderAdapter struct {
	provider.PjskProviderAdapterBase
}

func (a *ProviderAdapter) WithContext(ctx context.Context) DataSource {
	if a == nil {
		return nil
	}
	return &ProviderAdapter{PjskProviderAdapterBase: a.CloneWithContext(ctx)}
}

func (a *ProviderAdapter) GetCostumeByID(id int) (*masterdata.Costume3d, error) {
	return a.P.Costumes().GetByID(a.Context(), id)
}

func (a *ProviderAdapter) FilterCostumes(filter Filter) ([]*masterdata.Costume3d, error) {
	return a.P.Costumes().Filter(a.Context(), &provider.CostumeFilter{
		PartType:     filter.PartType,
		CostumeType:  filter.CostumeType,
		CharacterID:  filter.CharacterID,
		CharacterIDs: append([]int(nil), filter.CharacterIDs...),
		ColorID:      filter.ColorID,
		Keyword:      filter.Keyword,
		Limit:        filter.Limit,
		Offset:       filter.Offset,
	})
}

func (a *ProviderAdapter) GetCostumeVariants(groupID int, partType string, characterID int) ([]*masterdata.Costume3d, error) {
	return a.P.Costumes().GetVariants(a.Context(), groupID, partType, characterID)
}

func (a *ProviderAdapter) GetCostumeSourceCardIDs(costumeIDs []int) (map[int][]int, error) {
	return a.P.Costumes().GetSourceCardIDs(a.Context(), costumeIDs)
}

func (a *ProviderAdapter) GetCharacterByID(id int) (*masterdata.Character, error) {
	return a.P.Characters().GetByID(a.Context(), id)
}
