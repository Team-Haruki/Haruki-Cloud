package sk

import (
	"fmt"

	"haruki-cloud/utils/drawing"
)

func (c *Controller) BuildWinRateRequest(req drawing.WinRateRequest) (*drawing.WinRateRequest, error) {
	if len(req.TeamInfo) == 0 {
		return nil, fmt.Errorf("sk winrate request has no teams")
	}
	return &req, nil
}

func (c *Controller) RenderWinRate(req drawing.WinRateRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildWinRateRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKWinRate(payload)
}
