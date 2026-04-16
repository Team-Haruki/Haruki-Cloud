package education

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/region"
	rendersource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/internal/pjsk/drawing"
)

type Controller struct {
	drawing  *drawing.HarukiDrawingClient
	assets   *assets.AssetHelper
	sources  *rendersource.Registry[DataSource]
	snapshot userdata.Snapshot
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

func NewController(drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot userdata.Snapshot, defaultRegion renderregion.Value) *Controller {
	return &Controller{
		drawing:  drawingClient,
		assets:   assetHelper,
		sources:  rendersource.NewRegistry[DataSource](defaultRegion),
		snapshot: snapshot,
	}
}

func (c *Controller) RegisterSource(source DataSource) {
	if c == nil || c.sources == nil {
		return
	}
	c.sources.RegisterSource(source)
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.sources = rendersource.NewRegistry[DataSource](c.sources.ResolveRegion(renderregion.Unknown))
	for _, source := range c.sources.OrderedSources() {
		if contextual, ok := any(source).(contextualDataSource); ok {
			clone.sources.RegisterSource(contextual.WithContext(ctx))
			continue
		}
		clone.sources.RegisterSource(source)
	}
	return &clone
}

func (c *Controller) BuildChallengeLiveDetailsRequest(query ChallengeLiveQuery) (*drawing.ChallengeLiveDetailsRequest, error) {
	if c == nil || c.sources == nil {
		return nil, fmt.Errorf("education controller is not initialized")
	}

	snapshot := query.Snapshot
	if snapshot == nil {
		snapshot = c.snapshot
	}
	if snapshot == nil {
		return nil, fmt.Errorf("local user snapshot is not configured")
	}
	if err := snapshot.Require(); err != nil {
		return nil, err
	}

	region := c.sources.ResolveRegion(query.Region)
	source, ok := c.sources.SourceForRegion(region)
	if !ok {
		return nil, fmt.Errorf("education data source is not configured")
	}

	challenge := snapshot.ChallengeLive()
	if challenge == nil {
		return nil, fmt.Errorf("user snapshot is missing challenge live data")
	}
	profile := query.Profile
	if profile == nil {
		profile = snapshot.DetailedProfile(region)
	}
	if profile == nil {
		return nil, fmt.Errorf("user snapshot is missing profile data")
	}

	scoreByCharacter := make(map[int]int, len(challenge.Results))
	for _, item := range challenge.Results {
		scoreByCharacter[item.CharacterID] = item.HighScore
	}
	rankByCharacter := make(map[int]int, len(challenge.Stages))
	for _, item := range challenge.Stages {
		if item.Rank > rankByCharacter[item.CharacterID] {
			rankByCharacter[item.CharacterID] = item.Rank
		}
	}
	claimedRewards := make(map[int]struct{}, len(challenge.Rewards))
	for _, item := range challenge.Rewards {
		claimedRewards[item.RewardID] = struct{}{}
	}

	maxScore := 0
	characterChallenges := make([]drawing.CharacterChallengeInfo, 0, 26)
	for characterID := 1; characterID <= 26; characterID++ {
		score := scoreByCharacter[characterID]
		if score > maxScore {
			maxScore = score
		}
		jewel, shard := c.pickChallengeRewards(source, characterID, claimedRewards)
		characterChallenges = append(characterChallenges, drawing.CharacterChallengeInfo{
			CharaID:       characterID,
			Rank:          rankByCharacter[characterID],
			Score:         score,
			Jewel:         jewel,
			Shard:         shard,
			CharaIconPath: c.characterIconPath(characterID),
		})
	}

	displayMax := c.estimateChallengeMaxScore(source)
	if maxScore > displayMax {
		displayMax = maxScore
	}

	request := &drawing.ChallengeLiveDetailsRequest{
		Profile:             *profile,
		CharacterChallenges: characterChallenges,
		MaxScore:            displayMax,
	}
	if icon := c.findStaticIcon("jewel.png"); icon != "" {
		request.JewelIconPath = &icon
	}
	if icon := c.findStaticIcon("shard.png"); icon != "" {
		request.ShardIconPath = &icon
	}
	return request, nil
}

func (c *Controller) RenderChallengeLiveDetails(query ChallengeLiveQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildChallengeLiveDetailsRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateChallengeLiveDetails(payload)
}

func (c *Controller) BuildPowerBonusDetailRequest(req drawing.PowerBonusDetailRequest) (*drawing.PowerBonusDetailRequest, error) {
	if len(req.CharaBonuses) == 0 && len(req.UnitBonuses) == 0 && len(req.AttrBonuses) == 0 {
		return nil, fmt.Errorf("power bonus request has no bonuses")
	}
	return &req, nil
}

func (c *Controller) RenderPowerBonusDetail(req drawing.PowerBonusDetailRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildPowerBonusDetailRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GeneratePowerBonusDetail(payload)
}

func (c *Controller) BuildAreaItemUpgradeMaterialsRequest(req drawing.AreaItemUpgradeMaterialsRequest) (*drawing.AreaItemUpgradeMaterialsRequest, error) {
	if len(req.AreaItems) == 0 {
		return nil, fmt.Errorf("area item request has no items")
	}
	return &req, nil
}

func (c *Controller) RenderAreaItemUpgradeMaterials(req drawing.AreaItemUpgradeMaterialsRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildAreaItemUpgradeMaterialsRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateAreaItemUpgradeMaterials(payload)
}

func (c *Controller) BuildBondsRequest(req drawing.BondsRequest) (*drawing.BondsRequest, error) {
	if len(req.Bonds) == 0 {
		return nil, fmt.Errorf("bonds request has no items")
	}
	return &req, nil
}

func (c *Controller) RenderBonds(req drawing.BondsRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildBondsRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateBonds(payload)
}

func (c *Controller) BuildLeaderCountRequest(req drawing.LeaderCountRequest) (*drawing.LeaderCountRequest, error) {
	if len(req.LeaderCounts) == 0 {
		return nil, fmt.Errorf("leader-count request has no items")
	}
	return &req, nil
}

func (c *Controller) RenderLeaderCount(req drawing.LeaderCountRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildLeaderCountRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateLeaderCount(payload)
}

func (c *Controller) pickChallengeRewards(source DataSource, charID int, claimed map[int]struct{}) (int, int) {
	rewards := source.GetChallengeRewardsByCharacter(charID)
	jewelTotal := 0
	shardTotal := 0
	for _, reward := range rewards {
		if reward == nil {
			continue
		}
		if _, ok := claimed[reward.ID]; ok {
			continue
		}
		box := source.GetResourceBoxByPurpose("challenge_live_high_score", reward.ResourceBoxID)
		if box == nil {
			continue
		}
		for _, detail := range box.Details {
			switch strings.ToLower(detail.ResourceType) {
			case "jewel":
				jewelTotal += detail.ResourceQuantity
			case "material":
				if detail.ResourceID == 15 {
					shardTotal += detail.ResourceQuantity
				}
			}
		}
	}
	return jewelTotal, shardTotal
}

func (c *Controller) estimateChallengeMaxScore(source DataSource) int {
	maxScore := 0
	for characterID := 1; characterID <= 26; characterID++ {
		for _, reward := range source.GetChallengeRewardsByCharacter(characterID) {
			if reward != nil && reward.HighScore > maxScore {
				maxScore = reward.HighScore
			}
		}
	}
	if maxScore < 3_000_000 {
		maxScore = 3_000_000
	}
	return maxScore
}

func (c *Controller) characterIconPath(charID int) string {
	if nickname, ok := assets.CharacterIDToNickname[charID]; ok {
		return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir,
			filepath.Join("chara_icon", nickname+".png"),
			filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", charID)))
	}
	return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir,
		filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", charID)))
}

func (c *Controller) findStaticIcon(filename string) string {
	return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, filename)
}
