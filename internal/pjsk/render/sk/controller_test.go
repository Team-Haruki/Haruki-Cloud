package sk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type testForecastProvider struct {
	scores   map[int]ForecastScore
	bySource map[string]ForecastSourceData
	err      error
}

type ctxKey string

type contextAwareForecastProvider struct {
	wantKey   ctxKey
	wantValue string
}

type countingForecastProvider struct {
	calls    atomic.Int32
	bySource map[string]ForecastSourceData
}

type scopedForecastProvider struct {
	mu      sync.Mutex
	queries []ForecastQuery
	data    map[string]map[string]ForecastSourceData
}

func (p contextAwareForecastProvider) Fetch(ctx context.Context, _ string, _ int, ranks []int) (map[int]ForecastScore, error) {
	value, _ := ctx.Value(p.wantKey).(string)
	if value != p.wantValue {
		return nil, context.Canceled
	}
	if len(ranks) == 0 {
		ranks = []int{100}
	}
	out := make(map[int]ForecastScore, len(ranks))
	for _, rank := range ranks {
		out[rank] = ForecastScore{
			Score:     8_000_000 + rank,
			Timestamp: 1_700_000_000,
			Source:    "forecast",
		}
	}
	return out, nil
}

func (p *countingForecastProvider) Fetch(context.Context, string, int, []int) (map[int]ForecastScore, error) {
	p.calls.Add(1)
	out := make(map[int]ForecastScore)
	for _, source := range p.bySource {
		for rank, score := range source.Scores {
			existing, ok := out[rank]
			if !ok || score.Score > existing.Score {
				out[rank] = score
			}
		}
	}
	return out, nil
}

func (p *countingForecastProvider) FetchBySource(context.Context, string, int, []int) (map[string]ForecastSourceData, error) {
	p.calls.Add(1)
	out := make(map[string]ForecastSourceData, len(p.bySource))
	for source, data := range p.bySource {
		copied := make(map[int]ForecastScore, len(data.Scores))
		for rank, score := range data.Scores {
			copied[rank] = score
		}
		out[source] = ForecastSourceData{
			Scores:    copied,
			FetchedAt: data.FetchedAt,
		}
	}
	return out, nil
}

func (p *scopedForecastProvider) Fetch(context.Context, string, int, []int) (map[int]ForecastScore, error) {
	return nil, errors.New("not implemented")
}

func (p *scopedForecastProvider) FetchBySourceQuery(_ context.Context, query ForecastQuery) (map[string]ForecastSourceData, error) {
	normalized := normalizeForecastQuery(query)
	p.mu.Lock()
	p.queries = append(p.queries, normalized)
	p.mu.Unlock()
	key := string(normalized.Scope)
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		key += ":" + strconv.Itoa(*normalized.WlCharacterID)
	}
	sourceData, ok := p.data[key]
	if !ok {
		return nil, errors.New("unexpected forecast query")
	}
	out := make(map[string]ForecastSourceData, len(sourceData))
	for source, data := range sourceData {
		copied := make(map[int]ForecastScore, len(data.Scores))
		for rank, score := range data.Scores {
			copied[rank] = score
		}
		out[source] = ForecastSourceData{
			Scores:    copied,
			FetchedAt: data.FetchedAt,
		}
	}
	return out, nil
}

func (p *scopedForecastProvider) querySnapshot() []ForecastQuery {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]ForecastQuery(nil), p.queries...)
}

func (p testForecastProvider) Fetch(context.Context, string, int, []int) (map[int]ForecastScore, error) {
	if p.err != nil {
		return nil, p.err
	}
	if len(p.bySource) > 0 {
		out := make(map[int]ForecastScore)
		for _, source := range p.bySource {
			for rank, score := range source.Scores {
				existing, ok := out[rank]
				if !ok || score.Score > existing.Score {
					out[rank] = score
				}
			}
		}
		return out, nil
	}
	out := make(map[int]ForecastScore, len(p.scores))
	for rank, score := range p.scores {
		out[rank] = score
	}
	return out, nil
}

func (p testForecastProvider) FetchBySource(context.Context, string, int, []int) (map[string]ForecastSourceData, error) {
	if p.err != nil {
		return nil, p.err
	}
	out := make(map[string]ForecastSourceData, len(p.bySource))
	for source, data := range p.bySource {
		copied := make(map[int]ForecastScore, len(data.Scores))
		for rank, score := range data.Scores {
			copied[rank] = score
		}
		out[source] = ForecastSourceData{
			Scores:    copied,
			FetchedAt: data.FetchedAt,
		}
	}
	return out, nil
}

type testTrackerSource struct{}

func (testTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) GetLatestWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) GetLatestWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) GetUserEventData(server string, eventID int, userID int64) (*sekaiapi.UserEventData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) GetWorldBloomRankingScoreGrowth(server string, eventID, characterID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) TraceWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) TraceWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

type lineNameTrackerSource struct{}

func (lineNameTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.Itoa(10000 + rank),
			Score:     1000000 + rank,
			Rank:      rank,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.Itoa(10000 + rank),
			Name:   "LineNameUser",
		},
	}, nil
}

func (lineNameTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (lineNameTrackerSource) GetLatestWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (lineNameTrackerSource) GetLatestWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (lineNameTrackerSource) GetUserEventData(server string, eventID int, userID int64) (*sekaiapi.UserEventData, error) {
	return &sekaiapi.UserEventData{
		UserID: strconv.FormatInt(userID, 10),
		Name:   "LineEventUser",
	}, nil
}

func (lineNameTrackerSource) GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return nil, fmt.Errorf("not implemented")
}

func (lineNameTrackerSource) GetWorldBloomRankingScoreGrowth(server string, eventID, characterID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return nil, fmt.Errorf("not implemented")
}

func (lineNameTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (lineNameTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (lineNameTrackerSource) TraceWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (lineNameTrackerSource) TraceWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

type missingDefaultRankLineTrackerSource struct {
	lineNameTrackerSource
}

func (missingDefaultRankLineTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	if rank == 300000 {
		return nil, sekaiapi.ErrRankingNotFound
	}
	return lineNameTrackerSource{}.GetLatestRankingByRank(server, eventID, rank)
}

type worldBloomLineTrackerSource struct {
	lineNameTrackerSource
}

func (worldBloomLineTrackerSource) GetLatestWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	return &sekaiapi.WorldBloomLatestRankingResponse{
		RankData: sekaiapi.WorldBloomRankDataPoint{
			RankDataPoint: sekaiapi.RankDataPoint{
				UserID:    "",
				Score:     2_000_000 + rank + characterID,
				Rank:      rank,
				Timestamp: 1704067200,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "",
			Name:   "WorldBloomLineUser",
		},
	}, nil
}

type lineMetricsOnlyTrackerSource struct {
	latestRankCalls      atomic.Int32
	traceRankCalls       atomic.Int32
	latestUserCalls      atomic.Int32
	traceUserCalls       atomic.Int32
	userEventDataCalls   atomic.Int32
	latestWorldUserCalls atomic.Int32
	traceWorldUserCalls  atomic.Int32
	latestWorldRankCalls atomic.Int32
	traceWorldRankCalls  atomic.Int32
}

func (s *lineMetricsOnlyTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	s.latestRankCalls.Add(1)
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.Itoa(90000 + rank),
			Score:     1_000_000 + rank,
			Rank:      rank,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.Itoa(90000 + rank),
			Name:   "ShouldNotMatter",
		},
	}, nil
}

func (s *lineMetricsOnlyTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	s.latestUserCalls.Add(1)
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.FormatInt(userID, 10),
			Score:     2_000_000,
			Rank:      12,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "ShouldNotMatter",
		},
	}, nil
}

func (s *lineMetricsOnlyTrackerSource) GetLatestWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	s.latestWorldRankCalls.Add(1)
	charID := characterID
	return &sekaiapi.WorldBloomLatestRankingResponse{
		RankData: sekaiapi.WorldBloomRankDataPoint{
			RankDataPoint: sekaiapi.RankDataPoint{
				UserID:    strconv.Itoa(70000 + rank),
				Score:     3_000_000 + rank + characterID,
				Rank:      rank,
				Timestamp: 1704067200,
			},
			CharacterID: &charID,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.Itoa(70000 + rank),
			Name:   "ShouldNotMatter",
		},
	}, nil
}

func (s *lineMetricsOnlyTrackerSource) GetLatestWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	s.latestWorldUserCalls.Add(1)
	charID := characterID
	return &sekaiapi.WorldBloomLatestRankingResponse{
		RankData: sekaiapi.WorldBloomRankDataPoint{
			RankDataPoint: sekaiapi.RankDataPoint{
				UserID:    strconv.FormatInt(userID, 10),
				Score:     4_000_000 + characterID,
				Rank:      8,
				Timestamp: 1704067200,
			},
			CharacterID: &charID,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "ShouldNotMatter",
		},
	}, nil
}

func (s *lineMetricsOnlyTrackerSource) GetUserEventData(server string, eventID int, userID int64) (*sekaiapi.UserEventData, error) {
	s.userEventDataCalls.Add(1)
	return &sekaiapi.UserEventData{
		UserID: strconv.FormatInt(userID, 10),
		Name:   "ShouldNotMatter",
	}, nil
}

func (s *lineMetricsOnlyTrackerSource) GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *lineMetricsOnlyTrackerSource) GetWorldBloomRankingScoreGrowth(server string, eventID, characterID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *lineMetricsOnlyTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	s.traceRankCalls.Add(1)
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{Score: 900000 + rank, Timestamp: 1704060000},
			{Score: 1000000 + rank, Timestamp: 1704067200},
		},
	}, nil
}

func (s *lineMetricsOnlyTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	s.traceUserCalls.Add(1)
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{Score: 1900000, Timestamp: 1704060000},
			{Score: 2000000, Timestamp: 1704067200},
		},
	}, nil
}

func (s *lineMetricsOnlyTrackerSource) TraceWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	s.traceWorldRankCalls.Add(1)
	charID := characterID
	return &sekaiapi.WorldBloomTraceRankingResponse{
		RankData: []sekaiapi.WorldBloomRankDataPoint{
			{RankDataPoint: sekaiapi.RankDataPoint{Score: 2900000 + rank, Timestamp: 1704060000}, CharacterID: &charID},
			{RankDataPoint: sekaiapi.RankDataPoint{Score: 3000000 + rank, Timestamp: 1704067200}, CharacterID: &charID},
		},
	}, nil
}

