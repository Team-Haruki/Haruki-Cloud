package sekai

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/core/upstreamerr"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"

	"github.com/go-resty/resty/v2"
)

const (
	apiTimeout  = 5 * time.Second
	tokenHeader = "X-Haruki-Sekai-Token"
	// sekaiTargetDownWindow is how long a target that could not be reached
	// (isNoRouteFailure) is skipped by later calls.
	sekaiTargetDownWindow = 30 * time.Second
	// sekaiDialTimeout bounds connecting to a Sekai API entry, so a dark
	// host is detected on a new connection well inside apiTimeout.
	sekaiDialTimeout = 2 * time.Second
)

type HarukiSekaiAPIClient struct {
	http       *resty.Client
	config     *config.SekaiAPIConfig
	pool       *upstream.Pool
	health     *upstream.Health // nil unless the pool has two or more targets
	requestCtx context.Context
}

// NewSekaiAPIClient constructs a HarukiSekaiAPIClient bound to the supplied config.
// Callers own the returned client; pass it via dependency injection rather than
// reaching for a package-level singleton.
func NewSekaiAPIClient(cfg *config.SekaiAPIConfig) *HarukiSekaiAPIClient {
	var resolvedTargets []upstream.TargetConfig
	if cfg != nil {
		resolvedTargets = cfg.EffectiveTargets()
	}
	pool := upstream.NewPool(resolvedTargets)
	var health *upstream.Health
	if pool.Size() > 1 {
		health = upstream.NewHealth(sekaiTargetDownWindow, nil, logSekaiTargetHealth)
	}
	return &HarukiSekaiAPIClient{
		http:   newRestyClientWithTransport(retryConfig(cfg), upstream.TunedTransportConfig{DialTimeout: sekaiDialTimeout}).SetTimeout(apiTimeout),
		config: cfg,
		pool:   pool,
		health: health,
	}
}

var sekaiAPILogger = logger.NewLoggerFromGlobal("SekaiAPI")

// logSekaiTargetHealth logs a target going down or coming back. The target
// name is logged, never its URL.
func logSekaiTargetHealth(name string, down bool) {
	if down {
		sekaiAPILogger.Warn("sekai api target marked down",
			"upstream", "sekai_api",
			"target", name,
			"down_window_ms", sekaiTargetDownWindow.Milliseconds(),
		)
		return
	}
	sekaiAPILogger.Info("sekai api target recovered",
		"upstream", "sekai_api",
		"target", name,
	)
}

func (c *HarukiSekaiAPIClient) WithContext(ctx context.Context) *HarukiSekaiAPIClient {
	if c == nil {
		return nil
	}
	clone := *c
	clone.requestCtx = ctx
	return &clone
}

func (c *HarukiSekaiAPIClient) requestContext() context.Context {
	if c != nil && c.requestCtx != nil {
		return c.requestCtx
	}
	return context.TODO()
}

// authReq returns a pre-configured request with the token header set.
func (c *HarukiSekaiAPIClient) authReq() *resty.Request {
	if c == nil {
		return newRestyClient(config.UpstreamRetryConfig{}).R()
	}
	request := c.http.R()
	if c.requestCtx != nil {
		request.SetContext(c.requestCtx)
	}
	if c.config != nil && c.config.Token != "" {
		request.SetHeader(tokenHeader, c.config.Token)
	}
	return request
}

// GetUserProfile fetches a user's game profile.
//
//	GET /api/{server}/{userID}/profile
//
// Returns ErrUserNotFound on HTTP 404.
func (c *HarukiSekaiAPIClient) GetUserProfile(server, userID string) (*GetAnotherProfileResponse, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	url := fmt.Sprintf("/api/%s/%s/profile", server, userID)
	body, err := c.get(url)
	if err != nil {
		return nil, err
	}
	var result GetAnotherProfileResponse
	finishDecode := commandtrace.MeasureOperation(c.requestContext(), sekaiDecodeStage)
	decodeErr := json.Unmarshal(body, &result)
	finishDecode()
	if decodeErr != nil {
		return nil, upstreamerr.Tag(upstreamerr.ServiceGameData, upstreamerr.KindBadResponse, "", fmt.Errorf("sekai api: failed to unmarshal profile response: %w", decodeErr))
	}
	return &result, nil
}

