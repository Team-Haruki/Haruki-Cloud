package provider

import "context"

// EducationProvider exposes area-item, challenge-reward, and character-rank
// masterdata used by the education (power bonus / challenge info) module.
type EducationProvider interface {
	GetChallengeRewardsByCharacter(ctx context.Context, charID int) []*ChallengeReward
	GetResourceBoxByPurpose(ctx context.Context, purpose string, id int) *ResourceBox
	GetResourceBoxesByPurpose(ctx context.Context, purpose string) []*ResourceBox
	GetAreaItems(ctx context.Context) []*AreaItem
	GetAreaItem(ctx context.Context, id int) *AreaItem
	GetAreaItemLevels(ctx context.Context, areaItemID int) []*AreaItemLevel
	GetAreaItemLevel(ctx context.Context, areaItemID, level int) *AreaItemLevel
	GetCharacterRank(ctx context.Context, characterID, rank int) *CharacterRank
	GetBonds(ctx context.Context) []*Bond
	GetBondLevels(ctx context.Context) []*BondLevel
	GetGameCharacterStyle(ctx context.Context, gameID int) *GameCharacterStyle
	GetLeaderMissionRequirements(ctx context.Context) ([]LeaderMissionRequirement, int)
	GetMysekaiGateLevel(ctx context.Context, gateID, level int) *MysekaiGateLevel
	GetShopItemByResourceBoxID(ctx context.Context, resourceBoxID int) *ShopItem
	GetShopItems(ctx context.Context) []*ShopItem
}

// Types used by EducationProvider, mirroring education package structs.

type ChallengeReward struct {
	ID            int
	CharacterID   int
	HighScore     int
	ResourceBoxID int
}

type ResourceBox struct {
	ID                 int
	ResourceBoxPurpose string
	ResourceBoxType    string
	Description        string
	Details            []ResourceBoxDetail
}

type ResourceBoxDetail struct {
	ResourceType     string `json:"resourceType"`
	ResourceID       int    `json:"resourceId"`
	ResourceLevel    int    `json:"resourceLevel"`
	ResourceQuantity int    `json:"resourceQuantity"`
}

type AreaItem struct {
	ID              int
	AreaID          int
	Name            string
	AssetbundleName string
}

type AreaItemLevel struct {
	AreaItemID            int
	Level                 int
	TargetUnit            string
	TargetCardAttr        string
	TargetGameCharacterID int
	Power1BonusRate       float64
}

type CharacterRank struct {
	CharacterID     int
	Rank            int
	Power1BonusRate float64
}

type Bond struct {
	GroupID      int
	CharacterID1 int
	CharacterID2 int
}

type BondLevel struct {
	Level    int
	TotalExp int
}

type GameCharacterStyle struct {
	GameID      int
	CharacterID int
	ColorCode   string
}

type LeaderMissionRequirement struct {
	Seq         int
	Requirement int
}

type MysekaiGateLevel struct {
	GateID         int
	Level          int
	PowerBonusRate float64
}

type ShopItem struct {
	ID                 int
	ShopID             int
	Seq                int
	ResourceBoxID      int
	ReleaseConditionID int
	StartAt            int64
	Costs              []ShopItemCost
}

type ShopItemCost struct {
	ResourceType string `json:"resourceType"`
	ResourceID   int    `json:"resourceId"`
	Quantity     int    `json:"quantity"`
}
