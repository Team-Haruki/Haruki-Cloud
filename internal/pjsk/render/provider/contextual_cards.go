package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

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
