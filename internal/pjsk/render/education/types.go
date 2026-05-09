package education

import (
	"context"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/internal/pjsk/render/snapshot"
	rendersource "haruki-cloud/internal/pjsk/render/source"
)

// ── Data source ─────────────────────────────────────────────────────────────

type DataSource interface {
	DefaultRegion() renderregion.Value
	GetChallengeRewardsByCharacter(charID int) []*ChallengeReward
	GetResourceBoxByPurpose(purpose string, id int) *ResourceBox
	GetResourceBoxesByPurpose(purpose string) []*ResourceBox
	GetAreaItems() []*AreaItem
	GetAreaItem(id int) *AreaItem
	GetAreaItemLevels(areaItemID int) []*AreaItemLevel
	GetAreaItemLevel(areaItemID, level int) *AreaItemLevel
	GetCharacterRank(characterID, rank int) *CharacterRank
	GetBonds() []*Bond
	GetBondLevels() []*BondLevel
	GetGameCharacterStyle(gameID int) *GameCharacterStyle
	GetCharacterMissions(characterID int) []*CharacterMission
	GetCharacterMissionParameterGroups(parameterGroupID int) []*CharacterMissionParameterGroup
	GetLeaderMissionRequirements() ([]LeaderMissionRequirement, int)
	GetMysekaiGateLevel(gateID, level int) *MysekaiGateLevel
	GetShopItemByResourceBoxID(resourceBoxID int) *ShopItem
	GetShopItems() []*ShopItem
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

// ── Data structs ────────────────────────────────────────────────────────────

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

type CharacterMission struct {
	ID                   int
	CharacterID          int
	CharacterMissionType string
	ParameterGroupID     int
	IsAchievementMission bool
}

type CharacterMissionParameterGroup struct {
	GameID      int
	Seq         int
	Requirement int
	Exp         int
	Quantity    int
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

// ── Controller ──────────────────────────────────────────────────────────────

type Controller struct {
	drawing  *drawing.HarukiDrawingClient
	assets   *assets.AssetHelper
	sources  *rendersource.Registry[DataSource]
	snapshot snapshot.Snapshot
}

// ProviderAdapter bridges provider.MasterDataProvider to education.DataSource.
type ProviderAdapter struct {
	provider.PjskProviderAdapterBase
}

// ── Snapshot helpers ────────────────────────────────────────────────────────

const maxRenderedBonds = 20

const areaCoinMaterialID = -1

const (
	areaTreeAreaID   = 11
	areaFlowerAreaID = 13
)

type resolvedSnapshotContext struct {
	region   renderregion.Value
	source   DataSource
	snapshot snapshot.Snapshot
	raw      *snapshot.RawUserData
	profile  *drawing.DetailedProfileCardRequest
}

type areaItemShopTarget struct {
	itemID       int
	sortedLevels []int
}

// ── Query types ─────────────────────────────────────────────────────────────

type ChallengeLiveQuery struct {
	Region   renderregion.Value                  `json:"region,omitempty"`
	Profile  *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot snapshot.Snapshot                   `json:"-"`
}

type PowerBonusQuery struct {
	Region   renderregion.Value                  `json:"region,omitempty"`
	Profile  *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot snapshot.Snapshot                   `json:"-"`
}

type BondsQuery struct {
	Region         renderregion.Value                  `json:"region,omitempty"`
	Profile        *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot       snapshot.Snapshot                   `json:"-"`
	Cid            int                                 `json:"cid,omitempty"`
	CharacterQuery string                              `json:"character_query,omitempty"`
}

type LeaderCountQuery struct {
	Region   renderregion.Value                  `json:"region,omitempty"`
	Profile  *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot snapshot.Snapshot                   `json:"-"`
}

type CharacterMissionQuery struct {
	Region         renderregion.Value                  `json:"region,omitempty"`
	Profile        *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot       snapshot.Snapshot                   `json:"-"`
	Cid            int                                 `json:"cid,omitempty"`
	CharacterQuery string                              `json:"character_query,omitempty"`
	MissionType    string                              `json:"mission_type,omitempty"`
	ShowAll        bool                                `json:"show_all,omitempty"`
}

type AreaItemQuery struct {
	Region         renderregion.Value                  `json:"region,omitempty"`
	Profile        *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot       snapshot.Snapshot                   `json:"-"`
	ShowFull       bool                                `json:"show_full,omitempty"`
	Unit           string                              `json:"unit,omitempty"`
	Cid            int                                 `json:"cid,omitempty"`
	CharacterQuery string                              `json:"character_query,omitempty"`
	Attr           string                              `json:"attr,omitempty"`
	Tree           bool                                `json:"tree,omitempty"`
	Flower         bool                                `json:"flower,omitempty"`
}
