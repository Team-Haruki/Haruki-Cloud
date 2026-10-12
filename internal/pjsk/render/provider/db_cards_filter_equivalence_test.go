package provider

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/card"
	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/database/sekai/eventcard"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// cardFilterFixture is a small region that exercises every filter field:
// characters of several units (piapro with and without a support unit),
// three skill types, every supply type including the world-link override,
// rarities, attributes, release years and two events.
type cardFilterFixture struct {
	client *sekaiDB.Client
	cards  int
}

type fixtureCard struct {
	id, character int64
	rarity, attr  string
	support       string
	skill, supply int64
	released      time.Time
}

var cardFilterFixtureCards = []fixtureCard{
	{id: 1001, character: 1, rarity: "rarity_4", attr: "cool", skill: 11, supply: 1, released: time.Date(2020, time.September, 30, 0, 0, 0, 0, time.UTC)},
	{id: 1002, character: 2, rarity: "rarity_3", attr: "cute", skill: 12, supply: 1, released: time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC)},
	{id: 1003, character: 1, rarity: "rarity_4", attr: "happy", skill: 13, supply: 2, released: time.Date(2021, time.June, 15, 0, 0, 0, 0, time.UTC)},
	{id: 1004, character: 3, rarity: "rarity_2", attr: "pure", skill: 11, supply: 0, released: time.Date(2021, time.December, 31, 23, 59, 59, 999_000_000, time.UTC)},
	{id: 1005, character: 21, rarity: "rarity_4", attr: "mysterious", support: "light_sound", skill: 11, supply: 2, released: time.Date(2022, time.March, 3, 0, 0, 0, 0, time.UTC)},
	{id: 1006, character: 21, rarity: "rarity_4", attr: "cool", support: "", skill: 12, supply: 3, released: time.Date(2022, time.March, 3, 0, 0, 0, 0, time.UTC)},
	{id: 1007, character: 22, rarity: "rarity_birthday", attr: "cute", support: "none", skill: 13, supply: 0, released: time.Date(2022, time.August, 31, 0, 0, 0, 0, time.UTC)},
	{id: 1008, character: 2, rarity: "rarity_4", attr: "cool", skill: 11, supply: 4, released: time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC)},
	{id: 1009, character: 3, rarity: "rarity_4", attr: "happy", skill: 12, supply: 5, released: time.Date(2023, time.May, 5, 0, 0, 0, 0, time.UTC)},
	{id: 1010, character: 1, rarity: "rarity_1", attr: "pure", skill: 13, supply: 6, released: time.Date(2023, time.May, 5, 0, 0, 0, 0, time.UTC)},
	{id: 1011, character: 22, rarity: "rarity_4", attr: "cool", support: "idol", skill: 11, supply: 1, released: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)},
	{id: 1012, character: 2, rarity: "rarity_4", attr: "mysterious", skill: 11, supply: 2, released: time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC)},
	{id: 1013, character: 1, rarity: "rarity_3", attr: "cool", skill: 12, supply: 7, released: time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC)},
	{id: 1014, character: 3, rarity: "rarity_4", attr: "cute", skill: 13, supply: 3, released: time.Date(2024, time.December, 31, 0, 0, 0, 0, time.UTC)},
	{id: 1015, character: 21, rarity: "rarity_4", attr: "happy", support: "street", skill: 11, supply: 99, released: time.Date(2025, time.February, 2, 0, 0, 0, 0, time.UTC)},
	{id: 1016, character: 1, rarity: "rarity_4", attr: "cool", skill: 11, supply: 1, released: time.Date(2025, time.February, 2, 0, 0, 0, 0, time.UTC)},
	{id: 1017, character: 2, rarity: "rarity_birthday", attr: "pure", skill: 12, supply: 1, released: time.Date(2025, time.July, 7, 0, 0, 0, 0, time.UTC)},
	{id: 1018, character: 22, rarity: "rarity_4", attr: "cool", support: "light_sound", skill: 13, supply: 2, released: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)},
	// The supply override list (cardSupplyTypeOverride) forces term_limited.
	{id: 1345, character: 3, rarity: "rarity_4", attr: "cool", skill: 11, supply: 1, released: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)},
	// A card without a release time sorts first (release_at 0).
	{id: 1020, character: 2, rarity: "rarity_2", attr: "happy", skill: 12, supply: 1},
}

