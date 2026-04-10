package card

import (
	"context"
	"fmt"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

// ProviderAdapter bridges provider.MasterDataProvider to card.DataSource.
type ProviderAdapter struct {
	provider.ProviderAdapterBase
}

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{ProviderAdapterBase: provider.NewProviderAdapterBase(p)}
}

func (a *ProviderAdapter) WithContext(ctx context.Context) DataSource {
	if a == nil {
		return nil
	}
	return &ProviderAdapter{ProviderAdapterBase: a.CloneWithContext(ctx)}
}

func (a *ProviderAdapter) GetCardByID(id int) (*masterdata.Card, error) {
	return a.P.Cards().GetByID(a.Context(), id)
}

func (a *ProviderAdapter) GetCardByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error) {
	return a.P.Cards().GetByCharacterAndSeq(a.Context(), characterID, seq)
}

func (a *ProviderAdapter) FilterCards(info *CardQueryInfo) ([]*masterdata.Card, error) {
	if info == nil {
		return nil, fmt.Errorf("card query info is required")
	}

	eventID := info.EventID
	if eventID == 0 && info.BanCharID != 0 {
		events := a.P.Events().GetBanEvents(a.Context(), info.BanCharID)
		if len(events) == 0 {
			return nil, fmt.Errorf("no ban events found for character %d", info.BanCharID)
		}
		if info.BanSeq < 1 || info.BanSeq > len(events) {
			return nil, fmt.Errorf("ban event index out of range: %d", info.BanSeq)
		}
		eventID = events[info.BanSeq-1].ID
	}

	return a.P.Cards().Filter(a.Context(), &provider.CardFilter{
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
	return a.P.Characters().GetColorCode(a.Context(), id)
}

func (a *ProviderAdapter) GetCharacterByID(id int) (*masterdata.Character, error) {
	return a.P.Characters().GetByID(a.Context(), id)
}

func (a *ProviderAdapter) GetUnitByCardID(cardID int) (string, error) {
	return a.P.Cards().GetUnitByCardID(a.Context(), cardID)
}

func (a *ProviderAdapter) GetCardSupplyType(card *masterdata.Card) string {
	return a.P.Cards().GetSupplyType(a.Context(), card)
}

func (a *ProviderAdapter) GetSkillByID(id int) (*masterdata.Skill, error) {
	return a.P.Skills().GetByID(a.Context(), id)
}

func (a *ProviderAdapter) FormatSkillDescription(skill *masterdata.Skill, cardCharacterID int) string {
	return a.P.Skills().FormatDescription(a.Context(), skill, cardCharacterID)
}

func (a *ProviderAdapter) GetGachaByCardID(cardID int) (*masterdata.Gacha, error) {
	return a.P.Cards().GetGachaByCardID(a.Context(), cardID)
}

func (a *ProviderAdapter) GetCostume3dsByCardID(cardID int) ([]*masterdata.Costume3d, error) {
	return a.P.Cards().GetCostume3dsByCardID(a.Context(), cardID)
}
