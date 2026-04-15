package honor

import (
	"context"
	"fmt"

	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/internal/pjsk/drawing"
)

// contextualDataSource is the local interface for type-asserting
// DataSource implementations that support context injection.
type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

type Controller struct {
	sources *regionsource.Registry[DataSource]
	drawing *drawing.HarukiDrawingClient
	assets  *assets.AssetHelper
}

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

func (c *Controller) BuildHonorRequest(query Query) (*drawing.HonorRequest, error) {
	query.Region = c.sources.ResolveRegion(query.Region)
	src, ok := c.sources.SourceForRegion(query.Region)
	if !ok {
		return nil, fmt.Errorf("honor data source not configured")
	}
	return NewBuilder(src, c.assets).BuildHonorRequest(query)
}

func (c *Controller) RenderHonor(query Query) ([]byte, error) {
	if c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	req, err := c.BuildHonorRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateHonor(req)
}
