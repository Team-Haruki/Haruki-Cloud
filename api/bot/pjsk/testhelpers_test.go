package pjsk

import (
	"encoding/json"
	"fmt"
	"testing"

	onebot11 "haruki-cloud/internal/pjsk/onebot11"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/utils/imagecache"
)

// renderEnvelope is the standard JSON wrapper used by all API error responses.
type renderEnvelope struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// ── Minimal card source for bot handler tests ────────────────────────────────

type botCardSource struct{}

func (s *botCardSource) DefaultRegion() renderregion.Value { return renderregion.JP }

func (s *botCardSource) GetCardByID(id int) (*masterdata.Card, error) {
	if id == 1001 {
		return &masterdata.Card{
			ID: 1001, CharacterID: 5, CardRarityType: "rarity_4",
			Attr: "cute", Prefix: "Test Card", AssetBundleName: "card_1001",
			ReleaseAt: 1700000000000, SkillID: 9001, CardSkillName: "Score Up",
			CardParameters: []masterdata.CardParameter{
				{CardParameterType: "param1", Power: 100},
				{CardParameterType: "param2", Power: 200},
				{CardParameterType: "param3", Power: 300},
			},
		}, nil
	}
	return nil, onebot11.NewReplayError("card %d not found", id)
}

func (s *botCardSource) GetCardByCharacterAndSeq(_, _ int) (*masterdata.Card, error) {
	return nil, fmt.Errorf("not found")
}

func (s *botCardSource) FilterCards(_ *rendercard.CardQueryInfo) ([]*masterdata.Card, error) {
	return nil, nil
}

func (s *botCardSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	if id == 5 {
		return &masterdata.Character{ID: 5, FirstName: "花里", GivenName: "实乃理", Unit: "idol"}, nil
	}
	return nil, onebot11.NewReplayError("character %d not found", id)
}

func (s *botCardSource) GetCharacterColorCode(id int) (string, bool) {
	return "", false
}

func (s *botCardSource) GetUnitByCardID(_ int) (string, error)       { return "idol", nil }
func (s *botCardSource) GetCardSupplyType(_ *masterdata.Card) string { return "normal" }

func (s *botCardSource) GetSkillByID(id int) (*masterdata.Skill, error) {
	if id == 9001 {
		return &masterdata.Skill{ID: 9001, DescriptionSpriteName: "score_up"}, nil
	}
	return nil, onebot11.NewReplayError("skill %d not found", id)
}

func (s *botCardSource) FormatSkillDescription(_ *masterdata.Skill, _ int) string { return "" }

func (s *botCardSource) GetGachaByCardID(_ int) (*masterdata.Gacha, error) {
	return &masterdata.Gacha{ID: 3001, Name: "Test Gacha"}, nil
}

func (s *botCardSource) GetCostume3dsByCardID(_ int) ([]*masterdata.Costume3d, error) {
	return nil, nil
}

// ── Minimal event source (required by card.NewController) ───────────────────

type botEventSource struct{}

func (s *botEventSource) DefaultRegion() renderregion.Value { return renderregion.JP }
func (s *botEventSource) GetEventByID(_ int) (*masterdata.Event, error) {
	return nil, fmt.Errorf("not found")
}
func (s *botEventSource) GetEventByCardID(_ int) (*masterdata.Event, error) {
	return nil, fmt.Errorf("not found")
}
func (s *botEventSource) GetEvents() []*masterdata.Event                  { return nil }
func (s *botEventSource) GetEventCards(_ int) ([]*masterdata.Card, error) { return nil, nil }
func (s *botEventSource) GetEventBannerCharacterID(_ int) (int, error)    { return 0, nil }
func (s *botEventSource) GetEventDeckBonuses(_ int) ([]*masterdata.EventDeckBonus, error) {
	return nil, nil
}
func (s *botEventSource) GetGameCharacterUnit(_ int) (*masterdata.GameCharacterUnit, error) {
	return nil, fmt.Errorf("not found")
}
func (s *botEventSource) GetBanEvents(_ int) []*masterdata.Event               { return nil }
func (s *botEventSource) GetWorldBloomChapters(_ int) []*masterdata.WorldBloom { return nil }
func (s *botEventSource) GetCharacterByID(_ int) (*masterdata.Character, error) {
	return nil, fmt.Errorf("not found")
}

// ── testRenderApp creates a minimal render app for bot tests ─────────────────

func testRenderApp(t *testing.T, drawingClient *drawing.HarukiDrawingClient) *renderapp.App {
	t.Helper()
	cardController := rendercard.NewController(
		&botCardSource{},
		&botEventSource{},
		drawingClient,
		assets.NewAssetHelper("", nil),
	)
	return &renderapp.App{
		Drawing:    drawingClient,
		Cards:      cardController,
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}
}

// Ensure botCardSource implements rendercard.DataSource.
var _ rendercard.DataSource = (*botCardSource)(nil)

// Ensure botEventSource implements renderevent.DataSource.
var _ renderevent.DataSource = (*botEventSource)(nil)
