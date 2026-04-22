package provider

import (
	"context"
	"encoding/json"
	"slices"
	"sort"

	renderregion "haruki-cloud/internal/pjsk/region"
)

// ===========================================================================
// localVLiveProvider
// ===========================================================================

type localVLiveProvider struct {
	store *localStore
	lives lazyValue[[]*VLive]
}

func (p *localVLiveProvider) ensureLoaded() error {
	return p.lives.init(func() ([]*VLive, error) {
		items, err := loadJSON[localVirtualLiveJSON](p.store, "virtualLives.json")
		if err != nil {
			return nil, err
		}
		lives := make([]*VLive, 0, len(items))
		for _, item := range items {
			live := &VLive{
				ID:              item.ID,
				Name:            item.Name,
				AssetBundleName: item.AssetBundleName,
				StartAt:         item.StartAt,
				EndAt:           item.EndAt,
			}
			var schedules []map[string]any
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
			var rewards []map[string]any
			if len(item.VirtualLiveRewards) > 0 {
				_ = json.Unmarshal(item.VirtualLiveRewards, &rewards)
			}
			for _, reward := range rewards {
				resourceBoxID := vliveIntNumber(reward["resourceBoxId"])
				if resourceBoxID <= 0 {
					continue
				}
				live.Rewards = append(live.Rewards, VLiveReward{
					VirtualLiveType: vliveString(reward["virtualLiveType"]),
					ResourceBoxID:   resourceBoxID,
				})
			}
			var characters []map[string]any
			if len(item.VirtualLiveCharacters) > 0 {
				_ = json.Unmarshal(item.VirtualLiveCharacters, &characters)
			}
			for _, character := range characters {
				gameCharacterUnitID := vliveIntNumber(character["gameCharacterUnitId"])
				if gameCharacterUnitID <= 0 {
					continue
				}
				live.Characters = append(live.Characters, VLiveCharacter{
					GameCharacterUnitID:        gameCharacterUnitID,
					VirtualLivePerformanceType: vliveString(character["virtualLivePerformanceType"]),
				})
			}
			lives = append(lives, live)
		}
		sort.Slice(lives, func(i, j int) bool {
			if lives[i].StartAt == lives[j].StartAt {
				return lives[i].ID < lives[j].ID
			}
			return lives[i].StartAt < lives[j].StartAt
		})
		return lives, nil
	})
}

func (p *localVLiveProvider) GetLives(_ context.Context, _ renderregion.Value) ([]*VLive, error) {
	if err := p.ensureLoaded(); err != nil {
		return nil, err
	}
	result := make([]*VLive, 0, len(p.lives.v()))
	for _, live := range p.lives.v() {
		c := *live
		c.Schedules = slices.Clone(live.Schedules)
		c.Rewards = slices.Clone(live.Rewards)
		c.Characters = slices.Clone(live.Characters)
		result = append(result, &c)
	}
	return result, nil
}
