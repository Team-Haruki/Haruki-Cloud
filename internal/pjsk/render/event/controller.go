package event

import (
	"fmt"
	"sort"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
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

	if query.BanCharID != 0 {
		if query.BanSeq <= 0 {
			return query, src, fmt.Errorf("ban sequence must be greater than 0")
		}
		events := src.GetBanEvents(query.BanCharID)
		if len(events) == 0 {
			return query, src, fmt.Errorf("character %d does not have ban events", query.BanCharID)
		}
		sort.Slice(events, func(i, j int) bool {
			return events[i].StartAt < events[j].StartAt
		})
		if query.BanSeq > len(events) {
			return query, src, fmt.Errorf("character %d only has %d ban events", query.BanCharID, len(events))
		}
		query.EventID = events[query.BanSeq-1].ID
		return query, src, nil
	}

	events := src.GetEvents()
	sort.Slice(events, func(i, j int) bool {
		return events[i].StartAt < events[j].StartAt
	})
	if len(events) == 0 {
		return query, src, fmt.Errorf("no events found for region %s", query.Region)
	}

	if query.Keyword != "" {
		index, err := resolveEventKeywordIndex(events, query.Keyword)
		if err != nil {
			return query, src, err
		}
		query.EventID = events[index].ID
		return query, src, nil
	}

	if query.Index != nil {
		baseIndex, err := resolveCurrentEventIndex(events, "next_first")
		if err != nil {
			return query, src, err
		}
		targetIndex := baseIndex
		switch {
		case *query.Index < 0:
			targetIndex = baseIndex + *query.Index + 1
		case *query.Index > 0:
			targetIndex = baseIndex + *query.Index
		}
		if targetIndex < 0 || targetIndex >= len(events) {
			return query, src, fmt.Errorf("event index %d is out of range", *query.Index)
		}
		query.EventID = events[targetIndex].ID
		return query, src, nil
	}

	if !query.UseCurrent {
		return query, src, fmt.Errorf("event id is required")
	}
	index, err := resolveCurrentEventIndex(events, "next_first")
	if err != nil {
		return query, src, err
	}
	query.EventID = events[index].ID
	return query, src, nil
}

func resolveCurrentEventIndex(events []*masterdata.Event, fallback string) (int, error) {
	now := time.Now().UnixMilli()
	prevIndex := -1
	currentIndex := -1
	nextIndex := -1
	for i, eventInfo := range events {
		if eventInfo.StartAt <= now && now < eventInfo.AggregateAt+1000 {
			currentIndex = i
		}
		if eventInfo.AggregateAt+1000 < now {
			prevIndex = i
		}
		if nextIndex == -1 && eventInfo.StartAt > now {
			nextIndex = i
		}
	}
	if currentIndex >= 0 {
		return currentIndex, nil
	}
	switch fallback {
	case "prev":
		if prevIndex >= 0 {
			return prevIndex, nil
		}
	case "next":
		if nextIndex >= 0 {
			return nextIndex, nil
		}
	case "prev_first":
		if prevIndex >= 0 {
			return prevIndex, nil
		}
		if nextIndex >= 0 {
			return nextIndex, nil
		}
	case "next_first":
		if nextIndex >= 0 {
			return nextIndex, nil
		}
		if prevIndex >= 0 {
			return prevIndex, nil
		}
	}
	return -1, fmt.Errorf("no current event found")
}

func resolveEventKeywordIndex(events []*masterdata.Event, keyword string) (int, error) {
	now := time.Now().UnixMilli()
	prevIndex := -1
	nextIndex := -1
	for i, eventInfo := range events {
		if eventInfo.AggregateAt+1000 < now {
			prevIndex = i
		}
		if nextIndex == -1 && eventInfo.StartAt > now {
			nextIndex = i
		}
	}

	switch keyword {
	case "current":
		return resolveCurrentEventIndex(events, "next_first")
	case "prev":
		if prevIndex >= 0 {
			return prevIndex, nil
		}
		return -1, fmt.Errorf("no previous event found")
	case "next":
		if nextIndex >= 0 {
			return nextIndex, nil
		}
		return -1, fmt.Errorf("no next event found")
	default:
		return -1, fmt.Errorf("unsupported event keyword %q", keyword)
	}
}
