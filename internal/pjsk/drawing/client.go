package drawing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/utils/logger"

	"github.com/go-resty/resty/v2"
)

func WithTimeout(timeout time.Duration) ClientOption {
	return func(client *resty.Client) {
		client.SetTimeout(timeout)
	}
}

func WithRetryCount(retryCount int) ClientOption {
	return func(client *resty.Client) {
		client.SetRetryCount(retryCount)
	}
}

func NewHarukiDrawingClient(baseURL string, options ...ClientOption) *HarukiDrawingClient {
	return newHarukiDrawingClient(false, baseURL, nil, nil, options...)
}

func NewHarukiDrawingClientWithTargets(legacyBaseURL string, targets []upstream.TargetConfig, options ...ClientOption) *HarukiDrawingClient {
	return newHarukiDrawingClient(true, legacyBaseURL, targets, nil, options...)
}

func NewHarukiDrawingClientWithTargetsAndResources(legacyBaseURL string, targets []upstream.TargetConfig, shared *upstream.SharedResources, options ...ClientOption) *HarukiDrawingClient {
	return newHarukiDrawingClient(true, legacyBaseURL, targets, shared, options...)
}

func newHarukiDrawingClient(strict bool, legacyBaseURL string, targets []upstream.TargetConfig, shared *upstream.SharedResources, options ...ClientOption) *HarukiDrawingClient {
	resolvedTargets := upstream.ResolveTargets(legacyBaseURL, targets, "drawing")
	if strict && len(resolvedTargets) == 0 {
		return nil
	}

	client := resty.New()
	for _, option := range options {
		if option != nil {
			option(client)
		}
	}

	newLogger := logger.NewLogger("haruki.client", harukiConfig.Cfg.Backend.LogLevel, os.Stdout)
	baseURL := ""
	if len(resolvedTargets) > 0 {
		baseURL = resolvedTargets[0].BaseURL
	}
	return &HarukiDrawingClient{
		client:     client,
		baseURL:    baseURL,
		pool:       upstream.NewPoolWithResources(resolvedTargets, shared),
		logger:     newLogger,
		localCache: newLocalRenderCache(0),
	}
}

func (c *HarukiDrawingClient) SetRenderCache(cache *RenderCacheClient) {
	if c == nil {
		return
	}
	c.cache = cache
}

func (c *HarukiDrawingClient) WithContext(ctx context.Context) *HarukiDrawingClient {
	if c == nil {
		return nil
	}
	clone := *c
	clone.requestCtx = ctx
	return &clone
}

func (c *HarukiDrawingClient) RenderWithCache(endpoint string, request any, render func(any) ([]byte, error)) ([]byte, error) {
	var requestCtx context.Context
	if c != nil {
		requestCtx = c.requestCtx
	}
	prepared := prepareDrawingRequestBody(endpoint, request, time.Now(), requestCtx)
	if c == nil {
		return render(prepared)
	}
	if c.cache != nil {
		return c.cache.Render(endpoint, prepared, func() ([]byte, error) {
			return render(prepared)
		})
	}
	if c.localCache != nil {
		return c.localCache.Render(endpoint, prepared, func() ([]byte, error) {
			return render(prepared)
		})
	}
	return render(prepared)
}

func (c *HarukiDrawingClient) postPrepared(endpoint string, requestBody any) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}

	requestCtx := c.requestCtx
	targetBaseURL := c.baseURL
	var lease *upstream.Lease
	var err error
	if c.pool != nil && c.pool.Enabled() {
		lease, err = c.pool.Acquire(requestCtx)
		if err != nil {
			return nil, fmt.Errorf("drawing upstream is unavailable: %w", err)
		}
		defer lease.Release()
		targetBaseURL = lease.Target.BaseURL
	}
	if strings.TrimSpace(targetBaseURL) == "" {
		return nil, fmt.Errorf("drawing client base_url is empty")
	}

	request := c.client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(requestBody)
	if requestCtx != nil {
		request.SetContext(requestCtx)
	}

	resp, err := request.Post(targetBaseURL + endpoint)
	data, _ := json.Marshal(requestBody)
	c.logger.Debugf("POST %s: %s", targetBaseURL+endpoint, string(data))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("api request failed with status: %d, body: %s", resp.StatusCode(), resp.String())
	}
	c.logger.Debugf("Response from %s: type %s, length %s", targetBaseURL+endpoint, resp.Header().Get("Content-Type"), resp.Header().Get("content-length"))
	return resp.Body(), nil
}

func (c *HarukiDrawingClient) post(endpoint string, body any) ([]byte, error) {
	var requestCtx context.Context
	if c != nil {
		requestCtx = c.requestCtx
	}
	requestBody := prepareDrawingRequestBody(endpoint, body, time.Now(), requestCtx)
	return c.postPrepared(endpoint, requestBody)
}

func (c *HarukiDrawingClient) cachedPost(endpoint string, body any) ([]byte, error) {
	return c.RenderWithCache(endpoint, body, func(prepared any) ([]byte, error) {
		return c.postPrepared(endpoint, prepared)
	})
}

// =========================== Music API ===========================

func (c *HarukiDrawingClient) GenerateMusicDetail(req *MusicDetailRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/music/detail", req)
}

func (c *HarukiDrawingClient) GenerateMusicBriefList(req *MusicBriefListRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/music/brief-list", req)
}

func (c *HarukiDrawingClient) GenerateMusicList(req *MusicListRequest, showID bool, showLeak bool) ([]byte, error) {
	// Query params: show_id, show_leak
	endpoint := fmt.Sprintf("/api/pjsk/music/list?show_id=%v&show_leak=%v", showID, showLeak)
	return c.cachedPost(endpoint, req)
}

