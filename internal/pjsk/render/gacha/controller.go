package gacha

import (
	"context"
	"fmt"
	"sort"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/internal/pjsk/drawing"
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
	req, err := c.BuildGachaListRequest(query)
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
	req, err := c.BuildGachaDetailRequest(query)
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
	if query.GachaID != 0 {
		return query, src, nil
	}
	if query.NegIndex < 0 {
		all := src.GetGachas()
		gachas := make([]*masterdata.Gacha, 0, len(all))
		now := time.Now().UnixMilli()
		for _, item := range all {
			if item == nil || item.StartAt > now {
				continue
			}
			gachas = append(gachas, item)
		}
		if len(gachas) == 0 {
			return query, src, fmt.Errorf("no gacha data available for region %s", query.Region)
		}
		sort.Slice(gachas, func(i, j int) bool {
			if gachas[i].StartAt == gachas[j].StartAt {
				return gachas[i].ID < gachas[j].ID
			}
			return gachas[i].StartAt < gachas[j].StartAt
		})
		index := len(gachas) + query.NegIndex
		if index < 0 || index >= len(gachas) {
			return query, src, fmt.Errorf("gacha index %d is out of range", query.NegIndex)
		}
		query.GachaID = gachas[index].ID
		return query, src, nil
	}
	if query.EventID != 0 {
		gachaInfo, err := src.GetGachaByEventID(query.EventID)
		if err != nil {
			return query, src, err
		}
		query.GachaID = gachaInfo.ID
		return query, src, nil
	}
	return query, src, fmt.Errorf("gacha id is required")
}