// GetUserProfileContext is GetUserProfile with request cancellation and tracing.
func (c *HarukiSekaiAPIClient) GetUserProfileContext(ctx context.Context, server, userID string) (*GetAnotherProfileResponse, error) {
	return c.WithContext(ctx).GetUserProfile(server, userID)
}

// GetSystem fetches the current game system status.
//
//	GET /api/{server}/system
func (c *HarukiSekaiAPIClient) GetSystem(server string) (*GetSystemResponse, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	url := fmt.Sprintf("/api/%s/system", server)
	body, err := c.get(url)
	if err != nil {
		return nil, err
	}
	var result GetSystemResponse
	finishDecode := commandtrace.MeasureOperation(c.requestContext(), sekaiDecodeStage)
	decodeErr := json.Unmarshal(body, &result)
	finishDecode()
	if decodeErr != nil {
		return nil, upstreamerr.Tag(upstreamerr.ServiceGameData, upstreamerr.KindBadResponse, "", fmt.Errorf("sekai api: failed to unmarshal system response: %w", decodeErr))
	}
	return &result, nil
}

// GetInformation fetches in-game information / announcements.
//
//	GET /api/{server}/information
func (c *HarukiSekaiAPIClient) GetInformation(server string) (*GetInformationResponse, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	url := fmt.Sprintf("/api/%s/information", server)
	body, err := c.get(url)
	if err != nil {
		return nil, err
	}
	var result GetInformationResponse
	finishDecode := commandtrace.MeasureOperation(c.requestContext(), sekaiDecodeStage)
	decodeErr := json.Unmarshal(body, &result)
	finishDecode()
	if decodeErr != nil {
		return nil, upstreamerr.Tag(upstreamerr.ServiceGameData, upstreamerr.KindBadResponse, "", fmt.Errorf("sekai api: failed to unmarshal information response: %w", decodeErr))
	}
	return &result, nil
}

// GetMySekaiImage downloads a MySekai photo image.
//
//	GET /image/{server}/mysekai/{imagePath}
//
// imagePath is the sub-path identifying the photo, e.g. "12345/6" for CN/TW
// or the raw imagePath value returned by the suite API for JP/EN.
func (c *HarukiSekaiAPIClient) GetMySekaiImage(server, imagePath string) ([]byte, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	url := fmt.Sprintf("/image/%s/mysekai/%s", server, imagePath)
	return c.get(url)
}

// GetMySekaiHousingCompetitionList fetches the MySekai housing competition
// submission list.
//
//	GET /api/{server}/user/mysekai/housing-competition/{housingID}/list
func (c *HarukiSekaiAPIClient) GetMySekaiHousingCompetitionList(server string, housingID int, isLottery bool) (json.RawMessage, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	query := url.Values{}
	if isLottery {
		query.Set("isLottery", "True")
	} else {
		query.Set("isLottery", "False")
	}
	path := fmt.Sprintf("/api/%s/user/mysekai/housing-competition/%d/list?%s", server, housingID, query.Encode())
	body, err := c.get(path)
	if err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), body...), nil
}

// EnterMySekaiHousingCompetitionEntry enters / fetches detail for a single
// MySekai housing competition submission.
//
//	POST /api/{server}/user/mysekai/housing-competition/{housingID}/mysekai-owner/{ownerUserID}/entry
func (c *HarukiSekaiAPIClient) EnterMySekaiHousingCompetitionEntry(server string, housingID int, ownerUserID, submittedAt int64, isBackNumber bool) (json.RawMessage, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	query := url.Values{}
	query.Set("mysekaiOwnerUserSubmittedAt", fmt.Sprintf("%d", submittedAt))
	if isBackNumber {
		query.Set("isBackNumber", "true")
	} else {
		query.Set("isBackNumber", "false")
	}
	path := fmt.Sprintf(
		"/api/%s/user/mysekai/housing-competition/%d/mysekai-owner/%d/entry?%s",
		server,
		housingID,
		ownerUserID,
		query.Encode(),
	)
	body, err := c.post(path, nil)
	if err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), body...), nil
}

