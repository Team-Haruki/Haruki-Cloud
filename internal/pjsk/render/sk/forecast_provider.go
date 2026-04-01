package sk

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	forecast33KitURL       = "https://sekai-data.3-3.dev/predict.json"
	forecastSnowyMoeURL    = "https://rk.exmeaning.com/public/event/%d/latest?region=%s"
	forecastSnowyLegacyURL = "https://sekaibangdan.exmeaning.com/api/public/v1/%sdata/%d"
	forecastSekaURL        = "https://jiiku831.github.io/%sdata/sekarun.js"
)

// ForecastScore represents one forecasted final score for a rank.
type ForecastScore struct {
	Score     int
	Timestamp int64
	Source    string
}

// ForecastSourceData keeps one forecast source's scores and fetch metadata.
type ForecastSourceData struct {
	Scores    map[int]ForecastScore
	FetchedAt int64
}

// ForecastProvider fetches sk forecast scores from external sources.
type ForecastProvider interface {
	Fetch(ctx context.Context, region string, eventID int, ranks []int) (map[int]ForecastScore, error)
}

// ForecastProviderBySource can return forecast scores grouped by data source.
type ForecastProviderBySource interface {
	FetchBySource(ctx context.Context, region string, eventID int, ranks []int) (map[string]ForecastSourceData, error)
}

// RemoteForecastProvider fetches forecast data from public remote sources
// used by Lunabot (33kit / Snowy / SekaRun).
type RemoteForecastProvider struct {
	http *resty.Client
}

// NewRemoteForecastProvider creates a forecast provider with sane HTTP defaults.
func NewRemoteForecastProvider() *RemoteForecastProvider {
	return &RemoteForecastProvider{
		http: resty.New().
			SetTimeout(8 * time.Second).
			SetRetryCount(1),
	}
}

func (p *RemoteForecastProvider) Fetch(ctx context.Context, region string, eventID int, ranks []int) (map[int]ForecastScore, error) {
	bySource, err := p.FetchBySource(ctx, region, eventID, ranks)
	if err != nil {
		return nil, err
	}
	out := make(map[int]ForecastScore)
	for _, data := range bySource {
		for rank, item := range data.Scores {
			existing, ok := out[rank]
			if !ok || item.Score > existing.Score {
				out[rank] = item
			}
		}
	}
	return out, nil
}

