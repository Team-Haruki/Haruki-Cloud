package sk

import (
	"fmt"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/drawing"
)

func (c *Controller) BuildSpeedRequest(req drawing.SpeedRequest) (*drawing.SpeedRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, fmt.Errorf("sk speed request has no ranks")
	}
	return &req, nil
}

func (c *Controller) BuildSpeedRequestFromTracker(req TrackerRankQuery) (*drawing.SpeedRequest, error) {
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	if normalized.UserID != nil {
		return nil, fmt.Errorf("speed 暂不支持按用户查询，请使用排名")
	}
	speedPeriodSeconds, speedUnitText := normalizeTrackerSpeedConfig(normalized)
	speedInfos, err := c.buildSpeedInfosFromTracker(
		normalized.Region,
		normalized.EventID,
		normalized.Ranks,
		normalized.WlCharacterID,
		int(speedPeriodSeconds),
		speedPeriodSeconds,
		shouldSkipMissingTrackerRanks(normalized),
	)
	if err != nil {
		return nil, err
	}
	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)
	payload := drawing.SpeedRequest{
		EventID:          normalized.EventID,
		Region:           normalized.Region,
		EventName:        meta.name,
		EventStartAt:     meta.startAt,
		EventAggregateAt: meta.aggregateAt,
		Ranks:            speedInfos,
		IsWlEvent:        normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0,
		RequestType:      speedUnitText,
		Period:           speedPeriodSeconds,
	}
	if meta.bannerPath != "" {
		banner := meta.bannerPath
		payload.BannerImgPath = &banner
	}
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		if icon := c.resolveCharacterIconPath(*normalized.WlCharacterID, renderregion.Normalize(normalized.Region)); icon != "" {
			payload.WlCharaIconPath = &icon
		}
	}
	return c.BuildSpeedRequest(payload)
}

func (c *Controller) RenderSpeed(req drawing.SpeedRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildSpeedRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKSpeed(payload)
}