func (s *lineMetricsOnlyTrackerSource) TraceWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	s.traceWorldUserCalls.Add(1)
	charID := characterID
	return &sekaiapi.WorldBloomTraceRankingResponse{
		RankData: []sekaiapi.WorldBloomRankDataPoint{
			{RankDataPoint: sekaiapi.RankDataPoint{Score: 3900000, Timestamp: 1704060000}, CharacterID: &charID},
			{RankDataPoint: sekaiapi.RankDataPoint{Score: 4000000, Timestamp: 1704067200}, CharacterID: &charID},
		},
	}, nil
}

type rankNameFallbackTrackerSource struct {
	testTrackerSource
}

func (rankNameFallbackTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    "",
			Score:     1234567,
			Rank:      rank,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "34567890123456",
			Name:   "",
		},
	}, nil
}

func (rankNameFallbackTrackerSource) GetUserEventData(server string, eventID int, userID int64) (*sekaiapi.UserEventData, error) {
	return &sekaiapi.UserEventData{
		UserID: strconv.FormatInt(userID, 10),
		Name:   "EventFallbackName",
	}, nil
}

func (rankNameFallbackTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{
				UserID:    "34567890123456",
				Score:     1234000,
				Rank:      rank,
				Timestamp: 1704060000,
			},
			{
				UserID:    "34567890123456",
				Score:     1234567,
				Rank:      rank,
				Timestamp: 1704067200,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "34567890123456",
			Name:   "",
		},
	}, nil
}

type traceUserIDNameFallbackTrackerSource struct {
	testTrackerSource
}

func (traceUserIDNameFallbackTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    "55667788990011",
			Score:     2233445,
			Rank:      rank,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "55667788990011",
			Name:   "",
		},
	}, nil
}

func (traceUserIDNameFallbackTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.FormatInt(userID, 10),
			Score:     2233445,
			Rank:      1,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "",
		},
	}, nil
}

func (traceUserIDNameFallbackTrackerSource) GetUserEventData(server string, eventID int, userID int64) (*sekaiapi.UserEventData, error) {
	if userID == 55667788990011 {
		return &sekaiapi.UserEventData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "TracePointFallbackName",
		}, nil
	}
	return nil, fmt.Errorf("user not found")
}

func (traceUserIDNameFallbackTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{
				UserID:    "55667788990011",
				Score:     2233000,
				Rank:      rank,
				Timestamp: 1704060000,
			},
			{
				UserID:    "55667788990011",
				Score:     2233445,
				Rank:      rank,
				Timestamp: 1704067200,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "",
			Name:   "",
		},
	}, nil
}

func (traceUserIDNameFallbackTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	uid := strconv.FormatInt(userID, 10)
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{
				UserID:    uid,
				Score:     2233000,
				Rank:      2,
				Timestamp: 1704060000,
			},
			{
				UserID:    uid,
				Score:     2233445,
				Rank:      1,
				Timestamp: 1704067200,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: uid,
			Name:   "残照のInside Direction",
		},
	}, nil
}

type checkRoomMetricTrackerSource struct {
	testTrackerSource
}

func (checkRoomMetricTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.FormatInt(int64(11000+rank), 10),
			Score:     1900 + rank,
			Rank:      rank,
			Timestamp: 6000,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(int64(11000+rank), 10),
			Name:   fmt.Sprintf("Player-%d", rank),
		},
	}, nil
}

func (checkRoomMetricTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	rank := int(userID - 11000)
	if rank > 0 && rank < 1000 {
		return &sekaiapi.LatestRankingResponse{
			RankData: sekaiapi.RankDataPoint{
				UserID:    strconv.FormatInt(userID, 10),
				Score:     1900 + rank,
				Rank:      rank,
				Timestamp: 6000,
			},
			UserData: sekaiapi.RankingUserData{
				UserID: strconv.FormatInt(userID, 10),
				Name:   fmt.Sprintf("Player-%d", rank),
			},
		}, nil
	}
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.FormatInt(userID, 10),
			Score:     1925,
			Rank:      25,
			Timestamp: 6000,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "SelfPlayer",
		},
	}, nil
}

func (checkRoomMetricTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	// Intentionally noisy rank trace; check-room should prefer user trace.
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{UserID: strconv.FormatInt(int64(11000+rank), 10), Score: 100 + rank, Rank: rank, Timestamp: 1000},
			{UserID: strconv.FormatInt(int64(11000+rank), 10), Score: 200 + rank, Rank: rank, Timestamp: 2000},
			{UserID: strconv.FormatInt(int64(11000+rank), 10), Score: 500 + rank, Rank: rank, Timestamp: 3000},
			{UserID: strconv.FormatInt(int64(11000+rank), 10), Score: 900 + rank, Rank: rank, Timestamp: 4000},
			{UserID: strconv.FormatInt(int64(11000+rank), 10), Score: 1200 + rank, Rank: rank, Timestamp: 4700},
			{UserID: strconv.FormatInt(int64(11000+rank), 10), Score: 1600 + rank, Rank: rank, Timestamp: 5300},
			{UserID: strconv.FormatInt(int64(11000+rank), 10), Score: 1900 + rank, Rank: rank, Timestamp: 6000},
		},
		// Simulate bad tracker payload that accidentally returns event-like name;
		// controller should keep Player-X from latest ranking.
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(int64(11000+rank), 10),
			Name:   "Tracker Event Name",
		},
	}, nil
}

func (checkRoomMetricTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	points := make([]sekaiapi.RankDataPoint, 0, 31)
	score := 1_000_000
	for i := 0; i <= 30; i++ {
		if i > 0 {
			score += 74_000
		}
		points = append(points, sekaiapi.RankDataPoint{
			UserID:    strconv.FormatInt(userID, 10),
			Score:     score,
			Rank:      1,
			Timestamp: 1000 + int64(i*116),
		})
	}
	return &sekaiapi.TraceRankingResponse{
		RankData: points,
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "Player-1",
		},
	}, nil
}

type eventTitleNameTrackerSource struct {
	testTrackerSource
}

func (eventTitleNameTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	uid := int64(22000 + rank)
	uidStr := strconv.FormatInt(uid, 10)
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    uidStr,
			Score:     1500000 + rank,
			Rank:      rank,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: uidStr,
			Name:   "Tracker Event",
		},
	}, nil
}

func (eventTitleNameTrackerSource) GetUserEventData(server string, eventID int, userID int64) (*sekaiapi.UserEventData, error) {
	index := int(userID - 22000)
	if index <= 0 {
		return nil, fmt.Errorf("invalid user id")
	}
	return &sekaiapi.UserEventData{
		UserID: strconv.FormatInt(userID, 10),
		Name:   fmt.Sprintf("PlayerFromUserAPI-%d", index),
	}, nil
}

type speedFallbackTrackerSource struct {
	testTrackerSource
}

func (speedFallbackTrackerSource) GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	// rank=50 intentionally omits Growth/TimeDiff to exercise field-derivation fallback.
	latestTs := int64(1_001_490)
	return []sekaiapi.ScoreGrowthPoint{
		{
			Rank:             50,
			ScoreLatest:      23_171_700,
			ScoreEarlier:     new(22_527_600),
			TimestampLatest:  latestTs,
			TimestampEarlier: new(int64(1_000_000)),
		},
	}, nil
}

func (speedFallbackTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	if rank != 50 {
		return nil, fmt.Errorf("unexpected rank")
	}
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{UserID: "50050", Score: 22_527_600, Rank: 50, Timestamp: 1_000_000},
			{UserID: "50050", Score: 23_171_700, Rank: 50, Timestamp: 1_001_490},
		},
		UserData: sekaiapi.RankingUserData{UserID: "50050", Name: "SpeedPlayer"},
	}, nil
}

type speedTraceOnlyTrackerSource struct {
	testTrackerSource
}

func (speedTraceOnlyTrackerSource) GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	// no growth points for requested rank; controller should fallback to trace.
	return []sekaiapi.ScoreGrowthPoint{
		{
			Rank:            20,
			ScoreLatest:     3_699_591,
			TimestampLatest: 1_002_000,
		},
	}, nil
}

func (speedTraceOnlyTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	if rank != 50 {
		return nil, fmt.Errorf("unexpected rank")
	}
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{UserID: "50050", Score: 22_527_600, Rank: 50, Timestamp: 1_000_000},
			{UserID: "50050", Score: 23_171_700, Rank: 50, Timestamp: 1_001_490},
		},
		UserData: sekaiapi.RankingUserData{UserID: "50050", Name: "SpeedPlayer"},
	}, nil
}

type speedWindowTraceTrackerSource struct {
	testTrackerSource
}

func (speedWindowTraceTrackerSource) GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return nil, nil
}

func (speedWindowTraceTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	if rank != 50 {
		return nil, fmt.Errorf("unexpected rank")
	}
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{UserID: "50050", Score: 0, Rank: 50, Timestamp: 0},
			{UserID: "50050", Score: 3_500, Rank: 50, Timestamp: 3_500},
			{UserID: "50050", Score: 3_600, Rank: 50, Timestamp: 3_700},
			{UserID: "50050", Score: 7_200, Rank: 50, Timestamp: 7_200},
		},
		UserData: sekaiapi.RankingUserData{UserID: "50050", Name: "SpeedPlayer"},
	}, nil
}

type speedParkedTraceTrackerSource struct {
	testTrackerSource
}

func (speedParkedTraceTrackerSource) GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return nil, nil
}

func (speedParkedTraceTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	if rank != 50 {
		return nil, fmt.Errorf("unexpected rank")
	}
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{UserID: "50050", Score: 22_527_600, Rank: 50, Timestamp: 1_000_100},
			{UserID: "50050", Score: 23_171_700, Rank: 50, Timestamp: 1_000_490},
			{UserID: "50050", Score: 23_171_700, Rank: 50, Timestamp: 1_004_090},
		},
		UserData: sekaiapi.RankingUserData{UserID: "50050", Name: "SpeedPlayer"},
	}, nil
}

type staleSpeedGrowthTrackerSource struct {
	testTrackerSource
}

func (staleSpeedGrowthTrackerSource) GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	now := time.Now().UTC()
	latestTs := now.Add(-70 * time.Minute).Unix()
	earlierTs := now.Add(-130 * time.Minute).Unix()
	scoreEarlier := 1_000
	timeDiff := int64(60 * 60)
	growth := 1_000
	return []sekaiapi.ScoreGrowthPoint{
		{
			Rank:             50,
			ScoreLatest:      2_000,
			ScoreEarlier:     &scoreEarlier,
			TimestampLatest:  latestTs,
			TimestampEarlier: &earlierTs,
			TimeDiff:         &timeDiff,
			Growth:           &growth,
		},
	}, nil
}

