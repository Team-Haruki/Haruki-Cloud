package sk

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"haruki-cloud/config"

	"github.com/go-resty/resty/v2"
)

const (
	forecast33KitURL         = "https://sekai-data.3-3.dev/predict.json"
	forecastMoesekaiURL      = "https://rk.exmeaning.com/public/event/%d/latest?region=%s"
	forecastSnowyLegacyURL   = "https://sekaibangdan.exmeaning.com/api/public/v1/%sdata/%d"
	forecastSekaURL          = "https://jiiku831.github.io/%sdata/sekarun.js"
	forecastLocalBaseURL     = "http://100.109.13.111:18746"
	forecastMaxResponseBytes = 16 << 20
)

// NewRemoteForecastProvider creates a forecast provider with sane HTTP defaults.
func NewRemoteForecastProvider() *RemoteForecastProvider {
	return NewRemoteForecastProviderWithConfig(ForecastConfig{})
}

func NewRemoteForecastProviderWithConfig(cfg ForecastConfig) *RemoteForecastProvider {
	localBaseURL := strings.TrimRight(strings.TrimSpace(cfg.LocalBaseURL), "/")
	if localBaseURL == "" {
		localBaseURL = forecastLocalBaseURL
	}
	return &RemoteForecastProvider{
		http: resty.New().
			SetTimeout(config.SKForecastHTTPClientTimeout).
			SetResponseBodyLimit(forecastMaxResponseBytes).
			SetRetryCount(2),
		localForecastURL: localBaseURL,
	}
}

func (p *RemoteForecastProvider) Fetch(ctx context.Context, region string, eventID int, ranks []int) (map[int]ForecastScore, error) {
	return p.FetchQuery(ctx, ForecastQuery{
		Region:  region,
		EventID: eventID,
		Ranks:   ranks,
		Scope:   ForecastScopeTotal,
	})
}

func (p *RemoteForecastProvider) FetchQuery(ctx context.Context, query ForecastQuery) (map[int]ForecastScore, error) {
	bySource, err := p.FetchBySourceQuery(ctx, query)
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
	return p.FetchBySourceQuery(ctx, ForecastQuery{
		Region:  region,
		EventID: eventID,
		Ranks:   ranks,
		Scope:   ForecastScopeTotal,
	})
}

func (p *RemoteForecastProvider) FetchBySourceQuery(ctx context.Context, query ForecastQuery) (map[string]ForecastSourceData, error) {
	if p == nil || p.http == nil {
		return nil, fmt.Errorf("remote forecast provider is not configured")
	}
	normalizedQuery := normalizeForecastQuery(query)
	if normalizedQuery.Region == "" || normalizedQuery.EventID <= 0 {
		return nil, fmt.Errorf("invalid forecast params")
	}

	rankFilter := make(map[int]struct{}, len(normalizedQuery.Ranks))
	for _, rank := range normalizedQuery.Ranks {
		if rank > 0 {
			rankFilter[rank] = struct{}{}
		}
	}

	sources := p.sourcesForQuery(normalizedQuery)
	if len(sources) == 0 {
		return nil, fmt.Errorf("no forecast source supports region %s", normalizedQuery.Region)
	}

	out := make(map[string]ForecastSourceData, len(sources))
	errs := make([]string, 0, len(sources))
	for _, src := range sources {
		items, err := src.fn(ctx, normalizedQuery, rankFilter)
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

type remoteForecastSource struct {
	name string
	fn   func(context.Context, ForecastQuery, map[int]struct{}) (map[int]ForecastScore, error)
}

func (p *RemoteForecastProvider) sourcesForRegion(region string) []remoteForecastSource {
	return p.sourcesForQuery(ForecastQuery{Region: region, Scope: ForecastScopeTotal})
}

func (p *RemoteForecastProvider) sourcesForQuery(query ForecastQuery) []remoteForecastSource {
	region := strings.ToLower(strings.TrimSpace(query.Region))
	scope := normalizeForecastScope(query.Scope)
	if scope == ForecastScopeChapter {
		switch region {
		case "jp", "cn", "en", "tw", "kr":
			return []remoteForecastSource{
				{name: "local", fn: p.fetchLocalForecastByQuery},
			}
		default:
			return nil
		}
	}

	switch region {
	case "jp":
		return []remoteForecastSource{
			{name: "33kit", fn: p.fetch33KitByQuery},
			{name: "moesekai", fn: p.fetchMoesekaiByQuery},
			{name: "local", fn: p.fetchLocalForecastByQuery},
		}
	case "cn":
		return []remoteForecastSource{
			{name: "moesekai", fn: p.fetchMoesekaiByQuery},
			{name: "local", fn: p.fetchLocalForecastByQuery},
		}
	case "en":
		return []remoteForecastSource{
			{name: "sekarun", fn: p.fetchSekaRunByQuery},
			{name: "local", fn: p.fetchLocalForecastByQuery},
		}
	case "tw", "kr":
		return []remoteForecastSource{
			{name: "local", fn: p.fetchLocalForecastByQuery},
		}
	default:
		return nil
	}
}

func normalizeForecastQuery(query ForecastQuery) ForecastQuery {
	normalized := query
	normalized.Region = strings.ToLower(strings.TrimSpace(query.Region))
	normalized.Ranks = normalizeRanks(query.Ranks)
	normalized.Scope = normalizeForecastScope(query.Scope)
	if normalized.Scope == ForecastScopeTotal {
		normalized.WlCharacterID = nil
	}
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID <= 0 {
		normalized.WlCharacterID = nil
	}
	return normalized
}

func normalizeForecastScope(scope ForecastScope) ForecastScope {
	switch strings.ToLower(strings.TrimSpace(string(scope))) {
	case string(ForecastScopeChapter):
		return ForecastScopeChapter
	default:
		return ForecastScopeTotal
	}
}

func forecastSourceOrderForRegion(region string) []string {
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "jp":
		return []string{"33kit", "moesekai", "local"}
	case "cn":
		return []string{"moesekai", "local"}
	case "en":
		return []string{"sekarun", "local"}
	case "tw", "kr":
		return []string{"local"}
	default:
		return nil
	}
}

func forecastSourceDisplayOrder(region string, data map[string]ForecastSourceData) []string {
	if len(data) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(data))
	out := make([]string, 0, len(data))
	for _, source := range forecastSourceOrderForRegion(region) {
		if _, ok := data[source]; !ok {
			continue
		}
		seen[source] = struct{}{}
		out = append(out, source)
	}

	extras := make([]string, 0, len(data))
	for source := range data {
		if _, ok := seen[source]; ok {
			continue
		}
		extras = append(extras, source)
	}
	sort.Strings(extras)
	return append(out, extras...)
}
