package sekai

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/upstream"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"

	"github.com/go-resty/resty/v2"
)

const (
	maxRetries    = 4
	retryWaitTime = time.Second
	apiTimeout    = 5 * time.Second
	tokenHeader   = "X-Haruki-Sekai-Token"
)

type HarukiSekaiAPIClient struct {
	http       *resty.Client
	config     *config.SekaiAPIConfig
	pool       *upstream.Pool
	requestCtx context.Context
}

// NewSekaiAPIClient constructs a HarukiSekaiAPIClient bound to the supplied config.
// Callers own the returned client; pass it via dependency injection rather than
// reaching for a package-level singleton.
func NewSekaiAPIClient(cfg *config.SekaiAPIConfig) *HarukiSekaiAPIClient {
	var resolvedTargets []upstream.TargetConfig
	if cfg != nil {
		resolvedTargets = upstream.ResolveTargets(cfg.BaseURL, cfg.Targets, "sekai-api")
	}
	return &HarukiSekaiAPIClient{
		http:   newRestyClient().SetTimeout(apiTimeout),
		config: cfg,
		pool:   upstream.NewPool(resolvedTargets),
	}
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
		return newRestyClient().R()
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
	finishDecode := commandtrace.MeasureOperation(c.requestContext(), "sekai.decode")
	decodeErr := json.Unmarshal(body, &result)
	finishDecode()
	if decodeErr != nil {
		return nil, fmt.Errorf("sekai api: failed to unmarshal profile response: %w", decodeErr)
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
	finishDecode := commandtrace.MeasureOperation(c.requestContext(), "sekai.decode")
	decodeErr := json.Unmarshal(body, &result)
	finishDecode()
	if decodeErr != nil {
		return nil, fmt.Errorf("sekai api: failed to unmarshal system response: %w", decodeErr)
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
	finishDecode := commandtrace.MeasureOperation(c.requestContext(), "sekai.decode")
	decodeErr := json.Unmarshal(body, &result)
	finishDecode()
	if decodeErr != nil {
		return nil, fmt.Errorf("sekai api: failed to unmarshal information response: %w", decodeErr)
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
	finishDecode := commandtrace.MeasureOperation(c.requestContext(), "sekai.decode")
	decodeErr := json.Unmarshal(body, &result)
	finishDecode()
	if decodeErr != nil {
		return nil, fmt.Errorf("sekai api: failed to unmarshal custom music score response: %w", decodeErr)
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

// get executes a GET request with auth and maps HTTP status codes to typed errors.
func (c *HarukiSekaiAPIClient) get(path string) ([]byte, error) {
	baseURL, lease, err := c.acquireTarget()
	if err != nil {
		return nil, err
	}
	if lease != nil {
		defer lease.Release()
	}

	finishHTTP := commandtrace.MeasureOperation(c.requestContext(), "sekai.http")
	resp, err := c.authReq().Get(baseURL + path)
	finishHTTP()
	if err != nil {
		if ctxErr := c.requestContext().Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("sekai api: request failed after retries: %w", sanitizeNetworkError(err))
	}

	return handleSekaiAPIResponse(resp)
}

func (c *HarukiSekaiAPIClient) post(path string, body any) ([]byte, error) {
	baseURL, lease, err := c.acquireTarget()
	if err != nil {
		return nil, err
	}
	if lease != nil {
		defer lease.Release()
	}

	request := c.authReq()
	if body != nil {
		request.SetBody(body)
	}
	finishHTTP := commandtrace.MeasureOperation(c.requestContext(), "sekai.http")
	resp, err := request.Post(baseURL + path)
	finishHTTP()
	if err != nil {
		if ctxErr := c.requestContext().Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("sekai api: request failed after retries: %w", sanitizeNetworkError(err))
	}
	return handleSekaiAPIResponse(resp)
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

func (c *HarukiSekaiAPIClient) acquireTarget() (string, *upstream.Lease, error) {
	if c == nil {
		return "", nil, ErrClientNotConfigured
	}
	if c.pool != nil && c.pool.Enabled() {
		finishQueue := commandtrace.MeasureOperation(c.requestContext(), "sekai.queue")
		lease, err := c.pool.Acquire(c.requestContext())
		finishQueue()
		if err != nil {
			return "", nil, fmt.Errorf("sekai api: upstream unavailable: %w", err)
		}
		return lease.Target.BaseURL, lease, nil
	}
	if c.config == nil {
		return "", nil, fmt.Errorf("sekai api: base_url is empty")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(c.config.BaseURL), "/")
	if baseURL == "" {
		return "", nil, fmt.Errorf("sekai api: base_url is empty")
	}
	return baseURL, nil, nil
}
