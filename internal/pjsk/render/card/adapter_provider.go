package card

import (
	"fmt"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// ProviderAdapter bridges provider.MasterDataProvider to card.DataSource.
type ProviderAdapter struct {
	p provider.MasterDataProvider
}

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{p: p}
}

func (a *ProviderAdapter) DefaultRegion() renderregion.Value { return a.p.Region() }

func (a *ProviderAdapter) GetCardByID(id int) (*masterdata.Card, error) {
	return a.p.Cards().GetByID(id)
}

func (a *ProviderAdapter) GetCardByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error) {
	return a.p.Cards().GetByCharacterAndSeq(characterID, seq)
}

func (a *ProviderAdapter) FilterCards(info *CardQueryInfo) ([]*masterdata.Card, error) {
	if info == nil {
		return nil, fmt.Errorf("card query info is required")
	}

	eventID := info.EventID
	if eventID == 0 && info.BanCharID != 0 {
		events := a.p.Events().GetBanEvents(info.BanCharID)
		if len(events) == 0 {
			return nil, fmt.Errorf("no ban events found for character %d", info.BanCharID)
		}
		if info.BanSeq < 1 || info.BanSeq > len(events) {
			return nil, fmt.Errorf("ban event index out of range: %d", info.BanSeq)
		}
		eventID = events[info.BanSeq-1].ID
	}

	return a.p.Cards().Filter(&provider.CardFilter{
		CharacterID: info.CharacterID,
		Unit:        info.Unit,
		MainUnit:    info.MainUnit,
		SupportUnit: info.SupportUnit,
		Rarity:      info.Rarity,
		Attr:        info.Attr,
		SkillType:   info.SkillType,
		SupplyType:  info.SupplyType,
		Year:        info.Year,
		EventID:     eventID,
	})
}

func (a *ProviderAdapter) GetCharacterColorCode(id int) (string, bool) {
	return a.p.Characters().GetColorCode(id)
}

func (a *ProviderAdapter) GetCharacterByID(id int) (*masterdata.Character, error) {
	return a.p.Characters().GetByID(id)
}

func (a *ProviderAdapter) GetUnitByCardID(cardID int) (string, error) {
	return a.p.Cards().GetUnitByCardID(cardID)
}

func (a *ProviderAdapter) GetCardSupplyType(card *masterdata.Card) string {
	return a.p.Cards().GetSupplyType(card)
}

func (a *ProviderAdapter) GetSkillByID(id int) (*masterdata.Skill, error) {
	return a.p.Skills().GetByID(id)
}

func (a *ProviderAdapter) FormatSkillDescription(skill *masterdata.Skill, cardCharacterID int) string {
	return a.p.Skills().FormatDescription(skill, cardCharacterID)
}

func (a *ProviderAdapter) GetGachaByCardID(cardID int) (*masterdata.Gacha, error) {
	return a.p.Cards().GetGachaByCardID(cardID)
}

func (a *ProviderAdapter) GetCostume3dsByCardID(cardID int) ([]*masterdata.Costume3d, error) {
	return a.p.Cards().GetCostume3dsByCardID(cardID)
}