func (staleSpeedGrowthTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	if rank != 50 {
		return nil, fmt.Errorf("unexpected rank")
	}
	now := time.Now().UTC()
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{UserID: "50050", Score: 1_000, Rank: 50, Timestamp: now.Add(-130 * time.Minute).Unix()},
			{UserID: "50050", Score: 2_000, Rank: 50, Timestamp: now.Add(-70 * time.Minute).Unix()},
		},
		UserData: sekaiapi.RankingUserData{UserID: "50050", Name: "SpeedPlayer"},
	}, nil
}

type staleCSBTrackerSource struct {
	testTrackerSource
}

func (staleCSBTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	now := time.Now().UTC()
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    "60001",
			Score:     2_000,
			Rank:      rank,
			Timestamp: now.Add(-70 * time.Minute).Unix(),
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "60001",
			Name:   "TracePlayer",
		},
	}, nil
}

func (staleCSBTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	now := time.Now().UTC()
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.FormatInt(userID, 10),
			Score:     2_000,
			Rank:      1,
			Timestamp: now.Add(-70 * time.Minute).Unix(),
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "TracePlayer",
		},
	}, nil
}

func (staleCSBTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	now := time.Now().UTC()
	uid := strconv.FormatInt(userID, 10)
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{UserID: uid, Score: 1_000, Rank: 1, Timestamp: now.Add(-130 * time.Minute).Unix()},
			{UserID: uid, Score: 2_000, Rank: 1, Timestamp: now.Add(-70 * time.Minute).Unix()},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: uid,
			Name:   "TracePlayer",
		},
	}, nil
}

type missingDefaultRankSpeedTrackerSource struct {
	testTrackerSource
}

func (missingDefaultRankSpeedTrackerSource) GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return []sekaiapi.ScoreGrowthPoint{
		{
			Rank:            50,
			ScoreLatest:     3_699_591,
			TimestampLatest: 1_002_000,
		},
	}, nil
}

func (missingDefaultRankSpeedTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	switch rank {
	case 50:
		return &sekaiapi.TraceRankingResponse{
			RankData: []sekaiapi.RankDataPoint{
				{UserID: "50050", Score: 22_527_600, Rank: 50, Timestamp: 1_000_000},
				{UserID: "50050", Score: 23_171_700, Rank: 50, Timestamp: 1_001_490},
			},
			UserData: sekaiapi.RankingUserData{UserID: "50050", Name: "SpeedPlayer"},
		}, nil
	case 300000:
		return nil, sekaiapi.ErrRankingNotFound
	default:
		return nil, fmt.Errorf("unexpected rank")
	}
}

func (missingDefaultRankSpeedTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	if rank == 300000 {
		return nil, sekaiapi.ErrRankingNotFound
	}
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.Itoa(10000 + rank),
			Score:     1000000 + rank,
			Rank:      rank,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.Itoa(10000 + rank),
			Name:   "SpeedPlayer",
		},
	}, nil
}

type fuzzyEventNameTrackerSource struct {
	testTrackerSource
}

func (fuzzyEventNameTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	uid := int64(33000 + rank)
	uidStr := strconv.FormatInt(uid, 10)
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    uidStr,
			Score:     1700000 + rank,
			Rank:      rank,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: uidStr,
			Name:   "残照のInside Directi...",
		},
	}, nil
}

func (fuzzyEventNameTrackerSource) GetUserEventData(server string, eventID int, userID int64) (*sekaiapi.UserEventData, error) {
	return &sekaiapi.UserEventData{
		UserID: strconv.FormatInt(userID, 10),
		Name:   "FuzzyResolvedPlayer",
	}, nil
}

type unresolvedEventNameTrackerSource struct {
	testTrackerSource
}

func (unresolvedEventNameTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	uid := int64(44000 + rank)
	uidStr := strconv.FormatInt(uid, 10)
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    uidStr,
			Score:     1800000 + rank,
			Rank:      rank,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: uidStr,
			Name:   "残照のInside Direction",
		},
	}, nil
}

func (unresolvedEventNameTrackerSource) GetUserEventData(server string, eventID int, userID int64) (*sekaiapi.UserEventData, error) {
	return &sekaiapi.UserEventData{
		UserID: strconv.FormatInt(userID, 10),
		Name:   "残照のInside Direction",
	}, nil
}

type rankTraceNameMismatchTrackerSource struct {
	testTrackerSource
}

func (rankTraceNameMismatchTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    "77889900112233",
			Score:     2345678,
			Rank:      rank,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "77889900112233",
			Name:   "CurrentDisplayName",
		},
	}, nil
}

func (rankTraceNameMismatchTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.FormatInt(userID, 10),
			Score:     2345678,
			Rank:      100,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "CurrentDisplayName",
		},
	}, nil
}

func (rankTraceNameMismatchTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{
				UserID:    "77889900112233",
				Score:     2300000,
				Rank:      rank,
				Timestamp: 1704060000,
			},
			{
				UserID:    "77889900112233",
				Score:     2345678,
				Rank:      rank,
				Timestamp: 1704067200,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "77889900112233",
			Name:   "StaleTraceName",
		},
	}, nil
}

func (rankTraceNameMismatchTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	uid := strconv.FormatInt(userID, 10)
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{
				UserID:    uid,
				Score:     2300000,
				Rank:      100,
				Timestamp: 1704060000,
			},
			{
				UserID:    uid,
				Score:     2345678,
				Rank:      100,
				Timestamp: 1704067200,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: uid,
			Name:   "StaleTraceName",
		},
	}, nil
}

type playerTracePrefersUserHistoryTrackerSource struct {
	testTrackerSource
}

func (playerTracePrefersUserHistoryTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    "99887766554433",
			Score:     3500000,
			Rank:      rank,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "99887766554433",
			Name:   "CurrentPlayer",
		},
	}, nil
}

func (playerTracePrefersUserHistoryTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.FormatInt(userID, 10),
			Score:     3500000,
			Rank:      10,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "CurrentPlayer",
		},
	}, nil
}

func (playerTracePrefersUserHistoryTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{UserID: "111", Score: 1000000, Rank: rank, Timestamp: 1704060000},
			{UserID: "222", Score: 2000000, Rank: rank, Timestamp: 1704063600},
			{UserID: "99887766554433", Score: 3500000, Rank: rank, Timestamp: 1704067200},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "99887766554433",
			Name:   "RankLineHistory",
		},
	}, nil
}

func (playerTracePrefersUserHistoryTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	uid := strconv.FormatInt(userID, 10)
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{UserID: uid, Score: 1500000, Rank: 70, Timestamp: 1704060000},
			{UserID: uid, Score: 2500000, Rank: 35, Timestamp: 1704063600},
			{UserID: uid, Score: 3500000, Rank: 10, Timestamp: 1704067200},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: uid,
			Name:   "StaleUserTraceName",
		},
	}, nil
}

type testEventSource struct {
	region             renderregion.Value
	events             []*masterdata.Event
	byID               map[int]*masterdata.Event
	worldBloomChapters map[int][]*masterdata.WorldBloom
}

func (s *testEventSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testEventSource) GetEventByID(id int) (*masterdata.Event, error) {
	if eventInfo, ok := s.byID[id]; ok {
		return eventInfo, nil
	}
	return nil, fmt.Errorf("event not found")
}

func (s *testEventSource) GetEventByCardID(cardID int) (*masterdata.Event, error) {
	return nil, fmt.Errorf("event not found")
}

func (s *testEventSource) GetEvents() []*masterdata.Event { return s.events }

func (s *testEventSource) GetEventCards(eventID int) ([]*masterdata.Card, error) {
	return nil, nil
}

func (s *testEventSource) GetEventBannerCharacterID(eventID int) (int, error) {
	return 0, fmt.Errorf("not found")
}

func (s *testEventSource) GetEventDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	return nil, nil
}

func (s *testEventSource) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	return nil, fmt.Errorf("not found")
}

func (s *testEventSource) GetBanEvents(charID int) []*masterdata.Event { return nil }

func (s *testEventSource) GetWorldBloomChapters(_ context.Context, eventID int) []*masterdata.WorldBloom {
	return s.worldBloomChapters[eventID]
}

func (s *testEventSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	return nil, fmt.Errorf("not found")
}

func TestValidateTrackerQuerySelectsCurrentEventByRegion(t *testing.T) {
	now := time.Now().UnixMilli()
	jpEvent := &masterdata.Event{
		ID:          200,
		Name:        "JP Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(time.Hour/time.Millisecond),
	}
	cnEvent := &masterdata.Event{
		ID:          120,
		Name:        "CN Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(time.Hour/time.Millisecond),
	}

	controller := NewController(nil)
	controller.SetTrackerIntegration(testTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{jpEvent},
		byID:   map[int]*masterdata.Event{jpEvent.ID: jpEvent},
	}, nil)
	controller.RegisterEventSource(&testEventSource{
		region: renderregion.CN,
		events: []*masterdata.Event{cnEvent},
		byID:   map[int]*masterdata.Event{cnEvent.ID: cnEvent},
	})

	normalized, err := controller.validateTrackerQuery(TrackerRankQuery{
		Region: "cn",
		Ranks:  []int{100},
	})
	if err != nil {
		t.Fatalf("validateTrackerQuery() error = %v", err)
	}
	if normalized.EventID != cnEvent.ID {
		t.Fatalf("expected cn current event %d, got %d", cnEvent.ID, normalized.EventID)
	}
}

func TestValidateTrackerQueryUsesClosedWindowEventBeforeNextStart(t *testing.T) {
	now := time.Now().UnixMilli()
	prev := &masterdata.Event{
		ID:          119,
		Name:        "CN Prev",
		StartAt:     now - int64(2*time.Hour/time.Millisecond),
		AggregateAt: now - int64(time.Hour/time.Millisecond),
		ClosedAt:    now + int64(time.Hour/time.Millisecond),
	}
	next := &masterdata.Event{
		ID:          120,
		Name:        "CN Next",
		StartAt:     now + int64(2*time.Hour/time.Millisecond),
		AggregateAt: now + int64(4*time.Hour/time.Millisecond),
		ClosedAt:    now + int64(5*time.Hour/time.Millisecond),
	}

	controller := NewController(nil)
	controller.SetTrackerIntegration(testTrackerSource{}, &testEventSource{
		region: renderregion.CN,
		events: []*masterdata.Event{prev, next},
		byID:   map[int]*masterdata.Event{prev.ID: prev, next.ID: next},
	}, nil)

	normalized, err := controller.validateTrackerQuery(TrackerRankQuery{
		Region: "cn",
		Ranks:  []int{100},
	})
	if err != nil {
		t.Fatalf("validateTrackerQuery() error = %v", err)
	}
	if normalized.EventID != prev.ID {
		t.Fatalf("expected closed-window event %d, got %d", prev.ID, normalized.EventID)
	}
}

