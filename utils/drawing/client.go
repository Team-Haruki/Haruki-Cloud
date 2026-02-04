package drawing

import (
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
)

type HarukiDrawingClient struct {
	client  *resty.Client
	baseURL string
}

func NewHarukiDrawingClient(baseURL string) *HarukiDrawingClient {
	return &HarukiDrawingClient{
		client:  resty.New(),
		baseURL: baseURL,
	}
}

func (c *HarukiDrawingClient) post(endpoint string, body interface{}) ([]byte, error) {
	resp, err := c.client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(c.baseURL + endpoint)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("api request failed with status: %d, body: %s", resp.StatusCode(), resp.String())
	}

	return resp.Body(), nil
}

// =========================== Music API ===========================

func (c *HarukiDrawingClient) GenerateMusicDetail(req *MusicDetailRequest) ([]byte, error) {
	return c.post("/api/pjsk/music/detail", req)
}

func (c *HarukiDrawingClient) GenerateMusicBriefList(req *MusicBriefListRequest) ([]byte, error) {
	return c.post("/api/pjsk/music/brief-list", req)
}

func (c *HarukiDrawingClient) GenerateMusicList(req *MusicListRequest, showID bool, showLeak bool) ([]byte, error) {
	// Query params: show_id, show_leak
	endpoint := fmt.Sprintf("/api/pjsk/music/list?show_id=%v&show_leak=%v", showID, showLeak)
	return c.post(endpoint, req)
}

func (c *HarukiDrawingClient) GeneratePlayProgress(req *PlayProgressRequest) ([]byte, error) {
	return c.post("/api/pjsk/music/progress", req)
}

func (c *HarukiDrawingClient) GenerateDetailMusicRewards(req *DetailMusicRewardsRequest) ([]byte, error) {
	return c.post("/api/pjsk/music/rewards/detail", req)
}

func (c *HarukiDrawingClient) GenerateBasicMusicRewards(req *BasicMusicRewardsRequest) ([]byte, error) {
	return c.post("/api/pjsk/music/rewards/basic", req)
}

// =========================== Profile API ===========================

func (c *HarukiDrawingClient) GenerateProfile(req *ProfileRequest) ([]byte, error) {
	return c.post("/api/pjsk/profile/profile", req)
}

// =========================== Card API ===========================

func (c *HarukiDrawingClient) GenerateCardDetail(req *CardDetailRequest) ([]byte, error) {
	return c.post("/api/pjsk/card/detail", req)
}

func (c *HarukiDrawingClient) GenerateCardList(req *CardListRequest) ([]byte, error) {
	return c.post("/api/pjsk/card/list", req)
}

// =========================== Deck API ===========================

func (c *HarukiDrawingClient) GenerateDeckRecommendation(req *DeckRequest) ([]byte, error) {
	return c.post("/api/pjsk/deck/recommendation", req)
}

// =========================== Education API ===========================

func (c *HarukiDrawingClient) GenerateChallengeLiveDetails(req *ChallengeLiveDetailsRequest) ([]byte, error) {
	return c.post("/api/pjsk/education/challenge-live-details", req)
}

func (c *HarukiDrawingClient) GeneratePowerBonusDetail(req *PowerBonusDetailRequest) ([]byte, error) {
	return c.post("/api/pjsk/education/power-bonus-detail", req)
}

func (c *HarukiDrawingClient) GenerateAreaItemUpgradeMaterials(req *AreaItemUpgradeMaterialsRequest) ([]byte, error) {
	return c.post("/api/pjsk/education/area-item-upgrade-materials", req)
}

func (c *HarukiDrawingClient) GenerateBonds(req *BondsRequest) ([]byte, error) {
	return c.post("/api/pjsk/education/bonds", req)
}

func (c *HarukiDrawingClient) GenerateLeaderCount(req *LeaderCountRequest) ([]byte, error) {
	return c.post("/api/pjsk/education/leader-count", req)
}

// =========================== Event API ===========================

func (c *HarukiDrawingClient) GenerateEventDetail(req *EventDetailRequest) ([]byte, error) {
	return c.post("/api/pjsk/event/detail", req)
}

func (c *HarukiDrawingClient) GenerateEventRecord(req *EventRecordRequest) ([]byte, error) {
	return c.post("/api/pjsk/event/record", req)
}

