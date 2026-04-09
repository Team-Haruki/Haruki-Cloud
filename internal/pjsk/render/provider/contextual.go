package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type ContextualMasterDataProvider interface {
	MasterDataProvider
	WithContext(ctx context.Context) MasterDataProvider
}

func WithContext(p MasterDataProvider, ctx context.Context) MasterDataProvider {
	if p == nil {
		return nil
	}
	if contextual, ok := p.(ContextualMasterDataProvider); ok {
		return contextual.WithContext(ctx)
	}
	return p
}

func (p *LocalProvider) WithContext(context.Context) MasterDataProvider {
	return p
}

func (p *DatabaseProvider) WithContext(ctx context.Context) MasterDataProvider {
	if p == nil || ctx == nil {
		return p
	}
	return &contextualDatabaseProvider{
		base: p,
		ctx:  ctx,
	}
}

type contextualDatabaseProvider struct {
	base *DatabaseProvider
	ctx  context.Context
}

func (p *contextualDatabaseProvider) Region() renderregion.Value {
	return p.base.Region()
}

func (p *contextualDatabaseProvider) Cards() CardProvider {
	return &contextualCardProvider{base: p.base.cards, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Characters() CharacterProvider {
	return &contextualCharacterProvider{base: p.base.characters, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Skills() SkillProvider {
	return &contextualSkillProvider{base: p.base.skills, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Events() EventProvider {
	return &contextualEventProvider{base: p.base.events, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Musics() MusicProvider {
	return &contextualMusicProvider{base: p.base.musics, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Gachas() GachaProvider {
	return &contextualGachaProvider{base: p.base.gachas, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Honors() HonorProvider {
	return &contextualHonorProvider{base: p.base.honors, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Stamps() StampProvider {
	return &contextualStampProvider{base: p.base.stamps, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) VLives() VLiveProvider {
	return &contextualVLiveProvider{base: p.base.vlives, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Education() EducationProvider {
	return &contextualEducationProvider{base: p.base.education, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) PlayerFrames() PlayerFrameProvider {
	return &contextualPlayerFrameProvider{base: p.base.playerFrames, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) MySekai() MySekaiProvider {
	return p.base.mysekai
}

type contextualCardProvider struct {
	base *dbCardProvider
	ctx  context.Context
}

func (p *contextualCardProvider) GetByID(id int) (*masterdata.Card, error) {
	return p.base.getByID(p.ctx, id)
}

func (p *contextualCardProvider) GetByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error) {
	return p.base.getByCharacterAndSeq(p.ctx, characterID, seq)
}

func (p *contextualCardProvider) Filter(filter *CardFilter) ([]*masterdata.Card, error) {
	return p.base.filter(p.ctx, filter)
}

func (p *contextualCardProvider) GetSupplyType(card *masterdata.Card) string {
	return p.base.getSupplyType(p.ctx, card)
}

func (p *contextualCardProvider) GetGachaByCardID(cardID int) (*masterdata.Gacha, error) {
	return p.base.getGachaByCardID(p.ctx, cardID)
}

func (p *contextualCardProvider) GetCostume3dsByCardID(cardID int) ([]*masterdata.Costume3d, error) {
	return p.base.getCostume3dsByCardID(p.ctx, cardID)
}

func (p *contextualCardProvider) GetUnitByCardID(cardID int) (string, error) {
	return p.base.getUnitByCardID(p.ctx, cardID)
}

type contextualCharacterProvider struct {
	base *dbCharacterProvider
	ctx  context.Context
}

func (p *contextualCharacterProvider) GetByID(id int) (*masterdata.Character, error) {
	return p.base.getByID(p.ctx, id)
}

func (p *contextualCharacterProvider) GetColorCode(id int) (string, bool) {
	return p.base.getColorCode(p.ctx, id)
}

func (p *contextualCharacterProvider) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	return p.base.getGameCharacterUnit(p.ctx, id)
}

type contextualSkillProvider struct {
	base *dbSkillProvider
	ctx  context.Context
}

func (p *contextualSkillProvider) GetByID(id int) (*masterdata.Skill, error) {
	return p.base.getByID(p.ctx, id)
}

func (p *contextualSkillProvider) FormatDescription(skill *masterdata.Skill, cardCharacterID int) string {
	return p.base.formatDescription(p.ctx, skill, cardCharacterID)
}

type contextualEventProvider struct {
	base *dbEventProvider
	ctx  context.Context
}

func (p *contextualEventProvider) GetByID(id int) (*masterdata.Event, error) {
	return p.base.getByID(p.ctx, id)
}

func (p *contextualEventProvider) GetByCardID(cardID int) (*masterdata.Event, error) {
	return p.base.getByCardID(p.ctx, cardID)
}

func (p *contextualEventProvider) GetAll() []*masterdata.Event {
	return p.base.getAll(p.ctx)
}

func (p *contextualEventProvider) GetCards(eventID int) ([]*masterdata.Card, error) {
	return p.base.getCards(p.ctx, eventID)
}

func (p *contextualEventProvider) GetBannerCharacterID(eventID int) (int, error) {
	return p.base.getBannerCharacterID(p.ctx, eventID)
}

func (p *contextualEventProvider) GetDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	return p.base.getDeckBonuses(p.ctx, eventID)
}

func (p *contextualEventProvider) GetBanEvents(charID int) []*masterdata.Event {
	return p.base.getBanEvents(p.ctx, charID)
}

func (p *contextualEventProvider) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom {
	return p.base.getWorldBloomChapters(p.ctx, eventID)
}

type contextualMusicProvider struct {
	base *dbMusicProvider
	ctx  context.Context
}

func (p *contextualMusicProvider) Search(query string) (*masterdata.Music, error) {
	return p.base.search(p.ctx, query)
}

func (p *contextualMusicProvider) GetByID(id int) (*masterdata.Music, error) {
	return p.base.getByID(p.ctx, id)
}

func (p *contextualMusicProvider) GetByEventID(eventID int) (*masterdata.Music, error) {
	return p.base.getByEventID(p.ctx, eventID)
}

func (p *contextualMusicProvider) GetAll() []*masterdata.Music {
	return p.base.getAll(p.ctx)
}

func (p *contextualMusicProvider) GetLocalizedTitles(musicID int) ([]string, error) {
	return p.base.getLocalizedTitles(p.ctx, musicID)
}

func (p *contextualMusicProvider) GetDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	return p.base.getDifficulties(p.ctx, musicID)
}

func (p *contextualMusicProvider) GetVocals(musicID int) ([]*masterdata.MusicVocal, error) {
	return p.base.getVocals(p.ctx, musicID)
}

func (p *contextualMusicProvider) GetTags(musicID int) ([]string, error) {
	return p.base.getTags(p.ctx, musicID)
}

func (p *contextualMusicProvider) GetOutsideCharacterByID(id int) (string, error) {
	return p.base.getOutsideCharacterByID(p.ctx, id)
}

func (p *contextualMusicProvider) GetPrimaryEventByMusicID(musicID int) (*masterdata.Event, error) {
	return p.base.getPrimaryEventByMusicID(p.ctx, musicID)
}

func (p *contextualMusicProvider) GetLimitedTimeMusics(musicID int) []*masterdata.LimitedTimeMusic {
	return p.base.getLimitedTimeMusics(p.ctx, musicID)
}

type contextualGachaProvider struct {
	base *dbGachaProvider
	ctx  context.Context
}

func (p *contextualGachaProvider) GetByID(id int) (*masterdata.Gacha, error) {
	return p.base.getByID(p.ctx, id)
}

func (p *contextualGachaProvider) GetAll() []*masterdata.Gacha {
	return p.base.getAll(p.ctx)
}

func (p *contextualGachaProvider) GetCardByID(id int) (*masterdata.Card, error) {
	return p.base.getCardByID(p.ctx, id)
}

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
