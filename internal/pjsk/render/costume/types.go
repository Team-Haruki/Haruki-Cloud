package costume

import (
	"context"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

const (
	DefaultPageSize = 240
	MaxPageSize     = 480
)

type DataSource interface {
	DefaultRegion() renderregion.Value
	GetCostumeByID(id int) (*masterdata.Costume3d, error)
	FilterCostumes(filter Filter) ([]*masterdata.Costume3d, error)
	GetCostumeVariants(groupID int, partType string, characterID int) ([]*masterdata.Costume3d, error)
	GetCostumeSourceCardIDs(costumeIDs []int) (map[int][]int, error)
	GetCharacterByID(id int) (*masterdata.Character, error)
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

type Query struct {
	Query            string `json:"query,omitempty"`
	ID               int    `json:"id,omitempty"`
	Region           string `json:"region,omitempty"`
	ExpectedPartType string `json:"expected_part_type,omitempty"`
	OutfitID         int    `json:"outfit_id,omitempty"`
	AccessoryID      int    `json:"accessory_id,omitempty"`
	Character3DID    int    `json:"character_3d_id,omitempty"`
	ColorID          int    `json:"color_id,omitempty"`
}

type ListQuery struct {
	Query         string `json:"query,omitempty"`
	Region        string `json:"region,omitempty"`
	PartType      string `json:"part_type,omitempty"`
	Gender        string `json:"gender,omitempty"`
	Character     string `json:"character,omitempty"`
	Character3DID int    `json:"character_3d_id,omitempty"`
	Keyword       string `json:"keyword,omitempty"`
	Page          int    `json:"page,omitempty"`
	PageSize      int    `json:"page_size,omitempty"`
}

type Filter struct {
	PartType     string
	CostumeType  string
	CharacterID  int
	CharacterIDs []int
	Keyword      string
	ColorID      int
	Limit        int
	Offset       int
}

type ComboQuery struct {
	Query                string `json:"query,omitempty"`
	Region               string `json:"region,omitempty"`
	OutfitID             int    `json:"outfit_id,omitempty"`
	OutfitColorID        int    `json:"outfit_color_id,omitempty"`
	AccessoryID          int    `json:"accessory_id,omitempty"`
	AccessoryColorID     int    `json:"accessory_color_id,omitempty"`
	Character3DID        int    `json:"character_3d_id,omitempty"`
	HairID               int    `json:"hair_id,omitempty"`
	BodyCostume3DID      int    `json:"body_costume_3d_id,omitempty"`
	HairCostume3DID      int    `json:"hair_costume_3d_id,omitempty"`
	AccessoryCostume3DID int    `json:"accessory_costume_3d_id,omitempty"`
}
