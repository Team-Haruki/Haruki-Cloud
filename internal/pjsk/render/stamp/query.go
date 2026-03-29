package stamp

import renderregion "haruki-cloud/internal/pjsk/render/region"

type ListQuery struct {
	Region        renderregion.Value `json:"region"`
	PromptMessage string             `json:"prompt_message,omitempty"`
	IDs           []int              `json:"ids,omitempty"`
	Limit         int                `json:"limit,omitempty"`
	Page          int                `json:"page,omitempty"`
	All           bool               `json:"all,omitempty"`
}
