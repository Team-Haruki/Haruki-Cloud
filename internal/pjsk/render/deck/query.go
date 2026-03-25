package deck

import "haruki-cloud/utils/drawing"

type AutoQuery struct {
	Region        string                              `json:"region"`
	RecommendType string                              `json:"recommend_type"`
	EventID       *int                                `json:"event_id,omitempty"`
	Limit         int                                 `json:"limit,omitempty"`
	TargetBonuses []int                               `json:"target_bonuses,omitempty"`
	Args          string                              `json:"args,omitempty"`
	Profile       *drawing.DetailedProfileCardRequest `json:"-"`
}
