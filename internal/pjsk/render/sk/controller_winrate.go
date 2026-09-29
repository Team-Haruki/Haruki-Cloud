package sk

import (
	"fmt"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
)

func (c *Controller) BuildWinRateRequest(req drawing.WinRateRequest) (*drawing.WinRateRequest, error) {
	if len(req.TeamInfo) == 0 {
		return nil, fmt.Errorf("sk winrate request has no teams")
	}
	return &req, nil
}

func (c *Controller) RenderWinRate(req drawing.WinRateRequest) ([]byte, error) {
	image, err := c.RenderWinRateImage(req)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.contextOrBackground())
}

func (c *Controller) RenderWinRateImage(req drawing.WinRateRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	payload, err := c.BuildWinRateRequest(req)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateSKWinRateImage(payload)
}
