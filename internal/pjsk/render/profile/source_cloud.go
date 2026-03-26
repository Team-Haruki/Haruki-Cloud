package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/event"
	"haruki-cloud/database/sekai/playerframe"
	"haruki-cloud/database/sekai/playerframegroup"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	renderhonor "haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type CloudSource struct {
	client      *sekaiDB.Client
	region      renderregion.Value
	queryRegion renderregion.Value

	cards  *rendercard.CloudSource
	honors *renderhonor.CloudSource

	frameMu    sync.RWMutex
	frameCache map[int]*masterdata.PlayerFrame

	groupMu    sync.RWMutex
	groupCache map[int]*masterdata.PlayerFrameGroup

	eventHonorMu       sync.RWMutex
	eventByHonorID     map[int]int
	eventByHonorLoaded bool
}

type rewardRange struct {
	EventRankingRewardDetails []rewardDetail `json:"eventRankingRewardDetails"`
}

type rewardDetail struct {
	ResourceType string `json:"resourceType"`
	ResourceID   int    `json:"resourceId"`
}

func NewCloudSource(client *sekaiDB.Client, defaultRegion renderregion.Value) *CloudSource {
	if client == nil {
		return nil
	}
	region := renderregion.WithDefault(defaultRegion)
	return &CloudSource{
		client:         client,
		region:         region,
		queryRegion:    region,
		cards:          rendercard.NewCloudSource(client, region),
		honors:         renderhonor.NewCloudSource(client, region),
		frameCache:     make(map[int]*masterdata.PlayerFrame),
		groupCache:     make(map[int]*masterdata.PlayerFrameGroup),
		eventByHonorID: make(map[int]int),
	}
}

func (c *CloudSource) DefaultRegion() renderregion.Value {
	return c.region
}

func (c *CloudSource) GetCardByID(id int) (*masterdata.Card, error) {
	if c.cards == nil {
		return nil, fmt.Errorf("card source not configured")
	}
	return c.cards.GetCardByID(id)
}

func (c *CloudSource) GetHonorByID(id int) (*masterdata.Honor, error) {
	if c.honors == nil {
		return nil, fmt.Errorf("honor source not configured")
	}
	return c.honors.GetHonorByID(id)
}

func (c *CloudSource) GetHonorGroupByID(id int) (*masterdata.HonorGroup, error) {
	if c.honors == nil {
		return nil, fmt.Errorf("honor source not configured")
	}
	return c.honors.GetHonorGroupByID(id)
}

func (c *CloudSource) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	if c.honors == nil {
		return nil, fmt.Errorf("honor source not configured")
	}
	return c.honors.GetBondsHonorByID(id)
}

func (c *CloudSource) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	if c.honors == nil {
		return nil, false
	}
	return c.honors.GetGameCharacterUnitByID(id)
}

func (c *CloudSource) GetPlayerFrameByID(id int) (*masterdata.PlayerFrame, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid player frame id")
	}

	c.frameMu.RLock()
	if cached, ok := c.frameCache[id]; ok {
		c.frameMu.RUnlock()
		copy := *cached
		return &copy, nil
	}
	c.frameMu.RUnlock()

	entity, err := c.client.Playerframe.Query().
		Where(playerframe.ServerRegionEQ(c.queryRegion.String()), playerframe.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query player frame %d failed: %w", id, err)
	}

	model := &masterdata.PlayerFrame{
		ID:                 int(entity.GameID),
		Seq:                int(entity.Seq),
		PlayerFrameGroupID: int(entity.PlayerFrameGroupID),
		Description:        entity.Description,
		GameCharacterID:    int(entity.GameCharacterID),
	}

	c.frameMu.Lock()
	c.frameCache[id] = model
	c.frameMu.Unlock()

	copy := *model
	return &copy, nil
}

func (c *CloudSource) GetPlayerFrameGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid player frame group id")
	}

	c.groupMu.RLock()
	if cached, ok := c.groupCache[id]; ok {
		c.groupMu.RUnlock()
		copy := *cached
		return &copy, nil
	}
	c.groupMu.RUnlock()

	entity, err := c.client.Playerframegroup.Query().
		Where(playerframegroup.ServerRegionEQ(c.queryRegion.String()), playerframegroup.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query player frame group %d failed: %w", id, err)
	}

	model := &masterdata.PlayerFrameGroup{
		ID:              int(entity.GameID),
		Seq:             int(entity.Seq),
		Name:            entity.Name,
		AssetBundleName: entity.AssetbundleName,
	}

	c.groupMu.Lock()
	c.groupCache[id] = model
	c.groupMu.Unlock()

	copy := *model
	return &copy, nil
}

func (c *CloudSource) GetEventIDByHonorID(honorID int) int {
	if honorID == 0 {
		return 0
	}

	c.eventHonorMu.RLock()
	if c.eventByHonorLoaded {
		id := c.eventByHonorID[honorID]
		c.eventHonorMu.RUnlock()
		return id
	}
	c.eventHonorMu.RUnlock()

	c.eventHonorMu.Lock()
	defer c.eventHonorMu.Unlock()
	if c.eventByHonorLoaded {
		return c.eventByHonorID[honorID]
	}

	items, err := c.client.Event.Query().
		Where(event.ServerRegionEQ(c.queryRegion.String())).
		All(context.Background())
	if err != nil {
		return 0
	}

	for _, item := range items {
		var ranges []rewardRange
		if err := json.Unmarshal(item.EventRankingRewardRanges, &ranges); err != nil {
			continue
		}
		eventID := int(item.GameID)
		for _, rewardRange := range ranges {
			for _, detail := range rewardRange.EventRankingRewardDetails {
				if detail.ResourceType == "honor" && detail.ResourceID > 0 {
					c.eventByHonorID[detail.ResourceID] = eventID
				}
			}
		}
	}

	c.eventByHonorLoaded = true
	return c.eventByHonorID[honorID]
}

