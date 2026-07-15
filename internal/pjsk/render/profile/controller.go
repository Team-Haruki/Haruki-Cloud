package profile

import (
	"context"
	"regexp"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/snapshot"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/utils/censor"
)

var wordTagPattern = regexp.MustCompile(`<#.*?>`)

func NewController(defaultSource DataSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot snapshot.Snapshot) *Controller {
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
	return context.TODO()
}
