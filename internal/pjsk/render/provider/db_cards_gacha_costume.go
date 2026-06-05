package provider

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"

	"haruki-cloud/database/sekai/cardcostume3d"
	"haruki-cloud/database/sekai/costume3d"
	"haruki-cloud/database/sekai/gacha"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (p *dbCardProvider) GetGachaByCardID(ctx context.Context, cardID int) (*masterdata.Gacha, error) {
	if cardID == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	p.init()

	p.gachaMu.RLock()
	if cached, ok := p.gachaByCard[cardID]; ok {
		p.gachaMu.RUnlock()
		return new(*cached), nil
	}
	p.gachaMu.RUnlock()

	cardInfo, err := p.GetByID(ctx, cardID)
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
			return new(*model), nil
		}
	}
	return nil, fmt.Errorf("gacha not found for card: %d", cardID)
}

func (p *dbCardProvider) GetCostume3dsByCardID(ctx context.Context, cardID int) ([]*masterdata.Costume3d, error) {
	if cardID == 0 {
		return nil, nil
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
		if costumeInfo := common.ConvertCostumeEntity(entity); costumeInfo != nil {
			costumes = append(costumes, costumeInfo)
		}
	}
	if len(costumes) == 0 {
		return nil, nil
	}

	p.costumeMu.Lock()
	p.costumeByCard[cardID] = costumes
	p.costumeMu.Unlock()
	return common.CloneCostumes(costumes), nil
}

func cardContainsPickup(gachaInfo *masterdata.Gacha, cardID int) bool {
	for _, pickup := range gachaInfo.GachaPickups {
		if pickup.CardID == cardID {
			return true
		}
	}
	return false
}
