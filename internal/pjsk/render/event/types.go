package event

import (
	"context"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	regionsource "haruki-cloud/internal/pjsk/render/source"
)

type DataSource interface {
	DefaultRegion() renderregion.Value
	GetEventByID(id int) (*masterdata.Event, error)
	GetEventByCardID(cardID int) (*masterdata.Event, error)
	GetEvents() []*masterdata.Event
	GetEventCards(eventID int) ([]*masterdata.Card, error)
	GetEventBannerCharacterID(eventID int) (int, error)
	GetEventDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error)
	GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error)
	GetBanEvents(charID int) []*masterdata.Event
	GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom
	GetCharacterByID(id int) (*masterdata.Character, error)
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

type Controller struct {
	sources *regionsource.Registry[DataSource]
	drawing *drawing.HarukiDrawingClient
	assets  *assets.AssetHelper
}

type Builder struct {
	source DataSource
	assets *assets.AssetHelper
}

// ProviderAdapter bridges provider.MasterDataProvider to event.DataSource.
type ProviderAdapter struct {
	provider.ProviderAdapterBase
}

type DetailQuery struct {
	Region     renderregion.Value `json:"region"`
	EventID    int                `json:"event_id"`
	UseCurrent bool               `json:"use_current"`
	Index      *int               `json:"index,omitempty"`
	Keyword    string             `json:"keyword,omitempty"`
	BanCharID  int                `json:"ban_char_id,omitempty"`
	BanSeq     int                `json:"ban_seq,omitempty"`
}

type ListQuery struct {
	Region        renderregion.Value `json:"region"`
	EventType     string             `json:"event_type,omitempty"`
	Unit          string             `json:"unit,omitempty"`
	Blend         bool               `json:"blend,omitempty"`
	Attr          string             `json:"attr,omitempty"`
	Year          int                `json:"year,omitempty"`
	CharacterID   int                `json:"character_id,omitempty"`
	CharacterIDs  []int              `json:"character_ids,omitempty"`
	BannerCharID  *int               `json:"banner_char_id,omitempty"`
	IncludePast   bool               `json:"include_past,omitempty"`
	IncludeFuture bool               `json:"include_future,omitempty"`
	OnlyFuture    bool               `json:"only_future,omitempty"`
	Limit         int                `json:"limit,omitempty"`
}