func (c *HarukiDrawingClient) GenerateEventList(req *EventListRequest) ([]byte, error) {
	return c.post("/api/pjsk/event/list", req)
}

// =========================== Gacha API ===========================

func (c *HarukiDrawingClient) GenerateGachaList(req *GachaListRequest) ([]byte, error) {
	return c.post("/api/pjsk/gacha/list", req)
}

// =========================== Honor API ===========================

func (c *HarukiDrawingClient) GenerateHonor(req *HonorRequest) ([]byte, error) {
	return c.post("/api/pjsk/honor/", req)
}

// =========================== Misc API ===========================

func (c *HarukiDrawingClient) GenerateCharacterBirthday(req *CharaBirthdayRequest) ([]byte, error) {
	return c.post("/api/pjsk/misc/birthday", req)
}

// =========================== MySekai API ===========================

func (c *HarukiDrawingClient) GenerateMysekaiResource(req *MysekaiResourceRequest) ([]byte, error) {
	return c.post("/api/pjsk/mysekai/resource", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiFixtureList(req *MysekaiFixtureListRequest) ([]byte, error) {
	return c.post("/api/pjsk/mysekai/fixture-list", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiFixtureDetail(req *MysekaiFixtureDetailRequest) ([]byte, error) {
	return c.post("/api/pjsk/mysekai/fixture-detail", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiDoorUpgrade(req *MysekaiDoorUpgradeRequest) ([]byte, error) {
	return c.post("/api/pjsk/mysekai/door-upgrade", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiMusicRecord(req *MysekaiMusicrecordRequest) ([]byte, error) {
	return c.post("/api/pjsk/mysekai/music-record", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiTalkList(req *MysekaiTalkListRequest) ([]byte, error) {
	return c.post("/api/pjsk/mysekai/talk-list", req)
}

// =========================== Score API ===========================

func (c *HarukiDrawingClient) GenerateScoreControl(req *ScoreControlRequest) ([]byte, error) {
	return c.post("/api/pjsk/score/control", req)
}

func (c *HarukiDrawingClient) GenerateCustomRoomScore(req *CustomRoomScoreRequest) ([]byte, error) {
	return c.post("/api/pjsk/score/custom-room", req)
}

func (c *HarukiDrawingClient) GenerateMusicMeta(req *MusicMetaRequest) ([]byte, error) {
	return c.post("/api/pjsk/score/meta", req)
}

// =========================== Stamp API ===========================

func (c *HarukiDrawingClient) GenerateStampList(req *StampListRequest) ([]byte, error) {
	return c.post("/api/pjsk/stamp/list", req)
}

// =========================== Chart API ===========================

func (c *HarukiDrawingClient) GenerateMusicChart(req *GenerateMusicChartRequest) ([]byte, error) {
	return c.post("/api/pjsk/chart/generate", req)
}

// =========================== SK API ===========================

func (c *HarukiDrawingClient) GenerateSKLine(req *SklRequest, full bool) ([]byte, error) {
	// Full arg needs to be passed as query param. `post` helper doesn't support query params easily.
	// We might need to handle query params manually or modify `post` helper, but simplicity first:
	// Let's assume URL query construction.
	url := fmt.Sprintf("/api/pjsk/sk/line?full=%t", full)
	return c.post(url, req)
}

func (c *HarukiDrawingClient) GenerateSKQuery(req *SKRequest) ([]byte, error) {
	return c.post("/api/pjsk/sk/query", req)
}

func (c *HarukiDrawingClient) GenerateSKCheckRoom(req *CFRequest) ([]byte, error) {
	return c.post("/api/pjsk/sk/check-room", req)
}

func (c *HarukiDrawingClient) GenerateSKSpeed(req *SpeedRequest) ([]byte, error) {
	return c.post("/api/pjsk/sk/speed", req)
}

func (c *HarukiDrawingClient) GenerateSKPlayerTrace(req *PlayerTraceRequest) ([]byte, error) {
	return c.post("/api/pjsk/sk/player-trace", req)
}

func (c *HarukiDrawingClient) GenerateSKRankTrace(req *RankTraceRequest) ([]byte, error) {
	return c.post("/api/pjsk/sk/rank-trace", req)
}

func (c *HarukiDrawingClient) GenerateSKWinRate(req *WinRateRequest) ([]byte, error) {
	return c.post("/api/pjsk/sk/winrate", req)
}
