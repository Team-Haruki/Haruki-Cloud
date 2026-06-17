package sekai

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"haruki-cloud/config"
	"haruki-cloud/version"

	"github.com/bytedance/sonic"
	"github.com/go-resty/resty/v2"
	"golang.org/x/sync/singleflight"
)

type TrackerClient struct {
	http       *resty.Client
	config     *config.TrackerConfig
	requestCtx context.Context
	flight     *singleflight.Group
}

// NewTrackerClient constructs a TrackerClient bound to the supplied config.
// Callers own the returned client; pass it via dependency injection rather than
// reaching for a package-level singleton.
func NewTrackerClient(cfg *config.TrackerConfig) *TrackerClient {
	timeout := config.TrackerHTTPClientTimeout
	if cfg != nil && cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}
	return &TrackerClient{
		http:   newRestyClient().SetTimeout(timeout),
		config: cfg,
		flight: &singleflight.Group{},
	}
}

func (c *TrackerClient) WithContext(ctx context.Context) *TrackerClient {
	if c == nil {
		return nil
	}
	clone := *c
	clone.requestCtx = ctx
	clone.flight = c.flight
	return &clone
}

func (c *TrackerClient) requestContext() context.Context {
	if c != nil && c.requestCtx != nil {
		return c.requestCtx
	}
	return context.TODO()
}

func (c *TrackerClient) userAgent() string {
	if c != nil && c.config != nil {
		if ua := strings.TrimSpace(c.config.UserAgent); ua != "" {
			return ua
		}
	}
	return version.UserAgent()
}

func (c *TrackerClient) baseURL() (string, error) {
	if c == nil {
		return "", ErrClientNotConfigured
	}
	baseURL := strings.TrimRight(strings.TrimSpace(c.config.BaseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("tracker: base_url is empty")
	}
	return baseURL, nil
}

func (c *TrackerClient) GetEventStatus(server string, eventID int) (*EventStatusResponse, error) {
	path := fmt.Sprintf("/api/v2/cloud/events/%s/%d/leaderboards/total/sk/status", server, eventID)
	return getAs[EventStatusResponse](c, path)
}

func (c *TrackerClient) GetCloudSKQuery(server string, eventID int, characterID *int, ranks []int, userID *int64, includeAdjacent, skipMissing bool, intervalSeconds int64) (*CloudRankQueryResponse, error) {
	values := cloudSKValues(ranks, userID, intervalSeconds)
	values.Set("includeAdjacent", strconv.FormatBool(includeAdjacent))
	values.Set("skipMissing", strconv.FormatBool(skipMissing))
	path := fmt.Sprintf("/api/v2/cloud/events/%s/%d/leaderboards/%s/sk/query?%s", server, eventID, trackerLeaderboardScope(characterID), values.Encode())
	return getAs[CloudRankQueryResponse](c, path)
}

func (c *TrackerClient) GetCloudSKCheckRoom(server string, eventID int, characterID *int, ranks []int, userID *int64, skipMissing bool, intervalSeconds int64) (*CloudCheckRoomResponse, error) {
	values := cloudSKValues(ranks, userID, intervalSeconds)
	values.Set("includeAdjacent", "true")
	values.Set("skipMissing", strconv.FormatBool(skipMissing))
	path := fmt.Sprintf("/api/v2/cloud/events/%s/%d/leaderboards/%s/sk/check-room?%s", server, eventID, trackerLeaderboardScope(characterID), values.Encode())
	return getAs[CloudCheckRoomResponse](c, path)
}

func (c *TrackerClient) GetCloudSKLine(server string, eventID int, characterID *int, ranks []int, userID *int64, skipMissing bool, intervalSeconds int64) (*CloudLineResponse, error) {
	values := cloudSKValues(ranks, userID, intervalSeconds)
	values.Set("skipMissing", strconv.FormatBool(skipMissing))
	path := fmt.Sprintf("/api/v2/cloud/events/%s/%d/leaderboards/%s/sk/line?%s", server, eventID, trackerLeaderboardScope(characterID), values.Encode())
	return getAs[CloudLineResponse](c, path)
}

func (c *TrackerClient) GetCloudSKSpeed(server string, eventID int, characterID *int, ranks []int, intervalSeconds, unitSeconds int64, skipMissing bool) (*CloudSpeedResponse, error) {
	values := cloudSKValues(ranks, nil, intervalSeconds)
	values.Set("unitSeconds", strconv.FormatInt(unitSeconds, 10))
	values.Set("skipMissing", strconv.FormatBool(skipMissing))
	path := fmt.Sprintf("/api/v2/cloud/events/%s/%d/leaderboards/%s/sk/speed?%s", server, eventID, trackerLeaderboardScope(characterID), values.Encode())
	return getAs[CloudSpeedResponse](c, path)
}

func (c *TrackerClient) GetCloudSKTrace(server string, eventID int, characterID *int, subjectType string, subject string, limit int) (*CloudTraceResponse, error) {
	values := url.Values{}
	values.Set("subjectType", subjectType)
	values.Set("subject", subject)
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	path := fmt.Sprintf("/api/v2/cloud/events/%s/%d/leaderboards/%s/sk/trace?%s", server, eventID, trackerLeaderboardScope(characterID), values.Encode())
	return getAs[CloudTraceResponse](c, path)
}

func cloudSKValues(ranks []int, userID *int64, intervalSeconds int64) url.Values {
	values := url.Values{}
	for _, rank := range ranks {
		if rank > 0 {
			values.Add("rank", strconv.Itoa(rank))
		}
	}
	if userID != nil && *userID > 0 {
		values.Set("userId", strconv.FormatInt(*userID, 10))
	}
	if intervalSeconds > 0 {
		values.Set("interval", strconv.FormatInt(intervalSeconds, 10))
	}
	return values
}

func trackerLeaderboardScope(characterID *int) string {
	if characterID != nil && *characterID > 0 {
		return fmt.Sprintf("world-bloom/%d", *characterID)
	}
	return "total"
}

// getAs executes a GET and unmarshals the JSON body into T.
func getAs[T any](c *TrackerClient, path string) (*T, error) {
	body, err := c.getRaw(path)
	if err != nil {
		return nil, err
	}
	var result T
	if err := sonic.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("tracker: failed to unmarshal response: %w", err)
	}
	return &result, nil
}

// getRaw executes a GET request and returns the raw body, mapping HTTP errors
// to typed errors.
func (c *TrackerClient) getRaw(path string) ([]byte, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	baseURL, err := c.baseURL()
	if err != nil {
		return nil, err
	}
	if c.flight == nil {
		c.flight = &singleflight.Group{}
	}
	key := baseURL + path
	value, err, _ := c.flight.Do(key, func() (any, error) {
		url := baseURL + path
		resp, err := c.http.R().
			SetContext(c.requestContext()).
			SetHeader("User-Agent", c.userAgent()).
			Get(url)
		if err != nil {
			return nil, fmt.Errorf("tracker: request failed after retries: %w", sanitizeNetworkError(err))
		}

		switch resp.StatusCode() {
		case 200:
			return append([]byte(nil), resp.Body()...), nil
		case 404:
			return nil, ErrRankingNotFound
		case 429:
			return nil, &TrackerAPIError{StatusCode: 429, Message: "rate limited by tracker"}
		case 503:
			return nil, ErrServerMaintenance
		default:
			msg := parseMessage(resp.Body())
			return nil, &TrackerAPIError{StatusCode: resp.StatusCode(), Message: msg}
		}
	})
	if err != nil {
		return nil, err
	}
	body, _ := value.([]byte)
	return append([]byte(nil), body...), nil
}
