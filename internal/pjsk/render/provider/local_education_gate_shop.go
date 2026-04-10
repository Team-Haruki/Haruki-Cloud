package provider

import (
	"context"
	"encoding/json"
)

func (p *localEducationProvider) ensureGateLevels() error {
	return p.gates.init(func() (map[int]map[int]*MysekaiGateLevel, error) {
		items, err := loadJSON[localMysekaiGateLevelJSON](p.store, "mysekaiGateLevels.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]map[int]*MysekaiGateLevel)
		for _, item := range items {
			level := &MysekaiGateLevel{
				GateID:         item.MysekaiGateID,
				Level:          item.Level,
				PowerBonusRate: item.PowerBonusRate,
			}
			if _, ok := byID[level.GateID]; !ok {
				byID[level.GateID] = make(map[int]*MysekaiGateLevel)
			}
			byID[level.GateID][level.Level] = level
		}
		return byID, nil
	})
}

func (p *localEducationProvider) ensureShopItems() error {
	return p.shops.init(func() (map[int]*ShopItem, error) {
		items, err := loadJSON[localShopItemJSON](p.store, "shopItems.json")
		if err != nil {
			return nil, err
		}
		byBoxID := make(map[int]*ShopItem, len(items))
		for _, item := range items {
			entry := &ShopItem{
				ID:            item.ID,
				ResourceBoxID: item.ResourceBoxID,
			}
			if len(item.Costs) > 0 {
				var rawCosts []struct {
					Cost ShopItemCost `json:"cost"`
				}
				if err := json.Unmarshal(item.Costs, &rawCosts); err == nil {
					entry.Costs = make([]ShopItemCost, 0, len(rawCosts))
					for _, raw := range rawCosts {
						entry.Costs = append(entry.Costs, raw.Cost)
					}
				}
			}
			byBoxID[entry.ResourceBoxID] = entry
		}
		return byBoxID, nil
	})
}

func (p *localEducationProvider) GetMysekaiGateLevel(_ context.Context, gateID, level int) *MysekaiGateLevel {
	if gateID <= 0 || level <= 0 {
		return nil
	}
	if err := p.ensureGateLevels(); err != nil {
		return nil
	}
	if levels, ok := p.gates.v()[gateID]; ok {
		return cloneEdMysekaiGateLevel(levels[level])
	}
	return nil
}

func (p *localEducationProvider) GetShopItemByResourceBoxID(_ context.Context, resourceBoxID int) *ShopItem {
	if resourceBoxID <= 0 {
		return nil
	}
	if err := p.ensureShopItems(); err != nil {
		return nil
	}
	return cloneEdShopItem(p.shops.v()[resourceBoxID])
}
