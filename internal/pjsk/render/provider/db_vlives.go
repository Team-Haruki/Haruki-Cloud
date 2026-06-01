package provider

import (
	"context"
	"fmt"
	json "github.com/bytedance/sonic"
	"sort"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/virtuallive"
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
		live := &VLive{
			ID:              int(entity.GameID),
			Name:            entity.Name,
			AssetBundleName: entity.AssetbundleName,
			StartAt:         entity.StartAt,
			EndAt:           entity.EndAt,
		}
		var schedules []map[string]any
		if len(entity.VirtualLiveSchedules) > 0 {
			_ = json.Unmarshal(entity.VirtualLiveSchedules, &schedules)
		}
		for _, item := range schedules {
			startAt := vliveInt64Number(item["startAt"])
			endAt := vliveInt64Number(item["endAt"])
			if startAt <= 0 || endAt <= 0 {
				continue
			}
			live.Schedules = append(live.Schedules, VLiveSchedule{
				StartAt: startAt,
				EndAt:   endAt,
			})
		}
		var rewards []map[string]any
		if len(entity.VirtualLiveRewards) > 0 {
			_ = json.Unmarshal(entity.VirtualLiveRewards, &rewards)
		}
		for _, item := range rewards {
			resourceBoxID := vliveIntNumber(item["resourceBoxId"])
			if resourceBoxID <= 0 {
				continue
			}
			live.Rewards = append(live.Rewards, VLiveReward{
				VirtualLiveType: vliveString(item["virtualLiveType"]),
				ResourceBoxID:   resourceBoxID,
			})
		}
		var characters []map[string]any
		if len(entity.VirtualLiveCharacters) > 0 {
			_ = json.Unmarshal(entity.VirtualLiveCharacters, &characters)
		}
		for _, item := range characters {
			gameCharacterUnitID := vliveIntNumber(item["gameCharacterUnitId"])
			if gameCharacterUnitID <= 0 {
				continue
			}
			live.Characters = append(live.Characters, VLiveCharacter{
				GameCharacterUnitID:        gameCharacterUnitID,
				VirtualLivePerformanceType: vliveString(item["virtualLivePerformanceType"]),
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
