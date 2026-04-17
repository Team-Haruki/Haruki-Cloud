package sk

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	forecast33KitURL       = "https://sekai-data.3-3.dev/predict.json"
	forecastMoesekaiURL    = "https://rk.exmeaning.com/public/event/%d/latest?region=%s"
	forecastSnowyLegacyURL = "https://sekaibangdan.exmeaning.com/api/public/v1/%sdata/%d"
	forecastSekaURL        = "https://jiiku831.github.io/%sdata/sekarun.js"
)

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
		{name: "moesekai", fn: p.fetchMoesekai},
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
			FetchedAt: time.Now().UTC().UnixMilli(),
		}
	}

	if len(out) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("all forecast sources failed: %s", strings.Join(errs, "; "))
	}
	return out, nil
}
