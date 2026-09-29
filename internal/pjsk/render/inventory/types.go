package inventory

import (
	"context"
	"sync"
	"time"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/cachefill"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

// MasterdataOptions configures the inventory master tables: Sekai serves
// them from the database; LocalDir is the local masterdata root used only
// for a table the database cannot serve (empty or failing), which the
// composition root leaves blank unless the local fallback flag is on.
type MasterdataOptions struct {
	Sekai    *sekaiDB.Client
	LocalDir string
}

type Query struct {
	Region   renderregion.Value
	Profile  *drawing.DetailedProfileCardRequest
	Snapshot snapshot.Snapshot
	Filter   Filter
	// MaterialRows serves the region's materials rows as whole master rows
	// (SELECT *), which carry columns the typed query does not read yet,
	// such as expiredAt (JP 7.0.0). Nil or unserved: no expiry handling.
	MaterialRows MaterialRowSource
}

// MaterialRowSource is the region's generic master row store
// (provider.MasterRowSource).
type MaterialRowSource interface {
	LoadMasterRows(ctx context.Context, filename string) (map[int]map[string]any, bool)
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
	requestCtx    context.Context
	// now is the clock for material expiry; nil means time.Now.
	now func() time.Time
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
	client   *sekaiDB.Client
	localDir string
	mu       sync.RWMutex
	cache    map[string]*regionMasterdata
	// partial keeps what a failed fill loaded when local files supplied
	// every table the database failed, to serve while the region backs
	// off.
	partial    map[string]*regionMasterdata
	generation uint64
	fill       cachefill.Group
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
