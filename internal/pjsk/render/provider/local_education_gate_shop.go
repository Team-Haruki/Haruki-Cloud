package provider

import (
	"context"
	json "haruki-cloud/internal/jsonutil"
)

func (p *localEducationProvider) ensureGateLevels() error {
	return p.gates.init(func() (map[int]map[int]*MysekaiGateLevel, error) {
		items, err := p.store.loadJSON[localMysekaiGateLevelJSON]("mysekaiGateLevels.json")
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
	return p.shops.init(func() (shopIndex, error) {
		items, err := p.store.loadJSON[localShopItemJSON]("shopItems.json")
		if err != nil {
			return shopIndex{}, err
		}
		index := shopIndex{
			byBoxID: make(map[int]*ShopItem, len(items)),
			all:     make([]*ShopItem, 0, len(items)),
		}
		for _, item := range items {
			entry := &ShopItem{
				ID:                 item.ID,
				ShopID:             item.ShopID,
				Seq:                item.Seq,
				ResourceBoxID:      item.ResourceBoxID,
				ReleaseConditionID: item.ReleaseConditionID,
				StartAt:            item.StartAt,
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
			index.byBoxID[entry.ResourceBoxID] = entry
			index.all = append(index.all, entry)
		}
		return index, nil
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
	return cloneEdShopItem(p.shops.v().byBoxID[resourceBoxID])
}

func (p *localEducationProvider) GetShopItems(_ context.Context) []*ShopItem {
	if err := p.ensureShopItems(); err != nil {
		return nil
	}
	return cloneEdShopItems(p.shops.v().all)
}
