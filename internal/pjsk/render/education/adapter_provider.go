package education

import (
	"context"

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

func (a *ProviderAdapter) GetChallengeRewardsByCharacter(charID int) []*ChallengeReward {
	pvRewards := a.P.Education().GetChallengeRewardsByCharacter(a.Context(), charID)
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
	return convertResourceBox(a.P.Education().GetResourceBoxByPurpose(a.Context(), purpose, id))
}

func (a *ProviderAdapter) GetResourceBoxesByPurpose(purpose string) []*ResourceBox {
	pvBoxes := a.P.Education().GetResourceBoxesByPurpose(a.Context(), purpose)
	result := make([]*ResourceBox, len(pvBoxes))
	for i, b := range pvBoxes {
		result[i] = convertResourceBox(b)
	}
	return result
}

func (a *ProviderAdapter) GetAreaItems() []*AreaItem {
	pvItems := a.P.Education().GetAreaItems(a.Context())
	result := make([]*AreaItem, len(pvItems))
	for i, item := range pvItems {
		result[i] = convertAreaItem(item)
	}
	return result
}

func (a *ProviderAdapter) GetAreaItem(id int) *AreaItem {
	return convertAreaItem(a.P.Education().GetAreaItem(a.Context(), id))
}

func (a *ProviderAdapter) GetAreaItemLevels(areaItemID int) []*AreaItemLevel {
	pvLevels := a.P.Education().GetAreaItemLevels(a.Context(), areaItemID)
	result := make([]*AreaItemLevel, len(pvLevels))
	for i, l := range pvLevels {
		result[i] = convertAreaItemLevel(l)
	}
	return result
}

func (a *ProviderAdapter) GetAreaItemLevel(areaItemID, level int) *AreaItemLevel {
	return convertAreaItemLevel(a.P.Education().GetAreaItemLevel(a.Context(), areaItemID, level))
}

func (a *ProviderAdapter) GetCharacterRank(characterID, rank int) *CharacterRank {
	pv := a.P.Education().GetCharacterRank(a.Context(), characterID, rank)
	if pv == nil {
		return nil
	}
	return &CharacterRank{
		CharacterID:     pv.CharacterID,
		Rank:            pv.Rank,
		Power1BonusRate: pv.Power1BonusRate,
	}
}

func (a *ProviderAdapter) GetBonds() []*Bond {
	pvBonds := a.P.Education().GetBonds(a.Context())
	result := make([]*Bond, len(pvBonds))
	for i, item := range pvBonds {
		result[i] = &Bond{
			GroupID:      item.GroupID,
			CharacterID1: item.CharacterID1,
			CharacterID2: item.CharacterID2,
		}
	}
	return result
}

func (a *ProviderAdapter) GetBondLevels() []*BondLevel {
	pvLevels := a.P.Education().GetBondLevels(a.Context())
	result := make([]*BondLevel, len(pvLevels))
	for i, item := range pvLevels {
		result[i] = &BondLevel{
			Level:    item.Level,
			TotalExp: item.TotalExp,
		}
	}
	return result
}

func (a *ProviderAdapter) GetGameCharacterStyle(gameID int) *GameCharacterStyle {
	pv := a.P.Education().GetGameCharacterStyle(a.Context(), gameID)
	if pv == nil {
		return nil
	}
	return &GameCharacterStyle{
		GameID:      pv.GameID,
		CharacterID: pv.CharacterID,
		ColorCode:   pv.ColorCode,
	}
}

func (a *ProviderAdapter) GetCharacterMissions(characterID int) []*CharacterMission {
	pvItems := a.P.Education().GetCharacterMissions(a.Context(), characterID)
	result := make([]*CharacterMission, len(pvItems))
	for i, item := range pvItems {
		if item == nil {
			continue
		}
		result[i] = &CharacterMission{
			ID:                   item.ID,
			CharacterID:          item.CharacterID,
			CharacterMissionType: item.CharacterMissionType,
			ParameterGroupID:     item.ParameterGroupID,
			IsAchievementMission: item.IsAchievementMission,
		}
	}
	return result
}

func (a *ProviderAdapter) GetCharacterMissionParameterGroups(parameterGroupID int) []*CharacterMissionParameterGroup {
	pvItems := a.P.Education().GetCharacterMissionParameterGroups(a.Context(), parameterGroupID)
	result := make([]*CharacterMissionParameterGroup, len(pvItems))
	for i, item := range pvItems {
		if item == nil {
			continue
		}
		result[i] = &CharacterMissionParameterGroup{
			GameID:      item.GameID,
			Seq:         item.Seq,
			Requirement: item.Requirement,
			Exp:         item.Exp,
			Quantity:    item.Quantity,
		}
	}
	return result
}

func (a *ProviderAdapter) GetLeaderMissionRequirements() ([]LeaderMissionRequirement, int) {
	pvRequirements, maxPlayLimit := a.P.Education().GetLeaderMissionRequirements(a.Context())
	result := make([]LeaderMissionRequirement, len(pvRequirements))
	for i, item := range pvRequirements {
		result[i] = LeaderMissionRequirement{
			Seq:         item.Seq,
			Requirement: item.Requirement,
		}
	}
	return result, maxPlayLimit
}

func (a *ProviderAdapter) GetMysekaiGateLevel(gateID, level int) *MysekaiGateLevel {
	pv := a.P.Education().GetMysekaiGateLevel(a.Context(), gateID, level)
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
	pv := a.P.Education().GetShopItemByResourceBoxID(a.Context(), resourceBoxID)
	return convertShopItem(pv)
}

func (a *ProviderAdapter) GetShopItems() []*ShopItem {
	pvItems := a.P.Education().GetShopItems(a.Context())
	result := make([]*ShopItem, len(pvItems))
	for i, item := range pvItems {
		result[i] = convertShopItem(item)
	}
	return result
}

func convertShopItem(pv *provider.ShopItem) *ShopItem {
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
		ID:                 pv.ID,
		ShopID:             pv.ShopID,
		Seq:                pv.Seq,
		ResourceBoxID:      pv.ResourceBoxID,
		ReleaseConditionID: pv.ReleaseConditionID,
		StartAt:            pv.StartAt,
		Costs:              costs,
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
