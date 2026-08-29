//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package card

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

type adapterCoverageMySekaiProvider struct {
	configured bool
	lists      map[string][]map[string]any
	maps       map[string]map[int]map[string]any
}

type adapterCoverageContextKey struct{}

func (p *adapterCoverageMySekaiProvider) Configured() bool { return p != nil && p.configured }

func (p *adapterCoverageMySekaiProvider) LoadList(filename string) []map[string]any {
	return p.lists[filename]
}

func (p *adapterCoverageMySekaiProvider) LoadMapByID(filename string) map[int]map[string]any {
	return p.maps[filename]
}

func (*adapterCoverageMySekaiProvider) LoadObject(string, any) bool { return false }

type adapterCoverageMasterDataProvider struct {
	*adapterTestMasterDataProvider
	mysekai provider.MySekaiProvider
}

func (p *adapterCoverageMasterDataProvider) MySekai() provider.MySekaiProvider { return p.mysekai }

func TestProviderAdapterContextWrappersAndLocalProviderBehavior(t *testing.T) {
	var nilAdapter *ProviderAdapter
	if nilAdapter.WithContext(context.Background()) != nil {
		t.Fatal("nil adapter produced a contextual data source")
	}

	root := t.TempDir()
	fixtures := map[string]any{
		"cards.json": []masterdata.Card{
			{ID: 1, CharacterID: 5, SkillID: 10, CardRarityType: "rarity_4", Attr: "cute", AssetBundleName: "card_1", ReleaseAt: 1},
			{ID: 2, CharacterID: 5, SkillID: 10, CardRarityType: "rarity_3", Attr: "cool", AssetBundleName: "card_2", ReleaseAt: 2},
		},
		"gameCharacters.json": []map[string]any{
			{"id": 5, "firstName": "花里", "givenName": "实乃理", "unit": "idol"},
		},
		"gameCharacterUnits.json": []masterdata.GameCharacterUnit{
			{ID: 50, GameCharacterID: 5, Unit: "idol", ColorCode: "#abcdef"},
		},
		"skills.json": []masterdata.Skill{
			{ID: 10, Description: "plain description", DescriptionSpriteName: "score_up"},
		},
		"cardEpisodes.json": []masterdata.CardEpisode{
			{ID: 100, Seq: 1, CardID: 1},
			{ID: 0, Seq: 2, CardID: 1},
		},
		"areaItems.json": []provider.AreaItem{
			{ID: 11, Name: "item"},
			{ID: 12, Name: "empty"},
		},
		"areaItemLevels.json": []provider.AreaItemLevel{
			{AreaItemID: 11, Level: 1},
			{AreaItemID: 11, Level: 3},
			{AreaItemID: 11, Level: 2},
		},
	}
	for filename, value := range fixtures {
		writeAdapterProviderJSON(t, filepath.Join(root, filename), value)
	}

	adapter := NewProviderAdapter(provider.NewLocalProvider(root, renderregion.JP))
	contextual := adapter.WithContext(nil)
	if contextual == nil || contextual.DefaultRegion() != renderregion.JP {
		t.Fatalf("contextual adapter = %#v", contextual)
	}
	contextual = adapter.WithContext(context.WithValue(context.Background(), adapterCoverageContextKey{}, "bound"))
	if contextual == nil {
		t.Fatal("non-nil context produced nil adapter")
	}

	card, err := adapter.GetCardByID(1)
	if err != nil || card == nil || card.ID != 1 {
		t.Fatalf("GetCardByID() = %+v, %v", card, err)
	}
	card, err = adapter.GetCardByCharacterAndSeq(5, 2)
	if err != nil || card == nil || card.ID != 2 {
		t.Fatalf("GetCardByCharacterAndSeq() = %+v, %v", card, err)
	}
	all, err := adapter.GetAllCards()
	if err != nil || len(all) != 2 {
		t.Fatalf("GetAllCards() = %+v, %v", all, err)
	}
	if _, err := adapter.FilterCards(nil); err == nil {
		t.Fatal("nil card filter was accepted")
	}
	if _, err := adapter.FilterCards(&PjskCardQueryInfo{BanCharID: 5, BanSeq: 1}); err == nil {
		t.Fatal("missing ban event was accepted")
	}

	if caps := adapter.AreaItemLevelCaps(2); !reflect.DeepEqual(caps, map[int]int{11: 2, 12: 0}) {
		t.Fatalf("AreaItemLevelCaps(2) = %+v", caps)
	}
	if caps := adapter.AreaItemLevelCaps(0); caps[11] != 3 {
		t.Fatalf("AreaItemLevelCaps(0) = %+v", caps)
	}
	if color, ok := adapter.GetCharacterColorCode(5); !ok || color != "#abcdef" {
		t.Fatalf("GetCharacterColorCode() = %q, %v", color, ok)
	}
	character, err := adapter.GetCharacterByID(5)
	if err != nil || character == nil || character.Unit != "idol" {
		t.Fatalf("GetCharacterByID() = %+v, %v", character, err)
	}
	if unit, err := adapter.GetUnitByCardID(1); err != nil || unit != "idol" {
		t.Fatalf("GetUnitByCardID() = %q, %v", unit, err)
	}

	episodes, err := adapter.GetCardEpisodes(1)
	if err != nil || !reflect.DeepEqual(episodes, []snapshot.RawUserCardEpisode{{CardEpisodeID: 100, ScenarioStatus: "already_read"}}) {
		t.Fatalf("GetCardEpisodes(1) = %+v, %v", episodes, err)
	}
	if episodes, err := adapter.GetCardEpisodes(999); err != nil || episodes != nil {
		t.Fatalf("GetCardEpisodes(999) = %+v, %v", episodes, err)
	}
	unsupported := NewProviderAdapter(&adapterTestMasterDataProvider{cards: &adapterTestCardProvider{}})
	if _, err := unsupported.GetCardEpisodes(1); err == nil {
		t.Fatal("provider without episode support was accepted")
	}

	if got := adapter.GetCardSupplyType(card); got == "" {
		t.Fatal("expected normalized supply type")
	}
	skill, err := adapter.GetSkillByID(10)
	if err != nil || skill == nil {
		t.Fatalf("GetSkillByID() = %+v, %v", skill, err)
	}
	if got := adapter.FormatSkillDescription(skill, 5); got != "plain description" {
		t.Fatalf("FormatSkillDescription() = %q", got)
	}
	if _, err := adapter.GetGachaByCardID(1); err == nil {
		t.Fatal("missing gacha unexpectedly resolved")
	}
	if _, err := adapter.GetCostume3dsByCardID(1); err == nil {
		t.Fatal("missing costume masterdata unexpectedly resolved")
	}
}

