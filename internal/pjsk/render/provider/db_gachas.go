package provider

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
	renderregion "haruki-cloud/internal/pjsk/region"
)

type dbGachaProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
	once   sync.Once

	gachaMu    sync.RWMutex
	gachaCache map[int]*masterdata.Gacha
	gachas     []*masterdata.Gacha

	cardMu    sync.RWMutex
	cardCache map[int]*masterdata.Card
}

func (p *dbGachaProvider) init() {
	p.once.Do(func() {
		p.gachaCache = make(map[int]*masterdata.Gacha)
		p.cardCache = make(map[int]*masterdata.Card)
	})
}

func (p *dbGachaProvider) GetByID(ctx context.Context, id int) (*masterdata.Gacha, error) {
	if id == 0 {
		return nil, fmt.Errorf("gacha id is required")
	}
	p.init()

	p.gachaMu.RLock()
	if cached, ok := p.gachaCache[id]; ok {
		p.gachaMu.RUnlock()
		return common.CloneGacha(cached), nil
	}
	p.gachaMu.RUnlock()

	entity, err := p.client.Gacha.Query().
		Where(gachaent.ServerRegionEQ(p.region.String()), gachaent.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query gacha %d: %w", id, err)
	}

	model, err := common.ConvertGachaEntity(entity)
	if err != nil {
		return nil, err
	}

	p.gachaMu.Lock()
	p.gachaCache[id] = model
	p.gachaMu.Unlock()
	return common.CloneGacha(model), nil
}

func (p *dbGachaProvider) GetAll(ctx context.Context) []*masterdata.Gacha {
	p.init()

	p.gachaMu.RLock()
	if len(p.gachas) > 0 {
		defer p.gachaMu.RUnlock()
		return common.CloneGachaList(p.gachas)
	}
	p.gachaMu.RUnlock()

	entities, err := p.client.Gacha.Query().
		Where(gachaent.ServerRegionEQ(p.region.String())).
		All(ctx)
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

	p.gachaMu.Lock()
	p.gachas = items
	for id, item := range byID {
		p.gachaCache[id] = item
	}
	p.gachaMu.Unlock()

	return common.CloneGachaList(items)
}

func (p *dbGachaProvider) GetCardByID(ctx context.Context, id int) (*masterdata.Card, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	p.init()

	p.cardMu.RLock()
	if cached, ok := p.cardCache[id]; ok {
		p.cardMu.RUnlock()
		c := *cached
		return &c, nil
	}
	p.cardMu.RUnlock()

	entity, err := p.client.Card.Query().
		Where(card.ServerRegionEQ(p.region.String()), card.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query card %d: %w", id, err)
	}

	model := &masterdata.Card{
		ID:                              int(entity.GameID),
		CharacterID:                     int(entity.CharacterID),
		CardRarityType:                  entity.CardRarityType,
		Attr:                            entity.Attr,
		Prefix:                          entity.Prefix,
		AssetBundleName:                 entity.AssetbundleName,
		ReleaseAt:                       entity.ReleaseAt,
		SkillID:                         int(entity.SkillID),
		CardSkillName:                   entity.CardSkillName,
		SupportUnit:                     entity.SupportUnit,
		SpecialTrainingPower1BonusFixed: int(entity.SpecialTrainingPower1BonusFixed),
		SpecialTrainingPower2BonusFixed: int(entity.SpecialTrainingPower2BonusFixed),
		SpecialTrainingPower3BonusFixed: int(entity.SpecialTrainingPower3BonusFixed),
		SpecialTrainingSkillID:          int(entity.SpecialTrainingSkillID),
		SpecialTrainingSkillName:        entity.SpecialTrainingSkillName,
		CardSupplyID:                    int(entity.CardSupplyID),
	}

	p.cardMu.Lock()
	p.cardCache[id] = model
	p.cardMu.Unlock()

	c := *model
	return &c, nil
}
