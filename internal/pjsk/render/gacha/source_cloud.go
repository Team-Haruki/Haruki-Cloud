package gacha

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/card"
	gachaent "haruki-cloud/database/sekai/gacha"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type CloudSource struct {
	client      *sekaiDB.Client
	region      renderregion.Value
	queryRegion renderregion.Value

	gachaMu    sync.RWMutex
	gachaCache map[int]*masterdata.Gacha
	gachas     []*masterdata.Gacha

	cardMu    sync.RWMutex
	cardCache map[int]*masterdata.Card
}

func NewCloudSource(client *sekaiDB.Client, defaultRegion renderregion.Value) *CloudSource {
	if client == nil {
		return nil
	}
	region := renderregion.WithDefault(defaultRegion)
	return &CloudSource{
		client:      client,
		region:      region,
		queryRegion: region,
		gachaCache:  make(map[int]*masterdata.Gacha),
		cardCache:   make(map[int]*masterdata.Card),
	}
}

func (c *CloudSource) DefaultRegion() renderregion.Value {
	return c.region
}

func (c *CloudSource) GetGachaByID(id int) (*masterdata.Gacha, error) {
	if id == 0 {
		return nil, fmt.Errorf("gacha id is required")
	}

	c.gachaMu.RLock()
	if cached, ok := c.gachaCache[id]; ok {
		c.gachaMu.RUnlock()
		return cloneGacha(cached), nil
	}
	c.gachaMu.RUnlock()

	entity, err := c.client.Gacha.Query().
		Where(gachaent.ServerRegionEQ(c.queryRegion.String()), gachaent.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query gacha %d failed: %w", id, err)
	}

	model, err := convertGachaEntity(entity)
	if err != nil {
		return nil, err
	}

	c.gachaMu.Lock()
	c.gachaCache[id] = model
	c.gachaMu.Unlock()
	return cloneGacha(model), nil
}

func (c *CloudSource) GetGachas() []*masterdata.Gacha {
	c.gachaMu.RLock()
	if len(c.gachas) > 0 {
		defer c.gachaMu.RUnlock()
		return cloneGachaList(c.gachas)
	}
	c.gachaMu.RUnlock()

	entities, err := c.client.Gacha.Query().
		Where(gachaent.ServerRegionEQ(c.queryRegion.String())).
		All(context.Background())
	if err != nil {
		return nil
	}

	items := make([]*masterdata.Gacha, 0, len(entities))
	byID := make(map[int]*masterdata.Gacha, len(entities))
	for _, entity := range entities {
		model, convErr := convertGachaEntity(entity)
		if convErr != nil {
			continue
		}
		items = append(items, model)
		byID[model.ID] = model
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].StartAt == items[j].StartAt {
			return items[i].ID > items[j].ID
		}
		return items[i].StartAt > items[j].StartAt
	})

	c.gachaMu.Lock()
	c.gachas = items
	for id, item := range byID {
		c.gachaCache[id] = item
	}
	c.gachaMu.Unlock()

	return cloneGachaList(items)
}

func (c *CloudSource) GetCardByID(id int) (*masterdata.Card, error) {
	items, err := c.getCardsByIDs([]int{id})
	if err != nil {
		return nil, err
	}
	return items[0], nil
}

func (c *CloudSource) getCardsByIDs(ids []int) ([]*masterdata.Card, error) {
	result := make([]*masterdata.Card, len(ids))
	var missing []int64
	missingIndex := make(map[int]int)

	c.cardMu.RLock()
	for idx, id := range ids {
		if cached, ok := c.cardCache[id]; ok {
			result[idx] = cloneCard(cached)
			continue
		}
		missing = append(missing, int64(id))
		missingIndex[id] = idx
	}
	c.cardMu.RUnlock()

	if len(missing) == 0 {
		return result, nil
	}

	entities, err := c.client.Card.Query().
		Where(card.ServerRegionEQ(c.queryRegion.String()), card.GameIDIn(missing...)).
		All(context.Background())
	if err != nil {
		return nil, err
	}

	c.cardMu.Lock()
	for _, entity := range entities {
		model := convertCardEntity(entity)
		c.cardCache[model.ID] = model
		if idx, ok := missingIndex[model.ID]; ok {
			result[idx] = cloneCard(model)
		}
	}
	c.cardMu.Unlock()

	for idx, item := range result {
		if item == nil {
			return nil, fmt.Errorf("card not found for id %d", ids[idx])
		}
	}
	return result, nil
}

