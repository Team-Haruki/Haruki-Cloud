package provider

import (
	"context"
	"encoding/json"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ── Interface compliance (compile-time checks) ──────────────────────────

var _ MasterDataProvider = (*DatabaseProvider)(nil)
var _ CardProvider = (*dbCardProvider)(nil)
var _ CharacterProvider = (*dbCharacterProvider)(nil)
var _ SkillProvider = (*dbSkillProvider)(nil)
var _ EventProvider = (*dbEventProvider)(nil)
var _ MusicProvider = (*dbMusicProvider)(nil)
var _ GachaProvider = (*dbGachaProvider)(nil)
var _ HonorProvider = (*dbHonorProvider)(nil)
var _ StampProvider = (*dbStampProvider)(nil)
var _ VLiveProvider = (*dbVLiveProvider)(nil)
var _ EducationProvider = (*dbEducationProvider)(nil)
var _ PlayerFrameProvider = (*dbPlayerFrameProvider)(nil)
var _ MySekaiProvider = (*dbMySekaiProvider)(nil)

func TestDatabaseProviderImplementsMasterDataProvider(t *testing.T) {
	// Compile-time assertions above guarantee this; run-time confirmation.
	var p MasterDataProvider = &DatabaseProvider{}
	_ = p // interface satisfaction verified at compile time
}

// ── NewDatabaseProvider ─────────────────────────────────────────────────

func TestNewDatabaseProviderNilClient(t *testing.T) {
	p := NewDatabaseProvider(nil, renderregion.JP)
	if p != nil {
		t.Fatal("expected nil when client is nil")
	}
}

func TestNewDatabaseProviderDefaultsRegion(t *testing.T) {
	// Can't pass a real client, but we can verify the nil guard fires first.
	p := NewDatabaseProvider(nil, renderregion.Unknown)
	if p != nil {
		t.Fatal("expected nil when client is nil")
	}
}

// ── cloneSkill ──────────────────────────────────────────────────────────

func TestCloneSkillNil(t *testing.T) {
	if got := common.CloneSkill(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestCloneSkillBasic(t *testing.T) {
	orig := &masterdata.Skill{
		ID:               42,
		ShortDescription: "score up",
		Description:      "Increase score by {{1;v}}%",
	}
	clone := common.CloneSkill(orig)
	if clone == orig {
		t.Fatal("clone must not share the same pointer")
	}
	if clone.ID != 42 || clone.Description != orig.Description {
		t.Fatal("field values must match")
	}

	// Mutating the clone must not affect the original.
	clone.ID = 999
	if orig.ID != 42 {
		t.Fatal("mutation leaked to original")
	}
}

func TestCloneSkillDeepCopiesEffects(t *testing.T) {
	orig := &masterdata.Skill{
		ID: 1,
		SkillEffects: []masterdata.SkillEffect{
			{ID: 10, SkillEffectType: "score_up"},
			{ID: 20, SkillEffectType: "judgment_up"},
		},
	}
	clone := common.CloneSkill(orig)

	// Slices must be independent.
	clone.SkillEffects[0].ID = 999
	if orig.SkillEffects[0].ID != 10 {
		t.Fatal("SkillEffects slice is not independent")
	}

	clone.SkillEffects = append(clone.SkillEffects, masterdata.SkillEffect{ID: 30})
	if len(orig.SkillEffects) != 2 {
		t.Fatal("appending to clone affected original slice length")
	}
}

func TestCloneSkillEmptyEffects(t *testing.T) {
	orig := &masterdata.Skill{ID: 5}
	clone := common.CloneSkill(orig)
	if len(clone.SkillEffects) != 0 {
		t.Fatal("expected empty SkillEffects")
	}
}

func TestCloneMusicDifficultiesDeepCopy(t *testing.T) {
	orig := []*masterdata.MusicDifficulty{
		{ID: 1, MusicID: 1001, MusicDifficulty: "expert", PlayLevel: 26, TotalNoteCount: 700},
		{ID: 2, MusicID: 1001, MusicDifficulty: "master", PlayLevel: 31, TotalNoteCount: 900},
	}

	clone := common.CloneMusicDifficulties(orig)
	if len(clone) != 2 {
		t.Fatalf("expected 2 items, got %d", len(clone))
	}
	if clone[0] == orig[0] {
		t.Fatal("clone must not share the same pointer")
	}

	clone[0].PlayLevel = 99
	if orig[0].PlayLevel != 26 {
		t.Fatal("mutation leaked to original")
	}
}

// ── cloneCardFull ───────────────────────────────────────────────────────

func TestCloneCardFullNil(t *testing.T) {
	if got := common.CloneCard(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestCloneCardFullDeepCopy(t *testing.T) {
	orig := &masterdata.Card{
		ID:          100,
		CharacterID: 21,
		CardParameters: []masterdata.CardParameter{
			{ID: 1, Power: 5000},
			{ID: 2, Power: 6000},
		},
	}
	clone := common.CloneCard(orig)
	if clone == orig {
		t.Fatal("clone must not share the same pointer")
	}

	clone.CardParameters[0].Power = 9999
	if orig.CardParameters[0].Power != 5000 {
		t.Fatal("CardParameters slice is not independent")
	}
}

func TestCloneCardFullEmptyParams(t *testing.T) {
	orig := &masterdata.Card{ID: 7}
	clone := common.CloneCard(orig)
	if len(clone.CardParameters) != 0 {
		t.Fatal("expected empty CardParameters")
	}
}

// ── cloneCardCostumes ───────────────────────────────────────────────────

func TestCloneCardCostumesNil(t *testing.T) {
	if got := common.CloneCostumes(nil); got != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestCloneCardCostumesEmpty(t *testing.T) {
	if got := common.CloneCostumes([]*masterdata.Costume3d{}); got != nil {
		t.Fatal("expected nil for empty slice")
	}
}

func TestCloneCardCostumesIndependent(t *testing.T) {
	orig := []*masterdata.Costume3d{
		{ID: 1, CharacterID: 21, Description: "outfit1"},
		{ID: 2, CharacterID: 22, Description: "outfit2"},
	}
	clone := common.CloneCostumes(orig)
	if len(clone) != 2 {
		t.Fatalf("expected 2 items, got %d", len(clone))
	}
	clone[0].Description = "changed"
	if orig[0].Description != "outfit1" {
		t.Fatal("mutation leaked to original")
	}
}

func TestCloneCardCostumesSkipsNils(t *testing.T) {
	orig := []*masterdata.Costume3d{nil, {ID: 3}}
	clone := common.CloneCostumes(orig)
	if len(clone) != 1 || clone[0].ID != 3 {
		t.Fatal("nil entries should be skipped")
	}
}

func TestLocalCostume3dJSONToModelSupportsLegacyAssetBundleName(t *testing.T) {
	var payload localCostume3dJSON
	if err := json.Unmarshal([]byte(`{
		"id": 1,
		"characterId": 21,
		"name": "legacy costume",
		"partType": "head",
		"colorId": 3,
		"_assetbundleName": "head_default_01"
	}`), &payload); err != nil {
		t.Fatalf("unmarshal localCostume3dJSON: %v", err)
	}

	model := payload.toModel()
	if model == nil {
		t.Fatal("expected costume model")
	}
	if model.AssetBundleName != "head_default_01" {
		t.Fatalf("expected legacy asset bundle name, got %q", model.AssetBundleName)
	}
	if model.PartType != "head" {
		t.Fatalf("expected partType=head, got %q", model.PartType)
	}
	if model.ColorID != 3 {
		t.Fatalf("expected colorId=3, got %d", model.ColorID)
	}
}

func TestDBMusicProviderGetDifficultiesUsesCachedClone(t *testing.T) {
	provider := &dbMusicProvider{}
	provider.init()
	provider.difficultiesByID[1001] = []*masterdata.MusicDifficulty{
		{ID: 1, MusicID: 1001, MusicDifficulty: "expert", PlayLevel: 26, TotalNoteCount: 700},
	}

	got, err := provider.GetDifficulties(context.Background(), 1001)
	if err != nil {
		t.Fatalf("GetDifficulties() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 difficulty, got %d", len(got))
	}
	if got[0] == provider.difficultiesByID[1001][0] {
		t.Fatal("cached difficulty should be cloned before returning")
	}

	got[0].PlayLevel = 99
	if provider.difficultiesByID[1001][0].PlayLevel != 26 {
		t.Fatal("mutation leaked into provider cache")
	}
}

// ── formatEffectValues ──────────────────────────────────────────────────

func TestFormatEffectValuesEmpty(t *testing.T) {
	if got := formatEffectValues(nil); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
	if got := formatEffectValues([]int{}); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestFormatEffectValuesSingle(t *testing.T) {
	if got := formatEffectValues([]int{120}); got != "120" {
		t.Fatalf("expected '120', got %q", got)
	}
}

func TestFormatEffectValuesAllSame(t *testing.T) {
	if got := formatEffectValues([]int{50, 50, 50}); got != "50" {
		t.Fatalf("expected '50', got %q", got)
	}
}

func TestFormatEffectValuesDifferent(t *testing.T) {
	got := formatEffectValues([]int{100, 110, 120})
	if got != "100/110/120" {
		t.Fatalf("expected '100/110/120', got %q", got)
	}
}

func TestFormatEffectValuesDeduplicated(t *testing.T) {
	got := formatEffectValues([]int{100, 110, 100, 120, 110})
	if got != "100/110/120" {
		t.Fatalf("expected '100/110/120', got %q", got)
	}
}

// ── getEffectValues ─────────────────────────────────────────────────────

func TestGetEffectValuesNil(t *testing.T) {
	got := getEffectValues(nil)
	if len(got) != 1 || got[0] != 0 {
		t.Fatalf("expected [0], got %v", got)
	}
}

func TestGetEffectValuesNoDetails(t *testing.T) {
	got := getEffectValues(&masterdata.SkillEffect{})
	if len(got) != 1 || got[0] != 0 {
		t.Fatalf("expected [0], got %v", got)
	}
}

func TestGetEffectValuesWithDetails(t *testing.T) {
	effect := &masterdata.SkillEffect{
		SkillEffectDetails: []masterdata.SkillEffectDetail{
			{ActivateEffectValue: 100},
			{ActivateEffectValue: 110},
			{ActivateEffectValue: 120},
			{ActivateEffectValue: 130},
		},
	}
	got := getEffectValues(effect)
	expected := []int{100, 110, 120, 130}
	if len(got) != len(expected) {
		t.Fatalf("length mismatch: got %v", got)
	}
	for i, v := range expected {
		if got[i] != v {
			t.Fatalf("index %d: expected %d, got %d", i, v, got[i])
		}
	}
}

// ── getEnhancedValues ───────────────────────────────────────────────────

func TestGetEnhancedValuesNil(t *testing.T) {
	got := getEnhancedValues(nil, []int{1, 2})
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestGetEnhancedValuesUsesValue2(t *testing.T) {
	effect := &masterdata.SkillEffect{
		SkillEffectDetails: []masterdata.SkillEffectDetail{
			{ActivateEffectValue: 100, ActivateEffectValue2: new(200)},
		},
	}
	got := getEnhancedValues(effect, []int{100})
	if len(got) != 1 || got[0] != 200 {
		t.Fatalf("expected [200], got %v", got)
	}
}

func TestGetEnhancedValuesFallsBackToBase(t *testing.T) {
	effect := &masterdata.SkillEffect{
		SkillEffectDetails: []masterdata.SkillEffectDetail{
			{ActivateEffectValue: 100},
			{ActivateEffectValue: 110},
		},
	}
	base := []int{50, 60}
	got := getEnhancedValues(effect, base)
	if len(got) != 2 || got[0] != 50 || got[1] != 60 {
		t.Fatalf("expected [50 60], got %v", got)
	}
}

func TestGetEnhancedValuesMixed(t *testing.T) {
	effect := &masterdata.SkillEffect{
		SkillEffectDetails: []masterdata.SkillEffectDetail{
			{ActivateEffectValue: 100, ActivateEffectValue2: new(300)},
			{ActivateEffectValue: 110},
		},
	}
	base := []int{100, 110}
	got := getEnhancedValues(effect, base)
	if len(got) != 2 || got[0] != 300 || got[1] != 110 {
		t.Fatalf("expected [300 110], got %v", got)
	}
}

// ── formatSingleEffect ─────────────────────────────────────────────────

func TestFormatSingleEffectDurationMode(t *testing.T) {
	effect := &masterdata.SkillEffect{
		SkillEffectDetails: []masterdata.SkillEffectDetail{
			{ActivateEffectDuration: 5.0},
		},
	}
	got := formatSingleEffect(effect, "d")
	if got != "5.0" {
		t.Fatalf("expected '5.0', got %q", got)
	}
}

func TestFormatSingleEffectDurationEmpty(t *testing.T) {
	effect := &masterdata.SkillEffect{}
	got := formatSingleEffect(effect, "d")
	if got != "0.0" {
		t.Fatalf("expected '0.0', got %q", got)
	}
}

func TestFormatSingleEffectValueMode(t *testing.T) {
	effect := &masterdata.SkillEffect{
		SkillEffectDetails: []masterdata.SkillEffectDetail{
			{ActivateEffectValue: 120},
			{ActivateEffectValue: 130},
		},
	}
	got := formatSingleEffect(effect, "v")
	if got != "120/130" {
		t.Fatalf("expected '120/130', got %q", got)
	}
}

func TestFormatSingleEffectEnhanceMode(t *testing.T) {
	effect := &masterdata.SkillEffect{
		SkillEnhance: masterdata.SkillEnhance{ActivateEffectValue: 10},
	}
	got := formatSingleEffect(effect, "e")
	if got != "10" {
		t.Fatalf("expected '10', got %q", got)
	}
}

func TestFormatSingleEffectMaxMode(t *testing.T) {
	effect := &masterdata.SkillEffect{
		SkillEffectDetails: []masterdata.SkillEffectDetail{
			{ActivateEffectValue: 100},
			{ActivateEffectValue: 110},
		},
		SkillEnhance: masterdata.SkillEnhance{ActivateEffectValue: 2},
	}
	// m mode: value + SkillEnhance*5 for each detail
	got := formatSingleEffect(effect, "m")
	// 100+10=110, 110+10=120
	if got != "110/120" {
		t.Fatalf("expected '110/120', got %q", got)
	}
}

func TestFormatSingleEffectUnknownMode(t *testing.T) {
	effect := &masterdata.SkillEffect{}
	got := formatSingleEffect(effect, "z")
	if got != "?" {
		t.Fatalf("expected '?', got %q", got)
	}
}

// ── formatDualEffects ───────────────────────────────────────────────────

func TestFormatDualEffectsValueMode(t *testing.T) {
	e1 := &masterdata.SkillEffect{
		SkillEffectDetails: []masterdata.SkillEffectDetail{
			{ActivateEffectValue: 100},
			{ActivateEffectValue: 110},
		},
	}
	e2 := &masterdata.SkillEffect{
		SkillEffectDetails: []masterdata.SkillEffectDetail{
			{ActivateEffectValue: 50},
			{ActivateEffectValue: 60},
		},
	}
	got := formatDualEffects(e1, e2, "v")
	// sums: 150, 170
	if got != "150/170" {
		t.Fatalf("expected '150/170', got %q", got)
	}
}

func TestFormatDualEffectsValueModeAllSame(t *testing.T) {
	e1 := &masterdata.SkillEffect{
		SkillEffectDetails: []masterdata.SkillEffectDetail{
			{ActivateEffectValue: 100},
		},
	}
	e2 := &masterdata.SkillEffect{
		SkillEffectDetails: []masterdata.SkillEffectDetail{
			{ActivateEffectValue: 50},
		},
	}
	got := formatDualEffects(e1, e2, "v")
	if got != "150" {
		t.Fatalf("expected '150', got %q", got)
	}
}

func TestFormatDualEffectsEnhancedModeU(t *testing.T) {
	e1 := &masterdata.SkillEffect{
		SkillEffectDetails: []masterdata.SkillEffectDetail{
			{ActivateEffectValue: 100, ActivateEffectValue2: new(200)},
		},
	}
	e2 := &masterdata.SkillEffect{
		SkillEffectDetails: []masterdata.SkillEffectDetail{
			{ActivateEffectValue: 50, ActivateEffectValue2: new(80)},
		},
	}
	got := formatDualEffects(e1, e2, "u")
	// enhanced: 200+80 = 280
	if got != "280" {
		t.Fatalf("expected '280', got %q", got)
	}
}

func TestFormatDualEffectsRangeMode(t *testing.T) {
	e1 := &masterdata.SkillEffect{}
	e2 := &masterdata.SkillEffect{}
	for _, mode := range []string{"r", "s"} {
		got := formatDualEffects(e1, e2, mode)
		if got != "..." {
			t.Fatalf("mode %q: expected '...', got %q", mode, got)
		}
	}
}

func TestFormatDualEffectsUnknownMode(t *testing.T) {
	e1 := &masterdata.SkillEffect{}
	e2 := &masterdata.SkillEffect{}
	got := formatDualEffects(e1, e2, "z")
	if got != "?" {
		t.Fatalf("expected '?', got %q", got)
	}
}

// ── convertSkillEntity ──────────────────────────────────────────────────

func TestConvertSkillEntityNil(t *testing.T) {
	_, err := common.ConvertSkillEntity(nil)
	if err == nil {
		t.Fatal("expected error for nil entity")
	}
}

// ── card helpers ────────────────────────────────────────────────────────

func TestCardNormalizeUnit(t *testing.T) {
	tests := []struct{ in, want string }{
		{"light_sound", "light_sound"},
		{"Light_Sound_Club", "light_sound"},
		{"idol", "idol"},
		{"more_more_jump", "idol"},
		{"street", "street"},
		{"vivid_bad_squad", "street"},
		{"theme_park", "theme_park"},
		{"wonderlands_x_showtime", "theme_park"},
		{"school_refusal", "school_refusal"},
		{"25_ji_night_cord_de", "school_refusal"},
		{"piapro", "piapro"},
		{"", ""},
		{"none", ""},
		{"  Light_Sound  ", "light_sound"},
	}
	for _, tt := range tests {
		if got := cardNormalizeUnit(tt.in); got != tt.want {
			t.Errorf("cardNormalizeUnit(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCardNormalizeSupportUnit(t *testing.T) {
	if got := cardNormalizeSupportUnit(""); got != "none" {
		t.Errorf("expected 'none', got %q", got)
	}
	if got := cardNormalizeSupportUnit("none"); got != "none" {
		t.Errorf("expected 'none', got %q", got)
	}
	if got := cardNormalizeSupportUnit("idol"); got != "idol" {
		t.Errorf("expected 'idol', got %q", got)
	}
}

func TestCardNormalizeSupplyType(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "normal"},
		{"normal", "normal"},
		{"not_limited", "normal"},
		{"term_limited", "term_limited"},
		{"festival_limited", "colorful_festival_limited"},
		{"colorful_festival_limited", "colorful_festival_limited"},
		{"bloom_festival_limited", "bloom_festival_limited"},
		{"unit_event_limited", "unit_event_limited"},
		{"collaboration_limited", "collaboration_limited"},
		{"birthday", "birthday"},
		{"rarity_birthday", "birthday"},
		{"something_else", "something_else"},
	}
	for _, tt := range tests {
		if got := cardNormalizeSupplyType(tt.in); got != tt.want {
			t.Errorf("cardNormalizeSupplyType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsWorldLink3Event(t *testing.T) {
	tests := []struct {
		name string
		ev   *masterdata.Event
		want bool
	}{
		{name: "nil", ev: nil, want: false},
		{name: "normal world bloom unit", ev: &masterdata.Event{EventType: "world_bloom", Unit: "idol"}, want: false},
		{name: "world link 3", ev: &masterdata.Event{EventType: "world_bloom", Unit: "none"}, want: true},
		{name: "non world bloom none unit", ev: &masterdata.Event{EventType: "marathon", Unit: "none"}, want: false},
	}
	for _, tt := range tests {
		if got := isWorldLink3Event(tt.ev); got != tt.want {
			t.Errorf("isWorldLink3Event(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCardMatchesSupplyFilter(t *testing.T) {
	tests := []struct {
		filter, raw string
		want        bool
	}{
		{"festival", "colorful_festival_limited", true},
		{"festival", "bloom_festival_limited", true},
		{"colorful_festival_limited", "colorful_festival_limited", true},
		{"colorful_festival_limited", "bloom_festival_limited", false},
		{"bloom_festival_limited", "bloom_festival_limited", true},
		{"bloom_festival_limited", "colorful_festival_limited", false},
		{"unit_event_limited", "unit_event_limited", true},
		{"unit_event_limited", "term_limited", false},
		{"limited", "colorful_festival_limited", true},
		{"limited", "term_limited", true},
		{"limited", "unit_event_limited", true},
		{"limited", "collaboration_limited", true},
		{"collaboration_limited", "collaboration_limited", true},
		{"collaboration_limited", "term_limited", false},
		{"limited", "bloom_festival_limited", true},
		{"birthday", "birthday", true},
		{"birthday", "rarity_birthday", true},
		{"normal", "normal", true},
		{"normal", "", true},
		{"normal", "not_limited", true},
		{"festival", "normal", false},
		{"birthday", "normal", false},
		{"normal", "term_limited", false},
	}
	for _, tt := range tests {
		if got := cardMatchesSupplyFilter(tt.filter, tt.raw); got != tt.want {
			t.Errorf("cardMatchesSupplyFilter(%q, %q) = %v, want %v", tt.filter, tt.raw, got, tt.want)
		}
	}
}

func TestLocalCardProviderGetSupplyTypeTreatsWorldLink3TermLimitedAsWL(t *testing.T) {
	provider := &localCardProvider{}
	provider.supplies.init(func() (map[int]string, error) {
		return map[int]string{3: "term_limited"}, nil
	})
	provider.eventCards.init(func() (cardEventIndex, error) {
		return cardEventIndex{
			worldLink3ByCard: map[int]bool{
				1374: true,
			},
		}, nil
	})

	if got := provider.GetSupplyType(context.Background(), &masterdata.Card{ID: 1374, CardSupplyID: 3}); got != "unit_event_limited" {
		t.Fatalf("expected WL3 card to normalize as unit_event_limited, got %q", got)
	}
	if got := provider.GetSupplyType(context.Background(), &masterdata.Card{ID: 2000, CardSupplyID: 3}); got != "term_limited" {
		t.Fatalf("expected non-WL3 term limited card to stay term_limited, got %q", got)
	}
}

func TestCardContainsPickup(t *testing.T) {
	g := &masterdata.Gacha{
		GachaPickups: []masterdata.GachaPickup{
			{CardID: 10},
			{CardID: 20},
		},
	}
	if !cardContainsPickup(g, 10) {
		t.Error("expected true for card 10")
	}
	if cardContainsPickup(g, 30) {
		t.Error("expected false for card 30")
	}
}

// ── FormatDescription ───────────────────────────────────────────────────

func TestFormatDescriptionNilSkill(t *testing.T) {
	sp := &dbSkillProvider{}
	got := sp.FormatDescription(context.Background(), nil, 1)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestFormatDescriptionNoPlaceholders(t *testing.T) {
	sp := &dbSkillProvider{}
	skill := &masterdata.Skill{Description: "Score up by 100%%"}
	got := sp.FormatDescription(context.Background(), skill, 1)
	if got != "Score up by 100%%" {
		t.Fatalf("expected unchanged string, got %q", got)
	}
}

func TestFormatDescriptionValuePlaceholder(t *testing.T) {
	sp := &dbSkillProvider{}
	skill := &masterdata.Skill{
		Description: "Score up by {{1;v}}%%",
		SkillEffects: []masterdata.SkillEffect{
			{
				ID: 1,
				SkillEffectDetails: []masterdata.SkillEffectDetail{
					{ActivateEffectValue: 120},
				},
			},
		},
	}
	got := sp.FormatDescription(context.Background(), skill, 1)
	if got != "Score up by 120%%" {
		t.Fatalf("expected 'Score up by 120%%%%', got %q", got)
	}
}

func TestFormatDescriptionDurationPlaceholder(t *testing.T) {
	sp := &dbSkillProvider{}
	skill := &masterdata.Skill{
		Description: "for {{1;d}} seconds",
		SkillEffects: []masterdata.SkillEffect{
			{
				ID: 1,
				SkillEffectDetails: []masterdata.SkillEffectDetail{
					{ActivateEffectDuration: 5.0},
				},
			},
		},
	}
	got := sp.FormatDescription(context.Background(), skill, 1)
	if got != "for 5.0 seconds" {
		t.Fatalf("expected 'for 5.0 seconds', got %q", got)
	}
}

func TestFormatDescriptionMissingEffect(t *testing.T) {
	sp := &dbSkillProvider{}
	skill := &masterdata.Skill{
		Description:  "Score up by {{99;v}}%%",
		SkillEffects: []masterdata.SkillEffect{},
	}
	got := sp.FormatDescription(context.Background(), skill, 1)
	if got != "Score up by ?%%" {
		t.Fatalf("expected 'Score up by ?%%%%', got %q", got)
	}
}

func TestFormatDescriptionMalformedPlaceholder(t *testing.T) {
	sp := &dbSkillProvider{}
	skill := &masterdata.Skill{Description: "test {{bad_format}} end"}
	got := sp.FormatDescription(context.Background(), skill, 1)
	// No semicolon → parts != 2 → returns original match
	if got != "test {{bad_format}} end" {
		t.Fatalf("expected unchanged, got %q", got)
	}
}

func TestFormatDescriptionDualValuePlaceholder(t *testing.T) {
	sp := &dbSkillProvider{}
	skill := &masterdata.Skill{
		Description: "bonus {{1,2;v}}%%",
		SkillEffects: []masterdata.SkillEffect{
			{
				ID: 1,
				SkillEffectDetails: []masterdata.SkillEffectDetail{
					{ActivateEffectValue: 100},
				},
			},
			{
				ID: 2,
				SkillEffectDetails: []masterdata.SkillEffectDetail{
					{ActivateEffectValue: 50},
				},
			},
		},
	}
	got := sp.FormatDescription(context.Background(), skill, 1)
	// dual effects v mode: 100+50 = 150
	if got != "bonus 150%%" {
		t.Fatalf("expected 'bonus 150%%%%', got %q", got)
	}
}

func TestFormatDescriptionCharacterMode(t *testing.T) {
	// Without a character provider, should return "???"
	sp := &dbSkillProvider{}
	skill := &masterdata.Skill{
		Description: "{{1;c}} skill",
	}
	got := sp.FormatDescription(context.Background(), skill, 1)
	if got != "??? skill" {
		t.Fatalf("expected '??? skill', got %q", got)
	}
}
