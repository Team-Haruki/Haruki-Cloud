package sk

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (p *RemoteForecastProvider) fetch33KitByQuery(ctx context.Context, query ForecastQuery, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
	query = normalizeForecastQuery(query)
	if query.Scope != ForecastScopeTotal {
		return nil, nil
	}
	region := query.Region
	eventID := query.EventID
	if region != "jp" {
		return nil, nil
	}

	var resp struct {
		Event struct {
			ID int `json:"id"`
		} `json:"event"`
		Data map[string]any `json:"data"`
	}
	if err := p.getJSON(ctx, forecast33KitURL, &resp); err != nil {
		return nil, err
	}
	if resp.Event.ID != eventID {
		return nil, fmt.Errorf("event mismatch: got %d want %d", resp.Event.ID, eventID)
	}

	timestamp := int64(0)
	if ts, ok := asInt64(resp.Data["ts"]); ok {
		timestamp = normalizeForecastTimestamp(ts)
	}

	out := make(map[int]ForecastScore)
	for key, value := range resp.Data {
		rank, score, ok := forecastRankScore(key, value, rankFilter)
		if !ok {
			continue
		}
		out[rank] = ForecastScore{
			Score:     score,
			Timestamp: timestamp,
			Source:    "33kit",
		}
	}
	return out, nil
}

func forecastRankScore(key string, value any, rankFilter map[int]struct{}) (int, int, bool) {
	if key == "ts" {
		return 0, 0, false
	}
	rank, err := strconv.Atoi(strings.TrimSpace(key))
	if err != nil || !forecastRankSelected(rank, rankFilter) {
		return 0, 0, false
	}
	score, ok := asInt(value)
	return rank, score, ok && score > 0
}

func forecastRankSelected(rank int, rankFilter map[int]struct{}) bool {
	if rank <= 0 {
		return false
	}
	if len(rankFilter) == 0 {
		return true
	}
	_, selected := rankFilter[rank]
	return selected
}

func (p *RemoteForecastProvider) fetchMoesekaiByQuery(ctx context.Context, query ForecastQuery, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
	query = normalizeForecastQuery(query)
	if query.Scope != ForecastScopeTotal {
		return nil, nil
	}
	region := query.Region
	eventID := query.EventID
	if region != "jp" && region != "cn" {
		return nil, nil
	}
	items, rkErr := p.fetchMoe(ctx, region, eventID, rankFilter)
	if len(items) > 0 {
		return items, nil
	}

	legacyItems, legacyErr := p.fetchSnowyLegacy(ctx, region, eventID, rankFilter)
	if len(legacyItems) > 0 {
		return legacyItems, nil
	}
	if rkErr != nil && legacyErr != nil {
		return nil, fmt.Errorf("rk=%w; legacy=%w", rkErr, legacyErr)
	}
	if rkErr != nil {
		return nil, rkErr
	}
	if legacyErr != nil {
		return nil, legacyErr
	}
	return nil, nil
}

func (p *RemoteForecastProvider) fetchMoe(ctx context.Context, region string, eventID int, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
	url := fmt.Sprintf(forecastMoesekaiURL, eventID, region)

	var resp struct {
		EventID   int    `json:"event_id"`
		Status    string `json:"status"`
		UpdatedAt string `json:"updated_at"`
		Items     []struct {
			Rank       int  `json:"rank"`
			Score      int  `json:"score"`
			Prediction *int `json:"prediction"`
			IsFinal    bool `json:"is_final"`
		} `json:"items"`
	}
	if err := p.getJSON(ctx, url, &resp); err != nil {
		return nil, err
	}
	if resp.EventID > 0 && resp.EventID != eventID {
		return nil, fmt.Errorf("event mismatch: got %d want %d", resp.EventID, eventID)
	}

	timestamp := int64(0)
	if ts, ok := parseForecastRFC3339(resp.UpdatedAt); ok {
		timestamp = ts
	}

	out := make(map[int]ForecastScore)
	for _, item := range resp.Items {
		rank := item.Rank
		if rank <= 0 {
			continue
		}
		if !forecastRankSelected(rank, rankFilter) {
			continue
		}

		score := forecastMoeScore(item.Prediction, item.IsFinal, item.Score)
		if score <= 0 {
			continue
		}

		out[rank] = ForecastScore{
			Score:     score,
			Timestamp: normalizeForecastTimestamp(timestamp),
			Source:    "moesekai",
		}
	}
	return out, nil
}

func forecastMoeScore(prediction *int, isFinal bool, score int) int {
	if prediction != nil && *prediction > 0 {
		return *prediction
	}
	if isFinal && score > 0 {
		return score
	}
	return 0
}

