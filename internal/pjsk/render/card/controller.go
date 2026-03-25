package card

import (
	"fmt"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/utils/drawing"
)

type Controller struct {
	sources   *regionsource.Registry[DataSource]
	events    *regionsource.Registry[event.DataSource]
	drawing   *drawing.HarukiDrawingClient
	assets    *assets.AssetHelper
	nicknames map[string]int
}

func NewController(defaultSource DataSource, defaultEventSource event.DataSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	controller := &Controller{
		sources:   regionsource.NewRegistry[DataSource](renderregion.JP),
		events:    regionsource.NewRegistry[event.DataSource](renderregion.JP),
		drawing:   drawingClient,
		assets:    assetHelper,
		nicknames: cloneNicknames(defaultNicknames),
	}
	controller.RegisterSource(defaultSource)
	controller.RegisterEventSource(defaultEventSource)
	return controller
}

func (c *Controller) RegisterSource(source DataSource) {
	c.sources.RegisterSource(source)
}

func (c *Controller) RegisterEventSource(source event.DataSource) {
	c.events.RegisterSource(source)
}

func (c *Controller) BuildCardDetailRequest(query Query) (*drawing.CardDetailRequest, error) {
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	searcher := NewSearchService(source, NewParser(c.nicknames))
	card, err := searcher.Search(query.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to search card: %w", err)
	}
	return builder.BuildCardDetailRequest(card, region)
}

func (c *Controller) RenderCardDetail(query Query) ([]byte, error) {
	if c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	req, err := c.BuildCardDetailRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateCardDetail(req)
}

func (c *Controller) BuildCardListRequest(query ListRequest) (*drawing.CardListRequest, error) {
	if len(query.CardIDs) == 0 {
		return nil, fmt.Errorf("card ids are required")
	}
	region, _, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	req, err := builder.BuildCardListRequest(query.CardIDs, region)
	if err != nil {
		return nil, err
	}
	if query.DetailedProfile != nil {
		req.UserInfo = query.DetailedProfile
	}
	return req, nil
}

func (c *Controller) RenderCardList(query ListRequest) ([]byte, error) {
	if c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	req, err := c.BuildCardListRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateCardList(req)
}

func (c *Controller) BuildCardBoxRequest(queries []Query) (*drawing.CardBoxRequest, error) {
	if len(queries) == 0 {
		return nil, fmt.Errorf("no card query provided")
	}

	region, source, builder, err := c.resolveBuilder(queries[0].Region)
	if err != nil {
		return nil, err
	}

	queryText := strings.TrimSpace(queries[0].Query)
	var cards []*masterdata.Card
	if queryText == "" {
		cards, err = source.FilterCards(&CardQueryInfo{})
		if err != nil {
			return nil, fmt.Errorf("failed to load cards for card box: %w", err)
		}
		now := time.Now().UnixMilli()
		filtered := cards[:0]
		for _, card := range cards {
			if card == nil || card.ReleaseAt > now {
				continue
			}
			filtered = append(filtered, card)
		}
		cards = filtered
		if len(cards) == 0 {
			return nil, fmt.Errorf("no released cards found for region %s", region)
		}
	} else {
		searcher := NewSearchService(source, NewParser(c.nicknames))
		cards, err = searcher.SearchList(queryText)
		if err != nil {
			return nil, fmt.Errorf("failed to search card box: %w", err)
		}
	}
	req, err := builder.BuildCardBoxRequest(cards, region)
	if err != nil {
		return nil, err
	}
	if queries[0].DetailedProfile != nil {
		req.UserInfo = queries[0].DetailedProfile
	}
	return req, nil
}

func (c *Controller) RenderCardBox(queries []Query) ([]byte, error) {
	if c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	req, err := c.BuildCardBoxRequest(queries)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateCardBox(req)
}

func (c *Controller) resolveBuilder(region string) (renderregion.Value, DataSource, *Builder, error) {
	resolved := c.sources.ResolveRegion(renderregion.Normalize(region))
	source, ok := c.sources.SourceForRegion(resolved)
	if !ok {
		return resolved, nil, nil, fmt.Errorf("no card data source for region %s", resolved)
	}
	var eventSource event.DataSource
	if resolvedEventSource, ok := c.events.SourceForRegion(resolved); ok {
		eventSource = resolvedEventSource
	}
	return resolved, source, NewBuilder(source, c.translationSource(resolved), eventSource, c.assets), nil
}

func (c *Controller) translationSource(region renderregion.Value) DataSource {
	if region != renderregion.JP {
		return nil
	}
	if source, ok := c.sources.SourceForRegion(renderregion.CN); ok {
		return source
	}
	return nil
}
