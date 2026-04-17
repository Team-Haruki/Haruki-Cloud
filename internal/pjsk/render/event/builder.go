package event

import (
	"fmt"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/drawing"
)

func NewBuilder(source DataSource, assetHelper *assets.AssetHelper) *Builder {
	return &Builder{
		source: source,
		assets: assetHelper,
	}
}

func (b *Builder) BuildEventDetailRequest(query DetailQuery) (*drawing.EventDetailRequest, error) {
	if query.EventID == 0 {
		return nil, fmt.Errorf("event id is required")
	}

	eventInfo, err := b.source.GetEventByID(query.EventID)
	if err != nil {
		return nil, err
	}
	region := query.Region
	if region.IsZero() {
		region = b.source.DefaultRegion()
	}

	cards, err := b.source.GetEventCards(eventInfo.ID)
	if err != nil {
		return nil, err
	}
	cardThumbs := make([]drawing.CardFullThumbnailRequest, 0, len(cards))
	for _, card := range cards {
		cardThumbs = append(cardThumbs, common.BuildCardThumbnail(b.assets, card, region, common.ThumbnailOptions{AfterTraining: false}))
	}

	info, err := b.buildEventInfo(eventInfo)
	if err != nil {
		return nil, err
	}

	return &drawing.EventDetailRequest{
		Region:      region.String(),
		EventInfo:   info,
		EventAssets: b.buildEventAssets(eventInfo, info, region),
		EventCards:  cardThumbs,
	}, nil
}

func (b *Builder) BuildEventListRequest(query ListQuery) (*drawing.EventListRequest, error) {
	events := b.filterEvents(query)
	if len(events) == 0 {
		return nil, fmt.Errorf("no events matched filters")
	}

	region := query.Region
	if region.IsZero() {
		region = b.source.DefaultRegion()
	}

	briefs := make([]drawing.EventBrief, 0, len(events))
	for _, eventInfo := range events {
		brief, err := b.buildEventBrief(eventInfo, region)
		if err != nil {
			return nil, err
		}
		briefs = append(briefs, brief)
	}

	return &drawing.EventListRequest{
		EventInfo: briefs,
	}, nil
}