func (p *RemoteForecastProvider) fetchSnowyLegacy(ctx context.Context, region string, eventID int, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
	regionPrefix := ""
	if region != "cn" {
		regionPrefix = region + "/"
	}
	url := fmt.Sprintf(forecastSnowyLegacyURL, regionPrefix, eventID)

	var resp struct {
		Timestamp any `json:"timestamp"`
		Data      struct {
			Charts []struct {
				Rank           any `json:"Rank"`
				PredictedScore any `json:"PredictedScore"`
			} `json:"charts"`
		} `json:"data"`
	}
	if err := p.getJSON(ctx, url, &resp); err != nil {
		return nil, err
	}

	timestamp := int64(0)
	if ts, ok := asInt64(resp.Timestamp); ok {
		timestamp = normalizeForecastTimestamp(ts)
	}

	out := make(map[int]ForecastScore)
	for _, chart := range resp.Data.Charts {
		rank, ok := asInt(chart.Rank)
		if !ok || rank <= 0 {
			continue
		}
		if !forecastRankSelected(rank, rankFilter) {
			continue
		}
		score, ok := asInt(chart.PredictedScore)
		if !ok || score <= 0 {
			continue
		}
		out[rank] = ForecastScore{
			Score:     score,
			Timestamp: timestamp,
			Source:    "moesekai",
		}
	}
	return out, nil
}

func (p *RemoteForecastProvider) fetchSekaRunByQuery(ctx context.Context, query ForecastQuery, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
	query = normalizeForecastQuery(query)
	if query.Scope != ForecastScopeTotal {
		return nil, nil
	}
	region := query.Region
	eventID := query.EventID
	if region != "en" {
		return nil, nil
	}
	regionPrefix := ""
	if region != "jp" {
		regionPrefix = region + "/"
	}
	url := fmt.Sprintf(forecastSekaURL, regionPrefix)

	body, err := p.getText(ctx, url)
	if err != nil {
		return nil, err
	}
	return parseSekaRunForecast(body, eventID, rankFilter)
}

func (p *RemoteForecastProvider) fetchLocalForecast(ctx context.Context, region string, eventID int, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
	return p.fetchLocalForecastByQuery(ctx, ForecastQuery{
		Region:  region,
		EventID: eventID,
		Scope:   ForecastScopeTotal,
	}, rankFilter)
}