func TestProviderAdapterMySekaiGuardAndInvalidFixtureBranches(t *testing.T) {
	for _, adapter := range []*ProviderAdapter{
		nil,
		{},
		NewProviderAdapter(&adapterTestMasterDataProvider{}),
		NewProviderAdapter(&adapterCoverageMasterDataProvider{
			adapterTestMasterDataProvider: &adapterTestMasterDataProvider{},
			mysekai:                       &adapterCoverageMySekaiProvider{},
		}),
	} {
		if got := adapter.GetMaxProfileMysekaiGates(); got != nil {
			t.Fatalf("guarded gate result = %+v", got)
		}
		if got := adapter.GetMaxProfileMysekaiFixtureBonuses(); got != nil {
			t.Fatalf("guarded fixture result = %+v", got)
		}
	}

	mysekai := &adapterCoverageMySekaiProvider{
		configured: true,
		lists: map[string][]map[string]any{
			"mysekaiFixtures.json": {
				{"mysekaiFixtureGameCharacterGroupPerformanceBonusId": "bad"},
				{"mysekaiFixtureGameCharacterGroupPerformanceBonusId": 0},
				{"mysekaiFixtureGameCharacterGroupPerformanceBonusId": 10},
				{"mysekaiFixtureGameCharacterGroupPerformanceBonusId": 20},
				{"mysekaiFixtureGameCharacterGroupPerformanceBonusId": 30},
				{"mysekaiFixtureGameCharacterGroupPerformanceBonusId": 40},
				{"mysekaiFixtureGameCharacterGroupPerformanceBonusId": 50},
				{"mysekaiFixtureGameCharacterGroupPerformanceBonusId": 60},
				{"mysekaiFixtureGameCharacterGroupPerformanceBonusId": 70},
			},
		},
		maps: map[string]map[int]map[string]any{
			"mysekaiFixtureGameCharacterGroupPerformanceBonuses.json": {
				20: {"mysekaiFixtureGameCharacterGroupId": "bad"},
				30: {"mysekaiFixtureGameCharacterGroupId": 300},
				40: {"mysekaiFixtureGameCharacterGroupId": 400},
				50: {"mysekaiFixtureGameCharacterGroupId": 500, "bonusRate": "bad"},
				60: {"mysekaiFixtureGameCharacterGroupId": 600, "bonusRate": -1},
				70: {"mysekaiFixtureGameCharacterGroupId": 700, "bonusRate": 12.5},
			},
			"mysekaiFixtureGameCharacterGroups.json": {
				400: {"gameCharacterId": "bad"},
				500: {"gameCharacterId": 5},
				600: {"gameCharacterId": 6},
				700: {"gameCharacterId": 7},
			},
		},
	}
	adapter := NewProviderAdapter(&adapterCoverageMasterDataProvider{
		adapterTestMasterDataProvider: &adapterTestMasterDataProvider{},
		mysekai:                       mysekai,
	})
	if got := adapter.GetMaxProfileMysekaiFixtureBonuses(); !reflect.DeepEqual(got, []snapshot.RawUserFixtureBonus{{GameCharacterID: 7, TotalBonusRate: 12.5}}) {
		t.Fatalf("filtered fixture bonuses = %+v", got)
	}

	mysekai.lists["mysekaiFixtures.json"] = nil
	if got := adapter.GetMaxProfileMysekaiFixtureBonuses(); got != nil {
		t.Fatalf("empty fixture list result = %+v", got)
	}
}

