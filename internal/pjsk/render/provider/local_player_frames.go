package provider

import (
	"context"
	"fmt"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localPlayerFrameProvider
// ===========================================================================

type localPlayerFrameProvider struct {
	store  *localStore
	frames lazyValue[map[int]*masterdata.PlayerFrame]
	groups lazyValue[map[int]*masterdata.PlayerFrameGroup]
}

func (p *localPlayerFrameProvider) ensureFrames() error {
	return p.frames.init(func() (map[int]*masterdata.PlayerFrame, error) {
		items, err := loadJSON[masterdata.PlayerFrame](p.store, "playerFrames.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]*masterdata.PlayerFrame, len(items))
		for i := range items {
			byID[items[i].ID] = &items[i]
		}
		return byID, nil
	})
}

func (p *localPlayerFrameProvider) ensureGroups() error {
	return p.groups.init(func() (map[int]*masterdata.PlayerFrameGroup, error) {
		items, err := loadJSON[masterdata.PlayerFrameGroup](p.store, "playerFrameGroups.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]*masterdata.PlayerFrameGroup, len(items))
		for i := range items {
			byID[items[i].ID] = &items[i]
		}
		return byID, nil
	})
}

func (p *localPlayerFrameProvider) GetByID(_ context.Context, id int) (*masterdata.PlayerFrame, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid player frame id")
	}
	if err := p.ensureFrames(); err != nil {
		return nil, err
	}
	f, ok := p.frames.v()[id]
	if !ok {
		return nil, fmt.Errorf("player frame %d not found", id)
	}
	c := *f
	return &c, nil
}

func (p *localPlayerFrameProvider) GetGroupByID(_ context.Context, id int) (*masterdata.PlayerFrameGroup, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid player frame group id")
	}
	if err := p.ensureGroups(); err != nil {
		return nil, err
	}
	g, ok := p.groups.v()[id]
	if !ok {
		return nil, fmt.Errorf("player frame group %d not found", id)
	}
	c := *g
	return &c, nil
}
