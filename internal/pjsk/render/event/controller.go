package event

import (
	"fmt"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/utils/drawing"
)

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

func (c *Controller) BuildEventDetailRequest(query DetailQuery) (*drawing.EventDetailRequest, error) {
	query, src, err := c.resolveDetailQuery(query)
	if err != nil {
		return nil, err
	}
	return NewBuilder(src, c.assets).BuildEventDetailRequest(query)
}

func (c *Controller) RenderEventDetail(query DetailQuery) ([]byte, error) {
	if c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	req, err := c.BuildEventDetailRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateEventDetail(req)
}

func (c *Controller) BuildEventListRequest(query ListQuery) (*drawing.EventListRequest, error) {
	query.Region = c.sources.ResolveRegion(query.Region)
	src, ok := c.sources.SourceForRegion(query.Region)
	if !ok {
		return nil, fmt.Errorf("no event data source for region %s", query.Region)
	}
	return NewBuilder(src, c.assets).BuildEventListRequest(query)
}

func (c *Controller) RenderEventList(query ListQuery) ([]byte, error) {
	if c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	req, err := c.BuildEventListRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateEventList(req)
}

func (c *Controller) BuildEventRecordRequest(req drawing.EventRecordRequest) (*drawing.EventRecordRequest, error) {
	if len(req.EventInfo) == 0 && len(req.WlEventInfo) == 0 {
		return nil, fmt.Errorf("event record requires at least one history entry")
	}
	if req.UserInfo.Region == "" {
		return nil, fmt.Errorf("user_info.region is required")
	}
	if req.UserInfo.Nickname == "" {
		return nil, fmt.Errorf("user_info.nickname is required")
	}
	if req.UserInfo.LeaderImagePath == "" {
		return nil, fmt.Errorf("user_info.leader_image_path is required")
	}
	return &req, nil
}

func (c *Controller) RenderEventRecord(req drawing.EventRecordRequest) ([]byte, error) {
	if c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildEventRecordRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateEventRecord(payload)
}

func (c *Controller) resolveDetailQuery(query DetailQuery) (DetailQuery, DataSource, error) {
	query.Region = c.sources.ResolveRegion(query.Region)
	src, ok := c.sources.SourceForRegion(query.Region)
	if !ok {
		return query, nil, fmt.Errorf("no event data source for region %s", query.Region)
	}
	if query.EventID != 0 {
		return query, src, nil
	}
	if !query.UseCurrent {
		return query, src, fmt.Errorf("event id is required")
	}

	now := time.Now().UnixMilli()
	var selected *int
	var selectedStartAt int64
	for _, eventInfo := range src.GetEvents() {
		if eventInfo.StartAt <= now && now < eventInfo.AggregateAt+1000 {
			if selected == nil || eventInfo.StartAt > selectedStartAt {
				id := eventInfo.ID
				selected = &id
				selectedStartAt = eventInfo.StartAt
			}
		}
	}
	if selected == nil {
		return query, src, fmt.Errorf("no current event found for region %s", query.Region)
	}
	query.EventID = *selected
	return query, src, nil
}
