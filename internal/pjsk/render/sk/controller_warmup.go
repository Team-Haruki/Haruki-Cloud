package sk

import (
	"strings"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
)

var defaultPredictWarmupRanks = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	20, 30, 40, 50, 100, 200, 300, 400, 500,
	1000, 1500, 2000, 2500, 3000, 4000, 5000,
	10000, 20000, 30000, 40000, 50000,
	100000, 200000, 300000,
}

func (c *Controller) StartDefaultPredictWarmup() {
	if c == nil || c.predictCache == nil || c.forecast == nil || c.tracker == nil || c.events == nil {
		return
	}

	refreshController := c.withoutRequestContext()
	if refreshController == nil {
		refreshController = c
	}

	seen := make(map[string]struct{})
	for _, source := range c.events.OrderedSources() {
		if source == nil {
			continue
		}
		region := renderregion.WithDefault(source.DefaultRegion()).String()
		region = strings.ToLower(strings.TrimSpace(region))
		if region == "" {
			continue
		}
		if _, ok := seen[region]; ok {
			continue
		}
		seen[region] = struct{}{}

		req := TrackerRankQuery{
			Region:       region,
			Ranks:        append([]int(nil), defaultPredictWarmupRanks...),
			DefaultRanks: true,
		}
		key, err := buildPredictRenderCacheKey(req, 0)
		if err != nil {
			continue
		}
		c.predictCache.StartManaged(key, time.Now(), func() ([]byte, error) {
			return refreshController.renderPredictLineFromTracker(req)
		})
	}
}
