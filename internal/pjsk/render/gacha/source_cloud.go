package gacha

import (
	"context"
	"fmt"
	"sort"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/card"
	gachaent "haruki-cloud/database/sekai/gacha"
	"haruki-cloud/internal/pjsk/render/common"
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
		return common.CloneGacha(cached), nil
	}
	c.gachaMu.RUnlock()

	entity, err := c.client.Gacha.Query().
		Where(gachaent.ServerRegionEQ(c.queryRegion.String()), gachaent.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query gacha %d failed: %w", id, err)
	}

	model, err := common.ConvertGachaEntity(entity)
	if err != nil {
		return nil, err
	}

	c.gachaMu.Lock()
	c.gachaCache[id] = model
	c.gachaMu.Unlock()
	return common.CloneGacha(model), nil
}

func (c *CloudSource) GetGachas() []*masterdata.Gacha {
	c.gachaMu.RLock()
	if len(c.gachas) > 0 {
		defer c.gachaMu.RUnlock()
		return common.CloneGachaList(c.gachas)
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
		model, convErr := common.ConvertGachaEntity(entity)
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

	return common.CloneGachaList(items)
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
			result[idx] = common.CloneCard(cached)
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
		model, err := common.ConvertCardEntity(entity)
		if err != nil {
			c.cardMu.Unlock()
			return nil, err
		}
		c.cardCache[model.ID] = model
		if idx, ok := missingIndex[model.ID]; ok {
			result[idx] = common.CloneCard(model)
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