func newCardFilterFixture(t *testing.T, name string) *cardFilterFixture {
	t.Helper()
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:%s_%d?mode=memory&cache=shared&_fk=1", name, time.Now().UnixNano()))
	t.Cleanup(func() { _ = client.Close() })
	region := renderregion.JP.String()
	for _, character := range []struct {
		id   int64
		unit string
	}{{1, "light_sound"}, {2, "idol"}, {3, "street"}, {21, "piapro"}, {22, "piapro"}} {
		if _, err := client.Gamecharacter.Create().SetGameID(character.id).SetUnit(character.unit).SetGivenName(fmt.Sprintf("c%d", character.id)).SetServerRegion(region).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, skill := range []struct {
		id     int64
		sprite string
	}{{11, "score_up"}, {12, "life_recovery"}, {13, "judgment_accuracy_up"}} {
		if _, err := client.Skill.Create().SetGameID(skill.id).SetDescriptionSpriteName(skill.sprite).SetServerRegion(region).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, supply := range []struct {
		id   int64
		kind string
	}{{1, "normal"}, {2, "term_limited"}, {3, "colorful_festival_limited"}, {4, "bloom_festival_limited"}, {5, "unit_event_limited"}, {6, "collaboration_limited"}, {7, "not_limited"}} {
		if _, err := client.Cardsupplie.Create().SetGameID(supply.id).SetCardSupplyType(supply.kind).SetServerRegion(region).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, event := range []struct {
		id         int64
		kind, unit string
	}{{100, "marathon", "light_sound"}, {101, "world_bloom", "none"}, {102, "cheerful_carnival", "none"}} {
		if _, err := client.Event.Create().SetGameID(event.id).SetEventType(event.kind).SetUnit(event.unit).SetServerRegion(region).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for i, link := range []struct{ event, card int64 }{{100, 1001}, {100, 1003}, {100, 1016}, {101, 1005}, {101, 1018}} {
		if _, err := client.Eventcard.Create().SetGameID(int64(5000 + i)).SetEventID(link.event).SetCardID(link.card).SetServerRegion(region).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// Cards are inserted in id order so the SQL reference, which orders by
	// release_at alone, returns ties in id order like the index does.
	for _, item := range cardFilterFixtureCards {
		create := client.Card.Create().SetGameID(item.id).SetCharacterID(item.character).
			SetCardRarityType(item.rarity).SetAttr(item.attr).SetSupportUnit(item.support).
			SetSkillID(item.skill).SetCardSupplyID(item.supply).SetPrefix(fmt.Sprintf("card %d", item.id)).
			SetCardParameters([]byte(fmt.Sprintf(`[{"id":1,"cardId":%d,"cardLevel":1,"cardParameterType":"param1","power":100},{"id":2,"cardId":%d,"cardLevel":60,"cardParameterType":"param1","power":900}]`, item.id, item.id))).
			SetServerRegion(region)
		if !item.released.IsZero() {
			create = create.SetReleaseAt(item.released.UnixMilli())
		}
		if _, err := create.Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// Another region's rows must never leak into the JP results.
	for _, item := range cardFilterFixtureCards[:4] {
		if _, err := client.Card.Create().SetGameID(item.id).SetCharacterID(item.character).SetCardRarityType(item.rarity).
			SetAttr(item.attr).SetSkillID(item.skill).SetCardSupplyID(item.supply).SetReleaseAt(item.released.UnixMilli()).
			SetServerRegion(renderregion.TW.String()).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	return &cardFilterFixture{client: client, cards: len(cardFilterFixtureCards)}
}

func (f *cardFilterFixture) provider() *DatabaseProvider {
	return NewDatabaseProvider(f.client, renderregion.JP)
}

// legacyCardFilter is the SQL-backed Filter that the in-memory index
// replaced, kept as the reference for the equivalence test: the column
// conditions ran as WHERE predicates ordered by release_at, and the unit,
// skill and supply conditions were applied to the converted rows.
func legacyCardFilter(ctx context.Context, p *dbCardProvider, filter *CardFilter) ([]*masterdata.Card, error) {
	p.init()
	query := p.client.Card.Query().Where(card.ServerRegionEQ(p.region.String()))
	if !cardFilterUsesAllCards(filter) {
		if filter.EventID != 0 {
			links, err := p.client.Eventcard.Query().
				Where(eventcard.ServerRegionEQ(p.region.String()), eventcard.EventIDEQ(int64(filter.EventID))).
				All(ctx)
			if err != nil {
				return nil, err
			}
			if len(links) == 0 {
				return nil, nil
			}
			ids := make([]int64, 0, len(links))
			for _, link := range links {
				ids = append(ids, link.CardID)
			}
			query = query.Where(card.GameIDIn(ids...))
		}
		if filter.CharacterID != 0 {
			query = query.Where(card.CharacterIDEQ(int64(filter.CharacterID)))
		}
		if filter.Rarity != "" {
			query = query.Where(sql.FieldEQ("card_rarity_type", filter.Rarity))
		}
		if filter.Attr != "" {
			query = query.Where(sql.FieldEQ("attr", filter.Attr))
		}
		if filter.Year != 0 {
			start := time.Date(filter.Year, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
			end := time.Date(filter.Year+1, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
			query = query.Where(card.ReleaseAtGTE(start), card.ReleaseAtLT(end))
		}
	}
	matchingSkillIDs, err := p.cardFilterSkillTypeIDs(ctx, filter)
	if err != nil {
		return nil, err
	}
	if filter.SkillType != "" && len(matchingSkillIDs) == 0 {
		return nil, nil
	}
	entities, err := query.Order(card.ByReleaseAt()).All(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]*masterdata.Card, 0, len(entities))
	for _, entity := range entities {
		model, err := common.ConvertCardEntity(entity)
		if err != nil {
			return nil, err
		}
		if !cardFilterUsesAllCards(filter) && !p.cardMatchesFilter(ctx, filter, matchingSkillIDs, model) {
			continue
		}
		results = append(results, model)
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}
	return results, nil
}

// cardFilterValueSet gives each filter field one value; the combination
// tests enable every subset of the fields.
type cardFilterValueSet struct {
	name   string
	fields []func(*CardFilter)
}

var cardFilterValueSets = []cardFilterValueSet{
	{name: "light_sound_lead", fields: []func(*CardFilter){
		func(f *CardFilter) { f.CharacterID = 1 },
		func(f *CardFilter) { f.Unit = "light_sound" },
		func(f *CardFilter) { f.MainUnit = "light_sound" },
		func(f *CardFilter) { f.SupportUnit = "none" },
		func(f *CardFilter) { f.Rarity = "rarity_4" },
		func(f *CardFilter) { f.Attr = "cool" },
		func(f *CardFilter) { f.SkillType = "score_up" },
		func(f *CardFilter) { f.SkillIDs = []int{11, 13} },
		func(f *CardFilter) { f.SupplyType = "limited" },
		func(f *CardFilter) { f.Year = 2025 },
		func(f *CardFilter) { f.EventID = 100 },
	}},
	{name: "piapro_support", fields: []func(*CardFilter){
		func(f *CardFilter) { f.CharacterID = 21 },
		func(f *CardFilter) { f.Unit = "piapro" },
		func(f *CardFilter) { f.MainUnit = "piapro" },
		func(f *CardFilter) { f.SupportUnit = "light_sound" },
		func(f *CardFilter) { f.Rarity = "rarity_birthday" },
		func(f *CardFilter) { f.Attr = "happy" },
		func(f *CardFilter) { f.SkillType = "judgment_up" },
		func(f *CardFilter) { f.SkillIDs = []int{12} },
		func(f *CardFilter) { f.SupplyType = "festival" },
		func(f *CardFilter) { f.Year = 2022 },
		func(f *CardFilter) { f.EventID = 101 },
	}},
	{name: "misses", fields: []func(*CardFilter){
		func(f *CardFilter) { f.CharacterID = 3 },
		func(f *CardFilter) { f.Unit = "idol" },
		func(f *CardFilter) { f.MainUnit = "street" },
		func(f *CardFilter) { f.SupportUnit = "street" },
		func(f *CardFilter) { f.Rarity = "rarity_2" },
		func(f *CardFilter) { f.Attr = "mysterious" },
		func(f *CardFilter) { f.SkillType = "life_recovery" },
		func(f *CardFilter) { f.SkillIDs = []int{99} },
		func(f *CardFilter) { f.SupplyType = "birthday" },
		func(f *CardFilter) { f.Year = 2021 },
		func(f *CardFilter) { f.EventID = 999 },
	}},
	{name: "normal_supply", fields: []func(*CardFilter){
		func(f *CardFilter) { f.CharacterID = 2 },
		func(f *CardFilter) { f.Unit = "street" },
		func(f *CardFilter) { f.MainUnit = "idol" },
		func(f *CardFilter) { f.SupportUnit = "idol" },
		func(f *CardFilter) { f.Rarity = "rarity_3" },
		func(f *CardFilter) { f.Attr = "cute" },
		func(f *CardFilter) { f.SkillType = "judgment_accuracy_up" },
		func(f *CardFilter) { f.SkillIDs = []int{11, 12, 13} },
		func(f *CardFilter) { f.SupplyType = "normal" },
		func(f *CardFilter) { f.Year = 2024 },
		func(f *CardFilter) { f.EventID = 102 },
	}},
}

// forEachCardFilterCombination calls fn with every subset of the value
// set's fields and every limit, 2^11 filters per limit.
func forEachCardFilterCombination(set cardFilterValueSet, limits []int, fn func(mask int, filter *CardFilter)) {
	for mask := 0; mask < 1<<len(set.fields); mask++ {
		for _, limit := range limits {
			filter := &CardFilter{Limit: limit}
			for bit, apply := range set.fields {
				if mask&(1<<bit) != 0 {
					apply(filter)
				}
			}
			fn(mask, filter)
		}
	}
}

func cardIDs(cards []*masterdata.Card) []int {
	ids := make([]int, 0, len(cards))
	for _, item := range cards {
		ids = append(ids, item.ID)
	}
	return ids
}

// stripCardParameters drops the field the index deliberately reduces (the
// per-type maximum) before comparing a card with the SQL reference.
func stripCardParameters(cards []*masterdata.Card) []masterdata.Card {
	out := make([]masterdata.Card, 0, len(cards))
	for _, item := range cards {
		clone := *item
		clone.CardParameters = nil
		out = append(out, clone)
	}
	return out
}

func TestDBCardFilterMatchesSQLReferenceForEveryFieldCombination(t *testing.T) {
	ctx := context.Background()
	fixture := newCardFilterFixture(t, "card_filter_equivalence")
	// The reference and the index share one provider so the unit, skill
	// and supply lookups answer both from the same caches.
	provider := fixture.provider()
	nonEmpty, total := 0, 0
	for _, set := range cardFilterValueSets {
		forEachCardFilterCombination(set, []int{0, 1, 3}, func(mask int, filter *CardFilter) {
			total++
			want, err := legacyCardFilter(ctx, provider.cards, filter)
			if err != nil {
				t.Fatalf("%s/%011b: reference error: %v", set.name, mask, err)
			}
			got, err := provider.cards.Filter(ctx, filter)
			if err != nil {
				t.Fatalf("%s/%011b: Filter error: %v", set.name, mask, err)
			}
			if !reflect.DeepEqual(cardIDs(got), cardIDs(want)) {
				t.Fatalf("%s/%011b %+v: ids = %v, want %v", set.name, mask, *filter, cardIDs(got), cardIDs(want))
			}
			if !reflect.DeepEqual(stripCardParameters(got), stripCardParameters(want)) {
				t.Fatalf("%s/%011b: card fields differ from the reference", set.name, mask)
			}
			if len(got) > 0 {
				nonEmpty++
			}
			for _, item := range got {
				if len(item.CardParameters) != 1 || item.CardParameters[0].Power != 900 {
					t.Fatalf("%s/%011b: index must carry the strongest parameter per type, got %+v", set.name, mask, item.CardParameters)
				}
			}
		})
	}
	if nonEmpty < total/8 {
		t.Fatalf("only %d of %d combinations matched any card; the fixture does not exercise the filters", nonEmpty, total)
	}
	t.Logf("compared %d filter combinations (%d non-empty)", total, nonEmpty)
}

func TestDBCardFilterOrdersByReleaseThenID(t *testing.T) {
	ctx := context.Background()
	fixture := newCardFilterFixture(t, "card_filter_order")
	provider := fixture.provider()
	got, err := provider.cards.Filter(ctx, &CardFilter{Rarity: "rarity_4", Attr: "cool"})
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1001, 1006, 1008, 1011, 1016, 1018, 1345}
	if !reflect.DeepEqual(cardIDs(got), want) {
		t.Fatalf("ids = %v, want %v", cardIDs(got), want)
	}
	all, err := provider.cards.Filter(ctx, &CardFilter{})
	if err != nil || len(all) != fixture.cards || all[0].ID != 1020 || all[len(all)-1].ID != 1345 {
		t.Fatalf("all cards = %v, %v", cardIDs(all), err)
	}
	for i := 1; i < len(all); i++ {
		if all[i].ReleaseAt < all[i-1].ReleaseAt || (all[i].ReleaseAt == all[i-1].ReleaseAt && all[i].ID < all[i-1].ID) {
			t.Fatalf("index is not ordered by release_at, id at %d: %v", i, cardIDs(all))
		}
	}
}

func TestDBCardLookupsDoNotQueryAfterWarmUp(t *testing.T) {
	ctx := context.Background()
	fixture := newCardFilterFixture(t, "card_filter_no_db")
	provider := fixture.provider()
	if err := provider.WarmMasterdata(ctx); err != nil {
		t.Fatalf("WarmMasterdata() error = %v", err)
	}
	var queries atomic.Int32
	fixture.client.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			queries.Add(1)
			return next.Query(ctx, query)
		})
	}))

	calls := 0
	for _, set := range cardFilterValueSets {
		forEachCardFilterCombination(set, []int{0, 2}, func(mask int, filter *CardFilter) {
			calls++
			if _, err := provider.cards.Filter(ctx, filter); err != nil {
				t.Fatalf("%s/%011b: Filter error: %v", set.name, mask, err)
			}
		})
	}
	for _, id := range []int{1001, 1345, 1020} {
		if _, err := provider.cards.GetByID(ctx, id); err != nil {
			t.Fatalf("GetByID(%d) error = %v", id, err)
		}
	}
	if _, err := provider.cards.GetByID(ctx, 4242); err == nil {
		t.Fatal("GetByID(4242) must report a missing card from the index")
	}
	if item, err := provider.cards.GetByCharacterAndSeq(ctx, 1, 2); err != nil || item.ID != 1003 {
		t.Fatalf("GetByCharacterAndSeq(1, 2) = %+v, %v", item, err)
	}
	if item, err := provider.cards.GetByCharacterAndSeq(ctx, 1, -1); err != nil || item.ID != 1016 {
		t.Fatalf("GetByCharacterAndSeq(1, -1) = %+v, %v", item, err)
	}
	for _, seq := range []int{0, 7, -7} {
		if _, err := provider.cards.GetByCharacterAndSeq(ctx, 1, seq); err == nil {
			t.Fatalf("GetByCharacterAndSeq(1, %d) must fail", seq)
		}
	}
	if _, err := provider.cards.GetByCharacterAndSeq(ctx, 9, 1); err == nil {
		t.Fatal("GetByCharacterAndSeq(9, 1) must fail for a character without cards")
	}
	if got := queries.Load(); got != 0 {
		t.Fatalf("%d card lookups after warm-up issued %d queries, want 0", calls, got)
	}
}

func TestDBCardFilterSharesOneRegionIndex(t *testing.T) {
	ctx := context.Background()
	fixture := newCardFilterFixture(t, "card_filter_one_index")
	provider := fixture.provider()
	var cardQueries atomic.Int32
	fixture.client.Card.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			cardQueries.Add(1)
			return next.Query(ctx, query)
		})
	}))
	for _, filter := range []*CardFilter{
		{Unit: "light_sound"},
		{Rarity: "rarity_4", SupplyType: "limited"},
		{EventID: 100},
		{SkillType: "score_up", Year: 2025},
		{CharacterID: 21, Limit: 1},
	} {
		if _, err := provider.cards.Filter(ctx, filter); err != nil {
			t.Fatalf("Filter(%+v) error = %v", *filter, err)
		}
	}
	if cardQueries.Load() != 1 {
		t.Fatalf("filtered queries loaded the card table %d times, want once", cardQueries.Load())
	}
	// Mutating a result must not reach the shared index.
	got, err := provider.cards.Filter(ctx, &CardFilter{CharacterID: 1, Limit: 1})
	if err != nil || len(got) != 1 {
		t.Fatalf("Filter = %v, %v", got, err)
	}
	got[0].Prefix = "mutated"
	again, err := provider.cards.Filter(ctx, &CardFilter{CharacterID: 1, Limit: 1})
	if err != nil || again[0].Prefix != "card 1001" {
		t.Fatalf("caller mutation reached the index: %+v, %v", again, err)
	}
}

