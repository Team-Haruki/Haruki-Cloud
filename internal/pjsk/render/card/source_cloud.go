package card

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"entgo.io/ent/dialect/sql"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/card"
	"haruki-cloud/database/sekai/cardcostume3d"
	"haruki-cloud/database/sekai/cardsupplie"
	"haruki-cloud/database/sekai/costume3d"
	"haruki-cloud/database/sekai/gacha"
	"haruki-cloud/database/sekai/gamecharacter"
	"haruki-cloud/database/sekai/predicate"
	"haruki-cloud/database/sekai/skill"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

var skillPlaceholder = regexp.MustCompile(`\{\{(.*?)\}\}`)

type CloudSource struct {
	client      *sekaiDB.Client
	region      renderregion.Value
	queryRegion renderregion.Value

	cardMu    sync.RWMutex
	cardCache map[int]*masterdata.Card

	charMu    sync.RWMutex
	charCache map[int]*masterdata.Character

	supplyMu   sync.RWMutex
	supplyByID map[int]string

	skillMu    sync.RWMutex
	skillCache map[int]*masterdata.Skill

	gachaMu     sync.RWMutex
	gachaByCard map[int]*masterdata.Gacha
	gachaCache  map[int]*masterdata.Gacha

	costumeMu     sync.RWMutex
	costumeByCard map[int][]*masterdata.Costume3d
}

func NewCloudSource(client *sekaiDB.Client, defaultRegion renderregion.Value) *CloudSource {
	if client == nil {
		return nil
	}
	region := renderregion.WithDefault(defaultRegion)
	return &CloudSource{
		client:        client,
		region:        region,
		queryRegion:   region,
		cardCache:     make(map[int]*masterdata.Card),
		charCache:     make(map[int]*masterdata.Character),
		supplyByID:    make(map[int]string),
		skillCache:    make(map[int]*masterdata.Skill),
		gachaByCard:   make(map[int]*masterdata.Gacha),
		gachaCache:    make(map[int]*masterdata.Gacha),
		costumeByCard: make(map[int][]*masterdata.Costume3d),
	}
}

func (c *CloudSource) DefaultRegion() renderregion.Value {
	return c.region
}

func (c *CloudSource) GetCardByID(id int) (*masterdata.Card, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid card id")
	}

	c.cardMu.RLock()
	if cached, ok := c.cardCache[id]; ok {
		c.cardMu.RUnlock()
		return cloneCard(cached), nil
	}
	c.cardMu.RUnlock()

	entity, err := c.client.Card.Query().
		Where(card.ServerRegionEQ(c.queryRegion.String()), card.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query card %d failed: %w", id, err)
	}

	model, err := convertCardEntity(entity)
	if err != nil {
		return nil, err
	}
	c.cardMu.Lock()
	c.cardCache[id] = model
	c.cardMu.Unlock()
	return cloneCard(model), nil
}

func (c *CloudSource) GetCardByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error) {
	if characterID == 0 {
		return nil, fmt.Errorf("character id is required")
	}

	entities, err := c.client.Card.Query().
		Where(card.ServerRegionEQ(c.queryRegion.String()), card.CharacterIDEQ(int64(characterID))).
		Order(card.ByReleaseAt()).
		All(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query cards by character failed: %w", err)
	}
	if len(entities) == 0 {
		return nil, fmt.Errorf("no cards found for character %d", characterID)
	}

	var entity *sekaiDB.Card
	if seq < 0 {
		index := len(entities) + seq
		if index < 0 || index >= len(entities) {
			return nil, fmt.Errorf("card sequence out of range: %d (total: %d)", seq, len(entities))
		}
		entity = entities[index]
	} else {
		if seq < 1 || seq > len(entities) {
			return nil, fmt.Errorf("card sequence out of range: %d (total: %d)", seq, len(entities))
		}
		entity = entities[seq-1]
	}

	model, err := convertCardEntity(entity)
	if err != nil {
		return nil, err
	}
	c.cardMu.Lock()
	c.cardCache[model.ID] = model
	c.cardMu.Unlock()
	return cloneCard(model), nil
}

