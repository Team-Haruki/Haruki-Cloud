package card

import (
	"context"
	"fmt"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/masterdata"
	regionsource "haruki-cloud/internal/pjsk/render/source"
)

const cardListAutoBoxThreshold = 90

type Controller struct {
	sources   *regionsource.Registry[DataSource]
	events    *regionsource.Registry[event.DataSource]
	drawing   *drawing.HarukiDrawingClient
	assets    *assets.AssetHelper
	nicknames map[string]int
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

type contextualEventSource interface {
	WithContext(ctx context.Context) event.DataSource
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

func (c *Controller) MergeNicknames(items map[string]int) {
	if c == nil || len(items) == 0 {
		return
	}
	if c.nicknames == nil {
		c.nicknames = cloneNicknames(defaultNicknames)
	}
	for nickname, characterID := range items {
		key := normalizeNicknameQuery(nickname)
		if key == "" || characterID <= 0 {
			continue
		}
		if existing, ok := c.nicknames[key]; ok && existing != characterID {
			continue
		}
		c.nicknames[key] = characterID
	}
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.sources = regionsource.NewRegistry[DataSource](c.sources.ResolveRegion(renderregion.Unknown))
	for _, source := range c.sources.OrderedSources() {
		if contextual, ok := any(source).(contextualDataSource); ok {
			clone.sources.RegisterSource(contextual.WithContext(ctx))
			continue
		}
		clone.sources.RegisterSource(source)
	}
	clone.events = regionsource.NewRegistry[event.DataSource](c.events.ResolveRegion(renderregion.Unknown))
	for _, source := range c.events.OrderedSources() {
		if contextual, ok := any(source).(contextualEventSource); ok {
			clone.events.RegisterSource(contextual.WithContext(ctx))
			continue
		}
		clone.events.RegisterSource(source)
	}
	return &clone
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
	region, _, builder, cards, err := c.resolveCardsForListRequest(query)
	if err != nil {
		return nil, err
	}

	cardIDs := make([]int, 0, len(cards))
	seen := make(map[int]struct{}, len(cards))
	for _, item := range cards {
		if item == nil || item.ID <= 0 {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		cardIDs = append(cardIDs, item.ID)
	}
	if len(cardIDs) == 0 {
		return nil, fmt.Errorf("card ids are required")
	}
	req, err := builder.BuildCardListRequest(cardIDs, region)
	if err != nil {
		return nil, err
	}
	if query.Title != nil {
		req.Title = query.Title
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
	region, _, builder, cards, err := c.resolveCardsForListRequest(query)
	if err != nil {
		return nil, err
	}
	if len(cards) >= cardListAutoBoxThreshold {
		req, buildErr := builder.BuildCardBoxRequest(cards, region, query.DetailedProfile, false, false, true)
		if buildErr != nil {
			return nil, buildErr
		}
		if query.Title != nil {
			req.Title = query.Title
		}
		return c.drawing.GenerateCardBox(req)
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
	if queries[0].ShowBox && !hasOwnedCardData(queries[0].DetailedProfile) {
		return nil, fmt.Errorf("box 模式需要用户卡牌持有数据，请先提供 Suite 抓包或本地快照")
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
		if queries[0].StrictFilterOnly {
			cards, err = c.searchStrictFilterCards(source, queryText)
			if err != nil {
				return nil, fmt.Errorf("failed to search card box: %w", err)
			}
		} else {
			searcher := NewSearchService(source, NewParser(c.nicknames))
			cards, err = searcher.SearchList(queryText)
			if err != nil {
				return nil, fmt.Errorf("failed to search card box: %w", err)
			}
		}
	}
	useAfterTraining := true
	if queries[0].UseAfterTraining != nil {
		useAfterTraining = *queries[0].UseAfterTraining
	}
	req, err := builder.BuildCardBoxRequest(cards, region, queries[0].DetailedProfile, queries[0].ShowID, queries[0].ShowBox, useAfterTraining)
	if err != nil {
		return nil, err
	}
	req.ShowID = queries[0].ShowID
	req.ShowBox = queries[0].ShowBox
	userCardStates := extractOwnedCards(queries[0].DetailedProfile)
	for i := range req.Cards {
		state, ok := userCardStates[req.Cards[i].Card.CardID]
		req.Cards[i].Card.IsAfterTraining = common.BoolPtr(resolveCardBoxAfterTraining(req.Cards[i].Card, state, useAfterTraining, ok))
	}
	if queries[0].DetailedProfile != nil {
		req.UserInfo = queries[0].DetailedProfile
	}
	if queries[0].Title != nil {
		req.Title = queries[0].Title
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

func (c *Controller) resolveCardsForListRequest(query ListRequest) (renderregion.Value, DataSource, *Builder, []*masterdata.Card, error) {
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return region, nil, nil, nil, err
	}

	if len(query.CardIDs) > 0 {
		cards := make([]*masterdata.Card, 0, len(query.CardIDs))
		seen := make(map[int]struct{}, len(query.CardIDs))
		for _, cardID := range query.CardIDs {
			if cardID <= 0 {
				continue
			}
			if _, ok := seen[cardID]; ok {
				continue
			}
			seen[cardID] = struct{}{}
			card, getErr := source.GetCardByID(cardID)
			if getErr != nil || card == nil {
				continue
			}
			cards = append(cards, card)
		}
		if len(cards) == 0 {
			return region, nil, nil, nil, fmt.Errorf("card ids are required")
		}
		return region, source, builder, cards, nil
	}

	rawQuery := strings.TrimSpace(query.Query)
	if rawQuery == "" {
		return region, nil, nil, nil, fmt.Errorf("card ids are required")
	}

	if query.StrictFilterOnly {
		cards, err := c.searchStrictFilterCards(source, rawQuery)
		if err != nil {
			return region, nil, nil, nil, fmt.Errorf("failed to search card list: %w", err)
		}
		if len(cards) == 0 {
			return region, nil, nil, nil, fmt.Errorf("card ids are required")
		}
		return region, source, builder, cards, nil
	}
	searcher := NewSearchService(source, NewParser(c.nicknames))
	cards, err := searcher.SearchList(rawQuery)
	if err != nil {
		return region, nil, nil, nil, fmt.Errorf("failed to search card list: %w", err)
	}
	if len(cards) == 0 {
		return region, nil, nil, nil, fmt.Errorf("card ids are required")
	}
	return region, source, builder, cards, nil
}

func (c *Controller) searchStrictFilterCards(source DataSource, rawQuery string) ([]*masterdata.Card, error) {
	parser := NewParser(c.nicknames)
	info, parseErr := parser.ParseStrictFilter(rawQuery)
	if parseErr != nil {
		return nil, parseErr
	}
	if info == nil || info.Type != QueryTypeFilter {
		return nil, fmt.Errorf("无法解析的列表查询指令: %s", rawQuery)
	}
	items, err := source.FilterCards(info)
	if err != nil {
		return nil, err
	}
	items = filterVisibleCards(items, currentCardVisibilityTime())
	if len(items) == 0 {
		return nil, fmt.Errorf("no cards found for filter: %s", rawQuery)
	}
	sortCardsByReleaseAndID(items)
	return items, nil
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
