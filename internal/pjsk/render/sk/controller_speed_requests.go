package sk

import (
	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func (c *Controller) BuildSpeedRequest(req drawing.SpeedRequest) (*drawing.SpeedRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, usererror.Misuse(i18n.M("sk.target_required"))
	}
	return &req, nil
}

func (c *Controller) BuildSpeedRequestFromTracker(req TrackerRankQuery) (*drawing.SpeedRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	defer finishBuild()
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	if normalized.UserID != nil {
		return nil, usererror.Misuse(i18n.M("sk.speed.user_unsupported"))
	}
	speedPeriodSeconds, speedUnitPeriodSeconds, speedUnitText := normalizeTrackerSpeedConfig(normalized)
	speedInfos, err := c.buildSpeedInfosFromTracker(
		normalized.Region,
		normalized.EventID,
		normalized.Ranks,
		normalized.WlCharacterID,
		int(speedPeriodSeconds),
		speedUnitPeriodSeconds,
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
		payload.BannerImgPath = new(meta.bannerPath)
	}
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		if icon := c.resolveCharacterIconPath(*normalized.WlCharacterID, renderregion.Normalize(normalized.Region)); icon != "" {
			payload.WlCharaIconPath = &icon
		}
	}
	return c.BuildSpeedRequest(payload)
}

func (c *Controller) RenderSpeed(req drawing.SpeedRequest) ([]byte, error) {
	image, err := c.RenderSpeedImage(req)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.contextOrBackground())
}

func (c *Controller) RenderSpeedImage(req drawing.SpeedRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	payload, err := c.BuildSpeedRequest(req)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateSKSpeedImage(payload)
}