func TestMaxProfileNumberConvertersCoverSupportedRepresentations(t *testing.T) {
	intTests := []struct {
		value any
		want  int
		ok    bool
	}{
		{value: int(1), want: 1, ok: true},
		{value: int32(2), want: 2, ok: true},
		{value: int64(3), want: 3, ok: true},
		{value: float64(4.9), want: 4, ok: true},
		{value: json.Number("5"), want: 5, ok: true},
		{value: json.Number("bad")},
		{value: " 6 ", want: 6, ok: true},
		{value: "bad"},
		{value: true},
	}
	for _, tt := range intTests {
		got, ok := maxProfileNumberToInt(tt.value)
		if got != tt.want || ok != tt.ok {
			t.Errorf("maxProfileNumberToInt(%#v) = %d, %v; want %d, %v", tt.value, got, ok, tt.want, tt.ok)
		}
	}

	floatTests := []struct {
		value any
		want  float64
		ok    bool
	}{
		{value: float64(1.5), want: 1.5, ok: true},
		{value: float32(2.5), want: 2.5, ok: true},
		{value: int(3), want: 3, ok: true},
		{value: int32(4), want: 4, ok: true},
		{value: int64(5), want: 5, ok: true},
		{value: json.Number("6.5"), want: 6.5, ok: true},
		{value: json.Number("bad")},
		{value: " 7.5 ", want: 7.5, ok: true},
		{value: "bad"},
		{value: math.NaN(), want: math.NaN(), ok: true},
		{value: errors.New("bad")},
	}
	for _, tt := range floatTests {
		got, ok := maxProfileNumberToFloat64(tt.value)
		valuesMatch := got == tt.want || math.IsNaN(got) && math.IsNaN(tt.want)
		if !valuesMatch || ok != tt.ok {
			t.Errorf("maxProfileNumberToFloat64(%#v) = %v, %v; want %v, %v", tt.value, got, ok, tt.want, tt.ok)
		}
	}
}
