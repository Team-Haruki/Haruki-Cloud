package music

import (
	"fmt"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/sekai"
)

func (c *Controller) BuildMusicRewardsDetailRequest(query RewardsDetailQuery) (*drawing.DetailMusicRewardsRequest, error) {
	region := c.resolveRegion(query.Region)
	return &drawing.DetailMusicRewardsRequest{
		RankRewards:   query.RankRewards,
		ComboRewards:  ensureDetailComboRewards(query.ComboRewards),
		Profile:       c.resolveProfileCard(query.Profile, region),
		JewelIconPath: c.resolveStaticIcon(query.JewelIconPath, "jewel.png"),
		ShardIconPath: c.resolveStaticIcon(query.ShardIconPath, "shard.png"),
	}, nil
}

func (c *Controller) RenderMusicRewardsDetail(query RewardsDetailQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	payload, err := c.BuildMusicRewardsDetailRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateDetailMusicRewards(payload)
}

func (c *Controller) RenderMusicRewardsDetailFromAchievements(query RewardsDetailQuery, achievementsJSON []byte) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	payload, err := c.BuildMusicRewardsDetailRequestFromAchievements(query, achievementsJSON)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateDetailMusicRewards(payload)
}

func (c *Controller) RenderMusicRewardsDetailFromSnapshot(query RewardsDetailQuery, snapshot snapshot.Snapshot) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	payload, err := c.BuildMusicRewardsDetailRequestFromSnapshot(query, snapshot)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateDetailMusicRewards(payload)
}

func (c *Controller) BuildMusicRewardsBasicRequest(query RewardsBasicQuery) (*drawing.BasicMusicRewardsRequest, error) {
	region := c.resolveRegion(query.Region)
	combo := query.ComboRewards
	if combo == nil {
		combo = map[string]string{
			"hard":   "0",
			"expert": "0",
			"master": "0",
			"append": "0",
		}
	}
	return &drawing.BasicMusicRewardsRequest{
		RankRewards:   query.RankRewards,
		ComboRewards:  combo,
		Profile:       c.resolveProfileCard(query.Profile, region),
		JewelIconPath: c.resolveStaticIcon(query.JewelIconPath, "jewel.png"),
		ShardIconPath: c.resolveStaticIcon(query.ShardIconPath, "shard.png"),
	}, nil
}

func (c *Controller) RenderMusicRewardsBasic(query RewardsBasicQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	payload, err := c.BuildMusicRewardsBasicRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateBasicMusicRewards(payload)
}

func (c *Controller) RenderMusicRewardsBasicEstimate(query RewardsBasicQuery, clearCounts []sekai.AnotherUserMusicDifficultyClearCount, reason string) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	payload, err := c.BuildMusicRewardsBasicEstimateRequest(query, clearCounts, reason)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateBasicMusicRewards(payload)
}
