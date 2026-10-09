package sk

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/utils/usererror"
)

const skCheckRoomRankLimit = 100

var queryAdjacentRanksNormal = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	20, 30, 40, 50, 100, 200, 300, 400, 500,
	1000, 1500, 2000, 2500, 3000, 4000, 5000,
	10000, 20000, 30000, 40000, 50000,
	100000, 200000, 300000,
}

var queryAdjacentRanksWorldLink = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	20, 30, 40, 50, 100, 200, 300, 400, 500,
	1000, 2000, 3000, 4000, 5000, 7000,
	10000, 20000, 30000, 40000, 50000, 70000, 100000,
}

func (c *Controller) BuildQueryRequest(req drawing.SKRequest) (*drawing.SKRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, usererror.Misuse(i18n.M("sk.target_required"))
	}
	return &req, nil
}

func (c *Controller) RenderQuery(req drawing.SKRequest) ([]byte, error) {
	image, err := c.RenderQueryImage(req)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.contextOrBackground())
}

func (c *Controller) RenderQueryImage(req drawing.SKRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), payloadBuildStage)
	payload, err := c.BuildQueryRequest(req)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateSKQueryImage(payload)
}

func (c *Controller) BuildQueryRequestFromTracker(req TrackerRankQuery) (*drawing.SKRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), payloadBuildStage)
	defer finishBuild()
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	ranks, err := c.resolveQueryRanks(normalized)
	if err != nil {
		return nil, err
	}
	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)
	payload := drawing.SKRequest{
		ID:          normalized.EventID,
		Region:      normalized.Region,
		Name:        meta.name,
		AggregateAt: meta.aggregateAt,
		Ranks:       ranks.infos,
	}
	c.populateQueryAdjacentRanks(&payload, normalized, ranks)
	c.populateQueryCharacterIcon(&payload, normalized)
	return c.BuildQueryRequest(payload)
}

type trackerQueryRanks struct {
	infos    []drawing.RankInfo
	previous *drawing.RankInfo
	next     *drawing.RankInfo
}

func (c *Controller) resolveQueryRanks(query TrackerRankQuery) (trackerQueryRanks, error) {
	if query.UserID != nil && *query.UserID > 0 {
		return c.resolveUserQueryRanks(query)
	}
	return c.resolveListedQueryRanks(query)
}

func (c *Controller) resolveUserQueryRanks(query TrackerRankQuery) (trackerQueryRanks, error) {
	infos, previous, next, ok, err := c.buildUserQueryFromTrackerV2(query.Region, query.EventID, *query.UserID, query.WlCharacterID, true)
	if ok {
		return trackerQueryRanks{infos: infos, previous: previous, next: next}, err
	}
	infos, err = c.buildRanksOrUserFromTracker(query.Region, query.EventID, query.Ranks, query.UserID, query.WlCharacterID, shouldSkipMissingTrackerRanks(query))
	return trackerQueryRanks{infos: infos}, err
}

func (c *Controller) resolveListedQueryRanks(query TrackerRankQuery) (trackerQueryRanks, error) {
	skipMissing := shouldSkipMissingTrackerRanks(query)
	infos, previous, next, ok, err := c.buildRanksFromTrackerV2(query.Region, query.EventID, query.Ranks, query.WlCharacterID, true, skipMissing)
	if ok {
		return trackerQueryRanks{infos: infos, previous: previous, next: next}, err
	}
	infos, err = c.buildRanksOrUserFromTracker(query.Region, query.EventID, query.Ranks, query.UserID, query.WlCharacterID, skipMissing)
	return trackerQueryRanks{infos: infos}, err
}

func (c *Controller) populateQueryAdjacentRanks(payload *drawing.SKRequest, query TrackerRankQuery, ranks trackerQueryRanks) {
	if len(ranks.infos) != 1 {
		return
	}
	if ranks.previous != nil || ranks.next != nil {
		payload.PrevRanks = ranks.previous
		payload.NextRanks = ranks.next
		return
	}
	previousRank, nextRank, hasPrevious, hasNext := queryAdjacentSKLineRanks(ranks.infos[0].Rank, query.WlCharacterID != nil)
	if hasPrevious {
		payload.PrevRanks = c.tryBuildSingleRank(query, previousRank)
	}
	if hasNext {
		payload.NextRanks = c.tryBuildSingleRank(query, nextRank)
	}
}

