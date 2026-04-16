package music

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/drawing"
)

func (c *Controller) BuildMusicChartRequest(query ChartQuery) (*drawing.GenerateMusicChartRequest, error) {
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	searcher := c.newSearchService(source)
	info, musicInfo, err := searcher.SearchChart(query.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to search music chart: %w", err)
	}
	if strings.TrimSpace(query.Difficulty) == "" && info != nil {
		query.Difficulty = info.Difficulty
	}
	req, err := builder.BuildMusicChartRequest(query, musicInfo, region)
	if err != nil {
		return nil, err
	}
	if query.Skill {
		req.MusicMeta = c.resolveMusicChartMeta(region, musicInfo.ID, query.Difficulty)
	}
	return req, nil
}

func (c *Controller) RenderMusicChart(query ChartQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicChartRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMusicChart(payload)
}

func (c *Controller) BuildMusicProgressRequest(query ProgressQuery) (*drawing.PlayProgressRequest, error) {
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	diff := normalizeDifficulty(query.Difficulty)

	counts := append([]drawing.PlayProgressCount(nil), query.Counts...)
	if len(counts) == 0 {
		if query.UserResults != nil {
			counts = c.buildProgressCountsFromResults(source, builder, diff, query.UserResults)
		} else if userCounts := c.buildUserProgressCounts(source, builder, diff); len(userCounts) > 0 {
			counts = userCounts
		} else {
			counts = c.buildDefaultProgressCounts(source, builder, diff)
		}
	}

	return &drawing.PlayProgressRequest{
		Counts:     counts,
		Difficulty: diff,
		Profile:    c.resolveProfileCard(query.Profile, region),
	}, nil
}

func (c *Controller) RenderMusicProgress(query ProgressQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicProgressRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GeneratePlayProgress(payload)
}

func (c *Controller) BuildMusicProgressRequestFromSnapshot(query ProgressQuery, snapshot snapshot.Snapshot, fallbackProfile *drawing.ProfileCardRequest) (*drawing.PlayProgressRequest, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	controller := c
	if snapshot != nil {
		controller = c.WithSnapshot(snapshot)
		if query.Profile == nil {
			region := controller.resolveRegion(query.Region)
			if profile := snapshot.ProfileCard(region); profile != nil {
				query.Profile = profile
			}
		}
	}
	if query.Profile == nil {
		query.Profile = fallbackProfile
	}
	return controller.BuildMusicProgressRequest(query)
}

func (c *Controller) RenderMusicProgressFromSnapshot(query ProgressQuery, snapshot snapshot.Snapshot, fallbackProfile *drawing.ProfileCardRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicProgressRequestFromSnapshot(query, snapshot, fallbackProfile)
	if err != nil {
		return nil, err
	}
	return c.drawing.GeneratePlayProgress(payload)
}