func convertGachaEntity(entity *sekaiDB.Gacha) (*masterdata.Gacha, error) {
	rarityRates, err := decodeSlice[masterdata.GachaCardRarityRate](entity.GachaCardRarityRates)
	if err != nil {
		return nil, fmt.Errorf("decode gacha rarity rates: %w", err)
	}
	details, err := decodeSlice[masterdata.GachaDetail](entity.GachaDetails)
	if err != nil {
		return nil, fmt.Errorf("decode gacha details: %w", err)
	}
	behaviors, err := decodeSlice[masterdata.GachaBehavior](entity.GachaBehaviors)
	if err != nil {
		return nil, fmt.Errorf("decode gacha behaviors: %w", err)
	}
	pickups, err := decodeSlice[masterdata.GachaPickup](entity.GachaPickups)
	if err != nil {
		return nil, fmt.Errorf("decode gacha pickups: %w", err)
	}
	information, err := decodeMap[masterdata.GachaInformation](entity.GachaInformation)
	if err != nil {
		return nil, fmt.Errorf("decode gacha information: %w", err)
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
		GachaCardRarityRates:   rarityRates,
		GachaDetails:           details,
		GachaBehaviors:         behaviors,
		GachaPickups:           pickups,
		GachaInformation:       information,
	}, nil
}

func convertCardEntity(entity *sekaiDB.Card) *masterdata.Card {
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
		SpecialTrainingPower1BonusFixed: int(entity.SpecialTrainingPower1BonusFixed),
		SpecialTrainingPower2BonusFixed: int(entity.SpecialTrainingPower2BonusFixed),
		SpecialTrainingPower3BonusFixed: int(entity.SpecialTrainingPower3BonusFixed),
		SpecialTrainingSkillID:          int(entity.SpecialTrainingSkillID),
		SpecialTrainingSkillName:        entity.SpecialTrainingSkillName,
		CardSupplyID:                    int(entity.CardSupplyID),
	}
}

func decodeSlice[T any](raw json.RawMessage) ([]T, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var items []T
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func decodeMap[T any](raw json.RawMessage) (T, error) {
	var result T
	if len(raw) == 0 {
		return result, nil
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	return result, nil
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

func cloneGacha(item *masterdata.Gacha) *masterdata.Gacha {
	if item == nil {
		return nil
	}
	copy := *item
	if len(item.GachaCardRarityRates) > 0 {
		copy.GachaCardRarityRates = append([]masterdata.GachaCardRarityRate(nil), item.GachaCardRarityRates...)
	}
	if len(item.GachaPickups) > 0 {
		copy.GachaPickups = append([]masterdata.GachaPickup(nil), item.GachaPickups...)
	}
	if len(item.GachaDetails) > 0 {
		copy.GachaDetails = append([]masterdata.GachaDetail(nil), item.GachaDetails...)
	}
	if len(item.GachaBehaviors) > 0 {
		copy.GachaBehaviors = append([]masterdata.GachaBehavior(nil), item.GachaBehaviors...)
	}
	return &copy
}

func cloneGachaList(items []*masterdata.Gacha) []*masterdata.Gacha {
	if len(items) == 0 {
		return nil
	}
	result := make([]*masterdata.Gacha, 0, len(items))
	for _, item := range items {
		result = append(result, cloneGacha(item))
	}
	return result
}

func cloneCard(item *masterdata.Card) *masterdata.Card {
	if item == nil {
		return nil
	}
	copy := *item
	return &copy
}
