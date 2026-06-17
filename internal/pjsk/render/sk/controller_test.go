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
	renderassets "haruki-cloud/internal/pjsk/render/assets"
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

type testLegacyTrackerSource interface {
	GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error)
	GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error)
	GetLatestWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomLatestRankingResponse, error)
	GetLatestWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomLatestRankingResponse, error)
	GetUserEventData(server string, eventID int, userID int64) (*sekaiapi.UserEventData, error)
	GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error)
	GetWorldBloomRankingScoreGrowth(server string, eventID, characterID, interval int) ([]sekaiapi.ScoreGrowthPoint, error)
	TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error)
	TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error)
	TraceWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomTraceRankingResponse, error)
	TraceWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomTraceRankingResponse, error)
}

type testTrackerSource struct{}

func setTestTrackerIntegration(c *Controller, tracker testLegacyTrackerSource, events EventSource, assetHelper *renderassets.AssetHelper) {
	if _, ok := tracker.(trackerCloudV2Source); ok {
		c.SetTrackerIntegration(tracker.(TrackerSource), events, assetHelper)
		return
	}
	c.SetTrackerIntegration(testCloudV2TrackerSource{testLegacyTrackerSource: tracker}, events, assetHelper)
}

type testCloudV2TrackerSource struct {
	testLegacyTrackerSource
}

func (s testCloudV2TrackerSource) GetEventStatus(server string, eventID int) (*sekaiapi.EventStatusResponse, error) {
	source, ok := s.testLegacyTrackerSource.(trackerEventStatusSource)
	if !ok {
		return nil, fmt.Errorf("not implemented")
	}
	return source.GetEventStatus(server, eventID)
}

func (s testCloudV2TrackerSource) GetCloudSKQuery(server string, eventID int, characterID *int, ranks []int, userID *int64, includeAdjacent, skipMissing bool, intervalSeconds int64) (*sekaiapi.CloudRankQueryResponse, error) {
	out := &sekaiapi.CloudRankQueryResponse{}
	if userID != nil && *userID > 0 {
		item, err := s.cloudRankInfoByUser(server, eventID, characterID, *userID)
		if err != nil {
			return nil, err
		}
		if item.Rank <= 0 {
			return nil, sekaiapi.ErrRankingNotFound
		}
		if includeAdjacent {
			s.applyCloudTraceMetrics(server, eventID, characterID, &item, "user", cloudUserIDSubject(item))
		}
		out.Ranks = append(out.Ranks, item)
		if includeAdjacent {
			s.attachCloudAdjacent(server, eventID, characterID, item.Rank, out)
		}
		return out, nil
	}
	for _, rank := range ranks {
		item, err := s.cloudRankInfoByRank(server, eventID, characterID, rank)
		if err != nil {
			if shouldSkipMissingTrackerRankError(skipMissing, err) {
				continue
			}
			return nil, err
		}
		out.Ranks = append(out.Ranks, item)
		if includeAdjacent && len(ranks) == 1 {
			s.attachCloudAdjacent(server, eventID, characterID, item.Rank, out)
		}
	}
	return out, nil
}

func (s testCloudV2TrackerSource) GetCloudSKCheckRoom(server string, eventID int, characterID *int, ranks []int, userID *int64, skipMissing bool, intervalSeconds int64) (*sekaiapi.CloudCheckRoomResponse, error) {
	resp, err := s.GetCloudSKQuery(server, eventID, characterID, ranks, userID, true, skipMissing, intervalSeconds)
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Ranks) == 0 {
		return nil, sekaiapi.ErrRankingNotFound
	}
	rank := resp.Ranks[0]
	if rank.Rank > skCheckRoomRankLimit {
		return nil, fmt.Errorf("查房/查水表目前仅支持前100名查询")
	}
	s.applyCloudTraceMetrics(server, eventID, characterID, &rank, "user", cloudUserIDSubject(rank))
	return &sekaiapi.CloudCheckRoomResponse{
		Rank:     rank,
		Previous: resp.Previous,
		Next:     resp.Next,
	}, nil
}

func (s testCloudV2TrackerSource) GetCloudSKLine(server string, eventID int, characterID *int, ranks []int, userID *int64, skipMissing bool, intervalSeconds int64) (*sekaiapi.CloudLineResponse, error) {
	out := make([]sekaiapi.CloudRankInfo, 0, len(ranks))
	if userID != nil && *userID > 0 {
		item, err := s.cloudRankInfoByUser(server, eventID, characterID, *userID)
		if err != nil {
			return nil, err
		}
		item.Name = ""
		return &sekaiapi.CloudLineResponse{Ranks: []sekaiapi.CloudRankInfo{item}}, nil
	}
	for _, rank := range ranks {
		item, err := s.cloudLineInfoByRank(server, eventID, characterID, rank)
		if err != nil {
			if shouldSkipMissingTrackerRankError(skipMissing, err) {
				continue
			}
			return nil, err
		}
		out = append(out, item)
	}
	return &sekaiapi.CloudLineResponse{Ranks: out}, nil
}

func (s testCloudV2TrackerSource) GetCloudSKSpeed(server string, eventID int, characterID *int, ranks []int, intervalSeconds, unitSeconds int64, skipMissing bool) (*sekaiapi.CloudSpeedResponse, error) {
	out := make([]sekaiapi.CloudRankInfo, 0, len(ranks))
	for _, rank := range ranks {
		item, ok, err := s.cloudSpeedInfoByRank(server, eventID, characterID, rank, int(intervalSeconds), unitSeconds)
		if err != nil {
			if shouldSkipMissingTrackerRankError(skipMissing, err) {
				continue
			}
			return nil, err
		}
		if !ok {
			s.applyCloudTraceMetrics(server, eventID, characterID, &item, "rank", strconv.Itoa(item.Rank))
		}
		out = append(out, item)
	}
	return &sekaiapi.CloudSpeedResponse{Speeds: out, IntervalSeconds: intervalSeconds, UnitSeconds: unitSeconds}, nil
}

