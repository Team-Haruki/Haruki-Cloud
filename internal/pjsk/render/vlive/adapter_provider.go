package vlive

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/internal/i18n"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/utils/usererror"
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
		result[i] = liveFromProvider(pv)
	}
	return result, nil
}

func liveFromProvider(pv *provider.VLive) *Live {
	live := &Live{
		ID:              pv.ID,
		Name:            pv.Name,
		AssetBundleName: pv.AssetBundleName,
		StartAt:         pv.StartAt,
		EndAt:           pv.EndAt,
		VirtualLiveType: pv.VirtualLiveType,
		GroupID:         pv.VirtualLiveGroupID,
	}
	for _, item := range pv.TotalCheerPointRewards {
		live.TotalCheerPointRewards = append(live.TotalCheerPointRewards, CheerPointReward{Threshold: item.Threshold, ResourceBoxID: item.ResourceBoxID})
	}
	if item := pv.TotalCheerPointSurplusReward; item != nil {
		live.SurplusReward = &SurplusReward{BasePoint: item.BasePoint, ResourceBoxID: item.ResourceBoxID}
	}
	if item := pv.VirtualItemOverrideCost; item != nil {
		live.OverrideCost = &OverrideCost{ResourceType: item.CostResourceType, ResourceID: item.CostResourceID, AssetBundleName: item.AssetBundleName}
	}
	for _, item := range pv.Schedules {
		live.Schedules = append(live.Schedules, Schedule{StartAt: item.StartAt, EndAt: item.EndAt})
	}
	for _, item := range pv.Rewards {
		live.Rewards = append(live.Rewards, Reward{VirtualLiveType: item.VirtualLiveType, ResourceBoxID: item.ResourceBoxID})
	}
	for _, item := range pv.Characters {
		live.Characters = append(live.Characters, Character{
			GameCharacterUnitID:        item.GameCharacterUnitID,
			VirtualLivePerformanceType: item.VirtualLivePerformanceType,
		})
	}
	return live
}

func (a *ProviderAdapter) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	return a.P.Characters().GetGameCharacterUnit(a.Context(), id)
}

func (a *ProviderAdapter) GetEventByVirtualLiveID(id int) (*masterdata.Event, error) {
	if id <= 0 {
		return nil, usererror.Misuse(i18n.M("vlive.query_required"))
	}
	for _, item := range a.P.Events().GetAll(a.Context()) {
		if item == nil || item.VirtualLiveID != id {
			continue
		}
		clone := *item
		return &clone, nil
	}
	return nil, usererror.Wrap(usererror.CodeNotFound, i18n.M("vlive.not_found"), fmt.Errorf("event for virtual live %d not found", id))
}

func (a *ProviderAdapter) GetResourceBoxByPurpose(purpose string, id int) *provider.ResourceBox {
	return a.P.Education().GetResourceBoxByPurpose(a.Context(), purpose, id)
}

// GetGroups implements groupDataSource.
func (a *ProviderAdapter) GetGroups(region renderregion.Value) (map[int]*Group, bool) {
	groupProvider, ok := a.P.VLives().(provider.VLiveGroupProvider)
	if !ok {
		return nil, false
	}
	pvGroups, ok := groupProvider.GetGroups(a.Context(), region)
	if !ok {
		return nil, false
	}
	groups := make(map[int]*Group, len(pvGroups))
	for id, pv := range pvGroups {
		if pv == nil {
			continue
		}
		groups[id] = &Group{ID: pv.ID, Name: pv.Name, Type: pv.VirtualLiveGroupType, AssetBundleName: pv.AssetBundleName}
	}
	return groups, true
}

// GetMaterialName implements materialNameSource through the raw row store.
func (a *ProviderAdapter) GetMaterialName(id int) string {
	store := a.P.MySekai()
	if id <= 0 || store == nil {
		return ""
	}
	rowSource, ok := store.(provider.MasterRowSource)
	if !ok {
		return ""
	}
	rows, ok := rowSource.LoadMasterRows(a.Context(), "materials.json")
	if !ok {
		return ""
	}
	name, _ := rows[id]["name"].(string)
	return strings.TrimSpace(name)
}
