package inventory

import (
	"sync"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

type MasterdataOptions struct {
	LocalDir string
}

type Query struct {
	Region   renderregion.Value
	Profile  *drawing.DetailedProfileCardRequest
	Snapshot snapshot.Snapshot
	Filter   Filter
}

type Filter string

const (
	FilterDefault Filter = ""
	FilterJewel   Filter = "jewel"
	FilterBoost   Filter = "boost"
	FilterMysekai Filter = "mysekai"
	FilterMemory  Filter = "memory"
)

type Controller struct {
	drawing       *drawing.HarukiDrawingClient
	assets        *assets.AssetHelper
	snapshot      snapshot.Snapshot
	defaultRegion renderregion.Value
	masterdata    *masterdataStore
}

type materialMeta struct {
	ID           int    `json:"id"`
	Seq          int    `json:"seq"`
	MaterialType string `json:"materialType"`
	Name         string `json:"name"`
	FlavorText   string `json:"flavorText"`
}

type boostItemMeta struct {
	ID            int    `json:"id"`
	Seq           int    `json:"seq"`
	Name          string `json:"name"`
	RecoveryValue int    `json:"recoveryValue"`
	FlavorText    string `json:"flavorText"`
}

type eventItemMeta struct {
	ID              int    `json:"id"`
	EventID         int    `json:"eventId"`
	Name            string `json:"name"`
	AssetbundleName string `json:"assetbundleName"`
	FlavorText      string `json:"flavorText"`
}

type assetNamedMeta struct {
	ID              int    `json:"id"`
	Seq             int    `json:"seq"`
	Name            string `json:"name"`
	AssetbundleName string `json:"assetbundleName"`
	FlavorText      string `json:"flavorText"`
}

type ticketMeta struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	CharacterID int    `json:"characterId"`
	Exp         int    `json:"exp"`
	FlavorText  string `json:"flavorText"`
}

type mysekaiMaterialMeta struct {
	ID                  int    `json:"id"`
	Seq                 int    `json:"seq"`
	Type                string `json:"mysekaiMaterialType"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	IconAssetbundleName string `json:"iconAssetbundleName"`
}

type masterdataStore struct {
	localDir string
	mu       sync.RWMutex
	cache    map[string]*regionMasterdata
}

type regionMasterdata struct {
	materials            map[int]materialMeta
	boostItems           map[int]boostItemMeta
	eventItems           map[int]eventItemMeta
	gachaTickets         map[int]assetNamedMeta
	practiceTickets      map[int]ticketMeta
	skillPracticeTickets map[int]ticketMeta
	gachaCeilItems       map[int]assetNamedMeta
	mysekaiMaterials     map[int]mysekaiMaterialMeta
}