func (p *RemoteForecastProvider) fetchLocalForecastByQuery(ctx context.Context, query ForecastQuery, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
	query = normalizeForecastQuery(query)
	if query.Region == "" || query.EventID <= 0 {
		return nil, fmt.Errorf("invalid local forecast params")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(p.localForecastURL), "/")
	if baseURL == "" {
		return nil, nil
	}
	urls := buildLocalForecastURLs(baseURL, query)
	var lastErr error
	for _, url := range urls {
		items, err := p.fetchLocalForecastURL(ctx, url, query, rankFilter)
		if err == nil {
			return items, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func buildLocalForecastURLs(baseURL string, query ForecastQuery) []string {
	basePath := fmt.Sprintf("%s/prediction/%s", baseURL, query.Region)
	switch normalizeForecastScope(query.Scope) {
	case ForecastScopeChapter:
		return []string{basePath + "/chapter", basePath}
	default:
		return []string{basePath + "/total", basePath}
	}
}

type localForecastResponse struct {
	Region    string              `json:"region"`
	EventID   int                 `json:"event_id"`
	UpdatedAt any                 `json:"updated_at"`
	Scope     string              `json:"leaderboard_scope"`
	Lines     []localForecastLine `json:"lines"`
}

type localForecastLine struct {
	LeaderboardScope string             `json:"leaderboard_scope"`
	CharacterID      any                `json:"character_id"`
	CurrentTimestamp any                `json:"current_timestamp"`
	Rows             []localForecastRow `json:"rows"`
}

type localForecastRow struct {
	Rank             any `json:"rank"`
	Prediction       any `json:"prediction"`
	CurrentTimestamp any `json:"current_timestamp"`
}

func (p *RemoteForecastProvider) fetchLocalForecastURL(ctx context.Context, url string, query ForecastQuery, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
	var resp localForecastResponse
	if err := p.getJSON(ctx, url, &resp); err != nil {
		return nil, err
	}
	if err := validateLocalForecastResponse(resp, query); err != nil {
		return nil, err
	}

	out := make(map[int]ForecastScore)
	targetScope := string(normalizeForecastScope(query.Scope))
	for _, line := range resp.Lines {
		appendLocalForecastLine(out, line, query, targetScope, normalizedForecastTimestamp(resp.UpdatedAt, 0), rankFilter)
	}
	return out, nil
}

func validateLocalForecastResponse(resp localForecastResponse, query ForecastQuery) error {
	if resp.EventID > 0 && resp.EventID != query.EventID {
		return fmt.Errorf("event mismatch: got %d want %d", resp.EventID, query.EventID)
	}
	payloadRegion := strings.ToLower(strings.TrimSpace(resp.Region))
	if payloadRegion != "" && payloadRegion != query.Region {
		return fmt.Errorf("region mismatch: got %s want %s", payloadRegion, query.Region)
	}
	return nil
}

func appendLocalForecastLine(out map[int]ForecastScore, line localForecastLine, query ForecastQuery, targetScope string, payloadTimestamp int64, rankFilter map[int]struct{}) {
	if !localForecastLineSelected(line, query, targetScope) {
		return
	}
	lineTimestamp := normalizedForecastTimestamp(line.CurrentTimestamp, payloadTimestamp)
	for _, row := range line.Rows {
		rank, score, ok := localForecastRowScore(row, lineTimestamp, rankFilter)
		if ok {
			out[rank] = score
		}
	}
}

func localForecastLineSelected(line localForecastLine, query ForecastQuery, targetScope string) bool {
	lineScope := strings.ToLower(strings.TrimSpace(line.LeaderboardScope))
	if lineScope != "" && lineScope != targetScope {
		return false
	}
	if targetScope != string(ForecastScopeChapter) || query.WlCharacterID == nil {
		return true
	}
	characterID, ok := asInt(line.CharacterID)
	return ok && characterID > 0 && characterID == *query.WlCharacterID
}

func localForecastRowScore(row localForecastRow, fallbackTimestamp int64, rankFilter map[int]struct{}) (int, ForecastScore, bool) {
	rank, ok := asInt(row.Rank)
	if !ok || !forecastRankSelected(rank, rankFilter) {
		return 0, ForecastScore{}, false
	}
	score, ok := asInt(row.Prediction)
	if !ok || score <= 0 {
		return 0, ForecastScore{}, false
	}
	return rank, ForecastScore{
		Score:     score,
		Timestamp: normalizedForecastTimestamp(row.CurrentTimestamp, fallbackTimestamp),
		Source:    "local",
	}, true
}

func normalizedForecastTimestamp(value any, fallback int64) int64 {
	timestamp, ok := asInt64(value)
	if !ok {
		return fallback
	}
	return normalizeForecastTimestamp(timestamp)
}

func parseSekaRunForecast(body string, eventID int, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
	currentEvent, rows, err := extractSekaRunRows(body)
	if err != nil {
		return nil, err
	}

	accumulator := newSekaRunForecastAccumulator(eventID, rankFilter)
	for _, row := range rows {
		accumulator.addRow(row)
	}
	return accumulator.result(currentEvent)
}

type sekaRunForecastAccumulator struct {
	targetEvent      string
	rankFilter       map[int]struct{}
	currentScores    map[int]ForecastScore
	historicalScores map[int]ForecastScore
	matchedEvent     bool
}

func newSekaRunForecastAccumulator(eventID int, rankFilter map[int]struct{}) *sekaRunForecastAccumulator {
	return &sekaRunForecastAccumulator{
		targetEvent:      strconv.Itoa(eventID),
		rankFilter:       rankFilter,
		currentScores:    make(map[int]ForecastScore),
		historicalScores: make(map[int]ForecastScore),
	}
}

func (a *sekaRunForecastAccumulator) addRow(row string) {
	values := parseSekaRunRow(row)
	if len(values) < 10 || values[0] != a.targetEvent {
		return
	}
	a.matchedEvent = true
	targetScores, ok := a.targetScores(values[1])
	if !ok {
		return
	}
	rank, err := strconv.Atoi(values[5])
	if err != nil || !forecastRankSelected(rank, a.rankFilter) {
		return
	}
	score, ok := parseSekaRunScore(values)
	if !ok {
		return
	}
	timestamp, _ := strconv.ParseInt(values[6], 10, 64)
	a.storeHigherScore(targetScores, rank, ForecastScore{
		Score:     score,
		Timestamp: normalizeForecastTimestamp(timestamp),
		Source:    "sekarun",
	})
}

func (a *sekaRunForecastAccumulator) targetScores(rowType string) (map[int]ForecastScore, bool) {
	switch rowType {
	case "p":
		return a.currentScores, true
	case "h":
		return a.historicalScores, true
	default:
		return nil, false
	}
}

func (a *sekaRunForecastAccumulator) storeHigherScore(scores map[int]ForecastScore, rank int, item ForecastScore) {
	existing, ok := scores[rank]
	if !ok || item.Score > existing.Score {
		scores[rank] = item
	}
}

func (a *sekaRunForecastAccumulator) result(currentEvent string) (map[int]ForecastScore, error) {
	for rank, item := range a.historicalScores {
		if _, ok := a.currentScores[rank]; !ok {
			a.currentScores[rank] = item
		}
	}
	if len(a.currentScores) > 0 {
		return a.currentScores, nil
	}
	if !a.matchedEvent && currentEvent != "" && currentEvent != a.targetEvent {
		return nil, fmt.Errorf("event mismatch: got %s want %s", currentEvent, a.targetEvent)
	}
	return nil, nil
}
