package sk

import (
	"context"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/eventutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/storage"
)

var defaultPredictWarmupRanks = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	20, 30, 40, 50, 100, 200, 300, 400, 500,
	1000, 1500, 2000, 2500, 3000, 4000, 5000,
	10000, 20000, 30000, 40000, 50000,
	100000, 200000, 300000,
}

func (c *Controller) StartDefaultPredictWarmup() {
	c.StartDefaultPredictWarmupContext(c.contextOrBackground())
}

func (c *Controller) StartDefaultPredictWarmupContext(ctx context.Context) {
	if c == nil || c.forecastCache == nil || c.events == nil {
		return
	}

	if ctx == nil {
		ctx = c.contextOrBackground()
	}
	if ctx.Err() != nil {
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

	lifecycleCtx, finish, err := c.forecastCache.refreshTasks.Start()
	if err != nil {
		return
	}
	warmupCtx, cancel := context.WithCancel(storage.WithBackgroundIO(lifecycleCtx))
	stop := context.AfterFunc(ctx, cancel)
	if ctx.Err() != nil {
		cancel()
	}
	bound := c.WithContext(warmupCtx)
	go func() {
		defer finish()
		defer stop()
		defer cancel()
		ticker := time.NewTicker(forecastDataRefreshInterval)
		defer ticker.Stop()
		bound.runDefaultPredictWarmup(regions, ticker.C)
	}()
}

func (c *Controller) runDefaultPredictWarmup(regions []string, ticks <-chan time.Time) {
	ctx := c.contextOrBackground()
	c.refreshDefaultPredictData(regions)
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok {
				return
			}
			c.refreshDefaultPredictData(regions)
		}
	}
}

func (c *Controller) refreshDefaultPredictData(regions []string) {
	ctx := c.contextOrBackground()
	for _, region := range regions {
		if ctx.Err() != nil {
			return
		}
		eventID := c.pickCurrentOrNextEventID(region)
		if ctx.Err() != nil {
			return
		}
		if eventID <= 0 {
			continue
		}
		meta := c.resolveEventMeta(eventID, renderregion.Normalize(region))
		if ctx.Err() != nil {
			return
		}
		if ensureSKPredictionAllowed(meta) == nil {
			c.forecastCache.startRefreshQuery(ctx, ForecastQuery{Region: region, EventID: eventID, Scope: ForecastScopeTotal})
		}
		c.refreshCurrentWorldBloomChapterPredictData(region, eventID)
	}
}

func (c *Controller) refreshCurrentWorldBloomChapterPredictData(region string, eventID int) {
	ctx := c.contextOrBackground()
	if ctx.Err() != nil {
		return
	}
	eventSource := c.eventSourceForRegion(region)
	chapterSource, ok := eventSource.(WorldBloomChapterSource)
	if !ok || chapterSource == nil {
		return
	}
	now := time.Now().UnixMilli()
	var characterID int
	var chapterStartAt int64
	var chapterAggregateAt int64
	for _, chapter := range chapterSource.GetWorldBloomChapters(ctx, eventID) {
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
	if ctx.Err() != nil || characterID <= 0 {
		return
	}
	if ensureSKPredictionAllowed(eventMeta{aggregateAt: chapterAggregateAt}) != nil {
		return
	}
	c.forecastCache.startRefreshQuery(ctx, ForecastQuery{
		Region:        region,
		EventID:       eventID,
		Scope:         ForecastScopeChapter,
		WlCharacterID: &characterID,
	})
}
