package misc

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
)

type Controller struct {
	drawing    *drawing.HarukiDrawingClient
	requestCtx context.Context
}

func NewController(drawingClient *drawing.HarukiDrawingClient) *Controller {
	return &Controller{drawing: drawingClient}
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.requestCtx = ctx
	clone.drawing = c.drawing.WithContext(ctx)
	return &clone
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
	image, err := c.RenderCharaBirthdayImage(req)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderCharaBirthdayImage(req drawing.CharaBirthdayRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	validated, err := c.BuildCharaBirthdayRequest(req)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateCharacterBirthdayImage(validated)
}

func (c *Controller) BuildAliasListRequest(req drawing.AliasListRequest) (*drawing.AliasListRequest, error) {
	if strings.TrimSpace(req.Title) == "" {
		return nil, fmt.Errorf("alias list title is required")
	}
	if strings.TrimSpace(req.EntityLabel) == "" {
		return nil, fmt.Errorf("alias list entity label is required")
	}
	if req.EntityID <= 0 {
		return nil, fmt.Errorf("alias list entity id is invalid")
	}
	if strings.TrimSpace(req.EntityName) == "" {
		return nil, fmt.Errorf("alias list entity name is required")
	}
	if len(req.Aliases) == 0 {
		return nil, fmt.Errorf("alias list aliases are required")
	}
	return &req, nil
}

func (c *Controller) RenderAliasList(req drawing.AliasListRequest) ([]byte, error) {
	image, err := c.RenderAliasListImage(req)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderAliasListImage(req drawing.AliasListRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	validated, err := c.BuildAliasListRequest(req)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateAliasListImage(validated)
}
