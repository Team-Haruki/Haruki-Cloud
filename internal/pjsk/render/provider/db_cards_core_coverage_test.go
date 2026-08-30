package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	sekaiDB "haruki-cloud/database/sekai"
	sekaienttest "haruki-cloud/database/sekai/enttest"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func newDBCardCoreCoverageProvider(t *testing.T) (*DatabaseProvider, *sekaiDB.Client) {
	t.Helper()
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_card_core_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	for _, character := range []struct {
		id   int64
		unit string
	}{{1, "light_sound"}, {2, "piapro"}, {3, "idol"}} {
		if _, err := client.Gamecharacter.Create().SetGameID(character.id).SetUnit(character.unit).SetServerRegion("jp").Save(ctx); err != nil {
			t.Fatalf("create character %d: %v", character.id, err)
		}
	}
	seedDBCardCoreCoverageCards(t, client)
	for _, skill := range []struct {
		id     int64
		sprite string
	}{{11, "score_up"}, {12, "life_recovery"}} {
		if _, err := client.Skill.Create().SetGameID(skill.id).SetDescriptionSpriteName(skill.sprite).SetServerRegion("jp").Save(ctx); err != nil {
			t.Fatalf("create skill %d: %v", skill.id, err)
		}
	}
	for _, cardID := range []int64{101, 102} {
		if _, err := client.Eventcard.Create().SetGameID(cardID).SetCardID(cardID).SetEventID(50).SetServerRegion("jp").Save(ctx); err != nil {
			t.Fatalf("create event card %d: %v", cardID, err)
		}
	}
	return NewDatabaseProvider(client, renderregion.JP), client
}

func seedDBCardCoreCoverageCards(t *testing.T, client *sekaiDB.Client) {
	t.Helper()
	ctx := context.Background()
	cards := []struct {
		id, characterID, skillID, releaseAt int64
		rarity, attr, prefix, supportUnit   string
	}{
		{101, 1, 11, time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), "rarity_4", "cool", "Alpha", "none"},
		{102, 2, 12, time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), "rarity_3", "cute", "Beta", "idol"},
		{103, 2, 12, time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), "rarity_2", "pure", "Gamma", "none"},
		{1345, 3, 11, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), "rarity_4", "happy", "Limited", "none"},
	}
	for _, item := range cards {
		_, err := client.Card.Create().
			SetGameID(item.id).
			SetCharacterID(item.characterID).
			SetSkillID(item.skillID).
			SetReleaseAt(item.releaseAt).
			SetCardRarityType(item.rarity).
			SetAttr(item.attr).
			SetPrefix(item.prefix).
			SetSupportUnit(item.supportUnit).
			SetServerRegion("jp").
			Save(ctx)
		if err != nil {
			t.Fatalf("create card %d: %v", item.id, err)
		}
	}
	if _, err := client.Card.Create().SetGameID(201).SetCharacterID(1).SetPrefix("TW").SetServerRegion("tw").Save(ctx); err != nil {
		t.Fatalf("create other-region card: %v", err)
	}
}

func TestDBCardCoreLookupAndSequence(t *testing.T) {
	provider, _ := newDBCardCoreCoverageProvider(t)
	ctx := context.Background()
	if _, err := provider.cards.GetByID(ctx, 0); err == nil {
		t.Fatal("GetByID(0) should fail")
	}
	cardInfo, err := provider.cards.GetByID(ctx, 101)
	if err != nil || cardInfo.Prefix != "Alpha" {
		t.Fatalf("GetByID(101) = %+v, %v", cardInfo, err)
	}
	cardInfo.Prefix = "mutated"
	cached, err := provider.cards.GetByID(ctx, 101)
	if err != nil || cached.Prefix != "Alpha" {
		t.Fatalf("cached GetByID(101) = %+v, %v", cached, err)
	}
	if _, err := provider.cards.GetByID(ctx, 999); err == nil {
		t.Fatal("GetByID(999) should fail")
	}
	assertDBCardSequence(t, provider.cards, ctx)
}

func assertDBCardSequence(t *testing.T, cards *dbCardProvider, ctx context.Context) {
	t.Helper()
	if _, err := cards.GetByCharacterAndSeq(ctx, 0, 1); err == nil {
		t.Fatal("missing character ID should fail")
	}
	first, err := cards.GetByCharacterAndSeq(ctx, 2, 1)
	if err != nil || first.ID != 102 {
		t.Fatalf("first card = %+v, %v", first, err)
	}
	last, err := cards.GetByCharacterAndSeq(ctx, 2, -1)
	if err != nil || last.ID != 103 {
		t.Fatalf("last card = %+v, %v", last, err)
	}
	for _, sequence := range []int{-3, 0, 3} {
		if _, err := cards.GetByCharacterAndSeq(ctx, 2, sequence); err == nil {
			t.Fatalf("sequence %d should fail", sequence)
		}
	}
	if _, err := cards.GetByCharacterAndSeq(ctx, 99, 1); err == nil {
		t.Fatal("unknown character should fail")
	}
}