func (c *HarukiDrawingClient) GeneratePlayProgress(req *PlayProgressRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/music/progress", req)
}

func (c *HarukiDrawingClient) GenerateDetailMusicRewards(req *DetailMusicRewardsRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/music/rewards/detail", req)
}

func (c *HarukiDrawingClient) GenerateBasicMusicRewards(req *BasicMusicRewardsRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/music/rewards/basic", req)
}

// =========================== Profile API ===========================

func (c *HarukiDrawingClient) GenerateProfile(req *ProfileRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/profile", req)
}

// =========================== Card API ===========================

func (c *HarukiDrawingClient) GenerateCardDetail(req *CardDetailRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/card/detail", req)
}

func (c *HarukiDrawingClient) GenerateCardList(req *CardListRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/card/list", req)
}

func (c *HarukiDrawingClient) GenerateCardBox(req *CardBoxRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/card/box", req)
}

// =========================== Deck API ===========================

func (c *HarukiDrawingClient) GenerateDeckRecommendation(req *DeckRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/deck/recommend", req)
}

// =========================== Education API ===========================

func (c *HarukiDrawingClient) GenerateChallengeLiveDetails(req *ChallengeLiveDetailsRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/education/challenge-live", req)
}

func (c *HarukiDrawingClient) GeneratePowerBonusDetail(req *PowerBonusDetailRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/education/power-bonus", req)
}

func (c *HarukiDrawingClient) GenerateAreaItemUpgradeMaterials(req *AreaItemUpgradeMaterialsRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/education/area-item", req)
}

func (c *HarukiDrawingClient) GenerateBonds(req *BondsRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/education/bonds", req)
}

func (c *HarukiDrawingClient) GenerateLeaderCount(req *LeaderCountRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/education/leader-count", req)
}

// =========================== Event API ===========================

func (c *HarukiDrawingClient) GenerateEventDetail(req *EventDetailRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/event/detail", req)
}

func (c *HarukiDrawingClient) GenerateEventRecord(req *EventRecordRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/event/record", req)
}

func (c *HarukiDrawingClient) GenerateEventList(req *EventListRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/event/list", req)
}

// =========================== VLive API ===========================

func (c *HarukiDrawingClient) GenerateVLiveList(req *VLiveListRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/vlive/list", req)
}

// =========================== Gacha API ===========================

func (c *HarukiDrawingClient) GenerateGachaList(req *GachaListRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/gacha/list", req)
}

func (c *HarukiDrawingClient) GenerateGachaDetail(req *GachaDetailRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/gacha/detail", req)
}

// =========================== Honor API ===========================

func (c *HarukiDrawingClient) GenerateHonor(req *HonorRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/honor", req)
}

// =========================== Misc API ===========================

func (c *HarukiDrawingClient) GenerateCharacterBirthday(req *CharaBirthdayRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/misc/chara-birthday", req)
}

// =========================== MySekai API ===========================

func (c *HarukiDrawingClient) GenerateMysekaiResource(req *MysekaiResourceRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/mysekai/resource", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiMap(req *MysekaiMsrMapRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/mysekai/map", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiFixtureList(req *MysekaiFixtureListRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/mysekai/fixture-list", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiFixtureDetail(req *MysekaiFixtureDetailRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/mysekai/fixture-detail", []any{req})
}

func (c *HarukiDrawingClient) GenerateMysekaiDoorUpgrade(req *MysekaiDoorUpgradeRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/mysekai/door-upgrade", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiMusicRecord(req *MysekaiMusicrecordRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/mysekai/music-record", req)
}

func (c *HarukiDrawingClient) GenerateMysekaiTalkList(req *MysekaiTalkListRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/mysekai/talk-list", req)
}

// =========================== Score API ===========================

func (c *HarukiDrawingClient) GenerateScoreControl(req *ScoreControlRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/score/control", req)
}

func (c *HarukiDrawingClient) GenerateCustomRoomScore(req *CustomRoomScoreRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/score/custom-room", req)
}

func (c *HarukiDrawingClient) GenerateMusicMeta(req []MusicMetaRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/score/music-meta", req)
}

func (c *HarukiDrawingClient) GenerateMusicBoard(req *MusicBoardRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/score/music-board", req)
}

// =========================== Stamp API ===========================

func (c *HarukiDrawingClient) GenerateStampList(req *StampListRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/stamp/list", req)
}

// =========================== Chart API ===========================

func (c *HarukiDrawingClient) GenerateMusicChart(req *GenerateMusicChartRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/chart", req)
}

// =========================== SK API ===========================

func (c *HarukiDrawingClient) GenerateSKLine(req *SklRequest, full bool) ([]byte, error) {
	// Full arg needs to be passed as query param. `post` helper doesn't support query params easily.
	// We might need to handle query params manually or modify `post` helper, but simplicity first:
	// Let's assume URL query construction.
	url := fmt.Sprintf("/api/pjsk/sk/line?full=%t", full)
	return c.cachedPost(url, req)
}

func (c *HarukiDrawingClient) GenerateSKQuery(req *SKRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/sk/query", req)
}

func (c *HarukiDrawingClient) GenerateSKCheckRoom(req *CFRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/sk/check-room", req)
}

func (c *HarukiDrawingClient) GenerateSKCSB(req *CSBRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/sk/csb", req)
}

func (c *HarukiDrawingClient) GenerateSKSpeed(req *SpeedRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/sk/speed", req)
}

func (c *HarukiDrawingClient) GenerateSKPlayerTrace(req *PlayerTraceRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/sk/player-trace", req)
}

func (c *HarukiDrawingClient) GenerateSKRankTrace(req *RankTraceRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/sk/rank-trace", req)
}

func (c *HarukiDrawingClient) GenerateSKWinRate(req *WinRateRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/sk/winrate", req)
}
