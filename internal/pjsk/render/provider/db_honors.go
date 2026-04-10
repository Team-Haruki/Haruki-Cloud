package provider

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/bondshonor"
	"haruki-cloud/database/sekai/gamecharacterunit"
	sekaiHonor "haruki-cloud/database/sekai/honor"
	"haruki-cloud/database/sekai/honorgroup"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type dbHonorProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
	once   sync.Once

	honorMu    sync.RWMutex
	honorCache map[int]*masterdata.Honor

	groupMu    sync.RWMutex
	groupCache map[int]*masterdata.HonorGroup

	bondsMu    sync.RWMutex
	bondsCache map[int]*masterdata.BondsHonor

	gcuMu    sync.RWMutex
	gcuCache map[int]*masterdata.GameCharacterUnit

	birthdayMu      sync.RWMutex
	birthdayByGroup map[int]honorBirthdayAssets
	birthdayChars   []*sekaiDB.Gamecharacter
	birthdayLoaded  bool

	eventHonorMu       sync.RWMutex
	eventByHonorID     map[int]int
	eventByHonorLoaded bool
}

type honorBirthdayAssets struct {
	background string
	frame      string
}

func (p *dbHonorProvider) init() {
	p.once.Do(func() {
		p.honorCache = make(map[int]*masterdata.Honor)
		p.groupCache = make(map[int]*masterdata.HonorGroup)
		p.bondsCache = make(map[int]*masterdata.BondsHonor)
		p.gcuCache = make(map[int]*masterdata.GameCharacterUnit)
		p.birthdayByGroup = make(map[int]honorBirthdayAssets)
		p.eventByHonorID = make(map[int]int)
	})
}

func (p *dbHonorProvider) GetByID(ctx context.Context, id int) (*masterdata.Honor, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid honor id")
	}
	p.init()

	p.honorMu.RLock()
	if cached, ok := p.honorCache[id]; ok {
		p.honorMu.RUnlock()
		return common.CloneHonor(cached), nil
	}
	p.honorMu.RUnlock()

	entity, err := p.client.Honor.Query().
		Where(sekaiHonor.ServerRegionEQ(p.region.String()), sekaiHonor.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query honor %d: %w", id, err)
	}
	model, err := convertCloudHonor(entity)
	if err != nil {
		return nil, err
	}

	p.honorMu.Lock()
	p.honorCache[id] = model
	p.honorMu.Unlock()
	return common.CloneHonor(model), nil
}

func (p *dbHonorProvider) GetGroupByID(ctx context.Context, id int) (*masterdata.HonorGroup, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid honor group id")
	}
	p.init()

	p.groupMu.RLock()
	if cached, ok := p.groupCache[id]; ok {
		p.groupMu.RUnlock()
		return common.CloneHonorGroup(cached), nil
	}
	p.groupMu.RUnlock()

	entity, err := p.client.Honorgroup.Query().
		Where(honorgroup.ServerRegionEQ(p.region.String()), honorgroup.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query honor group %d: %w", id, err)
	}

	model := &masterdata.HonorGroup{
		ID:          int(entity.GameID),
		HonorType:   entity.HonorType,
		Name:        entity.Name,
		Description: "",
	}
	if value := entity.BackgroundAssetbundleName; value != "" {
		model.BackgroundAssetBundleName = &value
	}
	if value := entity.FrameName; value != "" {
		model.FrameName = &value
	}
	if model.HonorType == "birthday" && (model.BackgroundAssetBundleName == nil || model.FrameName == nil) {
		if derived, ok := p.deriveBirthdayAssetsForGroup(ctx, int(entity.GameID), model.Name); ok {
			if model.BackgroundAssetBundleName == nil && derived.background != "" {
				value := derived.background
				model.BackgroundAssetBundleName = &value
			}
			if model.FrameName == nil && derived.frame != "" {
				value := derived.frame
				model.FrameName = &value
			}
			slog.Info(
				"honor birthday derive trace",
				"group_id", entity.GameID,
				"group_name", model.Name,
				"derived_background_assetbundle_name", derived.background,
				"derived_frame_name", derived.frame,
			)
		}
	}

	p.groupMu.Lock()
	p.groupCache[id] = model
	p.groupMu.Unlock()
	return common.CloneHonorGroup(model), nil
}

func (p *dbHonorProvider) GetBondsHonorByID(ctx context.Context, id int) (*masterdata.BondsHonor, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid bonds honor id")
	}
	p.init()

	p.bondsMu.RLock()
	if cached, ok := p.bondsCache[id]; ok {
		p.bondsMu.RUnlock()
		return common.CloneBondsHonor(cached), nil
	}
	p.bondsMu.RUnlock()

	entity, err := p.client.Bondshonor.Query().
		Where(bondshonor.ServerRegionEQ(p.region.String()), bondshonor.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query bonds honor %d: %w", id, err)
	}
	model := &masterdata.BondsHonor{
		ID:                   int(entity.GameID),
		GameCharacterUnitID1: int(entity.GameCharacterUnitId1),
		GameCharacterUnitID2: int(entity.GameCharacterUnitId2),
		HonorRarity:          entity.HonorRarity,
		Name:                 entity.Name,
		Description:          entity.Description,
		BondsGroupID:         int(entity.BondsGroupID),
	}

	p.bondsMu.Lock()
	p.bondsCache[id] = model
	p.bondsMu.Unlock()
	return common.CloneBondsHonor(model), nil
}

func (p *dbHonorProvider) GetGameCharacterUnitByID(ctx context.Context, id int) (*masterdata.GameCharacterUnit, bool) {
	if id == 0 {
		return nil, false
	}
	p.init()

	p.gcuMu.RLock()
	if cached, ok := p.gcuCache[id]; ok {
		p.gcuMu.RUnlock()
		return common.CloneGameCharacterUnit(cached), true
	}
	p.gcuMu.RUnlock()

	entity, err := p.client.Gamecharacterunit.Query().
		Where(gamecharacterunit.ServerRegionEQ(p.region.String()), gamecharacterunit.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, false
	}
	model := &masterdata.GameCharacterUnit{
		ID:              int(entity.GameID),
		GameCharacterID: int(entity.GameCharacterID),
		Unit:            entity.Unit,
		ColorCode:       entity.ColorCode,
	}

	p.gcuMu.Lock()
	p.gcuCache[id] = model
	p.gcuMu.Unlock()
	return common.CloneGameCharacterUnit(model), true
}
