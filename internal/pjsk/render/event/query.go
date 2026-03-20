package event

import renderregion "haruki-cloud/internal/pjsk/render/region"

type DetailQuery struct {
	Region     renderregion.Value `json:"region"`
	EventID    int                `json:"event_id"`
	UseCurrent bool               `json:"use_current"`
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
