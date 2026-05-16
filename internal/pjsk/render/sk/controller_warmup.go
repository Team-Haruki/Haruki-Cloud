package sk

import (
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/eventutil"
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
		meta := c.resolveEventMeta(eventID, renderregion.Normalize(region))
		if ensureSKPredictionAllowed(meta) == nil {
			c.forecastCache.StartRefresh(region, eventID)
		}
		c.refreshCurrentWorldBloomChapterPredictData(region, eventID)
	}
}

func (c *Controller) refreshCurrentWorldBloomChapterPredictData(region string, eventID int) {
	eventSource := c.eventSourceForRegion(region)
	chapterSource, ok := eventSource.(WorldBloomChapterSource)
	if !ok || chapterSource == nil {
		return
	}
	now := time.Now().UnixMilli()
	var characterID int
	var chapterStartAt int64
	var chapterAggregateAt int64
	for _, chapter := range chapterSource.GetWorldBloomChapters(c.contextOrBackground(), eventID) {
		if chapter == nil || chapter.GameCharacterID == nil || *chapter.GameCharacterID <= 0 {
			continue
		}
		if !eventutil.IsRankingOpen(chapter.ChapterStartAt, chapter.AggregateAt, now) {
			continue
		}
		if characterID > 0 && chapter.ChapterStartAt <= chapterStartAt {
			continue
		}
		characterID = *chapter.GameCharacterID
		chapterStartAt = chapter.ChapterStartAt
		chapterAggregateAt = chapter.AggregateAt
	}
	if characterID <= 0 {
		return
	}
	if ensureSKPredictionAllowed(eventMeta{aggregateAt: chapterAggregateAt}) != nil {
		return
	}
	c.forecastCache.StartRefreshQuery(ForecastQuery{
		Region:        region,
		EventID:       eventID,
		Scope:         ForecastScopeChapter,
		WlCharacterID: &characterID,
	})
}
