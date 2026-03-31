package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/virtuallive"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type dbVLiveProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
}

func (p *dbVLiveProvider) GetLives(region renderregion.Value) ([]*VLive, error) {
	if p.client == nil {
		return nil, fmt.Errorf("vlive provider is not configured")
	}

	queryRegion := renderregion.WithDefault(region)
	entities, err := p.client.Virtuallive.Query().
		Where(virtuallive.ServerRegionEQ(queryRegion.String())).
		All(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query virtual lives: %w", err)
	}

	lives := make([]*VLive, 0, len(entities))
	for _, entity := range entities {
		live := &VLive{
			ID:      int(entity.GameID),
			Name:    entity.Name,
			StartAt: entity.StartAt,
			EndAt:   entity.EndAt,
		}
		var schedules []map[string]interface{}
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

func vliveInt64Number(value interface{}) int64 {
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

// Ensure the entity type is used to avoid import issues.
var _ *sekaiDB.Virtuallive
