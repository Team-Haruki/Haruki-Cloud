package provider

import "haruki-cloud/internal/pjsk/render/masterdata"

// CardProvider exposes card-related masterdata queries.
type CardProvider interface {
	GetByID(id int) (*masterdata.Card, error)
	GetByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error)
	Filter(filter *CardFilter) ([]*masterdata.Card, error)
	GetSupplyType(card *masterdata.Card) string
	GetGachaByCardID(cardID int) (*masterdata.Gacha, error)
	GetCostume3dsByCardID(cardID int) ([]*masterdata.Costume3d, error)
	GetUnitByCardID(cardID int) (string, error)
}

// CardFilter describes filtering criteria for card queries.
// Individual fields are optional; zero-value fields are ignored.
type CardFilter struct {
	CharacterID int
	Unit        string
	MainUnit    string
	SupportUnit string
	Rarity      string
	Attr        string
	SkillType   string
	SupplyType  string
	Year        int
	EventID     int
	Limit       int
}

func cardMatchesUnitFilter(filter *CardFilter, mainUnit, supportUnit string) bool {
	if filter == nil {
		return false
	}
	if filter.Unit == "" && filter.MainUnit == "" && filter.SupportUnit == "" {
		return true
	}

	mainUnit = cardNormalizeUnit(mainUnit)
	supportUnit = cardNormalizeSupportUnit(supportUnit)

	if filter.Unit != "" && filter.Unit != mainUnit && filter.Unit != supportUnit {
		return false
	}
	if filter.MainUnit != "" && filter.MainUnit != mainUnit {
		return false
	}
	if filter.SupportUnit != "" && filter.SupportUnit != supportUnit {
		return false
	}
	return true
}