func TestBuildLineRequestFromTrackerOmitsPlayerNames(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(lineNameTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildLineRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{1, 100},
	})
	if err != nil {
		t.Fatalf("build line request: %v", err)
	}
	if len(payload.Ranks) != 2 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	for _, rank := range payload.Ranks {
		if rank.Name != "" {
			t.Fatalf("expected line payload name to be empty, got %+v", rank)
		}
	}
}

func TestBuildLineRequestFromTrackerSkipsUserNameLookupRequests(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	tracker := &lineMetricsOnlyTrackerSource{}
	controller := NewController(nil)
	controller.SetTrackerIntegration(tracker, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildLineRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{1, 100},
	})
	if err != nil {
		t.Fatalf("build line request: %v", err)
	}
	if len(payload.Ranks) != 2 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	if tracker.userEventDataCalls.Load() != 0 {
		t.Fatalf("line request should not query user event data, got %d calls", tracker.userEventDataCalls.Load())
	}
	if tracker.latestUserCalls.Load() != 0 {
		t.Fatalf("line request should not query latest ranking by user, got %d calls", tracker.latestUserCalls.Load())
	}
	if tracker.traceUserCalls.Load() != 0 {
		t.Fatalf("line request should not query trace ranking by user, got %d calls", tracker.traceUserCalls.Load())
	}
	if tracker.latestRankCalls.Load() != 2 {
		t.Fatalf("expected 2 latest rank calls, got %d", tracker.latestRankCalls.Load())
	}
	if tracker.traceRankCalls.Load() != 2 {
		t.Fatalf("expected 2 trace rank calls, got %d", tracker.traceRankCalls.Load())
	}
}

func TestBuildWorldBloomLineRequestFromTrackerSkipsUserNameLookupRequests(t *testing.T) {
	now := time.Now().UnixMilli()
	eventInfo := &masterdata.Event{
		ID:          101,
		EventType:   "world_bloom",
		Name:        "World Bloom Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(time.Hour/time.Millisecond),
	}
	tracker := &lineMetricsOnlyTrackerSource{}
	controller := NewController(nil)
	controller.SetTrackerIntegration(tracker, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	charaID := 21
	payload, err := controller.BuildLineRequestFromTracker(TrackerRankQuery{
		EventID:       eventInfo.ID,
		Region:        "jp",
		Ranks:         []int{1, 100},
		WlCharacterID: &charaID,
	})
	if err != nil {
		t.Fatalf("build wl line request: %v", err)
	}
	if len(payload.Ranks) != 2 {
		t.Fatalf("unexpected wl ranks len: %d", len(payload.Ranks))
	}
	if tracker.userEventDataCalls.Load() != 0 {
		t.Fatalf("wl line request should not query user event data, got %d calls", tracker.userEventDataCalls.Load())
	}
	if tracker.latestWorldUserCalls.Load() != 0 {
		t.Fatalf("wl line request should not query latest world bloom ranking by user, got %d calls", tracker.latestWorldUserCalls.Load())
	}
	if tracker.traceWorldUserCalls.Load() != 0 {
		t.Fatalf("wl line request should not query trace world bloom ranking by user, got %d calls", tracker.traceWorldUserCalls.Load())
	}
	if tracker.latestWorldRankCalls.Load() != 2 {
		t.Fatalf("expected 2 latest world bloom rank calls, got %d", tracker.latestWorldRankCalls.Load())
	}
	if tracker.traceWorldRankCalls.Load() != 2 {
		t.Fatalf("expected 2 trace world bloom rank calls, got %d", tracker.traceWorldRankCalls.Load())
	}
}

func TestBuildLineRequestFromTrackerAllowsWorldBloomTotalRanking(t *testing.T) {
	now := time.Now().UnixMilli()
	eventInfo := &masterdata.Event{
		ID:          101,
		EventType:   "world_bloom",
		Name:        "World Bloom Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(time.Hour/time.Millisecond),
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(lineNameTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildLineRequestFromTracker(TrackerRankQuery{
		Region: "jp",
		Ranks:  []int{1, 100},
	})
	if err != nil {
		t.Fatalf("build line request: %v", err)
	}
	if payload.ID != eventInfo.ID {
		t.Fatalf("expected inferred event id %d, got %d", eventInfo.ID, payload.ID)
	}
	if payload.WlCid != nil {
		t.Fatalf("expected wl total ranking without chapter id, got %+v", payload.WlCid)
	}
	if len(payload.Ranks) != 2 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
}

func TestBuildLineRequestFromTrackerUsesWorldBloomChapterRanking(t *testing.T) {
	now := time.Now().UnixMilli()
	eventInfo := &masterdata.Event{
		ID:          101,
		EventType:   "world_bloom",
		Name:        "World Bloom Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(time.Hour/time.Millisecond),
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(worldBloomLineTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	charaID := 21
	payload, err := controller.BuildLineRequestFromTracker(TrackerRankQuery{
		EventID:       eventInfo.ID,
		Region:        "jp",
		Ranks:         []int{1, 100},
		WlCharacterID: &charaID,
	})
	if err != nil {
		t.Fatalf("build wl line request: %v", err)
	}
	if payload.WlCid == nil || *payload.WlCid != charaID {
		t.Fatalf("expected wl chapter id %d, got %+v", charaID, payload.WlCid)
	}
	if len(payload.Ranks) != 2 {
		t.Fatalf("unexpected wl ranks len: %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Score == nil || *payload.Ranks[0].Score != 2_000_022 {
		t.Fatalf("unexpected wl rank 1 payload: %+v", payload.Ranks[0])
	}
	if payload.Ranks[1].Score == nil || *payload.Ranks[1].Score != 2_000_121 {
		t.Fatalf("unexpected wl rank 100 payload: %+v", payload.Ranks[1])
	}
}

func TestBuildLineRequestFromTrackerSkipsMissingDefaultRanks(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(missingDefaultRankLineTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildLineRequestFromTracker(TrackerRankQuery{
		EventID:      101,
		Region:       "jp",
		Ranks:        []int{1, 300000},
		DefaultRanks: true,
	})
	if err != nil {
		t.Fatalf("build line request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("expected only existing ranks to remain, got %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Rank != 1 {
		t.Fatalf("unexpected remaining rank: %+v", payload.Ranks[0])
	}
}

func TestBuildLineRequestFromTrackerKeepsExplicitMissingRankError(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(missingDefaultRankLineTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	_, err := controller.BuildLineRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{300000},
	})
	if err == nil {
		t.Fatal("expected explicit missing rank to fail")
	}
}

func TestBuildQueryRequestFromTrackerPreservesResolvedNameWhenTraceNameMissing(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(rankNameFallbackTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{1},
	})
	if err != nil {
		t.Fatalf("build query request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Name != "EventFallbackName" {
		t.Fatalf("expected fallback name to be preserved, got %+v", payload.Ranks[0])
	}
}

func TestBuildQueryRequestFromTrackerResolvesNameFromTraceUserID(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(traceUserIDNameFallbackTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{1},
	})
	if err != nil {
		t.Fatalf("build query request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Name != "TracePointFallbackName" {
		t.Fatalf("expected trace-point fallback name, got %+v", payload.Ranks[0])
	}
}

func TestBuildPlayerTraceFromTrackerResolvesNameFromTraceUserID(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(traceUserIDNameFallbackTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildPlayerTraceFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{1},
	})
	if err != nil {
		t.Fatalf("build player trace request: %v", err)
	}
	if len(payload.Ranks) == 0 {
		t.Fatalf("expected trace data")
	}
	if payload.Ranks[0].Name != "TracePointFallbackName" {
		t.Fatalf("expected resolved trace user name, got %+v", payload.Ranks[0])
	}
}

func TestBuildPlayerTraceFromTrackerUserUsesResolvedName(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "残照のInside Direction",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(traceUserIDNameFallbackTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildPlayerTraceFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		UserID:  new(int64(55667788990011)),
		Ranks:   []int{1},
	})
	if err != nil {
		t.Fatalf("build player trace by user request: %v", err)
	}
	if len(payload.Ranks) == 0 {
		t.Fatalf("expected trace data")
	}
	if payload.Ranks[0].Name != "TracePointFallbackName" {
		t.Fatalf("expected resolved user trace name, got %+v", payload.Ranks[0])
	}
}

func TestBuildPlayerTraceFromTrackerUsesSameDisplayNameAsQueryForRank(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(rankTraceNameMismatchTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	queryPayload, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{100},
	})
	if err != nil {
		t.Fatalf("build query request: %v", err)
	}
	tracePayload, err := controller.BuildPlayerTraceFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{100},
	})
	if err != nil {
		t.Fatalf("build player trace request: %v", err)
	}
	if len(queryPayload.Ranks) != 1 {
		t.Fatalf("unexpected query ranks len: %d", len(queryPayload.Ranks))
	}
	if len(tracePayload.Ranks) == 0 {
		t.Fatalf("expected trace data")
	}
	if queryPayload.Ranks[0].Name != "CurrentDisplayName" {
		t.Fatalf("unexpected query name: %+v", queryPayload.Ranks[0])
	}
	if tracePayload.Ranks[0].Name != queryPayload.Ranks[0].Name {
		t.Fatalf("expected ptr name to match sk name, query=%q trace=%q", queryPayload.Ranks[0].Name, tracePayload.Ranks[0].Name)
	}
}

func TestBuildPlayerTraceFromTrackerRankUsesCurrentPlayerHistory(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(playerTracePrefersUserHistoryTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildPlayerTraceFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{100},
	})
	if err != nil {
		t.Fatalf("build player trace request: %v", err)
	}
	if len(payload.Ranks) != 3 {
		t.Fatalf("unexpected trace point count: %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Rank != 70 || payload.Ranks[1].Rank != 35 || payload.Ranks[2].Rank != 10 {
		t.Fatalf("expected current player's rank history, got %+v", payload.Ranks)
	}
	if payload.Ranks[0].Score == nil || *payload.Ranks[0].Score != 1500000 {
		t.Fatalf("expected user trace score history, got %+v", payload.Ranks[0])
	}
	if payload.Ranks[0].Name != "CurrentPlayer" {
		t.Fatalf("expected latest player display name, got %+v", payload.Ranks[0])
	}
}

type checkRoomOutOfTop100TrackerSource struct {
	testTrackerSource
}

func (checkRoomOutOfTop100TrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.FormatInt(userID, 10),
			Score:     1_234_567,
			Rank:      120,
			Timestamp: 6000,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "OutTop100",
		},
	}, nil
}

type latestUserWithoutRankTrackerSource struct {
	testTrackerSource
}

func (latestUserWithoutRankTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.FormatInt(userID, 10),
			Score:     0,
			Rank:      0,
			Timestamp: 0,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "ProfileOnly",
		},
	}, nil
}

