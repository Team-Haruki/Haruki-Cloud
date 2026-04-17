package provider

import (
	"context"
	"encoding/json"
	"slices"

	"haruki-cloud/database/sekai/mysekaigatelevel"
	"haruki-cloud/database/sekai/shopitem"
)

func (p *dbEducationProvider) GetMysekaiGateLevel(ctx context.Context, gateID, level int) *MysekaiGateLevel {
	if gateID <= 0 || level <= 0 || !p.ensureGateLevelsLoaded(ctx) {
		return nil
	}

	p.gateMu.RLock()
	defer p.gateMu.RUnlock()
	if levels, ok := p.gateByID[gateID]; ok {
		return cloneEdMysekaiGateLevel(levels[level])
	}
	return nil
}

func (p *dbEducationProvider) GetShopItemByResourceBoxID(ctx context.Context, resourceBoxID int) *ShopItem {
	if resourceBoxID <= 0 || !p.ensureShopItemsLoaded(ctx) {
		return nil
	}

	p.shopMu.RLock()
	defer p.shopMu.RUnlock()
	return cloneEdShopItem(p.shopByBoxID[resourceBoxID])
}

func (p *dbEducationProvider) GetShopItems(ctx context.Context) []*ShopItem {
	if !p.ensureShopItemsLoaded(ctx) {
		return nil
	}

	p.shopMu.RLock()
	defer p.shopMu.RUnlock()
	return cloneEdShopItems(p.shopItems)
}

func (p *dbEducationProvider) ensureGateLevelsLoaded(ctx context.Context) bool {
	p.init()
	p.gateMu.RLock()
	if p.gatesLoaded {
		p.gateMu.RUnlock()
		return true
	}
	p.gateMu.RUnlock()

	p.gateMu.Lock()
	defer p.gateMu.Unlock()

	if p.gatesLoaded {
		return true
	}

	items, err := p.client.Mysekaigatelevel.Query().
		Where(mysekaigatelevel.ServerRegionEQ(p.region.String())).
		All(ctx)
	if err != nil {
		return false
	}
	for _, item := range items {
		level := &MysekaiGateLevel{
			GateID:         int(item.MysekaiGateID),
			Level:          int(item.Level),
			PowerBonusRate: item.PowerBonusRate,
		}
		if _, ok := p.gateByID[level.GateID]; !ok {
			p.gateByID[level.GateID] = make(map[int]*MysekaiGateLevel)
		}
		p.gateByID[level.GateID][level.Level] = level
	}
	p.gatesLoaded = true
	return true
}

func (p *dbEducationProvider) ensureShopItemsLoaded(ctx context.Context) bool {
	p.init()
	p.shopMu.RLock()
	if p.shopsLoaded {
		p.shopMu.RUnlock()
		return true
	}
	p.shopMu.RUnlock()

	p.shopMu.Lock()
	defer p.shopMu.Unlock()

	if p.shopsLoaded {
		return true
	}

	items, err := p.client.Shopitem.Query().
		Where(shopitem.ServerRegionEQ(p.region.String())).
		Order(shopitem.ByShopID(), shopitem.BySeq(), shopitem.ByGameID()).
		All(ctx)
	if err != nil {
		return false
	}
	for _, item := range items {
		shopEntry := &ShopItem{
			ID:                 int(item.GameID),
			ShopID:             int(item.ShopID),
			Seq:                int(item.Seq),
			ResourceBoxID:      int(item.ResourceBoxID),
			ReleaseConditionID: int(item.ReleaseConditionID),
			StartAt:            item.StartAt,
		}
		if len(item.Costs) > 0 {
			var rawCosts []struct {
				Cost ShopItemCost `json:"cost"`
			}
			if err := json.Unmarshal(item.Costs, &rawCosts); err == nil {
				shopEntry.Costs = make([]ShopItemCost, 0, len(rawCosts))
				for _, raw := range rawCosts {
					shopEntry.Costs = append(shopEntry.Costs, raw.Cost)
				}
			}
		}
		p.shopByBoxID[shopEntry.ResourceBoxID] = shopEntry
		p.shopItems = append(p.shopItems, shopEntry)
	}
	p.shopsLoaded = true
	return true
}

func cloneEdMysekaiGateLevel(source *MysekaiGateLevel) *MysekaiGateLevel {
	if source == nil {
		return nil
	}
	c := *source
	return &c
}

func cloneEdShopItem(source *ShopItem) *ShopItem {
	if source == nil {
		return nil
	}
	c := *source
	c.Costs = slices.Clone(source.Costs)
	return &c
}

func cloneEdShopItems(source []*ShopItem) []*ShopItem {
	if len(source) == 0 {
		return nil
	}
	out := make([]*ShopItem, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		out = append(out, cloneEdShopItem(item))
	}
	return out
}
