package sk

import (
	"fmt"
	"sort"

	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	"golang.org/x/sync/errgroup"
)

const skLineTrackerConcurrency = 6

type lineRankResult struct {
	info drawing.RankInfo
	ok   bool
}

func (c *Controller) buildLineRanksOrUserFromTracker(server string, eventID int, ranks []int, userID *int64, wlCharacterID *int, skipMissing bool) ([]drawing.RankInfo, error) {
	if userID != nil && *userID > 0 {
		info, err := c.buildSingleUserLineFromTracker(server, eventID, *userID, wlCharacterID)
		if err != nil {
			return nil, fmt.Errorf("tracker user query failed: %w", err)
		}
		return []drawing.RankInfo{info}, nil
	}
	return c.buildLineRanksFromTracker(server, eventID, ranks, wlCharacterID, skipMissing)
}

func (c *Controller) buildLineRanksFromTracker(server string, eventID int, ranks []int, wlCharacterID *int, skipMissing bool) ([]drawing.RankInfo, error) {
	results := make([]lineRankResult, len(ranks))

	var group errgroup.Group
	group.SetLimit(skLineTrackerConcurrency)

	for idx, rank := range ranks {
		idx, rank := idx, rank
		group.Go(func() error {
			info, err := c.buildSingleRankLineFromTracker(server, eventID, rank, wlCharacterID)
			if err != nil {
				if shouldSkipMissingTrackerRankError(skipMissing, err) {
					return nil
				}
				return fmt.Errorf("tracker rank %d query failed: %w", rank, err)
			}
			results[idx] = lineRankResult{
				info: info,
				ok:   true,
			}
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	out := make([]drawing.RankInfo, 0, len(results))
	for _, result := range results {
		if !result.ok {
			continue
		}
		out = append(out, result.info)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Rank < out[j].Rank
	})
	return out, nil
}

func (c *Controller) buildSingleRankLineFromTracker(server string, eventID, rank int, wlCharacterID *int) (drawing.RankInfo, error) {
	if wlCharacterID != nil && *wlCharacterID > 0 {
		latest, err := c.tracker.GetLatestWorldBloomRankingByRank(server, eventID, *wlCharacterID, rank)
		if err != nil {
			return drawing.RankInfo{}, err
		}

		rankValue := rank
		if latest.RankData.Rank > 0 {
			rankValue = latest.RankData.Rank
		}

		info := drawing.RankInfo{
			Rank:  rankValue,
			Name:  "",
			Score: drawing.IntPtr(latest.RankData.Score),
			Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
		}
		c.enrichLineRankInfoByRank(server, eventID, rankValue, wlCharacterID, &info)
		return info, nil
	}

	latest, err := c.tracker.GetLatestRankingByRank(server, eventID, rank)
	if err != nil {
		return drawing.RankInfo{}, err
	}

	rankValue := rank
	if latest.RankData.Rank > 0 {
		rankValue = latest.RankData.Rank
	}

	info := drawing.RankInfo{
		Rank:  rankValue,
		Name:  "",
		Score: drawing.IntPtr(latest.RankData.Score),
		Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
	}
	c.enrichLineRankInfoByRank(server, eventID, rankValue, nil, &info)
	return info, nil
}

func (c *Controller) buildSingleUserLineFromTracker(server string, eventID int, userID int64, wlCharacterID *int) (drawing.RankInfo, error) {
	if wlCharacterID != nil && *wlCharacterID > 0 {
		latest, err := c.tracker.GetLatestWorldBloomRankingByUser(server, eventID, *wlCharacterID, userID)
		if err != nil {
			return drawing.RankInfo{}, err
		}
		if latest == nil || latest.RankData.Rank <= 0 {
			return drawing.RankInfo{}, sekaiapi.ErrRankingNotFound
		}

		rankValue := latest.RankData.Rank

		info := drawing.RankInfo{
			Rank:  rankValue,
			Name:  "",
			Score: drawing.IntPtr(latest.RankData.Score),
			Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
		}
		c.enrichLineRankInfoByUser(server, eventID, userID, wlCharacterID, &info)
		return info, nil
	}

	latest, err := c.tracker.GetLatestRankingByUser(server, eventID, userID)
	if err != nil {
		return drawing.RankInfo{}, err
	}
	if latest == nil || latest.RankData.Rank <= 0 {
		return drawing.RankInfo{}, sekaiapi.ErrRankingNotFound
	}

	rankValue := latest.RankData.Rank

	info := drawing.RankInfo{
		Rank:  rankValue,
		Name:  "",
		Score: drawing.IntPtr(latest.RankData.Score),
		Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
	}
	c.enrichLineRankInfoByUser(server, eventID, userID, nil, &info)
	return info, nil
}

func (c *Controller) enrichLineRankInfoByRank(server string, eventID, rank int, wlCharacterID *int, info *drawing.RankInfo) {
	if c == nil || c.tracker == nil || info == nil || rank <= 0 {
		return
	}

	if wlCharacterID != nil && *wlCharacterID > 0 {
		trace, err := c.tracker.TraceWorldBloomRankingByRank(server, eventID, *wlCharacterID, rank)
		if err != nil || trace == nil {
			return
		}
		applyRankInfoMetrics(info, worldBloomTraceSamples(trace.RankData))
		return
	}

	trace, err := c.tracker.TraceRankingByRank(server, eventID, rank)
	if err != nil || trace == nil {
		return
	}
	applyRankInfoMetrics(info, rankTraceSamples(trace.RankData))
}

func (c *Controller) enrichLineRankInfoByUser(server string, eventID int, userID int64, wlCharacterID *int, info *drawing.RankInfo) {
	if c == nil || c.tracker == nil || info == nil || userID <= 0 {
		return
	}

	if wlCharacterID != nil && *wlCharacterID > 0 {
		trace, err := c.tracker.TraceWorldBloomRankingByUser(server, eventID, *wlCharacterID, userID)
		if err != nil || trace == nil {
			return
		}
		applyRankInfoMetrics(info, worldBloomTraceSamples(trace.RankData))
		return
	}

	trace, err := c.tracker.TraceRankingByUser(server, eventID, userID)
	if err != nil || trace == nil {
		return
	}
	applyRankInfoMetrics(info, rankTraceSamples(trace.RankData))
}

func rankTraceSamples(points []sekaiapi.RankDataPoint) []trackerScoreSample {
	samples := make([]trackerScoreSample, 0, len(points))
	for _, point := range points {
		samples = append(samples, trackerScoreSample{
			score:     point.Score,
			timestamp: point.Timestamp,
		})
	}
	return samples
}

func worldBloomTraceSamples(points []sekaiapi.WorldBloomRankDataPoint) []trackerScoreSample {
	samples := make([]trackerScoreSample, 0, len(points))
	for _, point := range points {
		samples = append(samples, trackerScoreSample{
			score:     point.Score,
			timestamp: point.Timestamp,
		})
	}
	return samples
}