func (latestUserWithoutRankTrackerSource) GetLatestWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	charID := characterID
	return &sekaiapi.WorldBloomLatestRankingResponse{
		RankData: sekaiapi.WorldBloomRankDataPoint{
			RankDataPoint: sekaiapi.RankDataPoint{
				UserID:    strconv.FormatInt(userID, 10),
				Score:     0,
				Rank:      0,
				Timestamp: 0,
			},
			CharacterID: &charID,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "ProfileOnly",
		},
	}, nil
}

func TestBuildCheckRoomRequestFromTrackerKeepsPlayerNameAndUsesWindowMetrics(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(checkRoomMetricTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{1},
	})
	if err != nil {
		t.Fatalf("build check-room request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	got := payload.Ranks[0]
	if got.Name != "Player-1" {
		t.Fatalf("expected player name to be preserved, got %+v", got)
	}
	if got.Speed == nil || *got.Speed != 2296551 {
		t.Fatalf("unexpected speed: %+v", got.Speed)
	}
	if got.HourRound == nil || *got.HourRound != 30 {
		t.Fatalf("unexpected hour_round: %+v", got.HourRound)
	}
	if got.Min20Time3Speed == nil || *got.Min20Time3Speed != 2442000 {
		t.Fatalf("unexpected 20minx3 speed: %+v", got.Min20Time3Speed)
	}
}

func TestBuildCheckRoomRequestFromTrackerSupportsUserQuery(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(checkRoomMetricTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		UserID:  new(int64(99887766)),
	})
	if err != nil {
		t.Fatalf("build check-room request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Rank != 25 || payload.Ranks[0].Name != "SelfPlayer" {
		t.Fatalf("unexpected user rank payload: %+v", payload.Ranks[0])
	}
	if payload.Ranks[0].Speed == nil || *payload.Ranks[0].Speed != 2296551 {
		t.Fatalf("expected user query metrics to be enriched, got %+v", payload.Ranks[0])
	}
	if payload.PrevRank == nil || payload.PrevRank.Rank != 24 || payload.PrevRank.Name != "Player-24" {
		t.Fatalf("unexpected prev rank: %+v", payload.PrevRank)
	}
	if payload.NextRank == nil || payload.NextRank.Rank != 26 || payload.NextRank.Name != "Player-26" {
		t.Fatalf("unexpected next rank: %+v", payload.NextRank)
	}
}

func TestBuildQueryRequestFromTrackerSupportsUserQueryAdjacentRanks(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(checkRoomMetricTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		UserID:  new(int64(99887766)),
	})
	if err != nil {
		t.Fatalf("build query request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Rank != 25 || payload.Ranks[0].Name != "SelfPlayer" {
		t.Fatalf("unexpected user rank payload: %+v", payload.Ranks[0])
	}
	if payload.PrevRanks == nil || payload.PrevRanks.Rank != 20 || payload.PrevRanks.Name != "Player-20" {
		t.Fatalf("unexpected prev ranks: %+v", payload.PrevRanks)
	}
	if payload.NextRanks == nil || payload.NextRanks.Rank != 30 || payload.NextRanks.Name != "Player-30" {
		t.Fatalf("unexpected next ranks: %+v", payload.NextRanks)
	}
}

func TestBuildQueryRequestFromTrackerRejectsUserResponseWithoutRank(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(latestUserWithoutRankTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	_, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		UserID:  new(int64(99887766)),
	})
	if err == nil {
		t.Fatal("expected user query without rank to fail")
	}
	if err.Error() != "tracker user query failed: tracker: ranking record not found" {
		t.Fatalf("unexpected error: %v", err)
	}
	if !errors.Is(err, sekaiapi.ErrRankingNotFound) {
		t.Fatalf("expected ErrRankingNotFound, got %v", err)
	}
}

func TestBuildQueryRequestFromTrackerRejectsWorldBloomUserResponseWithoutRank(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		EventType:   "world_bloom",
		Name:        "World Bloom Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(latestUserWithoutRankTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	charaID := 21
	_, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{
		EventID:       101,
		Region:        "jp",
		UserID:        new(int64(99887766)),
		WlCharacterID: &charaID,
	})
	if err == nil {
		t.Fatal("expected world bloom user query without rank to fail")
	}
	if err.Error() != "tracker user query failed: tracker: ranking record not found" {
		t.Fatalf("unexpected error: %v", err)
	}
	if !errors.Is(err, sekaiapi.ErrRankingNotFound) {
		t.Fatalf("expected ErrRankingNotFound, got %v", err)
	}
}

func TestQueryAdjacentSKLineRanksUsesNearestNodes(t *testing.T) {
	prev, next, hasPrev, hasNext := queryAdjacentSKLineRanks(25, false)
	if !hasPrev || prev != 20 {
		t.Fatalf("unexpected prev rank: hasPrev=%t prev=%d", hasPrev, prev)
	}
	if !hasNext || next != 30 {
		t.Fatalf("unexpected next rank: hasNext=%t next=%d", hasNext, next)
	}
}

func TestQueryAdjacentSKLineRanksUsesNeighboringNodesWhenTargetIsNode(t *testing.T) {
	prev, next, hasPrev, hasNext := queryAdjacentSKLineRanks(10, false)
	if !hasPrev || prev != 9 {
		t.Fatalf("unexpected prev rank: hasPrev=%t prev=%d", hasPrev, prev)
	}
	if !hasNext || next != 20 {
		t.Fatalf("unexpected next rank: hasNext=%t next=%d", hasNext, next)
	}
}

func TestBuildCheckRoomRequestFromTrackerResolvesPlayerNameWhenLatestNameIsEventTitle(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(eventTitleNameTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{1},
	})
	if err != nil {
		t.Fatalf("build check-room request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Name != "PlayerFromUserAPI-1" {
		t.Fatalf("expected player name from user api, got %+v", payload.Ranks[0])
	}
	if payload.Ranks[0].Name == "Tracker Event" {
		t.Fatalf("expected not to expose event name as player name")
	}
	if payload.NextRank == nil || payload.NextRank.Name != "PlayerFromUserAPI-2" {
		t.Fatalf("expected next rank name fallback, got %+v", payload.NextRank)
	}
}

func TestBuildSpeedRequestFromTrackerDerivesSpeedWhenGrowthFieldsMissing(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(speedFallbackTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{50},
	})
	if err != nil {
		t.Fatalf("build speed request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	if payload.RequestType != "时" {
		t.Fatalf("unexpected request type: %q", payload.RequestType)
	}
	if payload.Period != 60*60 {
		t.Fatalf("unexpected period: %d", payload.Period)
	}
	got := payload.Ranks[0]
	if got.Rank != 50 {
		t.Fatalf("unexpected rank: %d", got.Rank)
	}
	if got.Speed == nil || *got.Speed != 1556214 {
		t.Fatalf("unexpected speed: %+v", got.Speed)
	}
}

