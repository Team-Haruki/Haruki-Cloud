package sk

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (p *RemoteForecastProvider) fetch33Kit(ctx context.Context, region string, eventID int, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
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
		if key == "ts" {
			continue
		}
		rank, err := strconv.Atoi(strings.TrimSpace(key))
		if err != nil || rank <= 0 {
			continue
		}
		if len(rankFilter) > 0 {
			if _, ok := rankFilter[rank]; !ok {
				continue
			}
		}
		score, ok := asInt(value)
		if !ok || score <= 0 {
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

func (p *RemoteForecastProvider) fetchMoesekai(ctx context.Context, region string, eventID int, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
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
		if len(rankFilter) > 0 {
			if _, ok := rankFilter[rank]; !ok {
				continue
			}
		}

		score := 0
		if item.Prediction != nil && *item.Prediction > 0 {
			score = *item.Prediction
		} else if item.IsFinal && item.Score > 0 {
			score = item.Score
		}
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
		if len(rankFilter) > 0 {
			if _, ok := rankFilter[rank]; !ok {
				continue
			}
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

func (p *RemoteForecastProvider) fetchSekaRun(ctx context.Context, region string, eventID int, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
	if region != "jp" && region != "en" && region != "tw" && region != "kr" {
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

func parseSekaRunForecast(body string, eventID int, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
	currentEvent, rows, err := extractSekaRunRows(body)
	if err != nil {
		return nil, err
	}

	targetEvent := strconv.Itoa(eventID)
	currentScores := make(map[int]ForecastScore)
	historicalScores := make(map[int]ForecastScore)
	matchedEvent := false
	for _, row := range rows {
		values := parseSekaRunRow(row)
		if len(values) < 10 {
			continue
		}
		if values[0] != targetEvent {
			continue
		}
		matchedEvent = true

		targetScores := currentScores
		switch values[1] {
		case "p":
		case "h":
			targetScores = historicalScores
		default:
			continue
		}

		rank, err := strconv.Atoi(values[5])
		if err != nil || rank <= 0 {
			continue
		}
		if len(rankFilter) > 0 {
			if _, ok := rankFilter[rank]; !ok {
				continue
			}
		}

		score, ok := parseSekaRunScore(values)
		if !ok {
			continue
		}

		ts, _ := strconv.ParseInt(values[6], 10, 64)
		item := ForecastScore{
			Score:     score,
			Timestamp: normalizeForecastTimestamp(ts),
			Source:    "sekarun",
		}
		existing, ok := targetScores[rank]
		if !ok || item.Score > existing.Score {
			targetScores[rank] = item
		}
	}

	for rank, item := range historicalScores {
		if _, ok := currentScores[rank]; !ok {
			currentScores[rank] = item
		}
	}
	if len(currentScores) > 0 {
		return currentScores, nil
	}
	if !matchedEvent && currentEvent != "" && currentEvent != targetEvent {
		return nil, fmt.Errorf("event mismatch: got %s want %s", currentEvent, targetEvent)
	}
	return nil, nil
}
