package gacha

import renderregion "haruki-cloud/internal/pjsk/render/region"

type ListQuery struct {
	Region        renderregion.Value `json:"region"`
	Page          int                `json:"page"`
	PageSize      int                `json:"page_size"`
	Year          int                `json:"year,omitempty"`
	IncludeFuture bool               `json:"include_future,omitempty"`
	IncludePast   bool               `json:"include_past,omitempty"`
	CardID        int                `json:"card_id,omitempty"`
	Keyword       string             `json:"keyword,omitempty"`
	IsRerelease   bool               `json:"is_rerelease,omitempty"`
	IsRecall      bool               `json:"is_recall,omitempty"`
	OnlyCurrent   bool               `json:"only_current,omitempty"`
}

type DetailQuery struct {
	Region   renderregion.Value `json:"region"`
	GachaID  int                `json:"gacha_id"`
	NegIndex int                `json:"neg_index,omitempty"`
	EventID  int                `json:"event_id,omitempty"`
}
