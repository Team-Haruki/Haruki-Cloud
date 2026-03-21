package misc

import (
	"fmt"

	"haruki-cloud/utils/drawing"
)

type Controller struct {
	drawing *drawing.HarukiDrawingClient
}

func NewController(drawingClient *drawing.HarukiDrawingClient) *Controller {
	return &Controller{drawing: drawingClient}
}

func (c *Controller) BuildCharaBirthdayRequest(req drawing.CharaBirthdayRequest) (*drawing.CharaBirthdayRequest, error) {
	if req.Cid <= 0 || req.Month <= 0 || req.Day <= 0 {
		return nil, fmt.Errorf("invalid birthday request")
	}
	if len(req.Cards) == 0 {
		return nil, fmt.Errorf("birthday cards are required")
	}
	return &req, nil
}

func (c *Controller) RenderCharaBirthday(req drawing.CharaBirthdayRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	validated, err := c.BuildCharaBirthdayRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateCharacterBirthday(validated)
}
