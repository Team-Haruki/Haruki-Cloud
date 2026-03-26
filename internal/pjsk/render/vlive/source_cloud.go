package vlive

import (
	"context"
	"fmt"
	"sort"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/virtuallive"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type Source interface {
	DefaultRegion() renderregion.Value
	GetLives(region renderregion.Value) ([]*Live, error)
}

type CloudSource struct {
	client *sekaiDB.Client
	region renderregion.Value
}

func NewCloudSource(client *sekaiDB.Client, defaultRegion renderregion.Value) *CloudSource {
	if client == nil {
		return nil
	}
	return &CloudSource{
		client: client,
		region: renderregion.WithDefault(defaultRegion),
	}
}

func (c *CloudSource) DefaultRegion() renderregion.Value {
	if c == nil {
		return renderregion.JP
	}
	return c.region
}

func (c *CloudSource) GetLives(region renderregion.Value) ([]*Live, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("vlive cloud source is not configured")
	}

	queryRegion := renderregion.WithDefault(region)
	entities, err := c.client.Virtuallive.Query().
		Where(virtuallive.ServerRegionEQ(queryRegion.String())).
		All(context.Background())
	if err != nil {
		return nil, err
	}

	lives := make([]*Live, 0, len(entities))
	for _, entity := range entities {
		live := &Live{
			ID:      int(entity.GameID),
			Name:    entity.Name,
			StartAt: entity.StartAt,
			EndAt:   entity.EndAt,
		}
		for _, raw := range entity.VirtualLiveSchedules {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			startAt := int64Number(item["startAt"])
			endAt := int64Number(item["endAt"])
			if startAt <= 0 || endAt <= 0 {
				continue
			}
			live.Schedules = append(live.Schedules, Schedule{
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

func int64Number(value interface{}) int64 {
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