// GetMySekaiHousingCompetitionBackNumberTopList fetches the historical MySekai
// housing competition overview.
//
//	GET /api/{server}/mysekai/housing-competition/back-number-top-list
func (c *HarukiSekaiAPIClient) GetMySekaiHousingCompetitionBackNumberTopList(server string) (json.RawMessage, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	path := fmt.Sprintf("/api/%s/mysekai/housing-competition/back-number-top-list", server)
	body, err := c.get(path)
	if err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), body...), nil
}

// GetMySekaiHousingCompetitionBackNumberList fetches a historical MySekai
// housing competition list.
//
//	GET /api/{server}/mysekai/housing-competition/{competitionID}/back-number-list
func (c *HarukiSekaiAPIClient) GetMySekaiHousingCompetitionBackNumberList(server string, competitionID int) (json.RawMessage, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	path := fmt.Sprintf("/api/%s/mysekai/housing-competition/%d/back-number-list", server, competitionID)
	body, err := c.get(path)
	if err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), body...), nil
}

// GetMySekaiHousingThumbnail downloads a MySekai housing competition thumbnail.
//
//	GET /image/{server}/mysekai-housing/{imagePath}
func (c *HarukiSekaiAPIClient) GetMySekaiHousingThumbnail(server, imagePath string) ([]byte, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	path := fmt.Sprintf("/image/%s/mysekai-housing/%s", server, strings.TrimLeft(imagePath, "/"))
	return c.get(path)
}

// GetCustomProfileCardThumbnail downloads a custom profile card thumbnail image.
//
//	GET /image/{server}/custom-profile-card/thumbnail/{imagePath}
//
// imagePath is the raw thumbnailPath value from suite.userCustomProfileCards,
// e.g. "{64-char-hex}/{uuid}" for colorful palette servers.
func (c *HarukiSekaiAPIClient) GetCustomProfileCardThumbnail(server, imagePath string) ([]byte, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	url := fmt.Sprintf("/image/%s/custom-profile-card/thumbnail/%s", server, imagePath)
	return c.get(url)
}

// GetCustomMusicScorePublished fetches a published custom music score by its
// share ID.
//
//	GET /api/{server}/user/%user_id/custom-music-score/published/search/{scoreID}
func (c *HarukiSekaiAPIClient) GetCustomMusicScorePublished(server, scoreID string) (*UserCustomMusicScorePublishedResponse, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	scoreID = strings.TrimSpace(scoreID)
	if scoreID == "" {
		return nil, ErrUserNotFound
	}
	path := fmt.Sprintf(
		"/api/%s/user/%%25user_id/custom-music-score/published/search/%s",
		server,
		url.PathEscape(scoreID),
	)
	body, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var result CustomMusicScorePublishedSearchResponse
	finishDecode := commandtrace.MeasureOperation(c.requestContext(), sekaiDecodeStage)
	decodeErr := json.Unmarshal(body, &result)
	finishDecode()
	if decodeErr != nil {
		return nil, upstreamerr.Tag(upstreamerr.ServiceGameData, upstreamerr.KindBadResponse, "", fmt.Errorf("sekai api: failed to unmarshal custom music score response: %w", decodeErr))
	}
	if result.UserCustomMusicScoreInfoJSON == nil {
		return nil, ErrUserNotFound
	}
	return result.UserCustomMusicScoreInfoJSON, nil
}

// GetCustomMusicScore downloads a published custom music score JSON blob.
//
//	GET /image/{server}/blob/custom-music-score/full/{scorePath}
//
// scorePath is the raw userCustomMusicScorePath value from suite custom score
// responses, e.g. "{64-char-hex}/{64-char-hex}" for colorful palette servers.
func (c *HarukiSekaiAPIClient) GetCustomMusicScore(server, scorePath string) ([]byte, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	url := fmt.Sprintf("/image/%s/blob/custom-music-score/full/%s", server, strings.TrimLeft(scorePath, "/"))
	return c.get(url)
}

// get executes a GET request with auth and maps HTTP status codes to typed
// errors. A transport failure moves the request to the next target.
func (c *HarukiSekaiAPIClient) get(path string) ([]byte, error) {
	return c.send(resty.MethodGet, path, nil)
}