func (c *Controller) tryBuildSingleRank(query TrackerRankQuery, rank int) *drawing.RankInfo {
	info, err := c.buildSingleRankLatestFromTracker(query.Region, query.EventID, rank, query.WlCharacterID)
	if err != nil {
		return nil
	}
	return &info
}

func (c *Controller) populateQueryCharacterIcon(payload *drawing.SKRequest, query TrackerRankQuery) {
	if query.WlCharacterID == nil || *query.WlCharacterID <= 0 {
		return
	}
	icon := c.resolveCharacterIconPath(*query.WlCharacterID, renderregion.Normalize(query.Region))
	if icon != "" {
		payload.WlCharaIconPath = &icon
		payload.CharaIconPath = &icon
	}
}

func queryAdjacentSKLineRanks(rank int, wlMode bool) (prev int, next int, hasPrev bool, hasNext bool) {
	if rank <= 0 {
		return 0, 0, false, false
	}
	nodes := queryAdjacentRanksNormal
	if wlMode {
		nodes = queryAdjacentRanksWorldLink
	}
	index := sort.SearchInts(nodes, rank)
	if index > 0 {
		prev = nodes[index-1]
		hasPrev = true
	}
	if index >= len(nodes) {
		return prev, 0, hasPrev, false
	}
	if nodes[index] > rank {
		return prev, nodes[index], hasPrev, true
	}
	if index+1 < len(nodes) {
		return prev, nodes[index+1], hasPrev, true
	}
	return prev, 0, hasPrev, false
}

func (c *Controller) BuildCheckRoomRequest(req drawing.CFRequest) (*drawing.CFRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, usererror.Misuse(i18n.M("sk.target_required"))
	}
	if err := validateSKCheckRoomSupportedRanks(req.Ranks); err != nil {
		return nil, err
	}
	return &req, nil
}

func (c *Controller) BuildCheckRoomRequestFromTracker(req TrackerRankQuery) (*drawing.CFRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), payloadBuildStage)
	defer finishBuild()
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)
	payload := drawing.CFRequest{
		Eid:         normalized.EventID,
		EventName:   meta.name,
		Region:      normalized.Region,
		AggregateAt: meta.aggregateAt,
		UpdateAt:    time.Now().UTC().UnixMilli(),
	}
	c.populateCheckRoomCharacterIcon(&payload, normalized)
	selection, err := c.resolveCheckRoomRanks(normalized)
	if err != nil {
		return nil, err
	}
	// The target's trace metrics and its neighbours are independent tracker
	// reads once the target rank is known, so they run concurrently.
	var wg sync.WaitGroup
	if selection.enrich != nil && len(selection.infos) > 0 {
		wg.Go(func() { selection.enrich(&selection.infos[0]) })
	}
	c.populateCheckRoomAdjacentRanks(&payload, normalized, selection)
	wg.Wait()
	payload.Ranks = selection.infos
	return c.BuildCheckRoomRequest(payload)
}

type checkRoomRankSelection struct {
	infos               []drawing.RankInfo
	target              int
	previous            *drawing.RankInfo
	next                *drawing.RankInfo
	alwaysFetchAdjacent bool
	// enrich adds trace metrics to infos[0] when the check-room answer left
	// them to the caller.
	enrich func(*drawing.RankInfo)
}

func (c *Controller) populateCheckRoomCharacterIcon(payload *drawing.CFRequest, query TrackerRankQuery) {
	if query.WlCharacterID == nil || *query.WlCharacterID <= 0 {
		return
	}
	icon := c.resolveCharacterIconPath(*query.WlCharacterID, renderregion.Normalize(query.Region))
	if icon != "" {
		payload.WlCharaIconPath = &icon
	}
}

