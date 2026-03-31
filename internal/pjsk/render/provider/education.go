package provider

// EducationProvider exposes area-item, challenge-reward, and character-rank
// masterdata used by the education (power bonus / challenge info) module.
type EducationProvider interface {
	GetChallengeRewardsByCharacter(charID int) []*ChallengeReward
	GetResourceBoxByPurpose(purpose string, id int) *ResourceBox
	GetResourceBoxesByPurpose(purpose string) []*ResourceBox
	GetAreaItems() []*AreaItem
	GetAreaItem(id int) *AreaItem
	GetAreaItemLevels(areaItemID int) []*AreaItemLevel
	GetAreaItemLevel(areaItemID, level int) *AreaItemLevel
	GetCharacterRank(characterID, rank int) *CharacterRank
	GetMysekaiGateLevel(gateID, level int) *MysekaiGateLevel
	GetShopItemByResourceBoxID(resourceBoxID int) *ShopItem
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

type MysekaiGateLevel struct {
	GateID         int
	Level          int
	PowerBonusRate float64
}

type ShopItem struct {
	ID            int
	ResourceBoxID int
	Costs         []ShopItemCost
}

type ShopItemCost struct {
	ResourceType string `json:"resourceType"`
	ResourceID   int    `json:"resourceId"`
	Quantity     int    `json:"quantity"`
}