func (s testCloudV2TrackerSource) GetCloudSKTrace(server string, eventID int, characterID *int, subjectType string, subject string, limit int) (*sekaiapi.CloudTraceResponse, error) {
	points, userData, err := s.cloudTracePoints(server, eventID, characterID, subjectType, subject)
	if err != nil {
		return nil, err
	}
	if len(points) == 0 {
		return nil, sekaiapi.ErrRankingNotFound
	}
	name := ""
	userID := ""
	if userData != nil {
		name = userData.Name
		userID = strings.TrimSpace(userData.UserID)
	}
	if name == "" {
		name = s.cloudResolvedName(server, eventID, userID, "")
	}
	if subjectType == "rank" && len(points) > 0 {
		currentUserID := strings.TrimSpace(points[len(points)-1].UserID)
		if currentUserID != "" {
			if currentTrace, currentUserData, err := s.cloudTracePoints(server, eventID, characterID, "user", currentUserID); err == nil && len(currentTrace) > 0 {
				points = currentTrace
				if currentUserData != nil && strings.TrimSpace(currentUserData.UserID) != "" {
					userID = currentUserData.UserID
				} else {
					userID = currentUserID
				}
			}
		}
	}
	if latestName, latestUserID := s.cloudCurrentSubjectName(server, eventID, characterID, subjectType, subject, userID); latestName != "" {
		name = latestName
		if latestUserID != "" {
			userID = latestUserID
		}
	}
	out := make([]sekaiapi.CloudRankInfo, 0, len(points))
	for _, point := range points {
		pointUserID := strings.TrimSpace(point.UserID)
		itemUserID := pointUserID
		if itemUserID == "" {
			itemUserID = userID
		}
		out = append(out, sekaiapi.CloudRankInfo{
			Rank:        point.Rank,
			UserID:      stringPtrIfNotEmpty(itemUserID),
			Name:        name,
			Score:       point.Score,
			Timestamp:   point.Timestamp,
			CharacterID: characterID,
		})
	}
	return &sekaiapi.CloudTraceResponse{
		Subject:  sekaiapi.SubjectTraceMeta{SubjectType: subjectType, Subject: subject, ResolvedUserID: stringPtrIfNotEmpty(userID)},
		RankData: out,
	}, nil
}

func (s testCloudV2TrackerSource) cloudRankInfoByRank(server string, eventID int, characterID *int, rank int) (sekaiapi.CloudRankInfo, error) {
	if characterID != nil {
		resp, err := s.GetLatestWorldBloomRankingByRank(server, eventID, *characterID, rank)
		if err != nil {
			return sekaiapi.CloudRankInfo{}, err
		}
		return cloudRankInfoFromLatest(resp.RankData.RankDataPoint, resp.UserData, characterID, s.cloudResolvedName(server, eventID, cloudLatestUserID(resp.RankData.RankDataPoint, resp.UserData), resp.UserData.Name)), nil
	}
	resp, err := s.GetLatestRankingByRank(server, eventID, rank)
	if err != nil {
		return sekaiapi.CloudRankInfo{}, err
	}
	return cloudRankInfoFromLatest(resp.RankData, resp.UserData, nil, s.cloudResolvedName(server, eventID, cloudLatestUserID(resp.RankData, resp.UserData), resp.UserData.Name)), nil
}

func (s testCloudV2TrackerSource) cloudRankInfoByUser(server string, eventID int, characterID *int, userID int64) (sekaiapi.CloudRankInfo, error) {
	if characterID != nil {
		resp, err := s.GetLatestWorldBloomRankingByUser(server, eventID, *characterID, userID)
		if err != nil {
			return sekaiapi.CloudRankInfo{}, err
		}
		return cloudRankInfoFromLatest(resp.RankData.RankDataPoint, resp.UserData, characterID, s.cloudResolvedName(server, eventID, cloudLatestUserID(resp.RankData.RankDataPoint, resp.UserData), resp.UserData.Name)), nil
	}
	resp, err := s.GetLatestRankingByUser(server, eventID, userID)
	if err != nil {
		return sekaiapi.CloudRankInfo{}, err
	}
	return cloudRankInfoFromLatest(resp.RankData, resp.UserData, nil, s.cloudResolvedName(server, eventID, cloudLatestUserID(resp.RankData, resp.UserData), resp.UserData.Name)), nil
}

func (s testCloudV2TrackerSource) cloudLineInfoByRank(server string, eventID int, characterID *int, rank int) (sekaiapi.CloudRankInfo, error) {
	if characterID != nil {
		resp, err := s.GetLatestWorldBloomRankingByRank(server, eventID, *characterID, rank)
		if err != nil {
			return sekaiapi.CloudRankInfo{}, err
		}
		return cloudRankInfoFromLatest(resp.RankData.RankDataPoint, resp.UserData, characterID, ""), nil
	}
	resp, err := s.GetLatestRankingByRank(server, eventID, rank)
	if err != nil {
		return sekaiapi.CloudRankInfo{}, err
	}
	return cloudRankInfoFromLatest(resp.RankData, resp.UserData, nil, ""), nil
}