func TestBuildSpeedRequestFromTrackerConvertsCustomMinuteWindowToHourlySpeed(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(speedFallbackTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{
		EventID:         101,
		Region:          "jp",
		Ranks:           []int{50},
		SpeedUnit:       "h",
		SpeedPeriodSecs: 30 * 60,
	})
	if err != nil {
		t.Fatalf("build speed request: %v", err)
	}
	if payload.RequestType != "时" {
		t.Fatalf("unexpected request type: %q", payload.RequestType)
	}
	if payload.Period != 30*60 {
		t.Fatalf("unexpected period: %d", payload.Period)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	got := payload.Ranks[0]
	if got.Speed == nil || *got.Speed != 1556214 {
		t.Fatalf("expected hourly normalized speed, got %+v", got.Speed)
	}
}

func TestBuildSpeedRequestFromTrackerAllowsWorldBloomTotalRanking(t *testing.T) {
	now := time.Now().UnixMilli()
	eventInfo := &masterdata.Event{
		ID:          101,
		EventType:   "world_bloom",
		Name:        "World Bloom Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(time.Hour/time.Millisecond),
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(speedFallbackTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{
		Region: "jp",
		Ranks:  []int{50},
	})
	if err != nil {
		t.Fatalf("build speed request: %v", err)
	}
	if payload.EventID != eventInfo.ID {
		t.Fatalf("expected inferred event id %d, got %d", eventInfo.ID, payload.EventID)
	}
	if payload.IsWlEvent {
		t.Fatalf("expected wl total speed to use total-ranking layout, got %+v", payload)
	}
	if payload.WlCharaIconPath != nil {
		t.Fatalf("expected no wl chapter icon for total ranking, got %+v", payload.WlCharaIconPath)
	}
}

func TestBuildSpeedRequestFromTrackerFallsBackToTraceWhenGrowthPointMissing(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(speedTraceOnlyTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{50},
	})
	if err != nil {
		t.Fatalf("build speed request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	got := payload.Ranks[0]
	if got.Speed == nil || *got.Speed != 1556214 {
		t.Fatalf("expected speed from trace fallback, got %+v", got.Speed)
	}
}

func TestBuildSpeedRequestFromTrackerTraceUsesLastPointBeforeWindowStart(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(speedWindowTraceTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{
		EventID:         101,
		Region:          "jp",
		Ranks:           []int{50},
		SpeedUnit:       "h",
		SpeedPeriodSecs: 60 * 60,
	})
	if err != nil {
		t.Fatalf("build speed request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	got := payload.Ranks[0]
	if got.Speed == nil || *got.Speed != 3600 {
		t.Fatalf("expected speed from last point before window start, got %+v", got.Speed)
	}
}

func TestBuildSpeedRequestFromTrackerReturnsZeroWhenTraceShowsParkedWindow(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(speedParkedTraceTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{
		EventID:         101,
		Region:          "jp",
		Ranks:           []int{50},
		SpeedUnit:       "h",
		SpeedPeriodSecs: 60 * 60,
	})
	if err != nil {
		t.Fatalf("build speed request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	got := payload.Ranks[0]
	if got.Speed == nil || *got.Speed != 0 {
		t.Fatalf("expected parked trace speed to be zero, got %+v", got.Speed)
	}
}

func TestSpeedInfoFromGrowthPointReturnsZeroWhenParked(t *testing.T) {
	scoreEarlier := 23_171_700
	timestampEarlier := int64(1_000_490)
	point := sekaiapi.ScoreGrowthPoint{
		Rank:             50,
		ScoreLatest:      23_171_700,
		ScoreEarlier:     &scoreEarlier,
		TimestampLatest:  1_004_090,
		TimestampEarlier: &timestampEarlier,
	}

	info := speedInfoFromGrowthPoint(point, 60*60)

	if info.Speed == nil || *info.Speed != 0 {
		t.Fatalf("expected parked growth point speed to be zero, got %+v", info.Speed)
	}
}

func TestBuildSpeedRequestFromTrackerTreatsStaleTrackerGrowthAsStopped(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(staleSpeedGrowthTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{50},
	})
	if err != nil {
		t.Fatalf("build speed request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	got := payload.Ranks[0]
	if got.Speed == nil || *got.Speed != 0 {
		t.Fatalf("expected stopped speed to decay to zero, got %+v", got.Speed)
	}
}

func TestBuildDailySpeedRequestFromTrackerUsesDayPeriod(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(speedFallbackTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{
		EventID:         101,
		Region:          "jp",
		Ranks:           []int{50},
		SpeedUnit:       "d",
		SpeedPeriodSecs: 24 * 60 * 60,
	})
	if err != nil {
		t.Fatalf("build daily speed request: %v", err)
	}
	if payload.RequestType != "日" {
		t.Fatalf("unexpected request type: %q", payload.RequestType)
	}
	if payload.Period != 24*60*60 {
		t.Fatalf("unexpected period: %d", payload.Period)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	got := payload.Ranks[0]
	if got.Speed == nil || *got.Speed != 37349154 {
		t.Fatalf("unexpected daily speed: %+v", got.Speed)
	}
}

func TestBuildDailySpeedRequestFromTrackerKeepsDailyNormalizationForCustomWindow(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(speedFallbackTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{
		EventID:         101,
		Region:          "jp",
		Ranks:           []int{50},
		SpeedUnit:       "d",
		SpeedPeriodSecs: 2 * 24 * 60 * 60,
	})
	if err != nil {
		t.Fatalf("build daily speed request: %v", err)
	}
	if payload.RequestType != "日" {
		t.Fatalf("unexpected request type: %q", payload.RequestType)
	}
	if payload.Period != 2*24*60*60 {
		t.Fatalf("unexpected period: %d", payload.Period)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	got := payload.Ranks[0]
	if got.Speed == nil || *got.Speed != 37349154 {
		t.Fatalf("expected daily normalized speed, got %+v", got.Speed)
	}
}

func TestApplyRankInfoMetricsReturnsZeroSpeedWhenParked(t *testing.T) {
	info := drawing.RankInfo{}
	applyRankInfoMetrics(&info, []trackerScoreSample{
		{score: 22_527_600, timestamp: 1_000_100},
		{score: 23_171_700, timestamp: 1_000_490},
		{score: 23_171_700, timestamp: 1_004_090},
	})

	if info.Speed == nil || *info.Speed != 0 {
		t.Fatalf("expected parked line speed to be zero, got %+v", info.Speed)
	}
}

func TestBuildSpeedRequestFromTrackerSkipsMissingDefaultRanks(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(missingDefaultRankSpeedTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{
		EventID:      101,
		Region:       "jp",
		Ranks:        []int{50, 300000},
		DefaultRanks: true,
	})
	if err != nil {
		t.Fatalf("build speed request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("expected only existing ranks to remain, got %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Rank != 50 {
		t.Fatalf("unexpected remaining speed rank: %+v", payload.Ranks[0])
	}
}

func TestBuildCheckRoomRequestFromTrackerResolvesFuzzyEventTitleName(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "残照のInside Direction",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(fuzzyEventNameTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{1},
	})
	if err != nil {
		t.Fatalf("build check-room request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Name != "FuzzyResolvedPlayer" {
		t.Fatalf("expected fuzzy-resolved player name, got %+v", payload.Ranks[0])
	}
}

func TestBuildCheckRoomRequestFromTrackerUsesRankPlaceholderWhenOnlyEventTitleAvailable(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "残照のInside Direction",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(unresolvedEventNameTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{1},
	})
	if err != nil {
		t.Fatalf("build check-room request: %v", err)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Name != "Rank 1" {
		t.Fatalf("expected rank placeholder when only event title exists, got %+v", payload.Ranks[0])
	}
}

func TestBuildCheckRoomRequestFromTrackerRejectsRanksOutsideTop100(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(checkRoomMetricTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	_, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{101},
	})
	if err == nil {
		t.Fatal("expected top-100 limit error, got nil")
	}
	if got := err.Error(); got != "查房/查水表目前仅支持前100名查询" {
		t.Fatalf("unexpected error: %v", got)
	}
}

func TestBuildCheckRoomRequestFromTrackerRejectsUserOutsideTop100(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(checkRoomOutOfTop100TrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	_, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		UserID:  new(int64(99887766)),
	})
	if err == nil {
		t.Fatal("expected top-100 limit error, got nil")
	}
	if got := err.Error(); got != "查房/查水表目前仅支持前100名查询" {
		t.Fatalf("unexpected error: %v", got)
	}
}

func TestBuildCSBRequestFromTrackerBuildsTracePayload(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(checkRoomMetricTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildCSBRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{1},
	})
	if err != nil {
		t.Fatalf("build csb request: %v", err)
	}
	if payload.EventName != eventInfo.Name {
		t.Fatalf("unexpected event name: %q", payload.EventName)
	}
	if len(payload.Ranks) != 31 {
		t.Fatalf("unexpected trace point count: %d", len(payload.Ranks))
	}
	if payload.Ranks[len(payload.Ranks)-1].Rank != 1 {
		t.Fatalf("unexpected latest rank: %+v", payload.Ranks[len(payload.Ranks)-1])
	}
	if payload.Ranks[len(payload.Ranks)-1].Name != "Player-1" {
		t.Fatalf("unexpected latest name: %+v", payload.Ranks[len(payload.Ranks)-1])
	}
	if payload.UpdateAt <= 0 {
		t.Fatalf("expected update time to be set, got %d", payload.UpdateAt)
	}
}

func TestBuildCSBRequestFromTrackerAppendsIdleTailForStoppedUser(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(staleCSBTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	before := time.Now().UTC().UnixMilli()
	payload, err := controller.BuildCSBRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{1},
	})
	if err != nil {
		t.Fatalf("build csb request: %v", err)
	}
	if len(payload.Ranks) != 3 {
		t.Fatalf("expected idle tail to be appended, got %d points", len(payload.Ranks))
	}
	last := payload.Ranks[len(payload.Ranks)-1]
	prev := payload.Ranks[len(payload.Ranks)-2]
	if last.Score == nil || prev.Score == nil || *last.Score != *prev.Score {
		t.Fatalf("expected idle tail to keep same score, got prev=%+v last=%+v", prev.Score, last.Score)
	}
	if last.Time <= prev.Time {
		t.Fatalf("expected idle tail time to move forward, got prev=%d last=%d", prev.Time, last.Time)
	}
	if last.Time < before-1000 {
		t.Fatalf("expected idle tail to extend near now, got %d before %d", last.Time, before)
	}
	if payload.UpdateAt < last.Time {
		t.Fatalf("expected payload update time to be no earlier than idle tail, got update=%d tail=%d", payload.UpdateAt, last.Time)
	}
}

func TestBuildCSBRequestFromTrackerRejectsMultipleRanks(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(checkRoomMetricTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	_, err := controller.BuildCSBRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{1, 2},
	})
	if err == nil {
		t.Fatal("expected single-target error, got nil")
	}
	if got := err.Error(); got != "查水表目前仅支持单人查询" {
		t.Fatalf("unexpected error: %v", got)
	}
}

func TestBuildPredictLineRequestFromTrackerUsesForecastScores(t *testing.T) {
	now := time.Now().UnixMilli()
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(2*time.Hour/time.Millisecond),
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(lineNameTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)
	controller.SetForecastProvider(testForecastProvider{
		bySource: map[string]ForecastSourceData{
			"33kit": {
				Scores: map[int]ForecastScore{
					50:  {Score: 12_345_678, Timestamp: 1_700_000_000, Source: "33kit"},
					100: {Score: 9_876_543, Timestamp: 1_700_000_100, Source: "33kit"},
				},
				FetchedAt: 1_700_000_200,
			},
			"sekarun": {
				Scores: map[int]ForecastScore{
					50: {Score: 11_111_111, Timestamp: 1_700_000_300, Source: "sekarun"},
				},
				FetchedAt: 1_700_000_400,
			},
			"local": {
				Scores: map[int]ForecastScore{
					50: {Score: 13_333_333, Timestamp: 1_700_000_500, Source: "local"},
				},
				FetchedAt: 1_700_000_600,
			},
		},
	})
	if err := controller.forecastCache.RefreshNow(context.Background(), "jp", 101); err != nil {
		t.Fatalf("prime forecast cache: %v", err)
	}

	payload, err := controller.BuildPredictLineRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{100, 50},
	})
	if err != nil {
		t.Fatalf("build predict line request: %v", err)
	}
	if payload.Name != "Tracker Event 预测" {
		t.Fatalf("unexpected payload name: %s", payload.Name)
	}
	if payload.PredictionNotice != skPredictionNotice {
		t.Fatalf("unexpected prediction notice: %q", payload.PredictionNotice)
	}
	if len(payload.Ranks) != 2 {
		t.Fatalf("unexpected current ranks len: %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Rank != 50 || payload.Ranks[0].Score == nil || *payload.Ranks[0].Score != 1_000_050 {
		t.Fatalf("unexpected first current rank payload: %+v", payload.Ranks[0])
	}
	if payload.Ranks[1].Rank != 100 || payload.Ranks[1].Score == nil || *payload.Ranks[1].Score != 1_000_100 {
		t.Fatalf("unexpected second current rank payload: %+v", payload.Ranks[1])
	}
	if len(payload.ForecastColumns) != 3 {
		t.Fatalf("unexpected forecast column len: %d", len(payload.ForecastColumns))
	}
	if payload.ForecastColumns[0].Key != "33kit" || payload.ForecastColumns[0].Name != "33Kit预测" {
		t.Fatalf("unexpected first forecast column: %+v", payload.ForecastColumns[0])
	}
	if len(payload.ForecastColumns[0].Ranks) != 2 {
		t.Fatalf("unexpected 33kit forecast rank len: %d", len(payload.ForecastColumns[0].Ranks))
	}
	if payload.ForecastColumns[0].Ranks[0].Rank != 50 || payload.ForecastColumns[0].Ranks[0].Score == nil || *payload.ForecastColumns[0].Ranks[0].Score != 12_345_678 {
		t.Fatalf("unexpected 33kit p50 payload: %+v", payload.ForecastColumns[0].Ranks[0])
	}
	if payload.ForecastColumns[1].Key != "local" || payload.ForecastColumns[1].Name != "本地预测" {
		t.Fatalf("unexpected second forecast column: %+v", payload.ForecastColumns[1])
	}
	if payload.ForecastColumns[2].Key != "sekarun" {
		t.Fatalf("unexpected third forecast column key: %s", payload.ForecastColumns[2].Key)
	}
}

func TestBuildPredictLineRequestFromTrackerUsesWorldBloomChapterMeta(t *testing.T) {
	now := time.Now().UnixMilli()
	charaID := 21
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "WL Event",
		EventType:   "world_bloom",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(5*time.Hour/time.Millisecond),
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(worldBloomLineTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
		worldBloomChapters: map[int][]*masterdata.WorldBloom{
			101: {
				{
					EventID:         101,
					GameCharacterID: &charaID,
					ChapterStartAt:  now - int64(time.Hour/time.Millisecond),
					AggregateAt:     now + int64(2*time.Hour/time.Millisecond),
				},
			},
		},
	}, nil)
	controller.SetForecastProvider(&scopedForecastProvider{
		data: map[string]map[string]ForecastSourceData{
			"chapter:21": {
				"local": {
					Scores: map[int]ForecastScore{
						100: {Score: 1_234_567, Timestamp: 1_700_000_000, Source: "local"},
					},
					FetchedAt: 1_700_000_100,
				},
			},
		},
	})
	if err := controller.forecastCache.RefreshNowQuery(context.Background(), ForecastQuery{
		Region:        "jp",
		EventID:       101,
		Scope:         ForecastScopeChapter,
		WlCharacterID: &charaID,
	}); err != nil {
		t.Fatalf("prime wl chapter forecast cache: %v", err)
	}

	payload, err := controller.BuildPredictLineRequestFromTracker(TrackerRankQuery{
		EventID:       101,
		Region:        "jp",
		Ranks:         []int{100},
		WlCharacterID: new(21),
	})
	if err != nil {
		t.Fatalf("build wl chapter predict line request: %v", err)
	}
	if payload.Name != "WL Event 预测" {
		t.Fatalf("unexpected payload name: %s", payload.Name)
	}
	if payload.AggregateAt != now+int64(2*time.Hour/time.Millisecond) {
		t.Fatalf("expected chapter aggregate time, got %d", payload.AggregateAt)
	}
	if payload.PredictionNotice != skPredictionNotice {
		t.Fatalf("unexpected prediction notice: %q", payload.PredictionNotice)
	}
	if payload.WlCid == nil || *payload.WlCid != 21 {
		t.Fatalf("expected wl chapter id to be preserved, got %+v", payload.WlCid)
	}
	if len(payload.ForecastColumns) != 1 {
		t.Fatalf("unexpected forecast columns len: %d", len(payload.ForecastColumns))
	}
	if payload.ForecastColumns[0].Ranks[0].Score == nil || *payload.ForecastColumns[0].Ranks[0].Score != 1_234_567 {
		t.Fatalf("unexpected chapter forecast payload: %+v", payload.ForecastColumns[0].Ranks[0])
	}
}

func TestBuildPredictLineRequestFromTrackerUsesSeparateScopesForWorldBloom(t *testing.T) {
	now := time.Now().UnixMilli()
	charaID := 21
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "WL Event",
		EventType:   "world_bloom",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(5*time.Hour/time.Millisecond),
	}
	provider := &scopedForecastProvider{
		data: map[string]map[string]ForecastSourceData{
			"total": {
				"local": {
					Scores: map[int]ForecastScore{
						100: {Score: 8_888_888, Timestamp: 1_700_000_000, Source: "local"},
					},
					FetchedAt: 1_700_000_100,
				},
			},
			"chapter:21": {
				"local": {
					Scores: map[int]ForecastScore{
						100: {Score: 9_999_999, Timestamp: 1_700_000_200, Source: "local"},
					},
					FetchedAt: 1_700_000_300,
				},
			},
		},
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(worldBloomLineTrackerSource{}, &testEventSource{
		region: renderregion.TW,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
		worldBloomChapters: map[int][]*masterdata.WorldBloom{
			101: {
				{
					EventID:         101,
					GameCharacterID: &charaID,
					ChapterStartAt:  now - int64(time.Hour/time.Millisecond),
					AggregateAt:     now + int64(2*time.Hour/time.Millisecond),
				},
			},
		},
	}, nil)
	controller.SetForecastProvider(provider)

	if err := controller.forecastCache.RefreshNowQuery(context.Background(), ForecastQuery{
		Region:  "tw",
		EventID: 101,
		Scope:   ForecastScopeTotal,
	}); err != nil {
		t.Fatalf("prime wl total forecast cache: %v", err)
	}
	if err := controller.forecastCache.RefreshNowQuery(context.Background(), ForecastQuery{
		Region:        "tw",
		EventID:       101,
		Scope:         ForecastScopeChapter,
		WlCharacterID: &charaID,
	}); err != nil {
		t.Fatalf("prime wl chapter forecast cache: %v", err)
	}

	totalPayload, err := controller.BuildPredictLineRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "tw",
		Ranks:   []int{100},
	})
	if err != nil {
		t.Fatalf("build wl total predict line request: %v", err)
	}
	if totalPayload.WlCid != nil {
		t.Fatalf("expected wl total predict line to avoid chapter id, got %+v", totalPayload.WlCid)
	}
	if len(totalPayload.ForecastColumns) != 1 || len(totalPayload.ForecastColumns[0].Ranks) != 1 {
		t.Fatalf("unexpected wl total forecast columns: %+v", totalPayload.ForecastColumns)
	}
	if totalPayload.ForecastColumns[0].Ranks[0].Score == nil || *totalPayload.ForecastColumns[0].Ranks[0].Score != 8_888_888 {
		t.Fatalf("unexpected wl total forecast score: %+v", totalPayload.ForecastColumns[0].Ranks[0])
	}

	chapterPayload, err := controller.BuildPredictLineRequestFromTracker(TrackerRankQuery{
		EventID:       101,
		Region:        "tw",
		Ranks:         []int{100},
		WlCharacterID: &charaID,
	})
	if err != nil {
		t.Fatalf("build wl chapter predict line request: %v", err)
	}
	if chapterPayload.Name != "WL Event 预测" {
		t.Fatalf("unexpected wl chapter predict payload name: %s", chapterPayload.Name)
	}
	if chapterPayload.WlCid == nil || *chapterPayload.WlCid != charaID {
		t.Fatalf("expected wl chapter id %d, got %+v", charaID, chapterPayload.WlCid)
	}
	if len(chapterPayload.ForecastColumns) != 1 || len(chapterPayload.ForecastColumns[0].Ranks) != 1 {
		t.Fatalf("unexpected wl chapter forecast columns: %+v", chapterPayload.ForecastColumns)
	}
	if chapterPayload.ForecastColumns[0].Ranks[0].Score == nil || *chapterPayload.ForecastColumns[0].Ranks[0].Score != 9_999_999 {
		t.Fatalf("unexpected wl chapter forecast score: %+v", chapterPayload.ForecastColumns[0].Ranks[0])
	}

	if len(provider.queries) != 2 {
		t.Fatalf("expected two scoped forecast queries, got %d", len(provider.queries))
	}
	if provider.queries[0].Scope != ForecastScopeTotal || provider.queries[0].WlCharacterID != nil {
		t.Fatalf("unexpected first scoped forecast query: %+v", provider.queries[0])
	}
	if provider.queries[1].Scope != ForecastScopeChapter || provider.queries[1].WlCharacterID == nil || *provider.queries[1].WlCharacterID != charaID {
		t.Fatalf("unexpected second scoped forecast query: %+v", provider.queries[1])
	}
}

func TestBuildPredictLineRequestFromTrackerStopsInLastEventHour(t *testing.T) {
	now := time.Now().UnixMilli()
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(30*time.Minute/time.Millisecond),
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(lineNameTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)
	controller.SetForecastProvider(testForecastProvider{
		scores: map[int]ForecastScore{100: {Score: 1234567, Timestamp: 1_700_000_000}},
	})

	_, err := controller.BuildPredictLineRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{100},
	})
	if err == nil {
		t.Fatal("expected last-hour prediction stop error, got nil")
	}
	if got := err.Error(); got != skPredictionStopMessage {
		t.Fatalf("unexpected error: %v", got)
	}
}

func TestBuildPredictLineRequestFromTrackerStopsInLastWorldBloomChapterHour(t *testing.T) {
	now := time.Now().UnixMilli()
	charaID := 21
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "WL Event",
		EventType:   "world_bloom",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(6*time.Hour/time.Millisecond),
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(worldBloomLineTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
		worldBloomChapters: map[int][]*masterdata.WorldBloom{
			101: {
				{
					EventID:         101,
					GameCharacterID: &charaID,
					ChapterStartAt:  now - int64(time.Hour/time.Millisecond),
					AggregateAt:     now + int64(30*time.Minute/time.Millisecond),
				},
			},
		},
	}, nil)
	controller.SetForecastProvider(testForecastProvider{
		scores: map[int]ForecastScore{100: {Score: 1234567, Timestamp: 1_700_000_000}},
	})

	_, err := controller.BuildPredictLineRequestFromTracker(TrackerRankQuery{
		EventID:       101,
		Region:        "jp",
		Ranks:         []int{100},
		WlCharacterID: &charaID,
	})
	if err == nil {
		t.Fatal("expected last-hour prediction stop error, got nil")
	}
	if got := err.Error(); got != skPredictionStopMessage {
		t.Fatalf("unexpected error: %v", got)
	}
}