func TestDBCardCoreFilters(t *testing.T) {
	provider, _ := newDBCardCoreCoverageProvider(t)
	ctx := context.Background()
	if _, err := provider.cards.Filter(ctx, nil); err == nil {
		t.Fatal("nil filter should fail")
	}
	filtered, err := provider.cards.Filter(ctx, &CardFilter{
		CharacterID: 2,
		Unit:        "idol",
		Rarity:      "rarity_3",
		Attr:        "cute",
		SkillType:   "life_recovery",
		SkillIDs:    []int{12},
		Year:        2024,
	})
	if err != nil || len(filtered) != 1 || filtered[0].ID != 102 {
		t.Fatalf("combined filter = %+v, %v", filtered, err)
	}
	assertDBCardEventAndSupplyFilters(t, provider.cards, ctx)
}

func assertDBCardEventAndSupplyFilters(t *testing.T, cards *dbCardProvider, ctx context.Context) {
	t.Helper()
	eventCards, err := cards.Filter(ctx, &CardFilter{EventID: 50, Limit: 1})
	if err != nil || len(eventCards) != 1 || eventCards[0].ID != 101 {
		t.Fatalf("event filter = %+v, %v", eventCards, err)
	}
	missing, err := cards.Filter(ctx, &CardFilter{EventID: 404})
	if err != nil || missing != nil {
		t.Fatalf("missing event filter = %+v, %v", missing, err)
	}
	unknownSkill, err := cards.Filter(ctx, &CardFilter{SkillType: "unknown"})
	if err != nil || unknownSkill != nil {
		t.Fatalf("unknown skill filter = %+v, %v", unknownSkill, err)
	}
	limited, err := cards.Filter(ctx, &CardFilter{SupplyType: "limited"})
	if err != nil || len(limited) != 1 || limited[0].ID != 1345 {
		t.Fatalf("supply filter = %+v, %v", limited, err)
	}
}

func TestDBCardCoreUnitResolution(t *testing.T) {
	provider, client := newDBCardCoreCoverageProvider(t)
	ctx := context.Background()
	for _, testCase := range []struct {
		cardID int
		unit   string
	}{{101, "light_sound"}, {102, "idol"}, {103, "piapro"}} {
		unit, err := provider.cards.GetUnitByCardID(ctx, testCase.cardID)
		if err != nil || unit != testCase.unit {
			t.Fatalf("GetUnitByCardID(%d) = %q, %v", testCase.cardID, unit, err)
		}
	}
	if _, err := provider.cards.GetUnitByCardID(ctx, 999); err == nil {
		t.Fatal("missing card unit lookup should fail")
	}
	withoutCharacters := &dbCardProvider{client: client, region: renderregion.JP}
	if _, err := withoutCharacters.GetUnitByCardID(ctx, 101); err == nil {
		t.Fatal("unit lookup without character provider should fail")
	}
}

func TestDBCardCoreEpisodeAndFilterHelpers(t *testing.T) {
	provider, _ := newDBCardCoreCoverageProvider(t)
	ctx := context.Background()
	if episodes, err := provider.cards.GetEpisodesByCardID(ctx, 0); err != nil || episodes != nil {
		t.Fatalf("GetEpisodesByCardID(0) = %+v, %v", episodes, err)
	}
	episode := &masterdata.CardEpisode{ID: 1, CardID: 101}
	cloned := cloneCardEpisodes([]*masterdata.CardEpisode{nil, episode})
	if len(cloned) != 1 || cloned[0] == episode || cloned[0].ID != 1 {
		t.Fatalf("cloneCardEpisodes() = %+v", cloned)
	}
	if cloneCardEpisodes(nil) != nil || cloneCardEpisodes([]*masterdata.CardEpisode{nil}) != nil {
		t.Fatal("empty episode clones should be nil")
	}
	if ids, err := provider.cards.resolveFilterEventCardIDs(ctx, nil); err != nil || ids != nil {
		t.Fatalf("nil event filter IDs = %+v, %v", ids, err)
	}
	if provider.cards.matchesUnitFilter(ctx, nil, nil) {
		t.Fatal("nil unit filter should not match")
	}
	if !provider.cards.matchesUnitFilter(ctx, &CardFilter{}, &masterdata.Card{}) {
		t.Fatal("empty unit filter should match")
	}
}
