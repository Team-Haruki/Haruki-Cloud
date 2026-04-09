package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"entgo.io/ent/dialect/sql"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/card"
	"haruki-cloud/database/sekai/cardcostume3d"
	"haruki-cloud/database/sekai/cardsupplie"
	"haruki-cloud/database/sekai/costume3d"
	"haruki-cloud/database/sekai/eventcard"
	"haruki-cloud/database/sekai/gacha"
	"haruki-cloud/database/sekai/predicate"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type dbCardProvider struct {
	client     *sekaiDB.Client
	region     renderregion.Value
	characters *dbCharacterProvider
	skills     *dbSkillProvider

	cardMu    sync.RWMutex
	cardCache map[int]*masterdata.Card

	supplyMu   sync.RWMutex
	supplyByID map[int]string

	gachaMu     sync.RWMutex
	gachaByCard map[int]*masterdata.Gacha
	gachaCache  map[int]*masterdata.Gacha

	costumeMu     sync.RWMutex
	costumeByCard map[int][]*masterdata.Costume3d
}

func (p *dbCardProvider) init() {
	if p.cardCache == nil {
		p.cardCache = make(map[int]*masterdata.Card)
	}
	if p.supplyByID == nil {
		p.supplyByID = make(map[int]string)
	}
	if p.gachaByCard == nil {
		p.gachaByCard = make(map[int]*masterdata.Gacha)
	}
	if p.gachaCache == nil {
		p.gachaCache = make(map[int]*masterdata.Gacha)
	}
	if p.costumeByCard == nil {
		p.costumeByCard = make(map[int][]*masterdata.Costume3d)
	}
}

func (p *dbCardProvider) GetByID(id int) (*masterdata.Card, error) {
	return p.getByID(nil, id)
}

func (p *dbCardProvider) getByID(ctx context.Context, id int) (*masterdata.Card, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.init()

	p.cardMu.RLock()
	if cached, ok := p.cardCache[id]; ok {
		p.cardMu.RUnlock()
		return common.CloneCard(cached), nil
	}
	p.cardMu.RUnlock()

	entity, err := p.client.Card.Query().
		Where(card.ServerRegionEQ(p.region.String()), card.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query card %d: %w", id, err)
	}

	model, err := common.ConvertCardEntity(entity)
	if err != nil {
		return nil, err
	}
	p.cardMu.Lock()
	p.cardCache[id] = model
	p.cardMu.Unlock()
	return common.CloneCard(model), nil
}

func (p *dbCardProvider) GetByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error) {
	return p.getByCharacterAndSeq(nil, characterID, seq)
}

