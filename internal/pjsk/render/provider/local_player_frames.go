package provider

import (
	"fmt"
	"sync"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localPlayerFrameProvider
// ===========================================================================

type localPlayerFrameProvider struct {
	store *localStore

	frameOnce sync.Once
	frameByID map[int]*masterdata.PlayerFrame
	frameErr  error

	groupOnce sync.Once
	groupByID map[int]*masterdata.PlayerFrameGroup
	groupErr  error
}

func (p *localPlayerFrameProvider) ensureFrames() error {
	p.frameOnce.Do(func() {
		items, err := loadJSON[masterdata.PlayerFrame](p.store, "playerFrames.json")
		if err != nil {
			p.frameErr = err
			return
		}
		p.frameByID = make(map[int]*masterdata.PlayerFrame, len(items))
		for i := range items {
			p.frameByID[items[i].ID] = &items[i]
		}
	})
	return p.frameErr
}

func (p *localPlayerFrameProvider) ensureGroups() error {
	p.groupOnce.Do(func() {
		items, err := loadJSON[masterdata.PlayerFrameGroup](p.store, "playerFrameGroups.json")
		if err != nil {
			p.groupErr = err
			return
		}
		p.groupByID = make(map[int]*masterdata.PlayerFrameGroup, len(items))
		for i := range items {
			p.groupByID[items[i].ID] = &items[i]
		}
	})
	return p.groupErr
}

func (p *localPlayerFrameProvider) GetByID(id int) (*masterdata.PlayerFrame, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid player frame id")
	}
	if err := p.ensureFrames(); err != nil {
		return nil, err
	}
	f, ok := p.frameByID[id]
	if !ok {
		return nil, fmt.Errorf("player frame %d not found", id)
	}
	c := *f
	return &c, nil
}

func (p *localPlayerFrameProvider) GetGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid player frame group id")
	}
	if err := p.ensureGroups(); err != nil {
		return nil, err
	}
	g, ok := p.groupByID[id]
	if !ok {
		return nil, fmt.Errorf("player frame group %d not found", id)
	}
	c := *g
	return &c, nil
}
