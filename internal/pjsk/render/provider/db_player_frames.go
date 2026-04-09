package provider

import (
	"context"
	"fmt"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/playerframe"
	"haruki-cloud/database/sekai/playerframegroup"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type dbPlayerFrameProvider struct {
	client *sekaiDB.Client
	region renderregion.Value

	frameMu    sync.RWMutex
	frameCache map[int]*masterdata.PlayerFrame

	groupMu    sync.RWMutex
	groupCache map[int]*masterdata.PlayerFrameGroup
}

func (p *dbPlayerFrameProvider) init() {
	if p.frameCache == nil {
		p.frameCache = make(map[int]*masterdata.PlayerFrame)
	}
	if p.groupCache == nil {
		p.groupCache = make(map[int]*masterdata.PlayerFrameGroup)
	}
}

func (p *dbPlayerFrameProvider) GetByID(id int) (*masterdata.PlayerFrame, error) {
	return p.getByID(nil, id)
}

func (p *dbPlayerFrameProvider) getByID(ctx context.Context, id int) (*masterdata.PlayerFrame, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid player frame id")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.init()

	p.frameMu.RLock()
	if cached, ok := p.frameCache[id]; ok {
		p.frameMu.RUnlock()
		c := *cached
		return &c, nil
	}
	p.frameMu.RUnlock()

	entity, err := p.client.Playerframe.Query().
		Where(playerframe.ServerRegionEQ(p.region.String()), playerframe.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query player frame %d: %w", id, err)
	}

	model := &masterdata.PlayerFrame{
		ID:                 int(entity.GameID),
		Seq:                int(entity.Seq),
		PlayerFrameGroupID: int(entity.PlayerFrameGroupID),
		Description:        entity.Description,
		GameCharacterID:    int(entity.GameCharacterID),
	}

	p.frameMu.Lock()
	p.frameCache[id] = model
	p.frameMu.Unlock()

	c := *model
	return &c, nil
}

func (p *dbPlayerFrameProvider) GetGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	return p.getGroupByID(nil, id)
}

func (p *dbPlayerFrameProvider) getGroupByID(ctx context.Context, id int) (*masterdata.PlayerFrameGroup, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid player frame group id")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.init()

	p.groupMu.RLock()
	if cached, ok := p.groupCache[id]; ok {
		p.groupMu.RUnlock()
		c := *cached
		return &c, nil
	}
	p.groupMu.RUnlock()

	entity, err := p.client.Playerframegroup.Query().
		Where(playerframegroup.ServerRegionEQ(p.region.String()), playerframegroup.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query player frame group %d: %w", id, err)
	}

	model := &masterdata.PlayerFrameGroup{
		ID:              int(entity.GameID),
		Seq:             int(entity.Seq),
		Name:            entity.Name,
		AssetBundleName: entity.AssetbundleName,
	}

	p.groupMu.Lock()
	p.groupCache[id] = model
	p.groupMu.Unlock()

	c := *model
	return &c, nil
}
