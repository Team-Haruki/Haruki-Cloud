package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type contextualHonorProvider struct {
	base *dbHonorProvider
	ctx  context.Context
}

func (p *contextualHonorProvider) GetByID(id int) (*masterdata.Honor, error) {
	return p.base.getByID(p.ctx, id)
}

func (p *contextualHonorProvider) GetGroupByID(id int) (*masterdata.HonorGroup, error) {
	return p.base.getGroupByID(p.ctx, id)
}

func (p *contextualHonorProvider) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	return p.base.getBondsHonorByID(p.ctx, id)
}

func (p *contextualHonorProvider) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	return p.base.getGameCharacterUnitByID(p.ctx, id)
}

func (p *contextualHonorProvider) GetEventIDByHonorID(honorID int) int {
	return p.base.getEventIDByHonorID(p.ctx, honorID)
}

type contextualPlayerFrameProvider struct {
	base *dbPlayerFrameProvider
	ctx  context.Context
}

func (p *contextualPlayerFrameProvider) GetByID(id int) (*masterdata.PlayerFrame, error) {
	return p.base.getByID(p.ctx, id)
}

func (p *contextualPlayerFrameProvider) GetGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	return p.base.getGroupByID(p.ctx, id)
}

type contextualStampProvider struct {
	base *dbStampProvider
	ctx  context.Context
}

func (p *contextualStampProvider) GetAll() ([]masterdata.Stamp, error) {
	return p.base.getAll(p.ctx)
}

type contextualVLiveProvider struct {
	base *dbVLiveProvider
	ctx  context.Context
}

func (p *contextualVLiveProvider) GetLives(region renderregion.Value) ([]*VLive, error) {
	return p.base.getLives(p.ctx, region)
}

type contextualEducationProvider struct {
	base *dbEducationProvider
	ctx  context.Context
}

func (p *contextualEducationProvider) GetChallengeRewardsByCharacter(charID int) []*ChallengeReward {
	return p.base.getChallengeRewardsByCharacter(p.ctx, charID)
}

func (p *contextualEducationProvider) GetResourceBoxByPurpose(purpose string, id int) *ResourceBox {
	return p.base.getResourceBoxByPurpose(p.ctx, purpose, id)
}

func (p *contextualEducationProvider) GetResourceBoxesByPurpose(purpose string) []*ResourceBox {
	return p.base.getResourceBoxesByPurpose(p.ctx, purpose)
}

func (p *contextualEducationProvider) GetAreaItems() []*AreaItem {
	return p.base.getAreaItems(p.ctx)
}

func (p *contextualEducationProvider) GetAreaItem(id int) *AreaItem {
	return p.base.getAreaItem(p.ctx, id)
}

func (p *contextualEducationProvider) GetAreaItemLevels(areaItemID int) []*AreaItemLevel {
	return p.base.getAreaItemLevels(p.ctx, areaItemID)
}

func (p *contextualEducationProvider) GetAreaItemLevel(areaItemID, level int) *AreaItemLevel {
	return p.base.getAreaItemLevel(p.ctx, areaItemID, level)
}

func (p *contextualEducationProvider) GetCharacterRank(characterID, rank int) *CharacterRank {
	return p.base.getCharacterRank(p.ctx, characterID, rank)
}

func (p *contextualEducationProvider) GetBonds() []*Bond {
	return p.base.getBonds(p.ctx)
}

func (p *contextualEducationProvider) GetBondLevels() []*BondLevel {
	return p.base.getBondLevels(p.ctx)
}

func (p *contextualEducationProvider) GetGameCharacterStyle(gameID int) *GameCharacterStyle {
	return p.base.getGameCharacterStyle(p.ctx, gameID)
}

func (p *contextualEducationProvider) GetLeaderMissionRequirements() ([]LeaderMissionRequirement, int) {
	return p.base.getLeaderMissionRequirements(p.ctx)
}

func (p *contextualEducationProvider) GetMysekaiGateLevel(gateID, level int) *MysekaiGateLevel {
	return p.base.getMysekaiGateLevel(p.ctx, gateID, level)
}

func (p *contextualEducationProvider) GetShopItemByResourceBoxID(resourceBoxID int) *ShopItem {
	return p.base.getShopItemByResourceBoxID(p.ctx, resourceBoxID)
}
