package drawing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"

	"github.com/go-resty/resty/v2"
	json "haruki-cloud/internal/jsonutil"
)

const drawingMaxResponseBytes = 64 << 20

const drawingErrorClassificationBytes = 8 << 10

const drawingErrorDetailMaxBytes = 512

var ErrDrawingDataInsufficient = errors.New("drawing response data is insufficient")

func WithTimeout(timeout time.Duration) ClientOption {
	return func(client *resty.Client, _ *HarukiDrawingClient) {
		client.SetTimeout(timeout)
	}
}

func WithRetryCount(retryCount int) ClientOption {
	return func(client *resty.Client, _ *HarukiDrawingClient) {
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

	newLogger := logger.NewLoggerFromGlobal("haruki.client")
	baseURL := ""
	if len(resolvedTargets) > 0 {
		baseURL = resolvedTargets[0].BaseURL
	}
	client := resty.New().
		SetResponseBodyLimit(drawingMaxResponseBytes).
		SetTransport(upstream.NewTunedTransport(upstream.TunedTransportConfig{}))
	drawingClient := &HarukiDrawingClient{
		client:     client,
		baseURL:    baseURL,
		pool:       upstream.NewPoolWithResources(resolvedTargets, shared),
		limiter:    newDrawingLimiter(LimiterConfig{}),
		logger:     newLogger,
		localCache: newLocalRenderCache(0),
	}
	for _, option := range options {
		if option != nil {
			option(client, drawingClient)
		}
	}
	return drawingClient
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
	return c.renderWithCacheRequestAndPrepare(endpoint, request, request, nil, func(_ context.Context, prepared any) ([]byte, error) {
		return render(prepared)
	}, true)
}

func (c *HarukiDrawingClient) RenderWithCacheAndPrepare(endpoint string, request any, prepare func(any) error, render func(any) ([]byte, error)) ([]byte, error) {
	return c.renderWithCacheRequestAndPrepare(endpoint, request, request, func(_ context.Context, prepared any) error {
		if prepare == nil {
			return nil
		}
		return prepare(prepared)
	}, func(_ context.Context, prepared any) ([]byte, error) {
		return render(prepared)
	}, true)
}

func (c *HarukiDrawingClient) RenderWithCacheRequestAndPrepare(endpoint string, cacheRequest any, renderRequest any, prepare func(any) error, render func(any) ([]byte, error)) ([]byte, error) {
	return c.renderWithCacheRequestAndPrepare(endpoint, cacheRequest, renderRequest, func(_ context.Context, prepared any) error {
		if prepare == nil {
			return nil
		}
		return prepare(prepared)
	}, func(_ context.Context, prepared any) ([]byte, error) {
		return render(prepared)
	}, false)
}

func (c *HarukiDrawingClient) renderWithCacheRequestAndPrepare(endpoint string, cacheRequest any, renderRequest any, prepare func(context.Context, any) error, render func(context.Context, any) ([]byte, error), sameRequest bool) ([]byte, error) {
	var requestCtx context.Context
	if c != nil {
		requestCtx = c.requestCtx
	}
	now := time.Now()
	finishCachePrepare := commandtrace.MeasureOperation(requestCtx, "drawing.prepare_cache")
	preparedCache := prepareDrawingRequestBody(endpoint, cacheRequest, now, requestCtx)
	finishCachePrepare()
	var prepareRenderOnce sync.Once
	var preparedRender any
	prepareRender := func(renderCtx context.Context) any {
		prepareRenderOnce.Do(func() {
			if sameRequest {
				preparedRender = preparedCache
				return
			}
			finishRenderPrepare := commandtrace.MeasureOperation(renderCtx, "drawing.prepare_render")
			preparedRender = prepareDrawingRequestBody(endpoint, renderRequest, now, renderCtx)
			finishRenderPrepare()
		})
		return preparedRender
	}
	if c == nil {
		body := prepareRender(requestCtx)
		if prepare != nil {
			if err := prepare(requestCtx, body); err != nil {
				return nil, err
			}
		}
		return render(requestCtx, body)
	}
	renderPrepared := func(renderCtx context.Context) ([]byte, error) {
		body := prepareRender(renderCtx)
		if prepare != nil {
			finishPrepareHook := commandtrace.MeasureOperation(renderCtx, "drawing.prepare_hook")
			err := prepare(renderCtx, body)
			finishPrepareHook()
			if err != nil {
				return nil, err
			}
		}
		active := c.WithContext(renderCtx)
		return active.renderWithPermit(endpoint, body, func(prepared any) ([]byte, error) {
			return render(renderCtx, prepared)
		})
	}
	if c.cache != nil {
		return c.cache.RenderSharedContext(requestCtx, endpoint, preparedRenderCachePayload{payload: preparedCache}, renderPrepared)
	}
	if c.localCache != nil {
		return c.localCache.RenderSharedContext(requestCtx, endpoint, preparedRenderCachePayload{payload: preparedCache}, renderPrepared)
	}
	return renderPrepared(requestCtx)
}

func (c *HarukiDrawingClient) renderWithPermit(endpoint string, prepared any, render func(any) ([]byte, error)) ([]byte, error) {
	finishQueue := commandtrace.MeasureOperation(c.requestCtx, "drawing.queue")
	permit, err := c.acquireRenderPermit(endpoint)
	finishQueue()
	if err != nil {
		return nil, err
	}
	defer permit.release()
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
		finishUpstreamQueue := commandtrace.MeasureOperation(requestCtx, "drawing.upstream_queue")
		lease, err = c.pool.Acquire(requestCtx)
		finishUpstreamQueue()
		if err != nil {
			return nil, fmt.Errorf("drawing upstream is unavailable: %w", err)
		}
		defer lease.Release()
		targetBaseURL = lease.Target.BaseURL
	}
	if strings.TrimSpace(targetBaseURL) == "" {
		return nil, fmt.Errorf("drawing client base_url is empty")
	}

	finishEncode := commandtrace.MeasureOperation(requestCtx, "drawing.encode")
	encodedBody, err := json.Marshal(requestBody)
	finishEncode()
	if err != nil {
		return nil, fmt.Errorf("drawing request encode failed: %w", err)
	}

	request := c.client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(encodedBody)
	if requestCtx != nil {
		request.SetContext(requestCtx)
	}

	tPost := time.Now()
	finishHTTP := commandtrace.MeasureOperation(requestCtx, "drawing.http")
	resp, err := request.Post(targetBaseURL + endpoint)
	finishHTTP()
	elapsed := time.Since(tPost)
	if err != nil {
		c.logger.WarnContext(requestCtx, "drawing request failed",
			"upstream", "drawing",
			"upstream_path", endpoint,
			"duration_ms", commandtrace.Milliseconds(elapsed),
			"error_type", fmt.Sprintf("%T", err),
		)
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		insufficientData := drawingResponseIndicatesInsufficientData(resp.Body())
		detail := ""
		if !insufficientData && resp.StatusCode() >= http.StatusBadRequest && resp.StatusCode() < http.StatusInternalServerError {
			detail = drawingResponseErrorDetail(resp.Body())
		}
		c.logger.WarnContext(requestCtx, "drawing request returned non-success status",
			"upstream", "drawing",
			"upstream_path", endpoint,
			"status_code", resp.StatusCode(),
			"duration_ms", commandtrace.Milliseconds(elapsed),
			"response_bytes", len(resp.Body()),
			"upstream_detail", detail,
		)
		if insufficientData {
			return nil, fmt.Errorf("drawing request failed with status %d: %w", resp.StatusCode(), ErrDrawingDataInsufficient)
		}
		if resp.StatusCode() >= http.StatusBadRequest && resp.StatusCode() < http.StatusInternalServerError && detail != "" {
			return nil, fmt.Errorf("drawing request failed with status %d: %s", resp.StatusCode(), detail)
		}
		return nil, fmt.Errorf("drawing request failed with status %d", resp.StatusCode())
	}
	c.logger.DebugContext(requestCtx, "drawing request completed",
		"upstream", "drawing",
		"upstream_path", endpoint,
		"status_code", resp.StatusCode(),
		"duration_ms", commandtrace.Milliseconds(elapsed),
		"response_bytes", len(resp.Body()),
	)
	return resp.Body(), nil
}

