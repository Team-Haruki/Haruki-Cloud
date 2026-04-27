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
	if c == nil || c.forecastCache == nil || c.events == nil {
		return
	}

	seen := make(map[string]struct{})
	regions := make([]string, 0)
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
		if len(forecastSourceOrderForRegion(region)) == 0 {
			continue
		}
		seen[region] = struct{}{}
		regions = append(regions, region)
	}
	if len(regions) == 0 {
		return
	}

	go c.runDefaultPredictWarmup(regions)
}

func (c *Controller) runDefaultPredictWarmup(regions []string) {
	c.refreshDefaultPredictData(regions)

	ticker := time.NewTicker(forecastDataRefreshInterval)
	defer ticker.Stop()
	for range ticker.C {
		c.refreshDefaultPredictData(regions)
	}
}

func (c *Controller) refreshDefaultPredictData(regions []string) {
	for _, region := range regions {
		eventID := c.pickCurrentOrNextEventID(region)
		if eventID <= 0 {
			continue
		}
		c.forecastCache.StartRefresh(region, eventID)
	}
}