func (c *Controller) resolveCheckRoomRanks(query TrackerRankQuery) (checkRoomRankSelection, error) {
	if (query.UserID != nil && *query.UserID > 0) || len(query.Ranks) == 1 {
		return c.resolveSingleCheckRoomRank(query)
	}
	return c.resolveMultipleCheckRoomRanks(query)
}

func (c *Controller) resolveSingleCheckRoomRank(query TrackerRankQuery) (checkRoomRankSelection, error) {
	room, ok, err := c.checkRoomFromTrackerCloudV2(
		query.Region,
		query.EventID,
		query.Ranks,
		query.UserID,
		query.WlCharacterID,
		shouldSkipMissingTrackerRanks(query),
	)
	if ok {
		selection, err := newCheckRoomRankSelection(room.info, room.previous, room.next, err)
		selection.enrich = room.enrich
		return selection, err
	}
	if query.UserID == nil || *query.UserID <= 0 {
		return checkRoomRankSelection{}, nil
	}
	info, err := c.buildSingleUserFromTracker(query.Region, query.EventID, *query.UserID, query.WlCharacterID)
	if err != nil {
		return checkRoomRankSelection{}, fmt.Errorf("tracker user query failed: %w", err)
	}
	return newCheckRoomRankSelection(info, nil, nil, nil)
}

func newCheckRoomRankSelection(info drawing.RankInfo, previous, next *drawing.RankInfo, sourceErr error) (checkRoomRankSelection, error) {
	if sourceErr != nil {
		return checkRoomRankSelection{}, sourceErr
	}
	if err := validateSKCheckRoomSupportedRank(info.Rank); err != nil {
		return checkRoomRankSelection{}, err
	}
	return checkRoomRankSelection{
		infos:               []drawing.RankInfo{info},
		target:              info.Rank,
		previous:            previous,
		next:                next,
		alwaysFetchAdjacent: true,
	}, nil
}

func (c *Controller) resolveMultipleCheckRoomRanks(query TrackerRankQuery) (checkRoomRankSelection, error) {
	skipMissing := shouldSkipMissingTrackerRanks(query)
	infos, previous, next, ok, err := c.buildCheckRoomRanksFromTrackerCloudV2(query.Region, query.EventID, query.Ranks, query.WlCharacterID, skipMissing)
	if !ok {
		infos, err = c.buildRanksFromTracker(query.Region, query.EventID, query.Ranks, query.WlCharacterID, skipMissing)
	}
	if err != nil {
		return checkRoomRankSelection{}, err
	}
	if err := validateSKCheckRoomSupportedRanks(infos); err != nil {
		return checkRoomRankSelection{}, err
	}
	selection := checkRoomRankSelection{infos: infos, previous: previous, next: next}
	if len(infos) > 0 {
		selection.target = infos[0].Rank
	}
	return selection, nil
}

func (c *Controller) populateCheckRoomAdjacentRanks(payload *drawing.CFRequest, query TrackerRankQuery, selection checkRoomRankSelection) {
	if selection.target <= 0 {
		return
	}
	if !selection.alwaysFetchAdjacent && (selection.previous != nil || selection.next != nil) {
		payload.PrevRank = selection.previous
		payload.NextRank = selection.next
		return
	}
	payload.PrevRank = selection.previous
	payload.NextRank = selection.next
	var previous, next *drawing.RankInfo
	var wg sync.WaitGroup
	if selection.target > 1 {
		wg.Go(func() { previous = c.tryBuildSingleRank(query, selection.target-1) })
	}
	next = c.tryBuildSingleRank(query, selection.target+1)
	wg.Wait()
	if previous != nil {
		payload.PrevRank = previous
	}
	if next != nil {
		payload.NextRank = next
	}
}

func (c *Controller) RenderCheckRoom(req drawing.CFRequest) ([]byte, error) {
	image, err := c.RenderCheckRoomImage(req)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.contextOrBackground())
}

func (c *Controller) RenderCheckRoomImage(req drawing.CFRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), payloadBuildStage)
	payload, err := c.BuildCheckRoomRequest(req)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateSKCheckRoomImage(payload)
}

