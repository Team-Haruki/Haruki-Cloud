package sk

import (
	"errors"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/utils/usererror"
)

func (c *Controller) BuildLineRequestFromTracker(req TrackerRankQuery) (*LineRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	defer finishBuild()
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	skipMissing := shouldSkipMissingTrackerRanks(normalized)
	rankInfos, err := c.buildLineRanksOrUserFromTracker(normalized.Region, normalized.EventID, normalized.Ranks, normalized.UserID, normalized.WlCharacterID, skipMissing)
	if err != nil {
		return nil, err
	}
	// SK line focuses on score borders; omit player names to keep output compact.
	for i := range rankInfos {
		rankInfos[i].Name = ""
	}
	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)
	line := LineRequest{
		ID:            normalized.EventID,
		Region:        normalized.Region,
		StartAt:       meta.startAt,
		AggregateAt:   meta.aggregateAt,
		Name:          meta.name,
		BannerImgPath: meta.bannerPath,
		Ranks:         rankInfos,
		Full:          normalized.Full,
	}
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		wl := *normalized.WlCharacterID
		line.WlCid = &wl
		if icon := c.resolveCharacterIconPath(wl, renderregion.Normalize(normalized.Region)); icon != "" {
			line.CharaIconPath = &icon
		}
	}
	return c.BuildLineRequest(line)
}

// BuildPredictLineRequestFromTracker builds an SK line payload using external
// forecast sources (33kit / Moesekai / SekaRun / local) for final score prediction.
func (c *Controller) BuildPredictLineRequestFromTracker(req TrackerRankQuery) (*LineRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	defer finishBuild()
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	if normalized.UserID != nil {
		return nil, usererror.Misuse(i18n.M("sk.predict.user_unsupported"))
	}
	if c.forecastCache == nil {
		return nil, usererror.Misconfigured(errors.New("forecast cache is not configured"))
	}

	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)
	meta = c.applyWorldBloomChapterMeta(normalized, meta)
	if err := ensureSKPredictionAllowed(meta); err != nil {
		return nil, err
	}
	forecastQuery := buildForecastQuery(normalized)
	bySource, forecastErr := c.forecastCache.CachedBySourceQuery(forecastQuery)
	if forecastErr != nil {
		c.forecastCache.StartRefreshQuery(forecastQuery)
		return nil, usererror.Wrap(usererror.CodeUnavailable, i18n.M("sk.predict.not_ready"), forecastErr)
	}

	sourceOrder := forecastSourceDisplayOrder(normalized.Region, bySource)
	forecastRanks := forecastProvidedRanks(bySource)
	if len(forecastRanks) == 0 {
		c.forecastCache.StartRefreshQuery(forecastQuery)
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("sk.predict.no_tiers"))
	}
	columns := buildForecastColumns(sourceOrder, bySource, forecastRanks)
	if len(columns) == 0 {
		c.forecastCache.StartRefreshQuery(forecastQuery)
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("sk.predict.no_tiers"))
	}

	currentRanks := c.buildCurrentForecastRanks(normalized, forecastRanks)

	line := LineRequest{
		ID:               normalized.EventID,
		Region:           normalized.Region,
		StartAt:          meta.startAt,
		AggregateAt:      meta.aggregateAt,
		Name:             strings.TrimSpace(i18n.T("render_sk.predict.title", i18n.Data{"Event": meta.name})),
		BannerImgPath:    meta.bannerPath,
		Ranks:            currentRanks,
		CurrentRanks:     currentRanks,
		ForecastColumns:  columns,
		PredictionNotice: i18n.T("render_sk.predict.notice"),
		Full:             normalized.Full,
	}
	c.applyForecastWorldBloomFields(&line, normalized)
	return c.BuildLineRequest(line)
}

func buildForecastColumns(sourceOrder []string, bySource map[string]ForecastSourceData, ranks []int) []drawing.SKForecastColumn {
	columns := make([]drawing.SKForecastColumn, 0, len(sourceOrder))
	for _, sourceKey := range sourceOrder {
		column, ok := buildForecastColumn(sourceKey, bySource[sourceKey], ranks)
		if ok {
			columns = append(columns, column)
		}
	}
	return columns
}

