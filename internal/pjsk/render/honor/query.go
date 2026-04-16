package honor

import renderregion "haruki-cloud/internal/pjsk/region"

type Query struct {
	Region              renderregion.Value `json:"region"`
	HonorID             int                `json:"honor_id"`
	HonorLevel          int                `json:"honor_level,omitempty"`
	IsMain              bool               `json:"is_main,omitempty"`
	BondsHonorViewType  string             `json:"bonds_honor_view_type,omitempty"`
	BondsHonorWordID    int                `json:"bonds_honor_word_id,omitempty"`
	FcOrApLevelOverride *int               `json:"fc_or_ap_level_override,omitempty"`
}
