package sk

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	sekaiapi "haruki-cloud/utils/sekai"
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

func (p contextAwareForecastProvider) Fetch(ctx context.Context, _ string, _ int, ranks []int) (map[int]ForecastScore, error) {
	value, _ := ctx.Value(p.wantKey).(string)
	if value != p.wantValue {
		return nil, context.Canceled
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
	earlierTs := int64(1_000_000)
	latestTs := int64(1_001_490)
	earlierScore := 22_527_600
	return []sekaiapi.ScoreGrowthPoint{
		{
			Rank:             50,
			ScoreLatest:      23_171_700,
			ScoreEarlier:     &earlierScore,
			TimestampLatest:  latestTs,
			TimestampEarlier: &earlierTs,
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
	region renderregion.Value
	events []*masterdata.Event
	byID   map[int]*masterdata.Event
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

func (s *testEventSource) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom { return nil }

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

	userID := int64(55667788990011)
	payload, err := controller.BuildPlayerTraceFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		UserID:  &userID,
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

func TestBuildPredictLineRequestFromTrackerUsesForecastScores(t *testing.T) {
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
		},
	})

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
	if len(payload.Ranks) != 2 {
		t.Fatalf("unexpected current ranks len: %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Rank != 50 || payload.Ranks[0].Score == nil || *payload.Ranks[0].Score != 1_000_050 {
		t.Fatalf("unexpected first current rank payload: %+v", payload.Ranks[0])
	}
	if payload.Ranks[1].Rank != 100 || payload.Ranks[1].Score == nil || *payload.Ranks[1].Score != 1_000_100 {
		t.Fatalf("unexpected second current rank payload: %+v", payload.Ranks[1])
	}
	if len(payload.ForecastColumns) != 2 {
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
	if payload.ForecastColumns[1].Key != "sekarun" {
		t.Fatalf("unexpected second forecast column key: %s", payload.ForecastColumns[1].Key)
	}
}

func TestBuildPredictLineRequestFromTrackerFallsBackToRealtimeForWorldBloomChapter(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "WL Event",
		EventType:   "world_bloom",
		StartAt:     111,
		AggregateAt: 222,
	}
	controller := NewController(nil)
	controller.SetTrackerIntegration(worldBloomLineTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)
	controller.SetForecastProvider(testForecastProvider{
		scores: map[int]ForecastScore{100: {Score: 1234567, Timestamp: 1_700_000_000}},
	})

	cid := 21
	payload, err := controller.BuildPredictLineRequestFromTracker(TrackerRankQuery{
		EventID:       101,
		Region:        "jp",
		Ranks:         []int{100},
		WlCharacterID: &cid,
	})
	if err != nil {
		t.Fatalf("build wl predict fallback: %v", err)
	}
	if payload.Name != "WL Event 预测(实时)" {
		t.Fatalf("unexpected payload name: %s", payload.Name)
	}
	if payload.WlCid == nil || *payload.WlCid != 21 {
		t.Fatalf("expected wl chapter id to be preserved, got %+v", payload.WlCid)
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Score == nil || *payload.Ranks[0].Score != 2_000_121 {
		t.Fatalf("unexpected fallback rank payload: %+v", payload.Ranks[0])
	}
}

func TestBuildPredictLineRequestFromTrackerFallsBackToRealtimeWhenForecastFails(t *testing.T) {
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
	controller.SetForecastProvider(testForecastProvider{
		err: fmt.Errorf("all forecast sources failed"),
	})

	payload, err := controller.BuildPredictLineRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{100},
	})
	if err != nil {
		t.Fatalf("build predict line request with fallback: %v", err)
	}
	if payload.Name != "Tracker Event 预测(实时)" {
		t.Fatalf("unexpected fallback payload name: %s", payload.Name)
	}
	if len(payload.ForecastColumns) != 0 {
		t.Fatalf("fallback payload should not include forecast columns")
	}
	if len(payload.Ranks) != 1 {
		t.Fatalf("unexpected fallback ranks len: %d", len(payload.Ranks))
	}
	if payload.Ranks[0].Rank != 100 || payload.Ranks[0].Score == nil || *payload.Ranks[0].Score != 1000100 {
		t.Fatalf("unexpected fallback rank payload: %+v", payload.Ranks[0])
	}
	if payload.Ranks[0].Name != "" {
		t.Fatalf("fallback rank name should stay empty, got %+v", payload.Ranks[0])
	}
}

func TestBuildPredictLineRequestFromTrackerUsesControllerRequestContext(t *testing.T) {
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
	controller.SetForecastProvider(contextAwareForecastProvider{
		wantKey:   ctxKey("trace"),
		wantValue: "sk-predict",
	})

	ctx := context.WithValue(context.Background(), ctxKey("trace"), "sk-predict")
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