func (s testCloudV2TrackerSource) attachCloudAdjacent(server string, eventID int, characterID *int, rank int, out *sekaiapi.CloudRankQueryResponse) {
	prevRank, nextRank, hasPrev, hasNext := queryAdjacentSKLineRanks(rank, characterID != nil)
	if hasPrev {
		if prev, err := s.cloudRankInfoByRank(server, eventID, characterID, prevRank); err == nil {
			out.Previous = &prev
		}
	}
	if hasNext {
		if next, err := s.cloudRankInfoByRank(server, eventID, characterID, nextRank); err == nil {
			out.Next = &next
		}
	}
}

func (s testCloudV2TrackerSource) applyCloudTraceMetrics(server string, eventID int, characterID *int, item *sekaiapi.CloudRankInfo, subjectType string, subject string) {
	points, _, err := s.cloudTracePoints(server, eventID, characterID, subjectType, subject)
	if err != nil || len(points) == 0 {
		return
	}
	info := drawing.RankInfo{}
	applyRankInfoMetrics(&info, rankTraceSamples(points))
	if info.AverageRound != nil {
		value := *info.AverageRound
		item.AverageRound = &value
	}
	if info.AveragePt != nil {
		value := *info.AveragePt
		item.AveragePt = &value
	}
	if info.LatestPt != nil {
		value := *info.LatestPt
		item.LatestPt = &value
	}
	if info.Speed != nil {
		item.Speed = info.Speed
	}
	if info.Min20Time3Speed != nil {
		value := *info.Min20Time3Speed
		item.Min20Time3Speed = &value
	}
	if info.HourRound != nil {
		value := *info.HourRound
		item.HourRound = &value
	}
	if info.RecordStartAt != nil {
		value := *info.RecordStartAt
		item.RecordStartAt = &value
	}
}

func (s testCloudV2TrackerSource) cloudSpeedInfoByRank(server string, eventID int, characterID *int, rank int, interval int, unitPeriodSeconds int64) (sekaiapi.CloudRankInfo, bool, error) {
	var points []sekaiapi.ScoreGrowthPoint
	var err error
	if characterID != nil {
		points, err = s.GetWorldBloomRankingScoreGrowth(server, eventID, *characterID, interval)
	} else {
		points, err = s.GetRankingScoreGrowth(server, eventID, interval)
	}
	if err == nil {
		for _, point := range points {
			if point.Rank != rank {
				continue
			}
			info := speedInfoFromGrowthPoint(point, unitPeriodSeconds)
			return sekaiapi.CloudRankInfo{
				Rank:        rank,
				Score:       info.Score,
				Timestamp:   info.RecordTime,
				Speed:       info.Speed,
				CharacterID: characterID,
			}, info.Speed != nil, nil
		}
	}
	item := sekaiapi.CloudRankInfo{Rank: rank, CharacterID: characterID}
	pointsTrace, userData, traceErr := s.cloudTracePoints(server, eventID, characterID, "rank", strconv.Itoa(rank))
	if traceErr != nil {
		return sekaiapi.CloudRankInfo{}, false, traceErr
	}
	if len(pointsTrace) > 0 {
		last := pointsTrace[len(pointsTrace)-1]
		item = cloudRankInfoFromLatest(last, derefRankingUserData(userData), characterID, "")
	}
	return item, false, nil
}

func (s testCloudV2TrackerSource) cloudTracePoints(server string, eventID int, characterID *int, subjectType string, subject string) ([]sekaiapi.RankDataPoint, *sekaiapi.RankingUserData, error) {
	switch subjectType {
	case "user":
		userID, err := strconv.ParseInt(subject, 10, 64)
		if err != nil {
			return nil, nil, err
		}
		if characterID != nil {
			resp, err := s.TraceWorldBloomRankingByUser(server, eventID, *characterID, userID)
			if err != nil {
				return nil, nil, err
			}
			return flattenWorldBloomPoints(resp.RankData), &resp.UserData, nil
		}
		resp, err := s.TraceRankingByUser(server, eventID, userID)
		if err != nil {
			return nil, nil, err
		}
		return resp.RankData, &resp.UserData, nil
	default:
		rank, err := strconv.Atoi(subject)
		if err != nil {
			return nil, nil, err
		}
		if characterID != nil {
			resp, err := s.TraceWorldBloomRankingByRank(server, eventID, *characterID, rank)
			if err != nil {
				return nil, nil, err
			}
			return flattenWorldBloomPoints(resp.RankData), &resp.UserData, nil
		}
		resp, err := s.TraceRankingByRank(server, eventID, rank)
		if err != nil {
			return nil, nil, err
		}
		return resp.RankData, &resp.UserData, nil
	}
}

