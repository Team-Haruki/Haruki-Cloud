package vlive

import (
	"context"

	"haruki-cloud/internal/pjsk/render/provider"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// ProviderAdapter bridges provider.MasterDataProvider to vlive.DataSource.
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

func (a *ProviderAdapter) GetLives(region renderregion.Value) ([]*Live, error) {
	pvLives, err := a.P.VLives().GetLives(a.Context(), region)
	if err != nil {
		return nil, err
	}
	result := make([]*Live, len(pvLives))
	for i, pv := range pvLives {
		live := &Live{
			ID:      pv.ID,
			Name:    pv.Name,
			StartAt: pv.StartAt,
			EndAt:   pv.EndAt,
		}
		if len(pv.Schedules) > 0 {
			live.Schedules = make([]Schedule, len(pv.Schedules))
			for j, s := range pv.Schedules {
				live.Schedules[j] = Schedule{
					StartAt: s.StartAt,
					EndAt:   s.EndAt,
				}
			}
		}
		result[i] = live
	}
	return result, nil
}