func (p *dbCardProvider) getByCharacterAndSeq(ctx context.Context, characterID, seq int) (*masterdata.Card, error) {
	if characterID == 0 {
		return nil, fmt.Errorf("character id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	entities, err := p.client.Card.Query().
		Where(card.ServerRegionEQ(p.region.String()), card.CharacterIDEQ(int64(characterID))).
		Order(card.ByReleaseAt()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query cards by character: %w", err)
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

	p.init()
	model, err := common.ConvertCardEntity(entity)
	if err != nil {
		return nil, err
	}
	p.cardMu.Lock()
	p.cardCache[model.ID] = model
	p.cardMu.Unlock()
	return common.CloneCard(model), nil
}

func (p *dbCardProvider) Filter(filter *CardFilter) ([]*masterdata.Card, error) {
	return p.filter(nil, filter)
}

func (p *dbCardProvider) filter(ctx context.Context, filter *CardFilter) ([]*masterdata.Card, error) {
	if filter == nil {
		return nil, fmt.Errorf("filter is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.init()

	query := p.client.Card.Query().Where(card.ServerRegionEQ(p.region.String()))

	if eventCardIDs, err := p.resolveFilterEventCardIDs(ctx, filter); err != nil {
		return nil, err
	} else if len(eventCardIDs) == 0 && filter.EventID != 0 {
		return nil, nil
	} else if len(eventCardIDs) > 0 {
		query = query.Where(card.GameIDIn(eventCardIDs...))
	}
	if filter.CharacterID != 0 {
		query = query.Where(card.CharacterIDEQ(int64(filter.CharacterID)))
	}
	if filter.Rarity != "" {
		query = query.Where(cardJsonFieldEQ("card_rarity_type", filter.Rarity))
	}
	if filter.Attr != "" {
		query = query.Where(cardJsonFieldEQ("attr", filter.Attr))
	}
	if filter.Year != 0 {
		start := time.Date(filter.Year, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		end := time.Date(filter.Year+1, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		query = query.Where(card.ReleaseAtGTE(start), card.ReleaseAtLT(end))
	}

	entities, err := query.Order(card.ByReleaseAt()).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("filter cards: %w", err)
	}

	results := make([]*masterdata.Card, 0, len(entities))
	for _, entity := range entities {
		model, err := common.ConvertCardEntity(entity)
		if err != nil {
			return nil, err
		}
		if !p.matchesUnitFilter(ctx, filter, model) {
			continue
		}
		if filter.SkillType != "" {
			if p.skills != nil {
				skillInfo, sErr := p.skills.getByID(ctx, model.SkillID)
				if sErr != nil || skillInfo == nil || skillInfo.DescriptionSpriteName != filter.SkillType {
					continue
				}
			} else {
				continue
			}
		}
		if filter.SupplyType != "" && !cardMatchesSupplyFilter(filter.SupplyType, p.getSupplyType(ctx, model)) {
			continue
		}
		results = append(results, common.CloneCard(model))
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}
	return results, nil
}

func (p *dbCardProvider) GetSupplyType(cardInfo *masterdata.Card) string {
	return p.getSupplyType(nil, cardInfo)
}

func (p *dbCardProvider) getSupplyType(ctx context.Context, cardInfo *masterdata.Card) string {
	if cardInfo == nil {
		return cardNormalizeSupplyType("")
	}
	if cardInfo.CardRarityType == "rarity_birthday" {
		return cardNormalizeSupplyType("birthday")
	}
	if cardInfo.CardSupplyID == 0 {
		return cardNormalizeSupplyType("")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	p.supplyMu.RLock()
	if cached, ok := p.supplyByID[cardInfo.CardSupplyID]; ok {
		p.supplyMu.RUnlock()
		return cached
	}
	p.supplyMu.RUnlock()

	entity, err := p.client.Cardsupplie.Query().
		Where(cardsupplie.ServerRegionEQ(p.region.String()), cardsupplie.GameIDEQ(int64(cardInfo.CardSupplyID))).
		Only(ctx)
	if err != nil {
		return cardNormalizeSupplyType("")
	}

	value := cardNormalizeSupplyType(entity.CardSupplyType)
	p.supplyMu.Lock()
	p.supplyByID[cardInfo.CardSupplyID] = value
	p.supplyMu.Unlock()
	return value
}

func (p *dbCardProvider) GetGachaByCardID(cardID int) (*masterdata.Gacha, error) {
	return p.getGachaByCardID(nil, cardID)
}

func (p *dbCardProvider) getGachaByCardID(ctx context.Context, cardID int) (*masterdata.Gacha, error) {
	if cardID == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.init()

	p.gachaMu.RLock()
	if cached, ok := p.gachaByCard[cardID]; ok {
		p.gachaMu.RUnlock()
		c := *cached
		return &c, nil
	}
	p.gachaMu.RUnlock()

	cardInfo, err := p.getByID(ctx, cardID)
	if err != nil {
		return nil, err
	}

	candidates, err := p.client.Gacha.Query().
		Where(
			gacha.ServerRegionEQ(p.region.String()),
			gacha.StartAtLTE(cardInfo.ReleaseAt),
			gacha.EndAtGTE(cardInfo.ReleaseAt),
		).
		Order(gacha.ByStartAt()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query gacha by card %d: %w", cardID, err)
	}
	if len(candidates) == 0 {
		candidates, err = p.client.Gacha.Query().
			Where(gacha.ServerRegionEQ(p.region.String())).
			Order(gacha.ByStartAt(sql.OrderDesc())).
			Limit(30).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("query recent gachas: %w", err)
		}
	}

	for _, candidate := range candidates {
		model, err := common.ConvertGachaEntity(candidate)
		if err != nil {
			continue
		}
		if cardContainsPickup(model, cardID) {
			p.gachaMu.Lock()
			p.gachaByCard[cardID] = model
			p.gachaCache[model.ID] = model
			p.gachaMu.Unlock()
			c := *model
			return &c, nil
		}
	}
	return nil, fmt.Errorf("gacha not found for card: %d", cardID)
}

func (p *dbCardProvider) GetCostume3dsByCardID(cardID int) ([]*masterdata.Costume3d, error) {
	return p.getCostume3dsByCardID(nil, cardID)
}

func (p *dbCardProvider) getCostume3dsByCardID(ctx context.Context, cardID int) ([]*masterdata.Costume3d, error) {
	if cardID == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.init()

	p.costumeMu.RLock()
	if cached, ok := p.costumeByCard[cardID]; ok {
		p.costumeMu.RUnlock()
		return common.CloneCostumes(cached), nil
	}
	p.costumeMu.RUnlock()

	links, err := p.client.Cardcostume3D.Query().
		Where(cardcostume3d.ServerRegionEQ(p.region.String()), cardcostume3d.CardIDEQ(int64(cardID))).
		Order(cardcostume3d.ByCostume3DID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query card costumes for card %d: %w", cardID, err)
	}
	if len(links) == 0 {
		return nil, nil
	}

	costumes := make([]*masterdata.Costume3d, 0, len(links))
	for _, link := range links {
		entity, err := p.client.Costume3D.Query().
			Where(costume3d.ServerRegionEQ(p.region.String()), costume3d.GameIDEQ(link.Costume3DID)).
			Only(ctx)
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

	p.costumeMu.Lock()
	p.costumeByCard[cardID] = costumes
	p.costumeMu.Unlock()
	return common.CloneCostumes(costumes), nil
}

func (p *dbCardProvider) GetUnitByCardID(cardID int) (string, error) {
	return p.getUnitByCardID(nil, cardID)
}

func (p *dbCardProvider) getUnitByCardID(ctx context.Context, cardID int) (string, error) {
	cardInfo, err := p.getByID(ctx, cardID)
	if err != nil {
		return "", err
	}
	if p.characters != nil {
		character, cErr := p.characters.getByID(ctx, cardInfo.CharacterID)
		if cErr == nil && character != nil {
			if character.Unit != "" && character.Unit != "piapro" {
				return character.Unit, nil
			}
			if cardInfo.SupportUnit != "" && cardInfo.SupportUnit != "none" {
				return cardInfo.SupportUnit, nil
			}
			return "piapro", nil
		}
	}
	return "", fmt.Errorf("character not found for card %d", cardID)
}

func (p *dbCardProvider) resolveFilterEventCardIDs(ctx context.Context, filter *CardFilter) ([]int64, error) {
	if filter == nil || filter.EventID == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	links, err := p.client.Eventcard.Query().
		Where(eventcard.ServerRegionEQ(p.region.String()), eventcard.EventIDEQ(int64(filter.EventID))).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query event cards for event %d: %w", filter.EventID, err)
	}
	if len(links) == 0 {
		return nil, nil
	}

	result := make([]int64, 0, len(links))
	for _, link := range links {
		result = append(result, link.CardID)
	}
	return result, nil
}

func (p *dbCardProvider) matchesUnitFilter(ctx context.Context, filter *CardFilter, cardInfo *masterdata.Card) bool {
	if filter == nil || cardInfo == nil {
		return false
	}
	if filter.Unit == "" && filter.MainUnit == "" && filter.SupportUnit == "" {
		return true
	}
	if p.characters == nil {
		return false
	}

	character, err := p.characters.getByID(ctx, cardInfo.CharacterID)
	if err != nil || character == nil {
		return false
	}
	return cardMatchesUnitFilter(filter, character.Unit, cardInfo.SupportUnit)
}

// cardJsonFieldEQ creates a predicate that matches a JSONB text field by its
// unquoted string value. Works with PostgreSQL's ->> operator.
func cardJsonFieldEQ(field, value string) predicate.Card {
	return predicate.Card(sql.FieldEQ(field, value))
}

func cardContainsPickup(gachaInfo *masterdata.Gacha, cardID int) bool {
	for _, pickup := range gachaInfo.GachaPickups {
		if pickup.CardID == cardID {
			return true
		}
	}
	return false
}

func cardNormalizeSupportUnit(raw string) string {
	value := cardNormalizeUnit(raw)
	if value == "" {
		return "none"
	}
	return value
}

func cardNormalizeUnit(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "light_sound", "light_sound_club":
		return "light_sound"
	case "idol", "more_more_jump":
		return "idol"
	case "street", "vivid_bad_squad":
		return "street"
	case "theme_park", "wonderlands_x_showtime":
		return "theme_park"
	case "school_refusal", "25_ji_night_cord_de":
		return "school_refusal"
	case "piapro":
		return "piapro"
	case "", "none":
		return ""
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func cardNormalizeSupplyType(raw string) string {
	switch strings.TrimSpace(raw) {
	case "", "normal", "not_limited":
		return "normal"
	case "term_limited":
		return "term_limited"
	case "festival_limited", "colorful_festival_limited":
		return "colorful_festival_limited"
	case "bloom_festival_limited":
		return "bloom_festival_limited"
	case "unit_event_limited":
		return "unit_event_limited"
	case "collaboration_limited":
		return "collaboration_limited"
	case "birthday", "rarity_birthday":
		return "birthday"
	default:
		return strings.TrimSpace(raw)
	}
}

func cardMatchesSupplyFilter(filter, raw string) bool {
	switch cardNormalizeSupplyType(raw) {
	case "colorful_festival_limited", "bloom_festival_limited":
		if filter == "festival" || filter == "limited" {
			return true
		}
		return cardNormalizeSupplyType(filter) == cardNormalizeSupplyType(raw)
	case "term_limited", "unit_event_limited", "collaboration_limited":
		if filter == "limited" {
			return true
		}
		return cardNormalizeSupplyType(filter) == cardNormalizeSupplyType(raw)
	case "birthday":
		return filter == "birthday"
	case "normal":
		return filter == "normal"
	default:
		return false
	}
}
