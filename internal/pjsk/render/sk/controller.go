package sk

import (
	"fmt"

	"haruki-cloud/utils/drawing"
)

type LineRequest struct {
	drawing.SklRequest
	Full bool `json:"full,omitempty"`
}

type Controller struct {
	drawing *drawing.HarukiDrawingClient
}

func NewController(drawingClient *drawing.HarukiDrawingClient) *Controller {
	return &Controller{drawing: drawingClient}
}

func (c *Controller) BuildLineRequest(req LineRequest) (*LineRequest, error) {
	if len(req.Ranks) == 0 {
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

func (c *Controller) BuildQueryRequest(req drawing.SKRequest) (*drawing.SKRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, fmt.Errorf("sk query request has no ranks")
	}
	return &req, nil
}

func (c *Controller) RenderQuery(req drawing.SKRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildQueryRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKQuery(payload)
}

func (c *Controller) BuildCheckRoomRequest(req drawing.CFRequest) (*drawing.CFRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, fmt.Errorf("sk check-room request has no ranks")
	}
	return &req, nil
}

func (c *Controller) RenderCheckRoom(req drawing.CFRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildCheckRoomRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKCheckRoom(payload)
}

func (c *Controller) BuildSpeedRequest(req drawing.SpeedRequest) (*drawing.SpeedRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, fmt.Errorf("sk speed request has no ranks")
	}
	return &req, nil
}

func (c *Controller) RenderSpeed(req drawing.SpeedRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildSpeedRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKSpeed(payload)
}

func (c *Controller) BuildPlayerTraceRequest(req drawing.PlayerTraceRequest) (*drawing.PlayerTraceRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, fmt.Errorf("sk player-trace request has no ranks")
	}
	return &req, nil
}

func (c *Controller) RenderPlayerTrace(req drawing.PlayerTraceRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildPlayerTraceRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKPlayerTrace(payload)
}

func (c *Controller) BuildRankTraceRequest(req drawing.RankTraceRequest) (*drawing.RankTraceRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, fmt.Errorf("sk rank-trace request has no ranks")
	}
	return &req, nil
}

func (c *Controller) RenderRankTrace(req drawing.RankTraceRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildRankTraceRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKRankTrace(payload)
}

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