func (c *Controller) BuildCSBRequest(req drawing.CSBRequest) (*drawing.CSBRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, usererror.Misuse(i18n.M("sk.target_required"))
	}
	if err := validateSKCheckRoomSupportedRank(req.Ranks[len(req.Ranks)-1].Rank); err != nil {
		return nil, err
	}
	return &req, nil
}

func (c *Controller) BuildCSBRequestFromTracker(req TrackerRankQuery) (*drawing.CSBRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), payloadBuildStage)
	defer finishBuild()
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	if normalized.UserID == nil && len(normalized.Ranks) > 1 {
		return nil, usererror.Misuse(i18n.M("sk.arrest.single_only"))
	}

	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)
	now := time.Now().UTC()
	payload := drawing.CSBRequest{
		Eid:         normalized.EventID,
		EventName:   meta.name,
		Region:      normalized.Region,
		AggregateAt: meta.aggregateAt,
		UpdateAt:    now.UnixMilli(),
	}
	c.populateCSBCharacterIcon(&payload, normalized)
	trace, err := c.resolveCSBTrace(normalized)
	if err != nil {
		return nil, err
	}
	payload.Ranks = appendIdleTrackerRankTraceAt(trace, now)
	return c.BuildCSBRequest(payload)
}

func (c *Controller) populateCSBCharacterIcon(payload *drawing.CSBRequest, query TrackerRankQuery) {
	if query.WlCharacterID == nil || *query.WlCharacterID <= 0 {
		return
	}
	icon := c.resolveCharacterIconPath(*query.WlCharacterID, renderregion.Normalize(query.Region))
	if icon != "" {
		payload.WlCharaIconPath = &icon
	}
}

func (c *Controller) resolveCSBTrace(query TrackerRankQuery) ([]drawing.RankInfo, error) {
	if query.UserID != nil && *query.UserID > 0 {
		return c.resolveCSBUserTrace(query)
	}
	return c.resolveCSBRankTrace(query)
}

func (c *Controller) resolveCSBUserTrace(query TrackerRankQuery) ([]drawing.RankInfo, error) {
	info, err := c.buildSingleUserFromTracker(query.Region, query.EventID, *query.UserID, query.WlCharacterID)
	if err != nil {
		return nil, fmt.Errorf("tracker user query failed: %w", err)
	}
	if err := validateSKCheckRoomSupportedRank(info.Rank); err != nil {
		return nil, err
	}
	return c.buildUserTraceFromTracker(query.Region, query.EventID, *query.UserID, query.WlCharacterID)
}

func (c *Controller) resolveCSBRankTrace(query TrackerRankQuery) ([]drawing.RankInfo, error) {
	if len(query.Ranks) == 0 {
		return nil, usererror.Misuse(i18n.M("sk.arrest.single_only"))
	}
	info, userID, hasUserID, err := c.buildSingleRankBaseFromTracker(query.Region, query.EventID, query.Ranks[0], query.WlCharacterID)
	if err != nil {
		return nil, err
	}
	if err := validateSKCheckRoomSupportedRank(info.Rank); err != nil {
		return nil, err
	}
	if !hasUserID {
		userID, err = c.resolveTrackerUserIDByRank(query.Region, query.EventID, query.Ranks[0], query.WlCharacterID)
		if err != nil {
			return nil, err
		}
	}
	return c.buildUserTraceFromTracker(query.Region, query.EventID, userID, query.WlCharacterID)
}

func (c *Controller) RenderCSB(req drawing.CSBRequest) ([]byte, error) {
	image, err := c.RenderCSBImage(req)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.contextOrBackground())
}

func (c *Controller) RenderCSBImage(req drawing.CSBRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), payloadBuildStage)
	payload, err := c.BuildCSBRequest(req)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateSKCSBImage(payload)
}

func validateSKCheckRoomSupportedRanks(ranks []drawing.RankInfo) error {
	for _, rank := range ranks {
		if err := validateSKCheckRoomSupportedRank(rank.Rank); err != nil {
			return err
		}
	}
	return nil
}

func validateSKCheckRoomSupportedRank(rank int) error {
	if rank > skCheckRoomRankLimit {
		return usererror.Invalid(i18n.M("sk.room.top100_only"))
	}
	return nil
}
