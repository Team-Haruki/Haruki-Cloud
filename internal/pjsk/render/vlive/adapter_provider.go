package vlive

import (
	"context"
	"fmt"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{PjskProviderAdapterBase: provider.NewProviderAdapterBase(p)}
}

func (a *ProviderAdapter) WithContext(ctx context.Context) DataSource {
	if a == nil {
		return nil
	}
	return &ProviderAdapter{PjskProviderAdapterBase: a.CloneWithContext(ctx)}
}

func (a *ProviderAdapter) GetLives(region renderregion.Value) ([]*Live, error) {
	pvLives, err := a.P.VLives().GetLives(a.Context(), region)
	if err != nil {
		return nil, err
	}
	result := make([]*Live, len(pvLives))
	for i, pv := range pvLives {
		live := &Live{
			ID:              pv.ID,
			Name:            pv.Name,
			AssetBundleName: pv.AssetBundleName,
			StartAt:         pv.StartAt,
			EndAt:           pv.EndAt,
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
		if len(pv.Rewards) > 0 {
			live.Rewards = make([]Reward, len(pv.Rewards))
			for j, reward := range pv.Rewards {
				live.Rewards[j] = Reward{
					VirtualLiveType: reward.VirtualLiveType,
					ResourceBoxID:   reward.ResourceBoxID,
				}
			}
		}
		if len(pv.Characters) > 0 {
			live.Characters = make([]Character, len(pv.Characters))
			for j, character := range pv.Characters {
				live.Characters[j] = Character{
					GameCharacterUnitID:        character.GameCharacterUnitID,
					VirtualLivePerformanceType: character.VirtualLivePerformanceType,
				}
			}
		}
		result[i] = live
	}
	return result, nil
}

func (a *ProviderAdapter) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	return a.P.Characters().GetGameCharacterUnit(a.Context(), id)
}

func (a *ProviderAdapter) GetEventByVirtualLiveID(id int) (*masterdata.Event, error) {
	if id <= 0 {
		return nil, fmt.Errorf("virtual live id is required")
	}
	for _, item := range a.P.Events().GetAll(a.Context()) {
		if item == nil || item.VirtualLiveID != id {
			continue
		}
		clone := *item
		return &clone, nil
	}
	return nil, fmt.Errorf("event for virtual live %d not found", id)
}

func (a *ProviderAdapter) GetResourceBoxByPurpose(purpose string, id int) *provider.ResourceBox {
	return a.P.Education().GetResourceBoxByPurpose(a.Context(), purpose, id)
}
