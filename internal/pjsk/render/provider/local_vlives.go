package provider

import (
	"encoding/json"
	"sort"
	"sync"

	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// ===========================================================================
// localVLiveProvider
// ===========================================================================

type localVLiveProvider struct {
	store *localStore

	once  sync.Once
	lives []*VLive
	err   error
}

func (p *localVLiveProvider) ensureLoaded() error {
	p.once.Do(func() {
		items, err := loadJSON[localVirtualLiveJSON](p.store, "virtualLives.json")
		if err != nil {
			p.err = err
			return
		}
		p.lives = make([]*VLive, 0, len(items))
		for _, item := range items {
			live := &VLive{
				ID:      item.ID,
				Name:    item.Name,
				StartAt: item.StartAt,
				EndAt:   item.EndAt,
			}
			var schedules []map[string]interface{}
			if len(item.VirtualLiveSchedules) > 0 {
				_ = json.Unmarshal(item.VirtualLiveSchedules, &schedules)
			}
			for _, s := range schedules {
				startAt := vliveInt64Number(s["startAt"])
				endAt := vliveInt64Number(s["endAt"])
				if startAt <= 0 || endAt <= 0 {
					continue
				}
				live.Schedules = append(live.Schedules, VLiveSchedule{
					StartAt: startAt,
					EndAt:   endAt,
				})
			}
			p.lives = append(p.lives, live)
		}
		sort.Slice(p.lives, func(i, j int) bool {
			if p.lives[i].StartAt == p.lives[j].StartAt {
				return p.lives[i].ID < p.lives[j].ID
			}
			return p.lives[i].StartAt < p.lives[j].StartAt
		})
	})
	return p.err
}

func (p *localVLiveProvider) GetLives(_ renderregion.Value) ([]*VLive, error) {
	if err := p.ensureLoaded(); err != nil {
		return nil, err
	}
	result := make([]*VLive, 0, len(p.lives))
	for _, live := range p.lives {
		c := *live
		c.Schedules = append([]VLiveSchedule(nil), live.Schedules...)
		result = append(result, &c)
	}
	return result, nil
}
