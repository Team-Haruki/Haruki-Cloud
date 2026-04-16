package drawing

// =========================== Honor Models ===========================

// HonorRequest from src/sekai/honor/model.py
type HonorRequest struct {
	HonorType               *string `json:"honor_type,omitempty"`
	GroupType               *string `json:"group_type,omitempty"`
	HonorRarity             *string `json:"honor_rarity,omitempty"`
	HonorLevel              *int    `json:"honor_level,omitempty"`
	FcOrApLevel             *string `json:"fc_or_ap_level,omitempty"`
	IsEmpty                 bool    `json:"is_empty"`
	IsMainHonor             bool    `json:"is_main_honor"`
	HonorImgPath            *string `json:"honor_img_path,omitempty"`
	RankImgPath             *string `json:"rank_img_path,omitempty"`
	LvImgPath               *string `json:"lv_img_path,omitempty"`
	Lv6ImgPath              *string `json:"lv6_img_path,omitempty"`
	EmptyHonorPath          *string `json:"empty_honor_path,omitempty"`
	ScrollImgPath           *string `json:"scroll_img_path,omitempty"`
	WordImgPath             *string `json:"word_img_path,omitempty"`
	CharaIconPath           *string `json:"chara_icon_path,omitempty"`
	CharaIconPath2          *string `json:"chara_icon_path2,omitempty"`
	CharaID                 *string `json:"chara_id,omitempty"`
	CharaID2                *string `json:"chara_id2,omitempty"`
	BondsBgPath             *string `json:"bonds_bg_path,omitempty"`
	BondsBgPath2            *string `json:"bonds_bg_path2,omitempty"`
	MaskImgPath             *string `json:"mask_img_path,omitempty"`
	FrameImgPath            *string `json:"frame_img_path,omitempty"`
	FrameDegreeLevelImgPath *string `json:"frame_degree_level_img_path,omitempty"`
}