func drawingResponseErrorDetail(body []byte) string {
	if len(body) > drawingErrorClassificationBytes {
		body = body[:drawingErrorClassificationBytes]
	}
	var payload struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	detail := strings.TrimSpace(payload.Detail)
	if detail == "" {
		return ""
	}
	detail = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, detail)
	if len(detail) > drawingErrorDetailMaxBytes {
		detail = strings.TrimSpace(detail[:drawingErrorDetailMaxBytes]) + "..."
	}
	return detail
}

func drawingResponseIndicatesInsufficientData(body []byte) bool {
	if len(body) > drawingErrorClassificationBytes {
		body = body[:drawingErrorClassificationBytes]
	}
	lower := strings.ToLower(string(body))
	for _, marker := range []string{
		"data insufficient",
		"insufficient data",
		"not enough data",
		"数据不足",
		"single positional indexer is out-of-bounds",
		"out-of-bounds",
		"out of bounds",
		"index out of range",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func (c *HarukiDrawingClient) post(endpoint string, body any) ([]byte, error) {
	var requestCtx context.Context
	if c != nil {
		requestCtx = c.requestCtx
	}
	finishPrepare := commandtrace.MeasureOperation(requestCtx, "drawing.prepare_render")
	requestBody := prepareDrawingRequestBody(endpoint, body, time.Now(), requestCtx)
	finishPrepare()
	return c.postPrepared(endpoint, requestBody)
}

func (c *HarukiDrawingClient) cachedPost(endpoint string, body any) ([]byte, error) {
	return c.renderWithCacheRequestAndPrepare(endpoint, body, body, nil, func(renderCtx context.Context, prepared any) ([]byte, error) {
		return c.WithContext(renderCtx).postPrepared(endpoint, prepared)
	}, true)
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

func (c *HarukiDrawingClient) GenerateModularProfile(req *ModularProfileRenderRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/profile/modular", req)
}

func (c *HarukiDrawingClient) GenerateCustomProfileCard(req *CustomProfileCardRenderRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/profile/custom-profile-card", req)
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

// =========================== Costume API ===========================

func (c *HarukiDrawingClient) GenerateCostumeList(req *CostumeListRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/costume/list", req)
}

func (c *HarukiDrawingClient) GenerateCostumeDetail(req *CostumeDetailRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/costume/detail", req)
}

func (c *HarukiDrawingClient) GenerateCostumeDetailWithPrepare(cacheReq any, req *CostumeDetailRequest, prepare func(any) error) ([]byte, error) {
	return c.GenerateCostumeDetailWithContextPrepare(cacheReq, req, func(_ context.Context, prepared any) error {
		if prepare == nil {
			return nil
		}
		return prepare(prepared)
	})
}

func (c *HarukiDrawingClient) GenerateCostumeDetailWithContextPrepare(cacheReq any, req *CostumeDetailRequest, prepare func(context.Context, any) error) ([]byte, error) {
	return c.renderWithCacheRequestAndPrepare("/api/pjsk/costume/detail", cacheReq, req, prepare, func(renderCtx context.Context, prepared any) ([]byte, error) {
		return c.WithContext(renderCtx).postPrepared("/api/pjsk/costume/detail", prepared)
	}, false)
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

func (c *HarukiDrawingClient) GenerateCharacterMissionOverview(req *CharacterMissionOverviewRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/education/character-mission-overview", req)
}

func (c *HarukiDrawingClient) GenerateCharacterMissionAll(req *CharacterMissionAllRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/education/character-mission-all", req)
}

// =========================== Inventory API ===========================

func (c *HarukiDrawingClient) GenerateInventoryList(req *InventoryListRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/inventory/list", req)
}

// =========================== Event API ===========================

func (c *HarukiDrawingClient) GenerateEventDetail(req *EventDetailRequest) ([]byte, error) {
	return c.post("/api/pjsk/event/detail", req)
}

func (c *HarukiDrawingClient) GenerateEventRecord(req *EventRecordRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/event/record", req)
}

func (c *HarukiDrawingClient) GenerateEventList(req *EventListRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/event/list", req)
}

func (c *HarukiDrawingClient) GenerateEventPlanner(req *EventPlannerRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/event/planner", req)
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

func (c *HarukiDrawingClient) GenerateAliasList(req *AliasListRequest) ([]byte, error) {
	// Alias-list watermarks include request DT, so we intentionally bypass the
	// render cache here to avoid serving stale timestamps.
	return c.post("/api/pjsk/misc/alias-list", req)
}

func (c *HarukiDrawingClient) GenerateCommandHelp(req *CommandHelpRenderRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/help/render", req)
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

func (c *HarukiDrawingClient) GenerateMysekaiHousingCompetition(req *MysekaiHousingCompetitionRequest) ([]byte, error) {
	return c.cachedPost("/api/pjsk/mysekai/housing-competition", req)
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
