package sk

import (
	"context"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/utils/usererror"
)

func NewController(drawingClient *drawing.HarukiDrawingClient) *Controller {
	return NewControllerWithConfig(drawingClient, ForecastConfig{})
}

func NewControllerWithConfig(drawingClient *drawing.HarukiDrawingClient, forecastConfig ForecastConfig) *Controller {
	forecast := NewRemoteForecastProviderWithConfig(forecastConfig)
	return &Controller{
		drawing:       drawingClient,
		drawingBase:   drawingClient,
		forecast:      forecast,
		forecastCache: newForecastCacheForConfig(forecast, forecastConfig),
		events:        regionsource.NewRegistry[EventSource](renderregion.JP),
		assets:        renderassets.NewAssetHelper("", nil),
	}
}

func newForecastCacheForConfig(provider ForecastProvider, cfg ForecastConfig) *forecastDataCache {
	if cfg.CacheStore != nil {
		return newForecastDataCacheWithStore(provider, cfg.CacheStore, cfg.CacheKey)
	}
	return newForecastDataCacheWithPath(provider, cfg.CachePath)
}

func (c *Controller) SetTrackerIntegration(tracker TrackerSource, events EventSource, assetHelper *renderassets.AssetHelper) {
	if c == nil {
		return
	}
	c.tracker = tracker
	c.trackerBase = tracker
	c.RegisterEventSource(events)
	if assetHelper != nil {
		c.assets = assetHelper
	}
}

func (c *Controller) RegisterEventSource(events EventSource) {
	if c == nil || events == nil {
		return
	}
	if c.events == nil {
		c.events = regionsource.NewRegistry[EventSource](renderregion.JP)
	}
	c.events.RegisterSource(events)
}

func (c *Controller) SetCensor(svc CensorService) {
	if c == nil {
		return
	}
	c.censor = svc
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.TODO()
	}
	clone := *c
	clone.requestCtx = ctx
	clone.assets = c.assets.WithContext(ctx)
	if c.drawingBase != nil {
		clone.drawing = c.drawingBase.WithContext(ctx)
	} else if c.drawing != nil {
		clone.drawing = c.drawing.WithContext(ctx)
	}
	if c.tracker != nil {
		if contextual, ok := c.tracker.(contextualTrackerSource); ok {
			clone.tracker = contextual.WithContext(ctx)
		}
	}
	if c.events != nil {
		clone.events = regionsource.NewRegistry[EventSource](c.events.ResolveRegion(renderregion.Unknown))
		for _, source := range c.events.OrderedSources() {
			if contextual, ok := any(source).(contextualEventSource); ok {
				clone.events.RegisterSource(contextual.WithContext(ctx))
				continue
			}
			clone.events.RegisterSource(source)
		}
	}
	return &clone
}

func (c *Controller) contextOrBackground() context.Context {
	if c != nil && c.requestCtx != nil {
		return c.requestCtx
	}
	return context.TODO()
}

func (c *Controller) SetForecastProvider(provider ForecastProvider) {
	if c == nil || provider == nil {
		return
	}
	c.forecast = provider
	if c.forecastCache == nil {
		c.forecastCache = newForecastDataCache(provider)
	} else {
		c.forecastCache.SetProvider(provider)
	}
}

func (c *Controller) BuildLineRequest(req LineRequest) (*LineRequest, error) {
	if len(req.Ranks) == 0 && len(req.ForecastColumns) == 0 {
		return nil, usererror.Misuse(i18n.M("sk.target_required"))
	}
	return &req, nil
}

func (c *Controller) RenderLine(req LineRequest) ([]byte, error) {
	image, err := c.RenderLineImage(req)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.contextOrBackground())
}

func (c *Controller) RenderLineImage(req LineRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	payload, err := c.BuildLineRequest(req)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateSKLineImage(&payload.SklRequest, payload.Full)
}
