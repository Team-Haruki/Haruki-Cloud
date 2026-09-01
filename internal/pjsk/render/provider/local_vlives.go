package provider

import (
	"context"
	json "haruki-cloud/internal/jsonutil"
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
		items, err := p.store.loadJSON[localVirtualLiveJSON]("virtualLives.json")
		if err != nil {
			return nil, err
		}
		lives := make([]*VLive, 0, len(items))
		for _, item := range items {
			lives = append(lives, buildLocalVLive(item))
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

func buildLocalVLive(item localVirtualLiveJSON) *VLive {
	return &VLive{
		ID:              item.ID,
		Name:            item.Name,
		AssetBundleName: item.AssetBundleName,
		StartAt:         item.StartAt,
		EndAt:           item.EndAt,
		Schedules:       decodeLocalVLiveSchedules(item.VirtualLiveSchedules),
		Rewards:         decodeLocalVLiveRewards(item.VirtualLiveRewards),
		Characters:      decodeLocalVLiveCharacters(item.VirtualLiveCharacters),
	}
}

func decodeLocalVLiveSchedules(raw json.RawMessage) []VLiveSchedule {
	var values []map[string]any
	_ = json.Unmarshal(raw, &values)
	result := make([]VLiveSchedule, 0, len(values))
	for _, value := range values {
		startAt := vliveInt64Number(value["startAt"])
		endAt := vliveInt64Number(value["endAt"])
		if startAt > 0 && endAt > 0 {
			result = append(result, VLiveSchedule{StartAt: startAt, EndAt: endAt})
		}
	}
	return result
}

func decodeLocalVLiveRewards(raw json.RawMessage) []VLiveReward {
	var values []map[string]any
	_ = json.Unmarshal(raw, &values)
	result := make([]VLiveReward, 0, len(values))
	for _, value := range values {
		resourceBoxID := vliveIntNumber(value["resourceBoxId"])
		if resourceBoxID > 0 {
			result = append(result, VLiveReward{
				VirtualLiveType: vliveString(value["virtualLiveType"]),
				ResourceBoxID:   resourceBoxID,
			})
		}
	}
	return result
}

func decodeLocalVLiveCharacters(raw json.RawMessage) []VLiveCharacter {
	var values []map[string]any
	_ = json.Unmarshal(raw, &values)
	result := make([]VLiveCharacter, 0, len(values))
	for _, value := range values {
		gameCharacterUnitID := vliveIntNumber(value["gameCharacterUnitId"])
		if gameCharacterUnitID > 0 {
			result = append(result, VLiveCharacter{
				GameCharacterUnitID:        gameCharacterUnitID,
				VirtualLivePerformanceType: vliveString(value["virtualLivePerformanceType"]),
			})
		}
	}
	return result
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
