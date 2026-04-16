package profile

import (
	"context"
	"regexp"

	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/censor"
	"haruki-cloud/internal/pjsk/drawing"
)

var wordTagPattern = regexp.MustCompile(`<#.*?>`)

type Controller struct {
	sources    *regionsource.Registry[DataSource]
	drawing    *drawing.HarukiDrawingClient
	assets     *assets.AssetHelper
	snapshot   userdata.Snapshot
	censor     *censor.Service
	requestCtx context.Context
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

func NewController(defaultSource DataSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot userdata.Snapshot) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	ctrl := &Controller{
		sources:  regionsource.NewRegistry[DataSource](renderregion.JP),
		drawing:  drawingClient,
		assets:   assetHelper,
		snapshot: snapshot,
	}
	ctrl.RegisterSource(defaultSource)
	return ctrl
}

func (c *Controller) RegisterSource(source DataSource) {
	if c == nil || c.sources == nil {
		return
	}
	c.sources.RegisterSource(source)
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.requestCtx = ctx
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

func (c *Controller) SetCensor(svc *censor.Service) {
	if c == nil {
		return
	}
	c.censor = svc
}

func (c *Controller) contextOrBackground() context.Context {
	if c != nil && c.requestCtx != nil {
		return c.requestCtx
	}
	return context.Background()
}
