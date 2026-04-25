package sk

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	regionsource "haruki-cloud/internal/pjsk/render/source"
)

func NewController(drawingClient *drawing.HarukiDrawingClient) *Controller {
	return &Controller{
		drawing:      drawingClient,
		forecast:     NewRemoteForecastProvider(),
		events:       regionsource.NewRegistry[EventSource](renderregion.JP),
		assets:       renderassets.NewAssetHelper("", nil),
		predictCache: newPredictRenderCache(),
	}
}

func (c *Controller) SetTrackerIntegration(tracker TrackerSource, events EventSource, assetHelper *renderassets.AssetHelper) {
	if c == nil {
		return
	}
	c.tracker = tracker
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
	clone := *c
	clone.requestCtx = ctx
	clone.drawing = c.drawing.WithContext(ctx)
	if c.tracker != nil {
		if contextual, ok := c.tracker.(contextualTrackerSource); ok {
			clone.tracker = contextual.WithContext(ctx)
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
}

// censorTrackerName runs the name through CensorService for logging only and
// always returns the original tracker name so ranking outputs keep in-game names.
func (c *Controller) censorTrackerName(name, server string) string {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return ""
	}
	if c.censor != nil {
		_ = c.censor.CensorName(c.contextOrBackground(), 0, "", clean, server)
	}
	return clean
}

func (c *Controller) BuildLineRequest(req LineRequest) (*LineRequest, error) {
	if len(req.Ranks) == 0 && len(req.ForecastColumns) == 0 {
		return nil, fmt.Errorf("sk line request has no ranks")
	}
	return &req, nil
}

func (c *Controller) RenderLine(req LineRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildLineRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKLine(&payload.SklRequest, payload.Full)
}
