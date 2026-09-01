package provider

import (
	"context"
	"fmt"
	"sort"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/virtuallive"
	json "haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
)

type dbVLiveProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
}

func (p *dbVLiveProvider) GetLives(ctx context.Context, region renderregion.Value) ([]*VLive, error) {
	if p.client == nil {
		return nil, fmt.Errorf("vlive provider is not configured")
	}

	queryRegion := renderregion.WithDefault(region)
	entities, err := p.client.Virtuallive.Query().
		Where(virtuallive.ServerRegionEQ(queryRegion.String())).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query virtual lives: %w", err)
	}

	lives := make([]*VLive, 0, len(entities))
	for _, entity := range entities {
		lives = append(lives, convertDBVLive(entity))
	}

	sort.Slice(lives, func(i, j int) bool {
		if lives[i].StartAt == lives[j].StartAt {
			return lives[i].ID < lives[j].ID
		}
		return lives[i].StartAt < lives[j].StartAt
	})
	return lives, nil
}

func convertDBVLive(entity *sekaiDB.Virtuallive) *VLive {
	return &VLive{
		ID:              int(entity.GameID),
		Name:            entity.Name,
		AssetBundleName: entity.AssetbundleName,
		StartAt:         entity.StartAt,
		EndAt:           entity.EndAt,
		Schedules:       decodeVLiveSchedules(entity.VirtualLiveSchedules),
		Rewards:         decodeVLiveRewards(entity.VirtualLiveRewards),
		Characters:      decodeVLiveCharacters(entity.VirtualLiveCharacters),
	}
}

func decodeVLiveSchedules(data []byte) []VLiveSchedule {
	var items []map[string]any
	_ = json.Unmarshal(data, &items)
	schedules := make([]VLiveSchedule, 0, len(items))
	for _, item := range items {
		startAt := vliveInt64Number(item["startAt"])
		endAt := vliveInt64Number(item["endAt"])
		if startAt > 0 && endAt > 0 {
			schedules = append(schedules, VLiveSchedule{StartAt: startAt, EndAt: endAt})
		}
	}
	return schedules
}

func decodeVLiveRewards(data []byte) []VLiveReward {
	var items []map[string]any
	_ = json.Unmarshal(data, &items)
	rewards := make([]VLiveReward, 0, len(items))
	for _, item := range items {
		resourceBoxID := vliveIntNumber(item["resourceBoxId"])
		if resourceBoxID > 0 {
			rewards = append(rewards, VLiveReward{
				VirtualLiveType: vliveString(item["virtualLiveType"]),
				ResourceBoxID:   resourceBoxID,
			})
		}
	}
	return rewards
}

func decodeVLiveCharacters(data []byte) []VLiveCharacter {
	var items []map[string]any
	_ = json.Unmarshal(data, &items)
	characters := make([]VLiveCharacter, 0, len(items))
	for _, item := range items {
		gameCharacterUnitID := vliveIntNumber(item["gameCharacterUnitId"])
		if gameCharacterUnitID > 0 {
			characters = append(characters, VLiveCharacter{
				GameCharacterUnitID:        gameCharacterUnitID,
				VirtualLivePerformanceType: vliveString(item["virtualLivePerformanceType"]),
			})
		}
	}
	return characters
}

func vliveInt64Number(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	default:
		return 0
	}
}

func vliveIntNumber(value any) int {
	return int(vliveInt64Number(value))
}

func vliveString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

// Ensure the entity type is used to avoid import issues.
var _ *sekaiDB.Virtuallive
