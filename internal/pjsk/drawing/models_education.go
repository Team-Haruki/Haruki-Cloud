package drawing

// =========================== Education Models ===========================

type CharacterChallengeInfo struct {
	CharaID       int    `json:"chara_id"`
	Rank          int    `json:"rank"`
	Score         int    `json:"score"`
	Jewel         int    `json:"jewel"`
	Shard         int    `json:"shard"`
	CharaIconPath string `json:"chara_icon_path"`
}

type ChallengeLiveDetailsRequest struct {
	Profile             DetailedProfileCardRequest `json:"profile"`
	CharacterChallenges []CharacterChallengeInfo   `json:"character_challenges"`
	MaxScore            int                        `json:"max_score"`
	JewelIconPath       *string                    `json:"jewel_icon_path,omitempty"`
	ShardIconPath       *string                    `json:"shard_icon_path,omitempty"`
}

type CharacterBonus struct {
	CharaID       int     `json:"chara_id"`
	CharaIconPath string  `json:"chara_icon_path"`
	AreaItem      float64 `json:"area_item"`
	Rank          float64 `json:"rank"`
	Fixture       float64 `json:"fixture"`
	Total         float64 `json:"total"`
}

type UnitBonus struct {
	Unit         string  `json:"unit"`
	UnitIconPath string  `json:"unit_icon_path"`
	AreaItem     float64 `json:"area_item"`
	Gate         float64 `json:"gate"`
	Total        float64 `json:"total"`
}

type AttrBonus struct {
	Attr         string  `json:"attr"`
	AttrIconPath string  `json:"attr_icon_path"`
	AreaItem     float64 `json:"area_item"`
	Total        float64 `json:"total"`
}

type PowerBonusDetailRequest struct {
	Profile      DetailedProfileCardRequest `json:"profile"`
	CharaBonuses []CharacterBonus           `json:"chara_bonuses"`
	UnitBonuses  []UnitBonus                `json:"unit_bonuses"`
	AttrBonuses  []AttrBonus                `json:"attr_bonuses"`
}

type AreaItemMaterial struct {
	MaterialID       int    `json:"material_id"`
	MaterialIconPath string `json:"material_icon_path"`
	Quantity         int    `json:"quantity"`
	HaveQuantity     int    `json:"have_quantity"`
	SumQuantity      int    `json:"sum_quantity"`
	IsEnough         bool   `json:"is_enough"`
}

type AreaItemLevel struct {
	Level      int                `json:"level"`
	Bonus      float64            `json:"bonus"`
	CanUpgrade bool               `json:"can_upgrade"`
	Materials  []AreaItemMaterial `json:"materials"`
}

type AreaItemInfo struct {
	ItemID         int             `json:"item_id"`
	CurrentLevel   int             `json:"current_level"`
	ItemIconPath   string          `json:"item_icon_path"`
	TargetIconPath *string         `json:"target_icon_path,omitempty"`
	Levels         []AreaItemLevel `json:"levels"`
}

type AreaItemUpgradeMaterialsRequest struct {
	Profile    *DetailedProfileCardRequest `json:"profile,omitempty"`
	AreaItems  []AreaItemInfo              `json:"area_items"`
	HasProfile bool                        `json:"has_profile"`
}

type BondInfo struct {
	CharaID1       int    `json:"chara_id1"`
	CharaID2       int    `json:"chara_id2"`
	CharaIconPath1 string `json:"chara_icon_path1"`
	CharaIconPath2 string `json:"chara_icon_path2"`
	CharaRank1     int    `json:"chara_rank1"`
	CharaRank2     int    `json:"chara_rank2"`
	BondLevel      int    `json:"bond_level"`
	NeedExp        *int   `json:"need_exp,omitempty"`
	HasBond        bool   `json:"has_bond"`
	Color1         []int  `json:"color1,omitempty"` // tuple in python, []int in go
	Color2         []int  `json:"color2,omitempty"`
}

type BondsRequest struct {
	Profile  DetailedProfileCardRequest `json:"profile"`
	Bonds    []BondInfo                 `json:"bonds"`
	MaxLevel int                        `json:"max_level"`
}

type LeaderCountInfo struct {
	CharaID       int    `json:"chara_id"`
	CharaIconPath string `json:"chara_icon_path"`
	PlayCount     int    `json:"play_count"`
	ExLevel       int    `json:"ex_level"`
	ExCount       int    `json:"ex_count"`
}

type LeaderCountRequest struct {
	Profile      DetailedProfileCardRequest `json:"profile"`
	LeaderCounts []LeaderCountInfo          `json:"leader_counts"`
	MaxPlayCount int                        `json:"max_play_count"`
}
