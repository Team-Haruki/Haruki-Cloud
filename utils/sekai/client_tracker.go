package sekai

import (
	"fmt"
	"strings"
	"sync"

	"haruki-cloud/config"

	"github.com/bytedance/sonic"
	"github.com/go-resty/resty/v2"
)

var (
	trackerOnce   sync.Once
	trackerClient *TrackerClient
)

type TrackerClient struct {
	http   *resty.Client
	config *config.TrackerConfig
}

func GetTrackerClient() *TrackerClient {
	trackerOnce.Do(func() {
		c := newRestyClient()

		trackerClient = &TrackerClient{
			http:   c,
			config: &config.Cfg.Tracker,
		}
	})
	return trackerClient
}

// GetLatestRankingByRank fetches the latest ranking snapshot for a specific rank
// in a normal event.
//
//	GET /event/{server}/{eventID}/latest-ranking/rank/{rank}
func (c *TrackerClient) GetLatestRankingByRank(server string, eventID, rank int) (*LatestRankingResponse, error) {
	path := fmt.Sprintf("/event/%s/%d/latest-ranking/rank/%d", server, eventID, rank)
	return getAs[LatestRankingResponse](c, path)
}

// GetLatestRankingByUser fetches the latest ranking snapshot for a specific user
// in a normal event.
//
//	GET /event/{server}/{eventID}/latest-ranking/user/{userID}
func (c *TrackerClient) GetLatestRankingByUser(server string, eventID int, userID int64) (*LatestRankingResponse, error) {
	path := fmt.Sprintf("/event/%s/%d/latest-ranking/user/%d", server, eventID, userID)
	return getAs[LatestRankingResponse](c, path)
}

// GetLatestWorldBloomRankingByRank fetches the latest World Bloom ranking snapshot
// for a specific rank.
//
//	GET /event/{server}/{eventID}/latest-world-bloom-ranking/character/{characterID}/rank/{rank}
func (c *TrackerClient) GetLatestWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*WorldBloomLatestRankingResponse, error) {
	path := fmt.Sprintf("/event/%s/%d/latest-world-bloom-ranking/character/%d/rank/%d", server, eventID, characterID, rank)
	return getAs[WorldBloomLatestRankingResponse](c, path)
}

// GetLatestWorldBloomRankingByUser fetches the latest World Bloom ranking snapshot
// for a specific user.
//
//	GET /event/{server}/{eventID}/latest-world-bloom-ranking/character/{characterID}/user/{userID}
func (c *TrackerClient) GetLatestWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*WorldBloomLatestRankingResponse, error) {
	path := fmt.Sprintf("/event/%s/%d/latest-world-bloom-ranking/character/%d/user/%d", server, eventID, characterID, userID)
	return getAs[WorldBloomLatestRankingResponse](c, path)
}

// TraceRankingByRank fetches the historical score trend for a specific rank
// in a normal event.
//
//	GET /event/{server}/{eventID}/trace-ranking/rank/{rank}
func (c *TrackerClient) TraceRankingByRank(server string, eventID, rank int) (*TraceRankingResponse, error) {
	path := fmt.Sprintf("/event/%s/%d/trace-ranking/rank/%d", server, eventID, rank)
	return getAs[TraceRankingResponse](c, path)
}

// TraceRankingByUser fetches the historical score trend for a specific user
// in a normal event.
//
//	GET /event/{server}/{eventID}/trace-ranking/user/{userID}
func (c *TrackerClient) TraceRankingByUser(server string, eventID int, userID int64) (*TraceRankingResponse, error) {
	path := fmt.Sprintf("/event/%s/%d/trace-ranking/user/%d", server, eventID, userID)
	return getAs[TraceRankingResponse](c, path)
}

// TraceWorldBloomRankingByRank fetches the historical score trend for a specific rank
// in a World Bloom event.
//
//	GET /event/{server}/{eventID}/trace-world-bloom-ranking/character/{characterID}/rank/{rank}
func (c *TrackerClient) TraceWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*WorldBloomTraceRankingResponse, error) {
	path := fmt.Sprintf("/event/%s/%d/trace-world-bloom-ranking/character/%d/rank/%d", server, eventID, characterID, rank)
	return getAs[WorldBloomTraceRankingResponse](c, path)
}