func (p *RemoteForecastProvider) FetchBySource(ctx context.Context, region string, eventID int, ranks []int) (map[string]ForecastSourceData, error) {
	if p == nil || p.http == nil {
		return nil, fmt.Errorf("remote forecast provider is not configured")
	}
	normalizedRegion := strings.ToLower(strings.TrimSpace(region))
	if normalizedRegion == "" || eventID <= 0 {
		return nil, fmt.Errorf("invalid forecast params")
	}

	rankFilter := make(map[int]struct{}, len(ranks))
	for _, rank := range ranks {
		if rank > 0 {
			rankFilter[rank] = struct{}{}
		}
	}

	type source struct {
		name string
		fn   func(context.Context, string, int, map[int]struct{}) (map[int]ForecastScore, error)
	}
	sources := []source{
		{name: "33kit", fn: p.fetch33Kit},
		{name: "snowy", fn: p.fetchSnowy},
		{name: "sekarun", fn: p.fetchSekaRun},
	}

	out := make(map[string]ForecastSourceData, len(sources))
	errs := make([]string, 0, len(sources))
	for _, src := range sources {
		items, err := src.fn(ctx, normalizedRegion, eventID, rankFilter)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s=%v", src.name, err))
			continue
		}
		if len(items) == 0 {
			continue
		}
		out[src.name] = ForecastSourceData{
			Scores:    items,
			FetchedAt: time.Now().UTC().Unix(),
		}
	}

	if len(out) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("all forecast sources failed: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

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

func (p *RemoteForecastProvider) fetchSnowy(ctx context.Context, region string, eventID int, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
	if region != "jp" && region != "cn" {
		return nil, nil
	}
	items, rkErr := p.fetchSnowyMoe(ctx, region, eventID, rankFilter)
	if len(items) > 0 {
		return items, nil
	}

	legacyItems, legacyErr := p.fetchSnowyLegacy(ctx, region, eventID, rankFilter)
	if len(legacyItems) > 0 {
		return legacyItems, nil
	}
	if rkErr != nil && legacyErr != nil {
		return nil, fmt.Errorf("rk=%v; legacy=%v", rkErr, legacyErr)
	}
	if rkErr != nil {
		return nil, rkErr
	}
	if legacyErr != nil {
		return nil, legacyErr
	}
	return nil, nil
}

func (p *RemoteForecastProvider) fetchSnowyMoe(ctx context.Context, region string, eventID int, rankFilter map[int]struct{}) (map[int]ForecastScore, error) {
	url := fmt.Sprintf(forecastSnowyMoeURL, eventID, region)

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
			Source:    "snowy",
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
			Source:    "snowy",
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
	start := strings.Index(body, "[[")
	end := strings.LastIndex(body, "]]")
	if start < 0 || end <= start+2 {
		return nil, fmt.Errorf("unexpected sekarun payload")
	}

	targetEvent := strconv.Itoa(eventID)
	rows := strings.Split(body[start+2:end], "], [")
	out := make(map[int]ForecastScore)
	for _, row := range rows {
		values := parseSekaRunRow(row)
		if len(values) < 10 {
			continue
		}
		if values[0] != targetEvent || values[1] != "p" {
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

		lower, errLower := strconv.ParseFloat(values[8], 64)
		upper, errUpper := strconv.ParseFloat(values[9], 64)
		if errLower != nil || errUpper != nil {
			continue
		}
		score := int(math.Round((lower + upper) / 2))
		if score <= 0 {
			continue
		}

		ts, _ := strconv.ParseInt(values[6], 10, 64)
		item := ForecastScore{
			Score:     score,
			Timestamp: normalizeForecastTimestamp(ts),
			Source:    "sekarun",
		}
		existing, ok := out[rank]
		if !ok || item.Score > existing.Score {
			out[rank] = item
		}
	}
	return out, nil
}

func parseSekaRunRow(row string) []string {
	parts := strings.Split(row, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := strings.TrimSpace(part)
		clean = strings.Trim(clean, "[]\"'")
		out = append(out, clean)
	}
	return out
}

func (p *RemoteForecastProvider) getJSON(ctx context.Context, url string, out any) error {
	resp, err := p.http.R().
		SetContext(ctx).
		SetHeader("User-Agent", "HarukiCloud/1.0").
		Get(url)
	if err != nil {
		return err
	}
	if resp.StatusCode() != 200 {
		return fmt.Errorf("http %d", resp.StatusCode())
	}
	if err := json.Unmarshal(resp.Body(), out); err != nil {
		return err
	}
	return nil
}

func (p *RemoteForecastProvider) getText(ctx context.Context, url string) (string, error) {
	resp, err := p.http.R().
		SetContext(ctx).
		SetHeader("User-Agent", "HarukiCloud/1.0").
		Get(url)
	if err != nil {
		return "", err
	}
	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("http %d", resp.StatusCode())
	}
	return string(resp.Body()), nil
}

func asInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0, false
		}
		if i, err := strconv.Atoi(text); err == nil {
			return i, true
		}
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			return int(f), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func asInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0, false
		}
		if i, err := strconv.ParseInt(text, 10, 64); err == nil {
			return i, true
		}
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			return int64(f), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func normalizeForecastTimestamp(ts int64) int64 {
	if ts <= 0 {
		return 0
	}
	if ts > 1_000_000_000_000 {
		return ts / 1000
	}
	return ts
}

func parseForecastRFC3339(raw string) (int64, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return 0, false
	}
	return parsed.Unix(), true
}