// post executes a POST request. It is never replayed on another target.
func (c *HarukiSekaiAPIClient) post(path string, body any) ([]byte, error) {
	return c.send(resty.MethodPost, path, body)
}

// send runs one call. With several targets, a no-route failure
// (isNoRouteFailure: the target could not be reached at all) marks the
// target down for sekaiTargetDownWindow; a GET then moves at once to the
// next target, each target tried at most once, while a POST returns the
// error. Any HTTP response, 5xx and 503 included, and any timeout after the
// connection was made keep the target up and end the call (after the usual
// same-URL retries). While another target is left to try, the attempt skips
// resty's same-URL retries of no-route failures only; the last target keeps
// the full retry policy, so a single-target client behaves as before.
func (c *HarukiSekaiAPIClient) send(method, path string, body any) ([]byte, error) {
	failover := method == resty.MethodGet
	var tried []string
	for {
		target, lease, err := c.acquireTarget(tried)
		if err != nil {
			return nil, err
		}
		request := c.authReq()
		if failover && c.health != nil && len(tried)+1 < c.pool.Size() {
			request.SetContext(context.WithValue(request.Context(), skipTransportRetryKey{}, true))
		}
		if body != nil {
			request.SetBody(body)
		}
		finishHTTP := commandtrace.MeasureOperation(c.requestContext(), "sekai.http")
		resp, err := request.Execute(method, target.BaseURL+path)
		finishHTTP()
		lease.Release()
		if err == nil {
			c.health.MarkUp(target.Name)
			return handleSekaiAPIResponse(resp)
		}
		if ctxErr := c.requestContext().Err(); ctxErr != nil {
			return nil, requestContextError(upstreamerr.ServiceGameData, ctxErr)
		}
		if c.health != nil && isNoRouteFailure(resp, err) {
			c.health.MarkDown(target.Name)
			tried = append(tried, target.Name)
			if failover && len(tried) < c.pool.Size() {
				commandtrace.RecordOperation(c.requestContext(), "sekai.failover", 0)
				continue
			}
		}
		return nil, sanitizeNetworkError(upstreamerr.ServiceGameData, "sekai api: request failed after retries", err)
	}
}

func handleSekaiAPIResponse(resp *resty.Response) ([]byte, error) {
	switch resp.StatusCode() {
	case 200:
		return resp.Body(), nil
	case 404:
		return nil, ErrUserNotFound
	case 503:
		return nil, ErrServerMaintenance
	default:
		msg := parseMessage(resp.Body())
		return nil, &APIError{StatusCode: resp.StatusCode(), Message: msg}
	}
}

// acquireTarget leases a target not in tried. Without health tracking
// (fewer than two targets) it is the plain pool pick, or the legacy
// base_url when no pool is configured.
func (c *HarukiSekaiAPIClient) acquireTarget(tried []string) (upstream.TargetConfig, *upstream.Lease, error) {
	if c == nil {
		return upstream.TargetConfig{}, nil, ErrClientNotConfigured
	}
	if c.pool != nil && c.pool.Enabled() {
		finishQueue := commandtrace.MeasureOperation(c.requestContext(), "sekai.queue")
		var lease *upstream.Lease
		var err error
		if c.health != nil {
			lease, err = c.pool.AcquireHealthy(c.requestContext(), c.health, tried)
		} else {
			lease, err = c.pool.Acquire(c.requestContext())
		}
		finishQueue()
		if err != nil {
			return upstream.TargetConfig{}, nil, upstreamerr.Tag(upstreamerr.ServiceGameData, upstreamerr.KindUnavailable, "", fmt.Errorf("sekai api: upstream unavailable: %w", err))
		}
		return lease.Target, lease, nil
	}
	if c.config == nil {
		return upstream.TargetConfig{}, nil, upstreamerr.Tag(upstreamerr.ServiceGameData, upstreamerr.KindNotConfigured, "sekai api: base_url is empty", nil)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(c.config.BaseURL), "/")
	if baseURL == "" {
		return upstream.TargetConfig{}, nil, upstreamerr.Tag(upstreamerr.ServiceGameData, upstreamerr.KindNotConfigured, "sekai api: base_url is empty", nil)
	}
	return upstream.TargetConfig{BaseURL: baseURL}, nil, nil
}
