package card

import "haruki-cloud/utils/drawing"

type Query struct {
	Query            string                              `json:"query"`
	Region           string                              `json:"region"`
	UserID           string                              `json:"user_id,omitempty"`
	Mode             string                              `json:"mode,omitempty"`
	ShowID           bool                                `json:"show_id,omitempty"`
	ShowBox          bool                                `json:"show_box,omitempty"`
	UseAfterTraining *bool                               `json:"use_after_training,omitempty"`
	DetailedProfile  *drawing.DetailedProfileCardRequest `json:"-"`
}

type ListRequest struct {
	Query           string                              `json:"query,omitempty"`
	CardIDs         []int                               `json:"card_ids"`
	Region          string                              `json:"region"`
	DetailedProfile *drawing.DetailedProfileCardRequest `json:"-"`
}
