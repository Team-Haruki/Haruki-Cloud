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
	if out, ok, err := c.buildLineRanksFromTrackerV2(server, eventID, ranks, wlCharacterID, skipMissing); ok {
		return out, err
	}

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
	infos, ok, err := c.buildLineRanksFromTrackerV2(server, eventID, []int{rank}, wlCharacterID, false)
	if !ok {
		return drawing.RankInfo{}, fmt.Errorf("tracker cloud v2 source is not configured")
	}
	if err != nil {
		return drawing.RankInfo{}, err
	}
	if len(infos) == 0 {
		return drawing.RankInfo{}, sekaiapi.ErrRankingNotFound
	}
	info := infos[0]
	return info, nil
}

func (c *Controller) buildSingleUserLineFromTracker(server string, eventID int, userID int64, wlCharacterID *int) (drawing.RankInfo, error) {
	source, ok := c.trackerCloudV2()
	if !ok {
		return drawing.RankInfo{}, fmt.Errorf("tracker cloud v2 source is not configured")
	}
	resp, err := source.GetCloudSKLine(server, eventID, wlCharacterID, nil, &userID, false, 3600)
	if err != nil {
		return drawing.RankInfo{}, err
	}
	if resp == nil || len(resp.Ranks) == 0 {
		return drawing.RankInfo{}, sekaiapi.ErrRankingNotFound
	}
	info := rankInfoFromCloudV2(resp.Ranks[0])
	info.Name = ""
	return info, nil
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