// BenchmarkDBCardFilterIndex measures a mixed filter answered from the
// in-memory index; BenchmarkDBCardFilterSQLReference runs the SQL path it
// replaced on the same (sqlite) fixture.
func BenchmarkDBCardFilterIndex(b *testing.B) {
	benchmarkCardFilter(b, func(ctx context.Context, p *dbCardProvider, filter *CardFilter) ([]*masterdata.Card, error) {
		return p.Filter(ctx, filter)
	})
}

func BenchmarkDBCardFilterSQLReference(b *testing.B) {
	benchmarkCardFilter(b, legacyCardFilter)
}

func benchmarkCardFilter(b *testing.B, run func(context.Context, *dbCardProvider, *CardFilter) ([]*masterdata.Card, error)) {
	b.Helper()
	ctx := context.Background()
	client := sekaienttest.Open(b, "sqlite3", fmt.Sprintf("file:card_filter_bench_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	defer client.Close()
	region := renderregion.JP.String()
	for _, character := range []struct {
		id   int64
		unit string
	}{{1, "light_sound"}, {2, "idol"}, {3, "street"}, {21, "piapro"}} {
		if _, err := client.Gamecharacter.Create().SetGameID(character.id).SetUnit(character.unit).SetServerRegion(region).Save(ctx); err != nil {
			b.Fatal(err)
		}
	}
	for _, item := range cardFilterFixtureCards {
		if _, err := client.Card.Create().SetGameID(item.id).SetCharacterID(item.character).SetCardRarityType(item.rarity).
			SetAttr(item.attr).SetSupportUnit(item.support).SetSkillID(item.skill).SetCardSupplyID(item.supply).
			SetReleaseAt(item.released.UnixMilli()).SetServerRegion(region).Save(ctx); err != nil {
			b.Fatal(err)
		}
	}
	provider := NewDatabaseProvider(client, renderregion.JP)
	filter := &CardFilter{Unit: "light_sound", Rarity: "rarity_4", Attr: "cool"}
	if _, err := run(ctx, provider.cards, filter); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := run(ctx, provider.cards, filter); err != nil {
			b.Fatal(err)
		}
	}
}
