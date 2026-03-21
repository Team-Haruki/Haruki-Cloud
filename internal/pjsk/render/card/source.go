package card

import (
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type DataSource interface {
	DefaultRegion() renderregion.Value
	GetCardByID(id int) (*masterdata.Card, error)
	GetCardByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error)
	FilterCards(info *CardQueryInfo) ([]*masterdata.Card, error)
	GetCharacterByID(id int) (*masterdata.Character, error)
	GetUnitByCardID(cardID int) (string, error)
	GetCardSupplyType(card *masterdata.Card) string
	GetSkillByID(id int) (*masterdata.Skill, error)
	FormatSkillDescription(skill *masterdata.Skill, cardCharacterID int) string
	GetGachaByCardID(cardID int) (*masterdata.Gacha, error)
	GetCostume3dsByCardID(cardID int) ([]*masterdata.Costume3d, error)
}