func TestBuildPredictLineRequestFromTrackerDoesNotFallbackWhenForecastCacheMissing(t *testing.T) {
	now := time.Now().UnixMilli()
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(2*time.Hour/time.Millisecond),
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(lineNameTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)
	controller.SetForecastProvider(testForecastProvider{
		err: fmt.Errorf("all forecast sources failed"),
	})

	_, err := controller.BuildPredictLineRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{100},
	})
	if err == nil {
		t.Fatal("expected missing forecast cache error, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "预测数据尚未就绪") {
		t.Fatalf("unexpected error: %v", got)
	}
}

func TestBuildPredictLineRequestFromTrackerUsesCachedGenericForecast(t *testing.T) {
	now := time.Now().UnixMilli()
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(2*time.Hour/time.Millisecond),
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(lineNameTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)
	controller.SetForecastProvider(contextAwareForecastProvider{
		wantKey:   "trace",
		wantValue: "sk-predict",
	})

	ctx := context.WithValue(context.Background(), ctxKey("trace"), "sk-predict")
	if err := controller.forecastCache.RefreshNow(ctx, "jp", 101); err != nil {
		t.Fatalf("prime forecast cache with context: %v", err)
	}
	payload, err := controller.WithContext(ctx).BuildPredictLineRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{100},
	})
	if err != nil {
		t.Fatalf("build predict line request: %v", err)
	}
	if len(payload.ForecastColumns) != 1 || payload.ForecastColumns[0].Key != "forecast" {
		t.Fatalf("unexpected forecast columns: %+v", payload.ForecastColumns)
	}
}

