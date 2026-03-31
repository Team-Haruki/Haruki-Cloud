package education

import (
	"haruki-cloud/internal/pjsk/render/provider"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// ProviderAdapter bridges provider.MasterDataProvider to education.DataSource.
type ProviderAdapter struct {
	p provider.MasterDataProvider
}

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{p: p}
}

func (a *ProviderAdapter) DefaultRegion() renderregion.Value { return a.p.Region() }

func (a *ProviderAdapter) GetChallengeRewardsByCharacter(charID int) []*ChallengeReward {
	pvRewards := a.p.Education().GetChallengeRewardsByCharacter(charID)
	result := make([]*ChallengeReward, len(pvRewards))
	for i, r := range pvRewards {
		result[i] = &ChallengeReward{
			ID:            r.ID,
			CharacterID:   r.CharacterID,
			HighScore:     r.HighScore,
			ResourceBoxID: r.ResourceBoxID,
		}
	}
	return result
}

func (a *ProviderAdapter) GetResourceBoxByPurpose(purpose string, id int) *ResourceBox {
	return convertResourceBox(a.p.Education().GetResourceBoxByPurpose(purpose, id))
}

func (a *ProviderAdapter) GetResourceBoxesByPurpose(purpose string) []*ResourceBox {
	pvBoxes := a.p.Education().GetResourceBoxesByPurpose(purpose)
	result := make([]*ResourceBox, len(pvBoxes))
	for i, b := range pvBoxes {
		result[i] = convertResourceBox(b)
	}
	return result
}

func (a *ProviderAdapter) GetAreaItems() []*AreaItem {
	pvItems := a.p.Education().GetAreaItems()
	result := make([]*AreaItem, len(pvItems))
	for i, item := range pvItems {
		result[i] = convertAreaItem(item)
	}
	return result
}

func (a *ProviderAdapter) GetAreaItem(id int) *AreaItem {
	return convertAreaItem(a.p.Education().GetAreaItem(id))
}

func (a *ProviderAdapter) GetAreaItemLevels(areaItemID int) []*AreaItemLevel {
	pvLevels := a.p.Education().GetAreaItemLevels(areaItemID)
	result := make([]*AreaItemLevel, len(pvLevels))
	for i, l := range pvLevels {
		result[i] = convertAreaItemLevel(l)
	}
	return result
}

func (a *ProviderAdapter) GetAreaItemLevel(areaItemID, level int) *AreaItemLevel {
	return convertAreaItemLevel(a.p.Education().GetAreaItemLevel(areaItemID, level))
}

func (a *ProviderAdapter) GetCharacterRank(characterID, rank int) *CharacterRank {
	pv := a.p.Education().GetCharacterRank(characterID, rank)
	if pv == nil {
		return nil
	}
	return &CharacterRank{
		CharacterID:     pv.CharacterID,
		Rank:            pv.Rank,
		Power1BonusRate: pv.Power1BonusRate,
	}
}

func (a *ProviderAdapter) GetMysekaiGateLevel(gateID, level int) *MysekaiGateLevel {
	pv := a.p.Education().GetMysekaiGateLevel(gateID, level)
	if pv == nil {
		return nil
	}
	return &MysekaiGateLevel{
		GateID:         pv.GateID,
		Level:          pv.Level,
		PowerBonusRate: pv.PowerBonusRate,
	}
}

func (a *ProviderAdapter) GetShopItemByResourceBoxID(resourceBoxID int) *ShopItem {
	pv := a.p.Education().GetShopItemByResourceBoxID(resourceBoxID)
	if pv == nil {
		return nil
	}
	costs := make([]ShopItemCost, len(pv.Costs))
	for i, c := range pv.Costs {
		costs[i] = ShopItemCost{
			ResourceType: c.ResourceType,
			ResourceID:   c.ResourceID,
			Quantity:     c.Quantity,
		}
	}
	return &ShopItem{
		ID:            pv.ID,
		ResourceBoxID: pv.ResourceBoxID,
		Costs:         costs,
	}
}

func convertResourceBox(pv *provider.ResourceBox) *ResourceBox {
	if pv == nil {
		return nil
	}
	details := make([]ResourceBoxDetail, len(pv.Details))
	for i, d := range pv.Details {
		details[i] = ResourceBoxDetail{
			ResourceType:     d.ResourceType,
			ResourceID:       d.ResourceID,
			ResourceLevel:    d.ResourceLevel,
			ResourceQuantity: d.ResourceQuantity,
		}
	}
	return &ResourceBox{
		ID:                 pv.ID,
		ResourceBoxPurpose: pv.ResourceBoxPurpose,
		ResourceBoxType:    pv.ResourceBoxType,
		Description:        pv.Description,
		Details:            details,
	}
}

func convertAreaItem(pv *provider.AreaItem) *AreaItem {
	if pv == nil {
		return nil
	}
	return &AreaItem{
		ID:              pv.ID,
		AreaID:          pv.AreaID,
		Name:            pv.Name,
		AssetbundleName: pv.AssetbundleName,
	}
}

func convertAreaItemLevel(pv *provider.AreaItemLevel) *AreaItemLevel {
	if pv == nil {
		return nil
	}
	return &AreaItemLevel{
		AreaItemID:            pv.AreaItemID,
		Level:                 pv.Level,
		TargetUnit:            pv.TargetUnit,
		TargetCardAttr:        pv.TargetCardAttr,
		TargetGameCharacterID: pv.TargetGameCharacterID,
		Power1BonusRate:       pv.Power1BonusRate,
	}
}
