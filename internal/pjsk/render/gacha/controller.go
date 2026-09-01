package gacha

import (
	"context"
	"fmt"
	"sort"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/releasecheck"
	regionsource "haruki-cloud/internal/pjsk/render/source"
)

func NewController(defaultSource DataSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	ctrl := &Controller{
		sources: regionsource.NewRegistry[DataSource](renderregion.JP),
		drawing: drawingClient,
		assets:  assetHelper,
	}
	ctrl.RegisterSource(defaultSource)
	return ctrl
}

func (c *Controller) RegisterSource(src DataSource) {
	c.sources.RegisterSource(src)
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.requestCtx = ctx
	clone.drawing = c.drawing.WithContext(ctx)
	clone.assets = c.assets.WithContext(ctx)
	clone.sources = regionsource.NewRegistry[DataSource](c.sources.ResolveRegion(renderregion.Unknown))
	for _, source := range c.sources.OrderedSources() {
		if contextual, ok := any(source).(contextualDataSource); ok {
			clone.sources.RegisterSource(contextual.WithContext(ctx))
			continue
		}
		clone.sources.RegisterSource(source)
	}
	return &clone
}

func (c *Controller) BuildGachaListRequest(query ListQuery) (*drawing.GachaListRequest, error) {
	query.Region = c.sources.ResolveRegion(query.Region)
	src, ok := c.sources.SourceForRegion(query.Region)
	if !ok {
		return nil, fmt.Errorf("no gacha data source for region %s", query.Region)
	}
	return NewBuilder(src, c.assets).BuildGachaListRequest(query)
}

func (c *Controller) RenderGachaList(query ListQuery) ([]byte, error) {
	if c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	req, err := c.BuildGachaListRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateGachaList(req)
}

func (c *Controller) BuildGachaDetailRequest(query DetailQuery) (*drawing.GachaDetailRequest, error) {
	query, src, err := c.resolveDetailQuery(query)
	if err != nil {
		return nil, err
	}
	return NewBuilder(src, c.assets).BuildGachaDetailRequest(query)
}

func (c *Controller) RenderGachaDetail(query DetailQuery) ([]byte, error) {
	if c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	req, err := c.BuildGachaDetailRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateGachaDetail(req)
}

func (c *Controller) resolveDetailQuery(query DetailQuery) (DetailQuery, DataSource, error) {
	query.Region = c.sources.ResolveRegion(query.Region)
	src, ok := c.sources.SourceForRegion(query.Region)
	if !ok {
		return query, nil, fmt.Errorf("no gacha data source for region %s", query.Region)
	}
	var err error
	switch {
	case query.GachaID != 0:
		err = validateGachaRelease(src, query.GachaID)
	case query.NegIndex < 0:
		query.GachaID, err = resolveGachaNegativeIndex(src, query.Region, query.NegIndex)
	case query.EventID != 0:
		query.GachaID, err = resolveEventGachaID(src, query.EventID)
	default:
		err = fmt.Errorf("gacha id is required")
	}
	return query, src, err
}

func validateGachaRelease(src DataSource, gachaID int) error {
	gachaInfo, err := src.GetGachaByID(gachaID)
	if err != nil {
		return err
	}
	return futureGachaError(gachaInfo)
}

func resolveGachaNegativeIndex(src DataSource, region renderregion.Value, negativeIndex int) (int, error) {
	gachas := releasedGachas(src.GetGachas(), time.Now().UnixMilli())
	if len(gachas) == 0 {
		return 0, fmt.Errorf("no gacha data available for region %s", region)
	}
	sort.Slice(gachas, func(i, j int) bool {
		if gachas[i].StartAt == gachas[j].StartAt {
			return gachas[i].ID < gachas[j].ID
		}
		return gachas[i].StartAt < gachas[j].StartAt
	})
	index := len(gachas) + negativeIndex
	if index < 0 || index >= len(gachas) {
		return 0, fmt.Errorf("gacha index %d is out of range", negativeIndex)
	}
	return gachas[index].ID, nil
}

func releasedGachas(all []*masterdata.Gacha, now int64) []*masterdata.Gacha {
	gachas := make([]*masterdata.Gacha, 0, len(all))
	for _, item := range all {
		if item != nil && item.StartAt <= now {
			gachas = append(gachas, item)
		}
	}
	return gachas
}

func resolveEventGachaID(src DataSource, eventID int) (int, error) {
	gachaInfo, err := src.GetGachaByEventID(eventID)
	if err != nil {
		return 0, err
	}
	if gachaInfo == nil {
		return 0, fmt.Errorf("no gacha found for event %d", eventID)
	}
	if err := futureGachaError(gachaInfo); err != nil {
		return 0, err
	}
	return gachaInfo.ID, nil
}

func futureGachaError(gachaInfo *masterdata.Gacha) error {
	if gachaInfo != nil && gachaInfo.StartAt > time.Now().UnixMilli() {
		return releasecheck.New(releasecheck.KindGacha, "", gachaInfo.ID)
	}
	return nil
}