func (c *CloudSource) FilterCards(info *CardQueryInfo) ([]*masterdata.Card, error) {
	if info == nil {
		return nil, fmt.Errorf("query info is required")
	}

	query := c.client.Card.Query().Where(card.ServerRegionEQ(c.queryRegion.String()))
	if info.CharacterID != 0 {
		query = query.Where(card.CharacterIDEQ(int64(info.CharacterID)))
	}
	if info.Rarity != "" {
		query = query.Where(jsonFieldEQ("card_rarity_type", info.Rarity))
	}
	if info.Attr != "" {
		query = query.Where(jsonFieldEQ("attr", info.Attr))
	}
	if info.Year != 0 {
		start := time.Date(info.Year, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		end := time.Date(info.Year+1, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		query = query.Where(card.ReleaseAtGTE(start), card.ReleaseAtLT(end))
	}

	entities, err := query.Order(card.ByReleaseAt()).All(context.Background())
	if err != nil {
		return nil, err
	}

	results := make([]*masterdata.Card, 0, len(entities))
	for _, entity := range entities {
		model, err := convertCardEntity(entity)
		if err != nil {
			return nil, err
		}
		if info.SkillType != "" {
			skillInfo, err := c.GetSkillByID(model.SkillID)
			if err != nil || skillInfo == nil || skillInfo.DescriptionSpriteName != info.SkillType {
				continue
			}
		}
		if info.SupplyType != "" && !matchesSupplyFilter(info.SupplyType, c.GetCardSupplyType(model)) {
			continue
		}
		results = append(results, cloneCard(model))
	}
	return results, nil
}

func (c *CloudSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	if id == 0 {
		return nil, fmt.Errorf("character id is required")
	}

	c.charMu.RLock()
	if cached, ok := c.charCache[id]; ok {
		c.charMu.RUnlock()
		copy := *cached
		return &copy, nil
	}
	c.charMu.RUnlock()

	entity, err := c.client.Gamecharacter.Query().
		Where(gamecharacter.ServerRegionEQ(c.queryRegion.String()), gamecharacter.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, err
	}
	model := &masterdata.Character{
		ID:        id,
		FirstName: entity.FirstName,
		GivenName: entity.GivenName,
		Unit:      jsonString(entity.Unit),
	}
	c.charMu.Lock()
	c.charCache[id] = model
	c.charMu.Unlock()
	copy := *model
	return &copy, nil
}

func (c *CloudSource) GetUnitByCardID(cardID int) (string, error) {
	cardInfo, err := c.GetCardByID(cardID)
	if err != nil {
		return "", err
	}
	character, err := c.GetCharacterByID(cardInfo.CharacterID)
	if err != nil || character == nil {
		return "", fmt.Errorf("character not found for card %d", cardID)
	}
	if character.Unit != "" && character.Unit != "piapro" {
		return character.Unit, nil
	}
	if cardInfo.SupportUnit != "" && cardInfo.SupportUnit != "none" {
		return cardInfo.SupportUnit, nil
	}
	return "piapro", nil
}

func (c *CloudSource) GetCardSupplyType(cardInfo *masterdata.Card) string {
	if cardInfo == nil {
		return formatSupplyType("")
	}
	if cardInfo.CardRarityType == "rarity_birthday" {
		return formatSupplyType("birthday")
	}
	if cardInfo.CardSupplyID == 0 {
		return formatSupplyType("")
	}

	c.supplyMu.RLock()
	if cached, ok := c.supplyByID[cardInfo.CardSupplyID]; ok {
		c.supplyMu.RUnlock()
		return cached
	}
	c.supplyMu.RUnlock()

	entity, err := c.client.Cardsupplie.Query().
		Where(cardsupplie.ServerRegionEQ(c.queryRegion.String()), cardsupplie.IDEQ(cardInfo.CardSupplyID)).
		Only(context.Background())
	if err != nil {
		return formatSupplyType("")
	}

	value := formatSupplyType(entity.CardSupplyType)
	c.supplyMu.Lock()
	c.supplyByID[cardInfo.CardSupplyID] = value
	c.supplyMu.Unlock()
	return value
}

func (c *CloudSource) GetSkillByID(id int) (*masterdata.Skill, error) {
	if id == 0 {
		return nil, nil
	}

	c.skillMu.RLock()
	if cached, ok := c.skillCache[id]; ok {
		c.skillMu.RUnlock()
		return cloneSkill(cached), nil
	}
	c.skillMu.RUnlock()

	entity, err := c.client.Skill.Query().
		Where(skill.ServerRegionEQ(c.queryRegion.String()), skill.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, err
	}
	model, err := convertSkillEntity(entity)
	if err != nil {
		return nil, err
	}
	c.skillMu.Lock()
	c.skillCache[id] = model
	c.skillMu.Unlock()
	return cloneSkill(model), nil
}

func (c *CloudSource) FormatSkillDescription(skillInfo *masterdata.Skill, cardCharacterID int) string {
	if skillInfo == nil {
		return ""
	}

	return skillPlaceholder.ReplaceAllStringFunc(skillInfo.Description, func(match string) string {
		content := match[2 : len(match)-2]
		parts := strings.Split(content, ";")
		if len(parts) != 2 {
			return match
		}

		ids := make([]int, 0, 2)
		for _, rawID := range strings.Split(parts[0], ",") {
			value, err := strconv.Atoi(strings.TrimSpace(rawID))
			if err == nil {
				ids = append(ids, value)
			}
		}
		if len(ids) == 0 {
			return match
		}

		formatValues := func(values []int) string {
			if len(values) == 0 {
				return ""
			}
			allSame := true
			first := values[0]
			for _, value := range values[1:] {
				if value != first {
					allSame = false
					break
				}
			}
			if allSame {
				return fmt.Sprintf("%d", first)
			}
			unique := make([]string, 0, len(values))
			seen := make(map[int]struct{})
			for _, value := range values {
				if _, ok := seen[value]; ok {
					continue
				}
				seen[value] = struct{}{}
				unique = append(unique, fmt.Sprintf("%d", value))
			}
			return strings.Join(unique, "/")
		}

		getValues := func(effect *masterdata.SkillEffect) []int {
			if effect == nil || len(effect.SkillEffectDetails) == 0 {
				return []int{0}
			}
			values := make([]int, 0, len(effect.SkillEffectDetails))
			for _, detail := range effect.SkillEffectDetails {
				values = append(values, detail.ActivateEffectValue)
			}
			return values
		}

		resolveCharacter := func() string {
			if name := c.lookupCharacterName(cardCharacterID); name != "" {
				return name
			}
			return "???"
		}

		if parts[1] == "c" {
			return resolveCharacter()
		}

		effects := make([]*masterdata.SkillEffect, 0, len(ids))
		for _, effectID := range ids {
			for idx := range skillInfo.SkillEffects {
				if skillInfo.SkillEffects[idx].ID == effectID {
					effects = append(effects, &skillInfo.SkillEffects[idx])
					break
				}
			}
		}
		if len(effects) != len(ids) {
			return "?"
		}

		if len(effects) == 1 {
			effect := effects[0]
			switch parts[1] {
			case "d":
				if len(effect.SkillEffectDetails) > 0 {
					return fmt.Sprintf("%.1f", effect.SkillEffectDetails[0].ActivateEffectDuration)
				}
				return "0.0"
			case "v":
				return formatValues(getValues(effect))
			case "e":
				return fmt.Sprintf("%d", effect.SkillEnhance.ActivateEffectValue)
			case "m":
				values := getValues(effect)
				for idx := range values {
					values[idx] += effect.SkillEnhance.ActivateEffectValue * 5
				}
				return formatValues(values)
			}
		}

		if len(effects) == 2 {
			values1 := getValues(effects[0])
			values2 := getValues(effects[1])
			switch parts[1] {
			case "v":
				var sums []int
				for idx := 0; idx < len(values1) && idx < len(values2); idx++ {
					sums = append(sums, values1[idx]+values2[idx])
				}
				return formatValues(sums)
			case "u", "o":
				getEnhanced := func(effect *masterdata.SkillEffect, base []int) []int {
					if effect == nil {
						return nil
					}
					out := make([]int, 0, len(effect.SkillEffectDetails))
					for idx, detail := range effect.SkillEffectDetails {
						if detail.ActivateEffectValue2 != nil {
							out = append(out, *detail.ActivateEffectValue2)
						} else if idx < len(base) {
							out = append(out, base[idx])
						}
					}
					return out
				}
				enhanced1 := getEnhanced(effects[0], values1)
				enhanced2 := getEnhanced(effects[1], values2)
				var sums []int
				for idx := 0; idx < len(enhanced1) && idx < len(enhanced2); idx++ {
					sums = append(sums, enhanced1[idx]+enhanced2[idx])
				}
				return formatValues(sums)
			case "r", "s":
				return "..."
			}
		}

		return match
	})
}

func (c *CloudSource) GetGachaByCardID(cardID int) (*masterdata.Gacha, error) {
	if cardID == 0 {
		return nil, fmt.Errorf("invalid card id")
	}

	c.gachaMu.RLock()
	if cached, ok := c.gachaByCard[cardID]; ok {
		c.gachaMu.RUnlock()
		copy := *cached
		return &copy, nil
	}
	c.gachaMu.RUnlock()

	cardInfo, err := c.GetCardByID(cardID)
	if err != nil {
		return nil, err
	}

	candidates, err := c.client.Gacha.Query().
		Where(
			gacha.ServerRegionEQ(c.queryRegion.String()),
			gacha.StartAtLTE(cardInfo.ReleaseAt),
			gacha.EndAtGTE(cardInfo.ReleaseAt),
		).
		Order(gacha.ByStartAt()).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		candidates, err = c.client.Gacha.Query().
			Where(gacha.ServerRegionEQ(c.queryRegion.String())).
			Order(gacha.ByStartAt(sql.OrderDesc())).
			Limit(30).
			All(context.Background())
		if err != nil {
			return nil, err
		}
	}

	for _, candidate := range candidates {
		model, err := convertGachaEntity(candidate)
		if err != nil {
			continue
		}
		if containsPickup(model, cardID) {
			c.gachaMu.Lock()
			c.gachaByCard[cardID] = model
			c.gachaCache[model.ID] = model
			c.gachaMu.Unlock()
			copy := *model
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("gacha not found for card: %d", cardID)
}

func (c *CloudSource) GetCostume3dsByCardID(cardID int) ([]*masterdata.Costume3d, error) {
	if cardID == 0 {
		return nil, nil
	}

	c.costumeMu.RLock()
	if cached, ok := c.costumeByCard[cardID]; ok {
		c.costumeMu.RUnlock()
		return cloneCostumes(cached), nil
	}
	c.costumeMu.RUnlock()

	links, err := c.client.Cardcostume3D.Query().
		Where(cardcostume3d.ServerRegionEQ(c.queryRegion.String()), cardcostume3d.CardIDEQ(int64(cardID))).
		Order(cardcostume3d.ByCostume3DID()).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, nil
	}

	costumes := make([]*masterdata.Costume3d, 0, len(links))
	for _, link := range links {
		entity, err := c.client.Costume3D.Query().
			Where(costume3d.ServerRegionEQ(c.queryRegion.String()), costume3d.GameIDEQ(link.Costume3DID)).
			Only(context.Background())
		if err != nil {
			continue
		}
		costumes = append(costumes, &masterdata.Costume3d{
			ID:              int(entity.GameID),
			CharacterID:     int(entity.CharacterID),
			AssetBundleName: entity.AssetbundleName,
			Description:     entity.Name,
		})
	}
	if len(costumes) == 0 {
		return nil, nil
	}

	c.costumeMu.Lock()
	c.costumeByCard[cardID] = costumes
	c.costumeMu.Unlock()
	return cloneCostumes(costumes), nil
}

func convertCardEntity(entity *sekaiDB.Card) (*masterdata.Card, error) {
	if entity == nil {
		return nil, fmt.Errorf("card entity is nil")
	}

	var parameters []masterdata.CardParameter
	if len(entity.CardParameters) > 0 {
		if err := json.Unmarshal(entity.CardParameters, &parameters); err != nil {
			return nil, fmt.Errorf("decode card parameters: %w", err)
		}
	}

	return &masterdata.Card{
		ID:                              int(entity.GameID),
		CharacterID:                     int(entity.CharacterID),
		CardRarityType:                  jsonString(entity.CardRarityType),
		Attr:                            jsonString(entity.Attr),
		Prefix:                          entity.Prefix,
		AssetBundleName:                 entity.AssetbundleName,
		ReleaseAt:                       entity.ReleaseAt,
		SkillID:                         int(entity.SkillID),
		CardSkillName:                   entity.CardSkillName,
		SupportUnit:                     jsonString(entity.SupportUnit),
		CardParameters:                  parameters,
		SpecialTrainingPower1BonusFixed: int(entity.SpecialTrainingPower1BonusFixed),
		SpecialTrainingPower2BonusFixed: int(entity.SpecialTrainingPower2BonusFixed),
		SpecialTrainingPower3BonusFixed: int(entity.SpecialTrainingPower3BonusFixed),
		SpecialTrainingSkillID:          int(entity.SpecialTrainingSkillID),
		SpecialTrainingSkillName:        entity.SpecialTrainingSkillName,
		CardSupplyID:                    int(entity.CardSupplyID),
	}, nil
}

func convertSkillEntity(entity *sekaiDB.Skill) (*masterdata.Skill, error) {
	if entity == nil {
		return nil, fmt.Errorf("skill entity is nil")
	}
	var effects []masterdata.SkillEffect
	if len(entity.SkillEffects) > 0 {
		if err := json.Unmarshal(entity.SkillEffects, &effects); err != nil {
			return nil, fmt.Errorf("decode skill effects: %w", err)
		}
	}
	return &masterdata.Skill{
		ID:                    int(entity.GameID),
		ShortDescription:      entity.ShortDescription,
		Description:           entity.Description,
		DescriptionSpriteName: jsonString(entity.DescriptionSpriteName),
		SkillEffects:          effects,
	}, nil
}

func convertGachaEntity(entity *sekaiDB.Gacha) (*masterdata.Gacha, error) {
	if entity == nil {
		return nil, fmt.Errorf("gacha entity is nil")
	}
	var rates []masterdata.GachaCardRarityRate
	if len(entity.GachaCardRarityRates) > 0 {
		if err := json.Unmarshal(entity.GachaCardRarityRates, &rates); err != nil {
			return nil, fmt.Errorf("decode gacha rarity rates: %w", err)
		}
	}
	var pickups []masterdata.GachaPickup
	if len(entity.GachaPickups) > 0 {
		if err := json.Unmarshal(entity.GachaPickups, &pickups); err != nil {
			return nil, fmt.Errorf("decode gacha pickups: %w", err)
		}
	}
	var details []masterdata.GachaDetail
	if len(entity.GachaDetails) > 0 {
		if err := json.Unmarshal(entity.GachaDetails, &details); err != nil {
			return nil, fmt.Errorf("decode gacha details: %w", err)
		}
	}
	var behaviors []masterdata.GachaBehavior
	if len(entity.GachaBehaviors) > 0 {
		if err := json.Unmarshal(entity.GachaBehaviors, &behaviors); err != nil {
			return nil, fmt.Errorf("decode gacha behaviors: %w", err)
		}
	}
	var information masterdata.GachaInformation
	if len(entity.GachaInformation) > 0 {
		if err := json.Unmarshal(entity.GachaInformation, &information); err != nil {
			return nil, fmt.Errorf("decode gacha information: %w", err)
		}
	}

	var ceilItemID *int
	if entity.GachaCeilItemID != 0 {
		value := int(entity.GachaCeilItemID)
		ceilItemID = &value
	}

	return &masterdata.Gacha{
		ID:                     int(entity.GameID),
		GachaType:              jsonString(entity.GachaType),
		Name:                   entity.Name,
		Seq:                    int(entity.Seq),
		AssetBundleName:        entity.AssetbundleName,
		StartAt:                entity.StartAt,
		EndAt:                  entity.EndAt,
		IsShowPeriod:           entity.IsShowPeriod,
		GachaCeilItemID:        ceilItemID,
		WishSelectCount:        int(entity.WishSelectCount),
		WishFixedSelectCount:   int(entity.WishFixedSelectCount),
		WishLimitedSelectCount: int(entity.WishLimitedSelectCount),
		GachaCardRarityRates:   rates,
		GachaPickups:           pickups,
		GachaDetails:           details,
		GachaBehaviors:         behaviors,
		GachaInformation:       information,
	}, nil
}

func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return string(raw)
	}
	return s
}