func buildForecastColumn(sourceKey string, sourceData ForecastSourceData, ranks []int) (drawing.SKForecastColumn, bool) {
	if len(sourceData.Scores) == 0 {
		return drawing.SKForecastColumn{}, false
	}
	rankInfos := make([]drawing.RankInfo, 0, len(ranks))
	forecastAt := int64(0)
	for _, rank := range ranks {
		item, ok := sourceData.Scores[rank]
		if !ok || item.Score <= 0 {
			continue
		}
		forecastAt = max(forecastAt, item.Timestamp)
		rankInfos = append(rankInfos, drawing.RankInfo{
			Rank: rank, Score: drawing.IntPtr(item.Score), Time: formatTrackerTimestamp(item.Timestamp),
		})
	}
	if len(rankInfos) == 0 {
		return drawing.SKForecastColumn{}, false
	}
	sort.Slice(rankInfos, func(i, j int) bool { return rankInfos[i].Rank < rankInfos[j].Rank })
	column := drawing.SKForecastColumn{Key: sourceKey, Name: forecastSourceName(sourceKey), Ranks: rankInfos}
	if forecastAt > 0 {
		column.ForecastTime = drawing.Int64Ptr(formatTrackerTimestamp(forecastAt))
	}
	if sourceData.FetchedAt > 0 {
		column.UpdateTime = drawing.Int64Ptr(formatTrackerTimestamp(sourceData.FetchedAt))
	}
	return column, true
}

func forecastSourceName(sourceKey string) string {
	if name, ok := forecastSourceNames[sourceKey]; ok {
		return name.String()
	}
	return sourceKey
}

var forecastSourceNames = map[string]i18n.Message{
	"33kit":    i18n.M("render_sk.predict.source.kit33"),
	"moesekai": i18n.M("render_sk.predict.source.moesekai"),
	"sekarun":  i18n.M("render_sk.predict.source.sekarun"),
	"local":    i18n.M("render_sk.predict.source.local"),
	"forecast": i18n.M("render_sk.predict.source.generic"),
}

func (c *Controller) buildCurrentForecastRanks(req TrackerRankQuery, ranks []int) []drawing.RankInfo {
	currentRanks, err := c.buildRanksFromTracker(
		req.Region, req.EventID, ranks, req.WlCharacterID, shouldSkipMissingTrackerRanks(req),
	)
	if err != nil {
		return nil
	}
	for i := range currentRanks {
		currentRanks[i].Name = ""
	}
	sort.Slice(currentRanks, func(i, j int) bool { return currentRanks[i].Rank < currentRanks[j].Rank })
	return currentRanks
}

func (c *Controller) applyForecastWorldBloomFields(line *LineRequest, req TrackerRankQuery) {
	if req.WlCharacterID == nil || *req.WlCharacterID <= 0 {
		return
	}
	worldBloomCharacterID := *req.WlCharacterID
	line.WlCid = &worldBloomCharacterID
	if icon := c.resolveCharacterIconPath(worldBloomCharacterID, renderregion.Normalize(req.Region)); icon != "" {
		line.CharaIconPath = &icon
	}
}

func (c *Controller) RenderPredictLineFromTracker(req TrackerRankQuery) ([]byte, error) {
	image, err := c.RenderPredictLineFromTrackerImage(req)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.contextOrBackground())
}

func (c *Controller) RenderPredictLineFromTrackerImage(req TrackerRankQuery) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	return c.renderPredictLineFromTrackerImage(req)
}

func (c *Controller) renderPredictLineFromTrackerImage(req TrackerRankQuery) (drawing.ImageResult, error) {
	payload, err := c.BuildPredictLineRequestFromTracker(req)
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.RenderLineImage(*payload)
}

func buildForecastQuery(req TrackerRankQuery) ForecastQuery {
	query := ForecastQuery{
		Region:  req.Region,
		EventID: req.EventID,
		Ranks:   req.Ranks,
		Scope:   ForecastScopeTotal,
	}
	if req.WlCharacterID != nil && *req.WlCharacterID > 0 {
		query.Scope = ForecastScopeChapter
		query.WlCharacterID = req.WlCharacterID
	}
	return query
}

func forecastProvidedRanks(bySource map[string]ForecastSourceData) []int {
	if len(bySource) == 0 {
		return nil
	}
	seen := make(map[int]struct{})
	for _, sourceData := range bySource {
		for rank, score := range sourceData.Scores {
			if rank <= 0 || score.Score <= 0 {
				continue
			}
			seen[rank] = struct{}{}
		}
	}
	ranks := make([]int, 0, len(seen))
	for rank := range seen {
		ranks = append(ranks, rank)
	}
	sort.Ints(ranks)
	return ranks
}

func ensureSKPredictionAllowed(meta eventMeta) error {
	if meta.aggregateAt <= 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	if now >= meta.aggregateAt {
		return usererror.New(usererror.CodeNotFound, i18n.M("sk.no_ongoing_event"))
	}
	stopAt := meta.aggregateAt - int64(time.Hour/time.Millisecond)
	if now >= stopAt {
		return usererror.Forbidden(i18n.M("sk.predict.stopped"))
	}
	return nil
}
