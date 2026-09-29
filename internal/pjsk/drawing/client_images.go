package drawing

import (
	"context"
	"fmt"
)

func (c *HarukiDrawingClient) GenerateBasicMusicRewardsImage(req *BasicMusicRewardsRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/music/rewards/basic", req)
}

func (c *HarukiDrawingClient) GenerateCostumeListImage(req *CostumeListRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/costume/list", req)
}

func (c *HarukiDrawingClient) GenerateCostumeDetailImage(req *CostumeDetailRequest) (ImageResult, error) {
	return c.cachedPostImage(costumeDetailEndpoint, req)
}

func (c *HarukiDrawingClient) GenerateDeckRecommendationImage(req *DeckRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/deck/recommend", req)
}

func (c *HarukiDrawingClient) GenerateChallengeLiveDetailsImage(req *ChallengeLiveDetailsRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/education/challenge-live", req)
}

func (c *HarukiDrawingClient) GeneratePowerBonusDetailImage(req *PowerBonusDetailRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/education/power-bonus", req)
}

func (c *HarukiDrawingClient) GenerateAreaItemUpgradeMaterialsImage(req *AreaItemUpgradeMaterialsRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/education/area-item", req)
}

func (c *HarukiDrawingClient) GenerateBondsImage(req *BondsRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/education/bonds", req)
}

func (c *HarukiDrawingClient) GenerateLeaderCountImage(req *LeaderCountRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/education/leader-count", req)
}

func (c *HarukiDrawingClient) GenerateCharacterMissionOverviewImage(req *CharacterMissionOverviewRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/education/character-mission-overview", req)
}

func (c *HarukiDrawingClient) GenerateCharacterMissionAllImage(req *CharacterMissionAllRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/education/character-mission-all", req)
}

func (c *HarukiDrawingClient) GenerateInventoryListImage(req *InventoryListRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/inventory/list", req)
}

func (c *HarukiDrawingClient) GenerateEventDetailImage(req *EventDetailRequest) (ImageResult, error) {
	data, err := c.postUncached("/api/pjsk/event/detail", req)
	return ImageBytes(data), err
}

func (c *HarukiDrawingClient) GenerateEventRecordImage(req *EventRecordRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/event/record", req)
}

func (c *HarukiDrawingClient) GenerateEventListImage(req *EventListRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/event/list", req)
}

func (c *HarukiDrawingClient) GenerateEventPlannerImage(req *EventPlannerRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/event/planner", req)
}

func (c *HarukiDrawingClient) GenerateVLiveListImage(req *VLiveListRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/vlive/list", req)
}

func (c *HarukiDrawingClient) GenerateVLiveDetailImage(req *VLiveDetailRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/vlive/detail", req)
}

func (c *HarukiDrawingClient) GenerateGachaListImage(req *GachaListRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/gacha/list", req)
}

func (c *HarukiDrawingClient) GenerateGachaDetailImage(req *GachaDetailRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/gacha/detail", req)
}

func (c *HarukiDrawingClient) GenerateHonorImage(req *HonorRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/honor", req)
}

func (c *HarukiDrawingClient) GenerateCharacterBirthdayImage(req *CharaBirthdayRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/misc/chara-birthday", req)
}

func (c *HarukiDrawingClient) GenerateAliasListImage(req *AliasListRequest) (ImageResult, error) {
	// Alias-list watermarks include request DT, so we intentionally bypass the
	// render cache here to avoid serving stale timestamps.
	data, err := c.postUncached("/api/pjsk/misc/alias-list", req)
	return ImageBytes(data), err
}

func (c *HarukiDrawingClient) GenerateCommandHelpImage(req *CommandHelpRenderRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/help/render", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiResourceImage(req *MysekaiResourceRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/mysekai/resource", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiMapImage(req *MysekaiMsrMapRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/mysekai/map", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiFixtureListImage(req *MysekaiFixtureListRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/mysekai/fixture-list", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiFixtureDetailImage(req *MysekaiFixtureDetailRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/mysekai/fixture-detail", []any{req})
}

func (c *HarukiDrawingClient) GenerateMysekaiDoorUpgradeImage(req *MysekaiDoorUpgradeRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/mysekai/door-upgrade", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiMusicRecordImage(req *MysekaiMusicrecordRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/mysekai/music-record", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiTalkListImage(req *MysekaiTalkListRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/mysekai/talk-list", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiShopImage(req *MysekaiShopRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/mysekai/shop", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiBlueprintTermImage(req *MysekaiBlueprintTermRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/mysekai/blueprint-term", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiHousingCompetitionImage(req *MysekaiHousingCompetitionRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/mysekai/housing-competition", req)
}

func (c *HarukiDrawingClient) GenerateScoreControlImage(req *ScoreControlRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/score/control", req)
}

func (c *HarukiDrawingClient) GenerateCustomRoomScoreImage(req *CustomRoomScoreRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/score/custom-room", req)
}

func (c *HarukiDrawingClient) GenerateMusicMetaImage(req []MusicMetaRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/score/music-meta", req)
}

func (c *HarukiDrawingClient) GenerateMusicBoardImage(req *MusicBoardRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/score/music-board", req)
}

func (c *HarukiDrawingClient) GenerateStampListImage(req *StampListRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/stamp/list", req)
}

func (c *HarukiDrawingClient) GenerateSKLineImage(req *SklRequest, full bool) (ImageResult, error) {
	url := fmt.Sprintf("/api/pjsk/sk/line?full=%t", full)
	return c.cachedPostImage(url, req)
}

func (c *HarukiDrawingClient) GenerateSKQueryImage(req *SKRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/sk/query", req)
}

func (c *HarukiDrawingClient) GenerateSKCheckRoomImage(req *CFRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/sk/check-room", req)
}

func (c *HarukiDrawingClient) GenerateSKCSBImage(req *CSBRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/sk/csb", req)
}

func (c *HarukiDrawingClient) GenerateSKSpeedImage(req *SpeedRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/sk/speed", req)
}

func (c *HarukiDrawingClient) GenerateSKPlayerTraceImage(req *PlayerTraceRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/sk/player-trace", req)
}

func (c *HarukiDrawingClient) GenerateSKRankTraceImage(req *RankTraceRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/sk/rank-trace", req)
}

func (c *HarukiDrawingClient) GenerateSKWinRateImage(req *WinRateRequest) (ImageResult, error) {
	return c.cachedPostImage("/api/pjsk/sk/winrate", req)
}

func (c *HarukiDrawingClient) GenerateCostumeDetailWithPrepareImage(cacheReq any, req *CostumeDetailRequest, prepare func(any) error) (ImageResult, error) {
	return c.GenerateCostumeDetailWithContextPrepareImage(cacheReq, req, func(_ context.Context, prepared any) error {
		if prepare == nil {
			return nil
		}
		return prepare(prepared)
	})
}

func (c *HarukiDrawingClient) GenerateCostumeDetailWithContextPrepareImage(cacheReq any, req *CostumeDetailRequest, prepare func(context.Context, any) error) (ImageResult, error) {
	return c.renderImageWithCacheRequestAndPrepare(costumeDetailEndpoint, cacheReq, req, prepare, func(renderCtx context.Context, prepared any) ([]byte, error) {
		return c.WithContext(renderCtx).postPrepared(costumeDetailEndpoint, prepared)
	}, false)
}