// jsonFieldEQ creates a predicate that matches a JSONB text field by its
// unquoted string value. Works with PostgreSQL's ->> operator.
func jsonFieldEQ(field, value string) predicate.Card {
	return predicate.Card(sql.FieldEQ(field, value))
}

func (c *CloudSource) lookupCharacterName(id int) string {
	character, err := c.GetCharacterByID(id)
	if err != nil || character == nil {
		return ""
	}
	return character.FirstName + character.GivenName
}

func containsPickup(gachaInfo *masterdata.Gacha, cardID int) bool {
	for _, pickup := range gachaInfo.GachaPickups {
		if pickup.CardID == cardID {
			return true
		}
	}
	return false
}

func cloneCard(item *masterdata.Card) *masterdata.Card {
	if item == nil {
		return nil
	}
	copy := *item
	if len(item.CardParameters) > 0 {
		copy.CardParameters = append([]masterdata.CardParameter(nil), item.CardParameters...)
	}
	return &copy
}

func cloneSkill(item *masterdata.Skill) *masterdata.Skill {
	if item == nil {
		return nil
	}
	copy := *item
	if len(item.SkillEffects) > 0 {
		copy.SkillEffects = make([]masterdata.SkillEffect, len(item.SkillEffects))
		for idx := range item.SkillEffects {
			copy.SkillEffects[idx] = item.SkillEffects[idx]
			if len(item.SkillEffects[idx].SkillEffectDetails) > 0 {
				copy.SkillEffects[idx].SkillEffectDetails = append([]masterdata.SkillEffectDetail(nil), item.SkillEffects[idx].SkillEffectDetails...)
			}
		}
	}
	return &copy
}

func cloneCostumes(items []*masterdata.Costume3d) []*masterdata.Costume3d {
	if len(items) == 0 {
		return nil
	}
	result := make([]*masterdata.Costume3d, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		copy := *item
		result = append(result, &copy)
	}
	return result
}
