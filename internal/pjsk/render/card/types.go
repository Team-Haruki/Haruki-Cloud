package card

import (
	"context"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	regionsource "haruki-cloud/internal/pjsk/render/source"
)

// ── Data source ─────────────────────────────────────────────────────────────

type DataSource interface {
	DefaultRegion() renderregion.Value
	GetCardByID(id int) (*masterdata.Card, error)
	GetCardByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error)
	FilterCards(info *PjskCardQueryInfo) ([]*masterdata.Card, error)
	GetCharacterColorCode(id int) (string, bool)
	GetCharacterByID(id int) (*masterdata.Character, error)
	GetUnitByCardID(cardID int) (string, error)
	GetCardSupplyType(card *masterdata.Card) string
	GetSkillByID(id int) (*masterdata.Skill, error)
	FormatSkillDescription(skill *masterdata.Skill, cardCharacterID int) string
	GetGachaByCardID(cardID int) (*masterdata.Gacha, error)
	GetCostume3dsByCardID(cardID int) ([]*masterdata.Costume3d, error)
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

type contextualEventSource interface {
	WithContext(ctx context.Context) event.DataSource
}

// ── Controller & builder ────────────────────────────────────────────────────

const cardListAutoBoxThreshold = 90

type Controller struct {
	sources   *regionsource.Registry[DataSource]
	events    *regionsource.Registry[event.DataSource]
	drawing   *drawing.HarukiDrawingClient
	assets    *assets.AssetHelper
	nicknames map[string]int
}

type Builder struct {
	source      DataSource
	translation DataSource
	events      event.DataSource
	assets      *assets.AssetHelper
}

// ProviderAdapter bridges provider.MasterDataProvider to card.DataSource.
type ProviderAdapter struct {
	provider.PjskProviderAdapterBase
}

type SearchService struct {
	source DataSource
	parser *Parser
}

type ImageResult struct {
	Card  *masterdata.Card
	Paths []string
}

// ── Parser ──────────────────────────────────────────────────────────────────

type QueryType int

const (
	QueryTypeUnknown QueryType = iota
	QueryTypeID
	QueryTypeSeq
	QueryTypeLatest
	QueryTypeFilter
)

type PjskCardQueryInfo struct {
	Type        QueryType
	Value       int
	Sequence    int
	CharacterID int
	Unit        string
	MainUnit    string
	SupportUnit string
	Rarity      string
	Attr        string
	SkillType   string
	SupplyType  string
	Year        int
	EventID     int
	BanCharID   int
	BanSeq      int
	Original    string
}

type Parser struct {
	extractor *Extractor
}

// ── Extractor ───────────────────────────────────────────────────────────────

type Extractor struct {
	nicknames    map[string]int
	banNicknames map[string]int
}

type BanEventRef struct {
	CharacterID int
	Sequence    int
}

type ExtractResult[T any] struct {
	Value             T
	Remaining         string
	Found             bool
	PrefixTightlyJoin bool
	SuffixTightlyJoin bool
}

const (
	SupplyNormal   = "normal"
	SupplyLimited  = "limited"
	SupplyFes      = "festival"
	SupplyCFes     = "colorful_festival_limited"
	SupplyBFes     = "bloom_festival_limited"
	SupplyCollab   = "collaboration_limited"
	SupplyBirthday = "birthday"
)

// ── Query types ─────────────────────────────────────────────────────────────

type Query struct {
	Query            string                              `json:"query"`
	Region           string                              `json:"region"`
	UserID           string                              `json:"user_id,omitempty"`
	Mode             string                              `json:"mode,omitempty"`
	ShowID           bool                                `json:"show_id,omitempty"`
	ShowBox          bool                                `json:"show_box,omitempty"`
	StrictFilterOnly bool                                `json:"strict_filter_only,omitempty"`
	UseAfterTraining *bool                               `json:"use_after_training,omitempty"`
	Title            *string                             `json:"-"`
	DetailedProfile  *drawing.DetailedProfileCardRequest `json:"-"`
}

type ListRequest struct {
	Query            string                              `json:"query,omitempty"`
	CardIDs          []int                               `json:"card_ids"`
	Region           string                              `json:"region"`
	StrictFilterOnly bool                                `json:"strict_filter_only,omitempty"`
	Title            *string                             `json:"-"`
	DetailedProfile  *drawing.DetailedProfileCardRequest `json:"-"`
}