func (s testCloudV2TrackerSource) cloudResolvedName(server string, eventID int, userID string, fallback string) string {
	parsed, err := strconv.ParseInt(strings.TrimSpace(userID), 10, 64)
	if err == nil && parsed > 0 {
		if data, dataErr := s.GetUserEventData(server, eventID, parsed); dataErr == nil && data != nil && strings.TrimSpace(data.Name) != "" {
			return data.Name
		}
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return fallback
}

func (s testCloudV2TrackerSource) cloudCurrentSubjectName(server string, eventID int, characterID *int, subjectType string, subject string, fallbackUserID string) (string, string) {
	var item sekaiapi.CloudRankInfo
	var err error
	if subjectType == "user" {
		userID, parseErr := strconv.ParseInt(subject, 10, 64)
		if parseErr != nil {
			return "", fallbackUserID
		}
		item, err = s.cloudRankInfoByUser(server, eventID, characterID, userID)
	} else {
		rank, parseErr := strconv.Atoi(subject)
		if parseErr != nil {
			return "", fallbackUserID
		}
		item, err = s.cloudRankInfoByRank(server, eventID, characterID, rank)
	}
	if err != nil {
		return "", fallbackUserID
	}
	userID := fallbackUserID
	if item.UserID != nil && strings.TrimSpace(*item.UserID) != "" {
		userID = *item.UserID
	}
	return strings.TrimSpace(item.Name), userID
}

func cloudRankInfoFromLatest(point sekaiapi.RankDataPoint, userData sekaiapi.RankingUserData, characterID *int, name string) sekaiapi.CloudRankInfo {
	userID := strings.TrimSpace(point.UserID)
	if userID == "" {
		userID = strings.TrimSpace(userData.UserID)
	}
	return sekaiapi.CloudRankInfo{
		Rank:        point.Rank,
		UserID:      stringPtrIfNotEmpty(userID),
		Name:        name,
		Score:       point.Score,
		Timestamp:   point.Timestamp,
		CharacterID: characterID,
	}
}

func cloudLatestUserID(point sekaiapi.RankDataPoint, userData sekaiapi.RankingUserData) string {
	userID := strings.TrimSpace(point.UserID)
	if userID == "" {
		userID = strings.TrimSpace(userData.UserID)
	}
	return userID
}

func derefRankingUserData(userData *sekaiapi.RankingUserData) sekaiapi.RankingUserData {
	if userData == nil {
		return sekaiapi.RankingUserData{}
	}
	return *userData
}

func cloudUserIDSubject(item sekaiapi.CloudRankInfo) string {
	if item.UserID != nil {
		return *item.UserID
	}
	return strconv.Itoa(item.Rank)
}

func flattenWorldBloomPoints(points []sekaiapi.WorldBloomRankDataPoint) []sekaiapi.RankDataPoint {
	out := make([]sekaiapi.RankDataPoint, 0, len(points))
	for _, point := range points {
		out = append(out, point.RankDataPoint)
	}
	return out
}

func stringPtrIfNotEmpty(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

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

type batchLineMetricsTrackerSource struct {
	lineMetricsOnlyTrackerSource
	batchTraceRankCalls      atomic.Int32
	batchWorldTraceRankCalls atomic.Int32
}

type leaderboardV2TrackerSource struct {
	testTrackerSource
	snapshotCalls     atomic.Int32
	subjectTraceCalls atomic.Int32
	legacyTraceCalls  atomic.Int32
}

func (s *leaderboardV2TrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	s.legacyTraceCalls.Add(1)
	return nil, fmt.Errorf("legacy trace should not be called")
}

func (s *leaderboardV2TrackerSource) GetCloudSKQuery(server string, eventID int, characterID *int, ranks []int, userID *int64, includeAdjacent, skipMissing bool, intervalSeconds int64) (*sekaiapi.CloudRankQueryResponse, error) {
	s.snapshotCalls.Add(1)
	out := &sekaiapi.CloudRankQueryResponse{}
	for _, rank := range ranks {
		userID := strconv.Itoa(10000 + rank)
		item := sekaiapi.CloudRankInfo{
			Rank:      rank,
			UserID:    &userID,
			Name:      "V2User",
			Score:     1_000_000 + rank,
			Timestamp: 1704067200,
			Speed:     drawing.IntPtr(12345),
		}
		out.Ranks = append(out.Ranks, item)
		if includeAdjacent {
			prevID := strconv.Itoa(9999 + rank)
			nextID := strconv.Itoa(10001 + rank)
			out.Previous = &sekaiapi.CloudRankInfo{Rank: rank - 1, UserID: &prevID, Name: "PrevUser", Score: 1_000_100 + rank, Timestamp: 1704067200}
			out.Next = &sekaiapi.CloudRankInfo{Rank: rank + 1, UserID: &nextID, Name: "NextUser", Score: 999_900 + rank, Timestamp: 1704067200}
		}
	}
	return out, nil
}

func (s *leaderboardV2TrackerSource) GetCloudSKTrace(server string, eventID int, characterID *int, subjectType string, subject string, limit int) (*sekaiapi.CloudTraceResponse, error) {
	s.subjectTraceCalls.Add(1)
	userID := "10001"
	rank := 1
	return &sekaiapi.CloudTraceResponse{
		Subject: sekaiapi.SubjectTraceMeta{SubjectType: subjectType, Subject: subject, ResolvedUserID: &userID, ResolvedRank: &rank},
		RankData: []sekaiapi.CloudRankInfo{
			{Rank: 1, UserID: &userID, Name: "TraceV2User", Score: 1_000_000, Timestamp: 1704063600},
			{Rank: 1, UserID: &userID, Name: "TraceV2User", Score: 1_100_000, Timestamp: 1704067200},
		},
	}, nil
}

func (s *leaderboardV2TrackerSource) GetCloudSKCheckRoom(server string, eventID int, characterID *int, ranks []int, userID *int64, skipMissing bool, intervalSeconds int64) (*sekaiapi.CloudCheckRoomResponse, error) {
	resp, err := s.GetCloudSKQuery(server, eventID, characterID, ranks, userID, true, skipMissing, intervalSeconds)
	if err != nil {
		return nil, err
	}
	if len(resp.Ranks) == 0 {
		return nil, sekaiapi.ErrRankingNotFound
	}
	return &sekaiapi.CloudCheckRoomResponse{Rank: resp.Ranks[0], Previous: resp.Previous, Next: resp.Next}, nil
}

func (s *leaderboardV2TrackerSource) GetCloudSKLine(server string, eventID int, characterID *int, ranks []int, userID *int64, skipMissing bool, intervalSeconds int64) (*sekaiapi.CloudLineResponse, error) {
	resp, err := s.GetCloudSKQuery(server, eventID, characterID, ranks, userID, false, skipMissing, intervalSeconds)
	if err != nil {
		return nil, err
	}
	return &sekaiapi.CloudLineResponse{Ranks: resp.Ranks}, nil
}

func (s *leaderboardV2TrackerSource) GetCloudSKSpeed(server string, eventID int, characterID *int, ranks []int, intervalSeconds, unitSeconds int64, skipMissing bool) (*sekaiapi.CloudSpeedResponse, error) {
	resp, err := s.GetCloudSKQuery(server, eventID, characterID, ranks, nil, false, skipMissing, intervalSeconds)
	if err != nil {
		return nil, err
	}
	return &sekaiapi.CloudSpeedResponse{Speeds: resp.Ranks, IntervalSeconds: intervalSeconds, UnitSeconds: unitSeconds}, nil
}

type cloudV2SparseCheckRoomTrackerSource struct {
	leaderboardV2TrackerSource
}

func (s *cloudV2SparseCheckRoomTrackerSource) GetCloudSKCheckRoom(server string, eventID int, characterID *int, ranks []int, userID *int64, skipMissing bool, intervalSeconds int64) (*sekaiapi.CloudCheckRoomResponse, error) {
	resp, err := s.leaderboardV2TrackerSource.GetCloudSKCheckRoom(server, eventID, characterID, ranks, userID, skipMissing, intervalSeconds)
	if err != nil {
		return nil, err
	}
	// Production EventTracker 2.5.14 only returns latest rank, score and speed.
	resp.Rank.AverageRound = nil
	resp.Rank.AveragePt = nil
	resp.Rank.LatestPt = nil
	resp.Rank.HourRound = nil
	resp.Rank.Min20Time3Speed = nil
	resp.Rank.RecordStartAt = nil
	return resp, nil
}

func (s *batchLineMetricsTrackerSource) TraceRankingsByRanks(server string, eventID int, ranks []int) (*sekaiapi.BatchTraceRankingResponse, error) {
	s.batchTraceRankCalls.Add(1)
	items := make([]sekaiapi.BatchTraceRankingItem, 0, len(ranks))
	for _, rank := range ranks {
		items = append(items, sekaiapi.BatchTraceRankingItem{
			Rank: rank,
			RankData: []sekaiapi.RankDataPoint{
				{Score: 900000 + rank, Rank: rank, Timestamp: 1704060000},
				{Score: 1000000 + rank, Rank: rank, Timestamp: 1704067200},
			},
		})
	}
	return &sekaiapi.BatchTraceRankingResponse{Items: items}, nil
}

func (s *batchLineMetricsTrackerSource) TraceWorldBloomRankingsByRanks(server string, eventID, characterID int, ranks []int) (*sekaiapi.BatchWorldBloomTraceRankingResponse, error) {
	s.batchWorldTraceRankCalls.Add(1)
	charID := characterID
	items := make([]sekaiapi.BatchWorldBloomTraceRankingItem, 0, len(ranks))
	for _, rank := range ranks {
		items = append(items, sekaiapi.BatchWorldBloomTraceRankingItem{
			Rank: rank,
			RankData: []sekaiapi.WorldBloomRankDataPoint{
				{RankDataPoint: sekaiapi.RankDataPoint{Score: 2900000 + rank + characterID, Rank: rank, Timestamp: 1704060000}, CharacterID: &charID},
				{RankDataPoint: sekaiapi.RankDataPoint{Score: 3000000 + rank + characterID, Rank: rank, Timestamp: 1704067200}, CharacterID: &charID},
			},
		})
	}
	return &sekaiapi.BatchWorldBloomTraceRankingResponse{Items: items}, nil
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

type csbRankOwnerTrackerSource struct {
	mu        sync.Mutex
	traceType []string
}

func (s *csbRankOwnerTrackerSource) GetCloudSKQuery(server string, eventID int, characterID *int, ranks []int, userID *int64, includeAdjacent, skipMissing bool, intervalSeconds int64) (*sekaiapi.CloudRankQueryResponse, error) {
	if len(ranks) != 1 || ranks[0] != 51 {
		return nil, fmt.Errorf("unexpected query ranks: %v", ranks)
	}
	return &sekaiapi.CloudRankQueryResponse{
		Ranks: []sekaiapi.CloudRankInfo{
			{
				Rank:      51,
				UserID:    stringPtrIfNotEmpty("20051"),
				Name:      "CurrentOwner",
				Score:     7654321,
				Timestamp: 1800000000,
			},
		},
	}, nil
}

func (s *csbRankOwnerTrackerSource) GetCloudSKCheckRoom(server string, eventID int, characterID *int, ranks []int, userID *int64, skipMissing bool, intervalSeconds int64) (*sekaiapi.CloudCheckRoomResponse, error) {
	return nil, fmt.Errorf("unexpected check-room call")
}

func (s *csbRankOwnerTrackerSource) GetCloudSKLine(server string, eventID int, characterID *int, ranks []int, userID *int64, skipMissing bool, intervalSeconds int64) (*sekaiapi.CloudLineResponse, error) {
	return nil, fmt.Errorf("unexpected line call")
}

func (s *csbRankOwnerTrackerSource) GetCloudSKSpeed(server string, eventID int, characterID *int, ranks []int, intervalSeconds, unitSeconds int64, skipMissing bool) (*sekaiapi.CloudSpeedResponse, error) {
	return nil, fmt.Errorf("unexpected speed call")
}

func (s *csbRankOwnerTrackerSource) GetCloudSKTrace(server string, eventID int, characterID *int, subjectType string, subject string, limit int) (*sekaiapi.CloudTraceResponse, error) {
	s.mu.Lock()
	s.traceType = append(s.traceType, subjectType+":"+subject)
	s.mu.Unlock()
	switch subjectType {
	case "user":
		if subject != "20051" {
			return nil, fmt.Errorf("unexpected user trace subject: %s", subject)
		}
		return &sekaiapi.CloudTraceResponse{
			Subject: sekaiapi.SubjectTraceMeta{SubjectType: "user", Subject: subject, ResolvedUserID: stringPtrIfNotEmpty(subject)},
			RankData: []sekaiapi.CloudRankInfo{
				{Rank: 53, UserID: stringPtrIfNotEmpty("20051"), Name: "CurrentOwner", Score: 7000000, Timestamp: 1700000000},
				{Rank: 51, UserID: stringPtrIfNotEmpty("20051"), Name: "CurrentOwner", Score: 7654321, Timestamp: 1800000000},
			},
		}, nil
	case "rank":
		return &sekaiapi.CloudTraceResponse{
			Subject: sekaiapi.SubjectTraceMeta{SubjectType: "rank", Subject: subject},
			RankData: []sekaiapi.CloudRankInfo{
				{Rank: 51, UserID: stringPtrIfNotEmpty("10001"), Name: "PreviousOwner", Score: 6500000, Timestamp: 1600000000},
				{Rank: 51, UserID: stringPtrIfNotEmpty("20051"), Name: "CurrentOwner", Score: 7654321, Timestamp: 1800000000},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected subject type: %s", subjectType)
	}
}

func (s *csbRankOwnerTrackerSource) traceCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.traceType...)
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

func (staleCSBTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	now := time.Now().UTC()
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{UserID: "60001", Score: 1_000, Rank: rank, Timestamp: now.Add(-130 * time.Minute).Unix()},
			{UserID: "60001", Score: 2_000, Rank: rank, Timestamp: now.Add(-70 * time.Minute).Unix()},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "60001",
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
	setTestTrackerIntegration(controller, testTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, testTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, lineNameTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, tracker, &testEventSource{
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
	if tracker.traceRankCalls.Load() != 0 {
		t.Fatalf("line request should not call legacy trace, got %d calls", tracker.traceRankCalls.Load())
	}
}

func TestBuildLineRequestFromTrackerUsesCloudV2Line(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	tracker := &batchLineMetricsTrackerSource{}
	controller := NewController(nil)
	setTestTrackerIntegration(controller, tracker, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildLineRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{100, 1},
	})
	if err != nil {
		t.Fatalf("build line request: %v", err)
	}
	if tracker.batchTraceRankCalls.Load() != 0 || tracker.traceRankCalls.Load() != 0 {
		t.Fatalf("expected no legacy trace calls, batch=%d trace=%d", tracker.batchTraceRankCalls.Load(), tracker.traceRankCalls.Load())
	}
	if tracker.latestRankCalls.Load() != 2 {
		t.Fatalf("expected cloud v2 line to read each rank once, got %d", tracker.latestRankCalls.Load())
	}
	if len(payload.Ranks) != 2 || payload.Ranks[0].Rank != 1 || payload.Ranks[1].Rank != 100 {
		t.Fatalf("unexpected sorted ranks: %+v", payload.Ranks)
	}
	if payload.Ranks[0].Score == nil || *payload.Ranks[0].Score != 1_000_001 {
		t.Fatalf("unexpected rank 1 payload: %+v", payload.Ranks[0])
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
	setTestTrackerIntegration(controller, tracker, &testEventSource{
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
	if tracker.traceWorldRankCalls.Load() != 0 {
		t.Fatalf("wl line request should not call legacy trace, got %d calls", tracker.traceWorldRankCalls.Load())
	}
}

func TestBuildWorldBloomLineRequestFromTrackerUsesCloudV2Line(t *testing.T) {
	now := time.Now().UnixMilli()
	eventInfo := &masterdata.Event{
		ID:          101,
		EventType:   "world_bloom",
		Name:        "World Bloom Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(time.Hour/time.Millisecond),
	}
	tracker := &batchLineMetricsTrackerSource{}
	controller := NewController(nil)
	setTestTrackerIntegration(controller, tracker, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	charaID := 21
	payload, err := controller.BuildLineRequestFromTracker(TrackerRankQuery{
		EventID:       eventInfo.ID,
		Region:        "jp",
		Ranks:         []int{100, 1},
		WlCharacterID: &charaID,
	})
	if err != nil {
		t.Fatalf("build wl line request: %v", err)
	}
	if tracker.batchWorldTraceRankCalls.Load() != 0 || tracker.traceWorldRankCalls.Load() != 0 {
		t.Fatalf("expected no legacy world bloom trace calls, batch=%d trace=%d", tracker.batchWorldTraceRankCalls.Load(), tracker.traceWorldRankCalls.Load())
	}
	if tracker.latestWorldRankCalls.Load() != 2 {
		t.Fatalf("expected cloud v2 wl line to read each rank once, got %d", tracker.latestWorldRankCalls.Load())
	}
	if len(payload.Ranks) != 2 || payload.Ranks[0].Rank != 1 || payload.Ranks[1].Rank != 100 {
		t.Fatalf("unexpected sorted wl ranks: %+v", payload.Ranks)
	}
	if payload.Ranks[0].Score == nil || *payload.Ranks[0].Score != 3_000_022 {
		t.Fatalf("unexpected wl rank 1 payload: %+v", payload.Ranks[0])
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
	setTestTrackerIntegration(controller, lineNameTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, worldBloomLineTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, missingDefaultRankLineTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, missingDefaultRankLineTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, rankNameFallbackTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, traceUserIDNameFallbackTrackerSource{}, &testEventSource{
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

func TestBuildQueryRequestFromTrackerUsesCloudV2Query(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	tracker := &batchLineMetricsTrackerSource{}
	controller := NewController(nil)
	setTestTrackerIntegration(controller, tracker, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{100, 1},
	})
	if err != nil {
		t.Fatalf("build query request: %v", err)
	}
	if tracker.latestRankCalls.Load() != 2 {
		t.Fatalf("expected 2 latest rank calls, got %d", tracker.latestRankCalls.Load())
	}
	if tracker.batchTraceRankCalls.Load() != 0 || tracker.traceRankCalls.Load() != 0 {
		t.Fatalf("expected no legacy trace calls, batch=%d trace=%d", tracker.batchTraceRankCalls.Load(), tracker.traceRankCalls.Load())
	}
	if len(payload.Ranks) != 2 || payload.Ranks[0].Rank != 1 || payload.Ranks[1].Rank != 100 {
		t.Fatalf("unexpected query ranks: %+v", payload.Ranks)
	}
}

func TestBuildQueryRequestFromTrackerUsesLeaderboardSnapshots(t *testing.T) {
	controller := NewController(nil)
	tracker := &leaderboardV2TrackerSource{}
	setTestTrackerIntegration(controller, tracker, &testEventSource{
		events: []*masterdata.Event{{ID: 170, Name: "Tracker Event", EventType: "marathon"}},
	}, nil)

	payload, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{
		Region:  "cn",
		EventID: 170,
		Ranks:   []int{2},
	})
	if err != nil {
		t.Fatalf("BuildQueryRequestFromTracker returned error: %v", err)
	}
	if len(payload.Ranks) != 1 || payload.Ranks[0].Name != "V2User" {
		t.Fatalf("unexpected v2 rank payload: %+v", payload.Ranks)
	}
	if payload.PrevRanks == nil || payload.NextRanks == nil {
		t.Fatalf("expected adjacent ranks from v2 snapshot")
	}
	if got := tracker.legacyTraceCalls.Load(); got != 0 {
		t.Fatalf("expected no legacy trace calls, got %d", got)
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
	setTestTrackerIntegration(controller, traceUserIDNameFallbackTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, traceUserIDNameFallbackTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, rankTraceNameMismatchTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, playerTracePrefersUserHistoryTrackerSource{}, &testEventSource{
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

func TestBuildRankTraceFromTrackerUsesLeaderboardSubjectTrace(t *testing.T) {
	controller := NewController(nil)
	tracker := &leaderboardV2TrackerSource{}
	setTestTrackerIntegration(controller, tracker, &testEventSource{
		events: []*masterdata.Event{{ID: 170, Name: "Tracker Event", EventType: "marathon"}},
	}, nil)

	payload, err := controller.BuildRankTraceRequestFromTracker(TrackerRankQuery{
		Region:  "cn",
		EventID: 170,
		Ranks:   []int{1},
	})
	if err != nil {
		t.Fatalf("BuildRankTraceRequestFromTracker returned error: %v", err)
	}
	if len(payload.Ranks) != 2 || payload.Ranks[0].Name != "TraceV2User" {
		t.Fatalf("unexpected v2 trace payload: %+v", payload.Ranks)
	}
	if got := tracker.legacyTraceCalls.Load(); got != 0 {
		t.Fatalf("expected no legacy trace calls, got %d", got)
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
	setTestTrackerIntegration(controller, checkRoomMetricTrackerSource{}, &testEventSource{
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
}

func TestBuildCheckRoomRequestFromTrackerUsesLatestOnlyForAdjacentRanks(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	tracker := &lineMetricsOnlyTrackerSource{}
	controller := NewController(nil)
	setTestTrackerIntegration(controller, tracker, &testEventSource{
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
	if tracker.latestRankCalls.Load() != 4 {
		t.Fatalf("expected cloud v2 check-room query and adjacent reads, got %d", tracker.latestRankCalls.Load())
	}
	if tracker.traceUserCalls.Load() != 1 {
		t.Fatalf("expected only current rank user trace call, got %d", tracker.traceUserCalls.Load())
	}
	if tracker.traceRankCalls.Load() != 0 {
		t.Fatalf("expected no adjacent rank trace calls, got %d", tracker.traceRankCalls.Load())
	}
}

func TestBuildCheckRoomRequestFromTrackerUsesCheckRoomMetricsForMultipleRanks(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	tracker := &lineMetricsOnlyTrackerSource{}
	controller := NewController(nil)
	setTestTrackerIntegration(controller, tracker, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{
		EventID: 101,
		Region:  "jp",
		Ranks:   []int{2, 3},
	})
	if err != nil {
		t.Fatalf("build check-room request: %v", err)
	}
	if len(payload.Ranks) != 2 {
		t.Fatalf("unexpected ranks len: %d", len(payload.Ranks))
	}
	for _, got := range payload.Ranks {
		if got.Speed == nil {
			t.Fatalf("expected check-room speed for rank %d, got %+v", got.Rank, got)
		}
	}
	if tracker.traceUserCalls.Load() != 2 {
		t.Fatalf("expected one user trace call per requested rank, got %d", tracker.traceUserCalls.Load())
	}
	if tracker.traceRankCalls.Load() != 0 {
		t.Fatalf("expected no rank trace calls for check-room metrics, got %d", tracker.traceRankCalls.Load())
	}
	if payload.PrevRank == nil || payload.PrevRank.Rank != 1 || payload.NextRank == nil || payload.NextRank.Rank != 3 {
		t.Fatalf("unexpected adjacent ranks: prev=%+v next=%+v", payload.PrevRank, payload.NextRank)
	}
}

func TestBuildCheckRoomRequestFromCloudV2BackfillsMissingRoundMetrics(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     111,
		AggregateAt: 222,
	}
	tracker := &cloudV2SparseCheckRoomTrackerSource{}
	controller := NewController(nil)
	setTestTrackerIntegration(controller, tracker, &testEventSource{
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
	if got.AverageRound == nil || got.AveragePt == nil || got.LatestPt == nil || got.HourRound == nil || got.RecordStartAt == nil {
		t.Fatalf("expected v2 trace backfill metrics, got %+v", got)
	}
	if tracker.subjectTraceCalls.Load() != 1 {
		t.Fatalf("expected one v2 subject trace call, got %d", tracker.subjectTraceCalls.Load())
	}
	if tracker.legacyTraceCalls.Load() != 0 {
		t.Fatalf("legacy trace should not be called, got %d", tracker.legacyTraceCalls.Load())
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
	setTestTrackerIntegration(controller, checkRoomMetricTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, checkRoomMetricTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, latestUserWithoutRankTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, latestUserWithoutRankTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, eventTitleNameTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, speedFallbackTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, speedFallbackTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, speedFallbackTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, speedTraceOnlyTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, speedWindowTraceTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, speedParkedTraceTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, staleSpeedGrowthTrackerSource{}, &testEventSource{
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
	if got.Speed == nil {
		t.Fatalf("expected stopped speed to be populated, got %+v", got.Speed)
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
	setTestTrackerIntegration(controller, speedFallbackTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, speedFallbackTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, missingDefaultRankSpeedTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, fuzzyEventNameTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, unresolvedEventNameTrackerSource{}, &testEventSource{
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
	if payload.Ranks[0].Name != "残照のInside Direction" {
		t.Fatalf("expected tracker-provided name to pass through, got %+v", payload.Ranks[0])
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
	setTestTrackerIntegration(controller, checkRoomMetricTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, checkRoomOutOfTop100TrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, checkRoomMetricTrackerSource{}, &testEventSource{
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

func TestBuildCSBRequestFromTrackerUsesCurrentRankOwnerTrace(t *testing.T) {
	eventInfo := &masterdata.Event{
		ID:          170,
		Name:        "World Bloom",
		StartAt:     111,
		AggregateAt: 222,
	}
	tracker := &csbRankOwnerTrackerSource{}
	controller := NewController(nil)
	controller.SetTrackerIntegration(tracker, &testEventSource{
		region: renderregion.CN,
		events: []*masterdata.Event{eventInfo},
		byID:   map[int]*masterdata.Event{eventInfo.ID: eventInfo},
	}, nil)

	payload, err := controller.BuildCSBRequestFromTracker(TrackerRankQuery{
		EventID: 170,
		Region:  "cn",
		Ranks:   []int{51},
	})
	if err != nil {
		t.Fatalf("build csb request: %v", err)
	}
	if calls := tracker.traceCalls(); len(calls) != 1 || calls[0] != "user:20051" {
		t.Fatalf("expected current owner user trace only, got %v", calls)
	}
	if len(payload.Ranks) != 2 {
		t.Fatalf("unexpected trace point count: %d", len(payload.Ranks))
	}
	for _, point := range payload.Ranks {
		if point.Name != "CurrentOwner" {
			t.Fatalf("expected current owner trace point, got %+v", point)
		}
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
	setTestTrackerIntegration(controller, staleCSBTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, checkRoomMetricTrackerSource{}, &testEventSource{
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
	tracker := &batchLineMetricsTrackerSource{}
	controller := NewController(nil)
	setTestTrackerIntegration(controller, tracker, &testEventSource{
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
		Ranks:   []int{1500, 100, 50},
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
	if tracker.batchTraceRankCalls.Load() != 0 || tracker.traceRankCalls.Load() != 0 {
		t.Fatalf("expected no legacy trace calls, batch=%d trace=%d", tracker.batchTraceRankCalls.Load(), tracker.traceRankCalls.Load())
	}
	if tracker.latestRankCalls.Load() != 2 {
		t.Fatalf("expected cloud v2 line to read each current rank, got %d", tracker.latestRankCalls.Load())
	}
	if payload.Ranks[0].Rank != 50 || payload.Ranks[0].Score == nil || *payload.Ranks[0].Score != 1_000_050 {
		t.Fatalf("unexpected first current rank payload: %+v", payload.Ranks[0])
	}
	if payload.Ranks[1].Rank != 100 || payload.Ranks[1].Score == nil || *payload.Ranks[1].Score != 1_000_100 {
		t.Fatalf("unexpected second current rank payload: %+v", payload.Ranks[1])
	}
	for _, rank := range payload.Ranks {
		if rank.Rank == 1500 {
			t.Fatalf("forecast payload should omit ranks no source provided: %+v", payload.Ranks)
		}
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
	setTestTrackerIntegration(controller, worldBloomLineTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, worldBloomLineTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, lineNameTrackerSource{}, &testEventSource{
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

func TestBuildPredictLineRequestFromTrackerReportsNoActiveAfterEventEnded(t *testing.T) {
	now := time.Now().UnixMilli()
	eventInfo := &masterdata.Event{
		ID:          101,
		Name:        "Tracker Event",
		StartAt:     now - int64(2*time.Hour/time.Millisecond),
		AggregateAt: now - int64(time.Minute/time.Millisecond),
		ClosedAt:    now + int64(time.Hour/time.Millisecond),
	}
	controller := NewController(nil)
	setTestTrackerIntegration(controller, lineNameTrackerSource{}, &testEventSource{
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
		t.Fatal("expected no-active prediction error, got nil")
	}
	if got := err.Error(); got != skPredictionNoActiveMsg {
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
	setTestTrackerIntegration(controller, worldBloomLineTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, lineNameTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, lineNameTrackerSource{}, &testEventSource{
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
	setTestTrackerIntegration(controller, tracker, &testEventSource{
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
		t.Fatalf("expected tracker cloud v2 line calls to run for each render, got %d", got)
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
	setTestTrackerIntegration(controller, tracker, &testEventSource{
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
