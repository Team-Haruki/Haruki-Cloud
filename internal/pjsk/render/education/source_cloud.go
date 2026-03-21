package education

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/challengelivehighscorereward"
	"haruki-cloud/database/sekai/resourceboxe"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type CloudSource struct {
	client      *sekaiDB.Client
	region      renderregion.Value
	queryRegion renderregion.Value

	rewardMu      sync.RWMutex
	rewardsByChar map[int][]*ChallengeReward
	rewardsLoaded bool

	boxMu        sync.RWMutex
	boxByID      map[int]*ResourceBox
	boxByPurpose map[string]map[int]*ResourceBox
	boxesLoaded  bool
}

func NewCloudSource(client *sekaiDB.Client, defaultRegion renderregion.Value) *CloudSource {
	if client == nil {
		return nil
	}
	region := renderregion.WithDefault(defaultRegion)
	return &CloudSource{
		client:        client,
		region:        region,
		queryRegion:   region,
		rewardsByChar: make(map[int][]*ChallengeReward),
		boxByID:       make(map[int]*ResourceBox),
		boxByPurpose:  make(map[string]map[int]*ResourceBox),
	}
}

func (c *CloudSource) DefaultRegion() renderregion.Value {
	return c.region
}

func (c *CloudSource) GetChallengeRewardsByCharacter(charID int) []*ChallengeReward {
	if charID <= 0 {
		return nil
	}

	c.rewardMu.RLock()
	if c.rewardsLoaded {
		out := cloneChallengeRewards(c.rewardsByChar[charID])
		c.rewardMu.RUnlock()
		return out
	}
	c.rewardMu.RUnlock()

	c.rewardMu.Lock()
	defer c.rewardMu.Unlock()

	if !c.rewardsLoaded {
		items, err := c.client.Challengelivehighscorereward.Query().
			Where(challengelivehighscorereward.ServerRegionEQ(c.queryRegion.String())).
			All(context.Background())
		if err != nil {
			return nil
		}
		for _, item := range items {
			reward := &ChallengeReward{
				ID:            int(item.GameID),
				CharacterID:   int(item.CharacterID),
				HighScore:     int(item.HighScore),
				ResourceBoxID: int(item.ResourceBoxID),
			}
			c.rewardsByChar[reward.CharacterID] = append(c.rewardsByChar[reward.CharacterID], reward)
		}
		c.rewardsLoaded = true
	}

	return cloneChallengeRewards(c.rewardsByChar[charID])
}

func (c *CloudSource) GetResourceBoxByPurpose(purpose string, id int) *ResourceBox {
	if id <= 0 {
		return nil
	}

	c.boxMu.RLock()
	if c.boxesLoaded {
		var box *ResourceBox
		if strings.TrimSpace(purpose) == "" {
			box = c.boxByID[id]
		} else if purposeMap, ok := c.boxByPurpose[purpose]; ok {
			box = purposeMap[id]
		}
		c.boxMu.RUnlock()
		return cloneResourceBox(box)
	}
	c.boxMu.RUnlock()

	c.boxMu.Lock()
	defer c.boxMu.Unlock()

	if !c.boxesLoaded {
		items, err := c.client.Resourceboxe.Query().
			Where(resourceboxe.ServerRegionEQ(c.queryRegion.String())).
			All(context.Background())
		if err != nil {
			return nil
		}
		for _, item := range items {
			box := &ResourceBox{
				ID:                 int(item.GameID),
				ResourceBoxPurpose: item.ResourceBoxPurpose,
				ResourceBoxType:    item.ResourceBoxType,
				Description:        item.Description,
			}
			if len(item.Details) > 0 {
				var details []ResourceBoxDetail
				if err := decodeFlexible(item.Details, &details); err != nil {
					continue
				}
				box.Details = details
			}
			c.boxByID[box.ID] = box
			if _, ok := c.boxByPurpose[box.ResourceBoxPurpose]; !ok {
				c.boxByPurpose[box.ResourceBoxPurpose] = make(map[int]*ResourceBox)
			}
			c.boxByPurpose[box.ResourceBoxPurpose][box.ID] = box
		}
		c.boxesLoaded = true
	}

	if strings.TrimSpace(purpose) == "" {
		return cloneResourceBox(c.boxByID[id])
	}
	if purposeMap, ok := c.boxByPurpose[purpose]; ok {
		return cloneResourceBox(purposeMap[id])
	}
	return nil
}

func cloneChallengeRewards(source []*ChallengeReward) []*ChallengeReward {
	if len(source) == 0 {
		return nil
	}
	out := make([]*ChallengeReward, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		copy := *item
		out = append(out, &copy)
	}
	return out
}

func cloneResourceBox(source *ResourceBox) *ResourceBox {
	if source == nil {
		return nil
	}
	copy := *source
	copy.Details = append([]ResourceBoxDetail(nil), source.Details...)
	return &copy
}

func decodeFlexible(source interface{}, target interface{}) error {
	raw, err := json.Marshal(source)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}
