package card

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

type cardEpisodeProvider interface {
	GetEpisodesByCardID(ctx context.Context, cardID int) ([]*masterdata.CardEpisode, error)
}

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{PjskProviderAdapterBase: provider.NewProviderAdapterBase(p)}
}

func (a *ProviderAdapter) WithContext(ctx context.Context) DataSource {
	if a == nil {
		return nil
	}
	return &ProviderAdapter{PjskProviderAdapterBase: a.CloneWithContext(ctx)}
}

func (a *ProviderAdapter) GetCardByID(id int) (*masterdata.Card, error) {
	return a.P.Cards().GetByID(a.Context(), id)
}

func (a *ProviderAdapter) GetCardByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error) {
	return a.P.Cards().GetByCharacterAndSeq(a.Context(), characterID, seq)
}

func (a *ProviderAdapter) FilterCards(info *PjskCardQueryInfo) ([]*masterdata.Card, error) {
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
		SkillIDs:    append([]int(nil), info.SkillIDs...),
		SupplyType:  info.SupplyType,
		Year:        info.Year,
		EventID:     eventID,
	})
}

func (a *ProviderAdapter) GetAllCards() ([]*masterdata.Card, error) {
	return a.P.Cards().Filter(a.Context(), &provider.CardFilter{})
}

func (a *ProviderAdapter) AreaItemLevelCaps(limit int) map[int]int {
	result := make(map[int]int)
	items := a.P.Education().GetAreaItems(a.Context())
	for _, item := range items {
		if item == nil {
			continue
		}
		maxLevel := 0
		for _, level := range a.P.Education().GetAreaItemLevels(a.Context(), item.ID) {
			if level == nil {
				continue
			}
			if limit > 0 && level.Level > limit {
				continue
			}
			if level.Level > maxLevel {
				maxLevel = level.Level
			}
		}
		result[item.ID] = maxLevel
	}
	return result
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

func (a *ProviderAdapter) GetCardEpisodes(cardID int) ([]snapshot.RawUserCardEpisode, error) {
	providerWithEpisodes, ok := a.P.Cards().(cardEpisodeProvider)
	if !ok {
		return nil, fmt.Errorf("card episode lookup is not supported")
	}
	episodes, err := providerWithEpisodes.GetEpisodesByCardID(a.Context(), cardID)
	if err != nil {
		return nil, err
	}
	if len(episodes) == 0 {
		return nil, nil
	}
	result := make([]snapshot.RawUserCardEpisode, 0, len(episodes))
	for _, episode := range episodes {
		if episode == nil || episode.ID == 0 {
			continue
		}
		result = append(result, snapshot.RawUserCardEpisode{
			CardEpisodeID:  episode.ID,
			ScenarioStatus: "already_read",
		})
	}
	return result, nil
}

func (a *ProviderAdapter) GetMaxProfileMysekaiGates() []snapshot.RawUserMysekaiGate {
	if a == nil || a.P == nil || a.P.Education() == nil {
		return nil
	}

	result := make([]snapshot.RawUserMysekaiGate, 0, 5)
	for gateID := 1; gateID <= 5; gateID++ {
		for level := 40; level >= 1; level-- {
			if a.P.Education().GetMysekaiGateLevel(a.Context(), gateID, level) == nil {
				continue
			}
			result = append(result, snapshot.RawUserMysekaiGate{
				MysekaiGateID:    gateID,
				MysekaiGateLevel: level,
			})
			break
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (a *ProviderAdapter) GetMaxProfileMysekaiFixtureBonuses() []snapshot.RawUserFixtureBonus {
	if a == nil || a.P == nil || a.P.MySekai() == nil || !a.P.MySekai().Configured() {
		return nil
	}

	fixtures := a.P.MySekai().LoadList("mysekaiFixtures.json")
	bonusesByID := a.P.MySekai().LoadMapByID("mysekaiFixtureGameCharacterGroupPerformanceBonuses.json")
	groupsByID := a.P.MySekai().LoadMapByID("mysekaiFixtureGameCharacterGroups.json")
	if len(fixtures) == 0 || len(bonusesByID) == 0 || len(groupsByID) == 0 {
		return nil
	}

	totalByCharacter := make(map[int]float64)
	for _, fixture := range fixtures {
		characterID, bonusRate, ok := resolveFixtureCharacterBonus(fixture, bonusesByID, groupsByID)
		if !ok {
			continue
		}
		totalByCharacter[characterID] += bonusRate
	}
	if len(totalByCharacter) == 0 {
		return nil
	}

	characterIDs := make([]int, 0, len(totalByCharacter))
	for characterID := range totalByCharacter {
		characterIDs = append(characterIDs, characterID)
	}
	sort.Ints(characterIDs)

	result := make([]snapshot.RawUserFixtureBonus, 0, len(characterIDs))
	for _, characterID := range characterIDs {
		bonusRate := totalByCharacter[characterID]
		if bonusRate > 100 {
			bonusRate = 100
		}
		result = append(result, snapshot.RawUserFixtureBonus{
			GameCharacterID: characterID,
			TotalBonusRate:  bonusRate,
		})
	}
	return result
}

func resolveFixtureCharacterBonus(fixture map[string]any, bonusesByID, groupsByID map[int]map[string]any) (int, float64, bool) {
	bonusID, ok := maxProfileNumberToInt(fixture["mysekaiFixtureGameCharacterGroupPerformanceBonusId"])
	if !ok || bonusID <= 0 {
		return 0, 0, false
	}
	bonus := bonusesByID[bonusID]
	if bonus == nil {
		return 0, 0, false
	}
	groupID, ok := maxProfileNumberToInt(bonus["mysekaiFixtureGameCharacterGroupId"])
	if !ok || groupID <= 0 {
		return 0, 0, false
	}
	group := groupsByID[groupID]
	if group == nil {
		return 0, 0, false
	}
	characterID, ok := maxProfileNumberToInt(group["gameCharacterId"])
	if !ok || characterID <= 0 {
		return 0, 0, false
	}
	bonusRate, ok := maxProfileNumberToFloat64(bonus["bonusRate"])
	if !ok || bonusRate <= 0 {
		return 0, 0, false
	}
	return characterID, bonusRate, true
}

func maxProfileNumberToInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func maxProfileNumberToFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		n, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return n, true
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
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