// TraceWorldBloomRankingByUser fetches the historical score trend for a specific user
// in a World Bloom event.
//
//	GET /event/{server}/{eventID}/trace-world-bloom-ranking/character/{characterID}/user/{userID}
func (c *TrackerClient) TraceWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*WorldBloomTraceRankingResponse, error) {
	path := fmt.Sprintf("/event/%s/%d/trace-world-bloom-ranking/character/%d/user/%d", server, eventID, characterID, userID)
	return getAs[WorldBloomTraceRankingResponse](c, path)
}

// GetRankingLines fetches all current border scores for a normal event.
//
//	GET /event/{server}/{eventID}/ranking-lines
func (c *TrackerClient) GetRankingLines(server string, eventID int) ([]RankingLine, error) {
	path := fmt.Sprintf("/event/%s/%d/ranking-lines", server, eventID)
	return getSliceAs[RankingLine](c, path)
}

// GetWorldBloomRankingLines fetches all current border scores for a World Bloom
// event character leaderboard.
//
//	GET /event/{server}/{eventID}/world-bloom-ranking-lines/character/{characterID}
func (c *TrackerClient) GetWorldBloomRankingLines(server string, eventID, characterID int) ([]RankingLine, error) {
	path := fmt.Sprintf("/event/%s/%d/world-bloom-ranking-lines/character/%d", server, eventID, characterID)
	return getSliceAs[RankingLine](c, path)
}

// GetRankingScoreGrowth fetches score growth data for a normal event, bucketed
// by the given interval in seconds.
//
//	GET /event/{server}/{eventID}/ranking-score-growth/interval/{interval}
func (c *TrackerClient) GetRankingScoreGrowth(server string, eventID, interval int) ([]ScoreGrowthPoint, error) {
	path := fmt.Sprintf("/event/%s/%d/ranking-score-growth/interval/%d", server, eventID, interval)
	return getSliceAs[ScoreGrowthPoint](c, path)
}

// GetWorldBloomRankingScoreGrowth fetches score growth data for a World Bloom event
// character leaderboard, bucketed by the given interval in seconds.
//
//	GET /event/{server}/{eventID}/world-bloom-ranking-score-growth/character/{characterID}/interval/{interval}
func (c *TrackerClient) GetWorldBloomRankingScoreGrowth(server string, eventID, characterID, interval int) ([]ScoreGrowthPoint, error) {
	path := fmt.Sprintf("/event/%s/%d/world-bloom-ranking-score-growth/character/%d/interval/%d", server, eventID, characterID, interval)
	return getSliceAs[ScoreGrowthPoint](c, path)
}

// GetUserEventData fetches a user's contribution data for a specific event.
//
//	GET /event/{server}/{eventID}/user-data/{userID}
func (c *TrackerClient) GetUserEventData(server string, eventID int, userID int64) (*UserEventData, error) {
	path := fmt.Sprintf("/event/%s/%d/user-data/%d", server, eventID, userID)
	return getAs[UserEventData](c, path)
}

// GetEventStatus fetches the latest heartbeat/status of the tracker for a given event.
//
//	GET /event/{server}/{eventID}/status
func (c *TrackerClient) GetEventStatus(server string, eventID int) (*EventStatusResponse, error) {
	path := fmt.Sprintf("/event/%s/%d/status", server, eventID)
	return getAs[EventStatusResponse](c, path)
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

// getSliceAs executes a GET and unmarshals the JSON body into []T.
func getSliceAs[T any](c *TrackerClient, path string) ([]T, error) {
	body, err := c.getRaw(path)
	if err != nil {
		return nil, err
	}
	var result []T
	if err := sonic.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("tracker: failed to unmarshal response: %w", err)
	}
	return result, nil
}

// getRaw executes a GET request and returns the raw body, mapping HTTP errors
// to typed errors.
func (c *TrackerClient) getRaw(path string) ([]byte, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(c.config.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("tracker: base_url is empty")
	}
	url := baseURL + path
	resp, err := c.http.R().
		SetHeader("User-Agent", c.config.UserAgent).
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("tracker: request failed after retries: %w", err)
	}

	switch resp.StatusCode() {
	case 200:
		return resp.Body(), nil
	case 404:
		return nil, ErrRankingNotFound
	default:
		msg := parseMessage(resp.Body())
		return nil, &TrackerAPIError{StatusCode: resp.StatusCode(), Message: msg}
	}
}