func TestRenderPredictLineFromTrackerUsesCachedForecastData(t *testing.T) {
	now := time.Now().UnixMilli()
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(2*time.Hour/time.Millisecond),
	}

	tracker := &lineMetricsOnlyTrackerSource{}
	forecast := &countingForecastProvider{
		bySource: map[string]ForecastSourceData{
			"33kit": {
				Scores: map[int]ForecastScore{
					50:  {Score: 12_345_678, Timestamp: 1_700_000_000, Source: "33kit"},
					100: {Score: 9_876_543, Timestamp: 1_700_000_100, Source: "33kit"},
				},
				FetchedAt: 1_700_000_200,
			},
		},
	}

	var drawingCalls atomic.Int32
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/line" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		drawingCalls.Add(1)
		_, _ = w.Write([]byte("predict-render"))
	}))
	defer drawingServer.Close()

	controller := NewController(drawing.NewHarukiDrawingClient(drawingServer.URL))
	controller.SetTrackerIntegration(tracker, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)
	controller.SetForecastProvider(forecast)
	if err := controller.forecastCache.RefreshNow(context.Background(), "jp", 101); err != nil {
		t.Fatalf("prime forecast cache: %v", err)
	}

	req := TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{100, 50},
	}

	first, err := controller.RenderPredictLineFromTracker(req)
	if err != nil {
		t.Fatalf("first render predict line: %v", err)
	}
	second, err := controller.RenderPredictLineFromTracker(req)
	if err != nil {
		t.Fatalf("second render predict line: %v", err)
	}
	if string(first) != "predict-render" || string(second) != "predict-render" {
		t.Fatalf("unexpected rendered bytes: %q / %q", string(first), string(second))
	}
	if got := tracker.latestRankCalls.Load(); got != 4 {
		t.Fatalf("expected tracker latest-rank calls to run for each render, got %d", got)
	}
	if got := forecast.calls.Load(); got != 1 {
		t.Fatalf("expected forecast fetch to run once, got %d", got)
	}
	if got := drawingCalls.Load(); got != 1 {
		t.Fatalf("expected drawing client cache to reuse rendered payload, got %d", got)
	}
}

func TestStartDefaultPredictWarmupPrimesDefaultPredictCache(t *testing.T) {
	now := time.Now().UnixMilli()
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(2*time.Hour/time.Millisecond),
	}

	tracker := &lineMetricsOnlyTrackerSource{}
	forecast := &countingForecastProvider{
		bySource: map[string]ForecastSourceData{
			"33kit": {
				Scores: map[int]ForecastScore{
					1:      {Score: 20_000_001, Timestamp: 1_700_000_000, Source: "33kit"},
					300000: {Score: 12_345_678, Timestamp: 1_700_000_100, Source: "33kit"},
				},
				FetchedAt: 1_700_000_200,
			},
		},
	}

	var drawingCalls atomic.Int32
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/line" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		drawingCalls.Add(1)
		_, _ = w.Write([]byte("predict-render"))
	}))
	defer drawingServer.Close()

	controller := NewController(drawing.NewHarukiDrawingClient(drawingServer.URL))
	controller.SetTrackerIntegration(tracker, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)
	controller.SetForecastProvider(forecast)
	controller.StartDefaultPredictWarmup()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := controller.forecastCache.CachedBySource("jp", 101, defaultPredictWarmupRanks); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for startup warmup: forecast=%d", forecast.calls.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}

	req := TrackerRankQuery{
		Region:       "jp",
		Ranks:        append([]int(nil), defaultPredictWarmupRanks...),
		DefaultRanks: true,
	}
	got, err := controller.RenderPredictLineFromTracker(req)
	if err != nil {
		t.Fatalf("render warmed predict line: %v", err)
	}
	if string(got) != "predict-render" {
		t.Fatalf("unexpected warmed predict bytes: %q", string(got))
	}
	if calls := forecast.calls.Load(); calls != 1 {
		t.Fatalf("expected warmed request to avoid extra forecast fetch, got %d", calls)
	}
	if calls := drawingCalls.Load(); calls != 1 {
		t.Fatalf("expected warmed request to render once, got %d", calls)
	}
}

func TestRefreshDefaultPredictDataPrimesCurrentWorldBloomChapterPredictCache(t *testing.T) {
	now := time.Now().UnixMilli()
	charaID := 15
	nextCharaID := 16
	eventInfo := &masterdata.Event{
		ID:          167,
		Name:        "World Bloom Event",
		EventType:   "world_bloom",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(8*time.Hour/time.Millisecond),
	}
	provider := &scopedForecastProvider{
		data: map[string]map[string]ForecastSourceData{
			"total": {
				"local": {
					Scores: map[int]ForecastScore{
						100: {Score: 8_888_888, Timestamp: 1_700_000_000, Source: "local"},
					},
					FetchedAt: 1_700_000_100,
				},
			},
			"chapter:15": {
				"local": {
					Scores: map[int]ForecastScore{
						100: {Score: 9_999_999, Timestamp: 1_700_000_200, Source: "local"},
					},
					FetchedAt: 1_700_000_300,
				},
			},
		},
	}
	controller := NewController(nil)
	controller.RegisterEventSource(&testEventSource{
		region: renderregion.CN,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
		worldBloomChapters: map[int][]*masterdata.WorldBloom{
			167: {
				{
					EventID:         167,
					GameCharacterID: &charaID,
					ChapterStartAt:  now - int64(time.Hour/time.Millisecond),
					AggregateAt:     now + int64(2*time.Hour/time.Millisecond),
				},
				{
					EventID:         167,
					GameCharacterID: &nextCharaID,
					ChapterStartAt:  now + int64(3*time.Hour/time.Millisecond),
					AggregateAt:     now + int64(5*time.Hour/time.Millisecond),
				},
			},
		},
	})
	controller.SetForecastProvider(provider)

	controller.refreshDefaultPredictData([]string{"cn"})

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := controller.forecastCache.CachedBySourceQuery(ForecastQuery{
			Region:  "cn",
			EventID: 167,
			Scope:   ForecastScopeTotal,
			Ranks:   []int{100},
		}); err == nil {
			if _, err := controller.forecastCache.CachedBySourceQuery(ForecastQuery{
				Region:        "cn",
				EventID:       167,
				Scope:         ForecastScopeChapter,
				WlCharacterID: &charaID,
				Ranks:         []int{100},
			}); err == nil {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for world bloom warmup: queries=%+v", provider.querySnapshot())
		}
		time.Sleep(10 * time.Millisecond)
	}

	var sawTotal, sawChapter bool
	for _, query := range provider.querySnapshot() {
		switch {
		case query.Scope == ForecastScopeTotal && query.WlCharacterID == nil:
			sawTotal = true
		case query.Scope == ForecastScopeChapter && query.WlCharacterID != nil && *query.WlCharacterID == charaID:
			sawChapter = true
		case query.Scope == ForecastScopeChapter && query.WlCharacterID != nil && *query.WlCharacterID == nextCharaID:
			t.Fatalf("unexpected future world bloom chapter warmup: %+v", query)
		}
	}
	if !sawTotal || !sawChapter {
		t.Fatalf("expected total and current chapter warmup, got %+v", provider.querySnapshot())
	}
}

func TestForecastDataCacheRefreshesStaleWorldBloomChapterInBackground(t *testing.T) {
	charaID := 15
	provider := &scopedForecastProvider{
		data: map[string]map[string]ForecastSourceData{
			"chapter:15": {
				"local": {
					Scores: map[int]ForecastScore{
						100: {Score: 1_000_000, Timestamp: 1_700_000_000, Source: "local"},
					},
					FetchedAt: 1_700_000_100,
				},
			},
		},
	}
	cache := newForecastDataCache(provider)
	query := ForecastQuery{
		Region:        "cn",
		EventID:       167,
		Scope:         ForecastScopeChapter,
		WlCharacterID: &charaID,
		Ranks:         []int{100},
	}
	if err := cache.RefreshNowQuery(context.Background(), query); err != nil {
		t.Fatalf("prime chapter forecast cache: %v", err)
	}
	key, ok := newForecastDataCacheKey(query)
	if !ok {
		t.Fatal("invalid test forecast query")
	}
	cache.mu.Lock()
	staleAt := time.Now().UTC().Add(-forecastDataRefreshInterval - time.Second)
	cache.entries[key].refreshedAt = staleAt
	cache.entries[key].lastAttemptAt = staleAt
	cache.mu.Unlock()

	next := provider.data["chapter:15"]["local"]
	next.Scores[100] = ForecastScore{Score: 2_000_000, Timestamp: 1_700_000_500, Source: "local"}
	next.FetchedAt = 1_700_000_600
	provider.data["chapter:15"]["local"] = next

	cached, err := cache.CachedBySourceQuery(query)
	if err != nil {
		t.Fatalf("read stale chapter forecast cache: %v", err)
	}
	if got := cached["local"].Scores[100].Score; got != 1_000_000 {
		t.Fatalf("expected stale read to return old score, got %d", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		cached, err = cache.CachedBySourceQuery(query)
		if err == nil && cached["local"].Scores[100].Score == 2_000_000 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for stale chapter refresh: queries=%+v cached=%+v err=%v", provider.querySnapshot(), cached, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(provider.querySnapshot()); got < 2 {
		t.Fatalf("expected stale cache read to trigger background refresh, got %d queries", got)
	}
}
