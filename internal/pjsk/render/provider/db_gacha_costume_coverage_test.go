package provider

import (
	"context"
	"encoding/json"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestDBCostumeProviderQueriesFiltersVariantsAndSources(t *testing.T) {
	ctx := context.Background()
	provider := openProviderBehaviorDB(t, "costume_coverage")
	client := provider.client

	costumes := []struct {
		id, seq, groupID, colorID, characterID int64
		name, colorName, partType, kind        string
		publishedAt                            int64
		region                                 renderregion.Value
	}{
		{id: 10, seq: 1, groupID: 50, colorID: 1, characterID: 7, name: "Red dress", colorName: "Red", partType: "body", kind: "normal", publishedAt: 100, region: renderregion.JP},
		{id: 11, seq: 2, groupID: 50, colorID: 2, characterID: 7, name: "Blue dress", colorName: "Blue", partType: "body", kind: "normal", publishedAt: 200, region: renderregion.JP},
		{id: 12, seq: 3, groupID: 50, colorID: 3, characterID: 8, name: "Other region", colorName: "Green", partType: "hair", kind: "normal", publishedAt: 300, region: renderregion.TW},
	}
	for _, item := range costumes {
		if _, err := client.Costume3D.Create().
			SetGameID(item.id).
			SetSeq(item.seq).
			SetCostume3DGroupID(item.groupID).
			SetCostume3DType(item.kind).
			SetName(item.name).
			SetPartType(item.partType).
			SetColorID(item.colorID).
			SetColorName(item.colorName).
			SetCharacterID(item.characterID).
			SetCostume3DRarity("rare").
			SetHowToObtain("gacha").
			SetAssetbundleName("costume_asset").
			SetDesigner("designer").
			SetArchiveDisplayType("normal").
			SetArchivePublishedAt(item.publishedAt - 1).
			SetPublishedAt(item.publishedAt).
			SetServerRegion(item.region.String()).
			Save(ctx); err != nil {
			t.Fatalf("create costume %d: %v", item.id, err)
		}
	}
	for _, link := range []struct {
		cardID, costumeID int64
	}{
		{cardID: 200, costumeID: 10},
		{cardID: 200, costumeID: 11},
		{cardID: 0, costumeID: 13},
		{cardID: 203, costumeID: 99},
	} {
		if _, err := client.Cardcostume3D.Create().
			SetCardID(link.cardID).
			SetCostume3DID(link.costumeID).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create card-costume link %+v: %v", link, err)
		}
	}

	costumeProvider := provider.costumes
	if _, err := costumeProvider.GetByID(ctx, 0); err == nil {
		t.Fatal("GetByID(0) should reject a missing costume ID")
	}
	got, err := costumeProvider.GetByID(ctx, 10)
	if err != nil || got.ID != 10 || got.Name != "Red dress" {
		t.Fatalf("GetByID(10) = %+v, %v", got, err)
	}
	if _, err := costumeProvider.GetByID(ctx, 404); err == nil {
		t.Fatal("missing costume should return an error")
	}

	all, err := costumeProvider.Filter(ctx, nil)
	if err != nil || len(all) != 2 || all[0].ID != 11 || all[1].ID != 10 {
		t.Fatalf("Filter(nil) = %+v, %v", all, err)
	}
	filtered, err := costumeProvider.Filter(ctx, &CostumeFilter{
		PartType:     " body ",
		CostumeType:  " normal ",
		CharacterID:  7,
		CharacterIDs: []int{7, -1},
		ColorID:      2,
		Keyword:      " blue ",
		Limit:        1,
	})
	if err != nil || len(filtered) != 1 || filtered[0].ID != 11 {
		t.Fatalf("Filter(full) = %+v, %v", filtered, err)
	}
	withoutValidCharacterIDs, err := costumeProvider.Filter(ctx, &CostumeFilter{CharacterIDs: []int{0, -1}, Offset: 1, Limit: 1})
	if err != nil || len(withoutValidCharacterIDs) != 1 || withoutValidCharacterIDs[0].ID != 10 {
		t.Fatalf("Filter(invalid character IDs) = %+v, %v", withoutValidCharacterIDs, err)
	}

	if _, err := costumeProvider.GetVariants(ctx, 0, "", 0); err == nil {
		t.Fatal("GetVariants(0) should reject a missing group ID")
	}
	variants, err := costumeProvider.GetVariants(ctx, 50, " body ", 7)
	if err != nil || len(variants) != 2 || variants[0].ID != 10 || variants[1].ID != 11 {
		t.Fatalf("GetVariants(50) = %+v, %v", variants, err)
	}
	missingVariants, err := costumeProvider.GetVariants(ctx, 404, "", 0)
	if err != nil || len(missingVariants) != 0 {
		t.Fatalf("GetVariants(404) = %+v, %v", missingVariants, err)
	}

	if sources, err := costumeProvider.GetSourceCardIDs(ctx, nil); err != nil || len(sources) != 0 {
		t.Fatalf("GetSourceCardIDs(nil) = %+v, %v", sources, err)
	}
	if sources, err := costumeProvider.GetSourceCardIDs(ctx, []int{0, -1}); err != nil || len(sources) != 0 {
		t.Fatalf("GetSourceCardIDs(invalid) = %+v, %v", sources, err)
	}
	sources, err := costumeProvider.GetSourceCardIDs(ctx, []int{11, 10, 13})
	if err != nil || len(sources) != 2 || len(sources[10]) != 1 || sources[10][0] != 200 || sources[11][0] != 200 {
		t.Fatalf("GetSourceCardIDs() = %+v, %v", sources, err)
	}

	if cloneCostumeEntities(nil) != nil {
		t.Fatal("cloneCostumeEntities(nil) should return nil")
	}
}

func TestDBGachaAndCardRelationProvidersQueryAndCache(t *testing.T) {
	ctx := context.Background()
	provider := openProviderBehaviorDB(t, "gacha_coverage")
	client := provider.client

	validRarityRates := json.RawMessage(`[{"id":1,"groupId":1,"cardRarityType":"rarity_4","lotteryType":"normal","rate":3}]`)
	validDetails := json.RawMessage(`[{"id":1,"gachaId":100,"cardId":200,"weight":1,"isWish":false}]`)
	validBehaviors := json.RawMessage(`[{"id":1,"gachaId":100,"gachaBehaviorType":"normal","spinCount":10}]`)
	validInformation := json.RawMessage(`{"gachaId":100,"summary":"summary","description":"description"}`)
	for _, item := range []struct {
		id, startAt, endAt, pickupCardID int64
		name                             string
		malformed                        bool
		region                           renderregion.Value
	}{
		{id: 100, startAt: 100, endAt: 200, pickupCardID: 200, name: "Active gacha", region: renderregion.JP},
		{id: 101, startAt: 200, endAt: 300, pickupCardID: 999, name: "Malformed gacha", malformed: true, region: renderregion.JP},
		{id: 102, startAt: 300, endAt: 400, pickupCardID: 201, name: "Recent gacha", region: renderregion.JP},
		{id: 103, startAt: 500, endAt: 600, pickupCardID: 202, name: "Other region", region: renderregion.TW},
	} {
		rarityRates := validRarityRates
		if item.malformed {
			rarityRates = json.RawMessage(`{}`)
		}
		pickups := json.RawMessage(`[{"id":1,"gachaId":100,"cardId":200,"gachaPickupType":"pickup"}]`)
		if item.pickupCardID != 200 {
			pickups = json.RawMessage(`[{"id":1,"gachaId":102,"cardId":201,"gachaPickupType":"pickup"}]`)
		}
		if _, err := client.Gacha.Create().
			SetGameID(item.id).
			SetGachaType("normal").
			SetName(item.name).
			SetSeq(item.id).
			SetAssetbundleName("gacha_asset").
			SetStartAt(item.startAt).
			SetEndAt(item.endAt).
			SetIsShowPeriod(true).
			SetGachaCeilItemID(500).
			SetWishSelectCount(1).
			SetWishFixedSelectCount(2).
			SetWishLimitedSelectCount(3).
			SetGachaCardRarityRates(rarityRates).
			SetGachaDetails(validDetails).
			SetGachaBehaviors(validBehaviors).
			SetGachaPickups(pickups).
			SetGachaInformation(validInformation).
			SetServerRegion(item.region.String()).
			Save(ctx); err != nil {
			t.Fatalf("create gacha %d: %v", item.id, err)
		}
	}

	for _, item := range []struct {
		id, releaseAt int64
	}{
		{id: 200, releaseAt: 150},
		{id: 201, releaseAt: 1_000},
		{id: 202, releaseAt: 1_100},
		{id: 203, releaseAt: 1_200},
	} {
		if _, err := client.Card.Create().
			SetGameID(item.id).
			SetCharacterID(7).
			SetCardRarityType("rarity_4").
			SetAttr("cute").
			SetPrefix("card").
			SetAssetbundleName("card_asset").
			SetReleaseAt(item.releaseAt).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create card %d: %v", item.id, err)
		}
	}
	for _, item := range []struct {
		id    int64
		asset string
	}{
		{id: 500, asset: "ceil_asset"},
		{id: 501, asset: ""},
	} {
		if _, err := client.Gachaceilitem.Create().
			SetGameID(item.id).
			SetGachaID(100).
			SetName("ceil item").
			SetAssetbundleName(item.asset).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create ceil item %d: %v", item.id, err)
		}
	}
	for _, link := range []struct {
		cardID, costumeID int64
	}{
		{cardID: 200, costumeID: 10},
		{cardID: 200, costumeID: 11},
		{cardID: 203, costumeID: 99},
	} {
		if _, err := client.Cardcostume3D.Create().
			SetCardID(link.cardID).
			SetCostume3DID(link.costumeID).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create card-costume link %+v: %v", link, err)
		}
	}
	for _, id := range []int64{10, 11} {
		if _, err := client.Costume3D.Create().
			SetGameID(id).
			SetCostume3DGroupID(50).
			SetName("costume").
			SetPartType("body").
			SetColorID(id - 9).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create costume %d: %v", id, err)
		}
	}

	gachas := provider.gachas
	if _, err := gachas.GetByID(ctx, 0); err == nil {
		t.Fatal("GetByID(0) should reject a missing gacha ID")
	}
	gachaInfo, err := gachas.GetByID(ctx, 100)
	if err != nil || gachaInfo.ID != 100 || len(gachaInfo.GachaPickups) != 1 {
		t.Fatalf("GetByID(100) = %+v, %v", gachaInfo, err)
	}
	gachaInfo.Name = "mutated"
	if cached, err := gachas.GetByID(ctx, 100); err != nil || cached.Name != "Active gacha" {
		t.Fatalf("cached GetByID(100) = %+v, %v", cached, err)
	}
	if _, err := gachas.GetByID(ctx, 101); err == nil {
		t.Fatal("malformed gacha should return a decode error")
	}
	if _, err := gachas.GetByID(ctx, 404); err == nil {
		t.Fatal("missing gacha should return a query error")
	}

	all := gachas.GetAll(ctx)
	if len(all) != 2 || all[0].ID != 102 || all[1].ID != 100 {
		t.Fatalf("GetAll() = %+v, want valid gachas sorted newest first", all)
	}
	all[0].Name = "mutated"
	if cached := gachas.GetAll(ctx); len(cached) != 2 || cached[0].Name != "Recent gacha" {
		t.Fatalf("cached GetAll() = %+v", cached)
	}

	if _, err := gachas.GetCardByID(ctx, 0); err == nil {
		t.Fatal("GetCardByID(0) should reject a missing card ID")
	}
	cardInfo, err := gachas.GetCardByID(ctx, 200)
	if err != nil || cardInfo.ID != 200 {
		t.Fatalf("GetCardByID(200) = %+v, %v", cardInfo, err)
	}
	cardInfo.Prefix = "mutated"
	if cached, err := gachas.GetCardByID(ctx, 200); err != nil || cached.Prefix != "card" {
		t.Fatalf("cached GetCardByID(200) = %+v, %v", cached, err)
	}
	if _, err := gachas.GetCardByID(ctx, 404); err == nil {
		t.Fatal("missing card should return a query error")
	}

	if _, err := gachas.GetCeilItemAssetbundleName(ctx, 0); err == nil {
		t.Fatal("GetCeilItemAssetbundleName(0) should reject a missing ID")
	}
	asset, err := gachas.GetCeilItemAssetbundleName(ctx, 500)
	if err != nil || asset != "ceil_asset" {
		t.Fatalf("GetCeilItemAssetbundleName(500) = %q, %v", asset, err)
	}
	if cached, err := gachas.GetCeilItemAssetbundleName(ctx, 500); err != nil || cached != asset {
		t.Fatalf("cached ceil asset = %q, %v", cached, err)
	}
	if _, err := gachas.GetCeilItemAssetbundleName(ctx, 501); err == nil {
		t.Fatal("empty ceil item asset should return an error")
	}
	if _, err := gachas.GetCeilItemAssetbundleName(ctx, 404); err == nil {
		t.Fatal("missing ceil item should return a query error")
	}

	cards := provider.cards
	if _, err := cards.GetGachaByCardID(ctx, 0); err == nil {
		t.Fatal("GetGachaByCardID(0) should reject a missing card ID")
	}
	byActiveWindow, err := cards.GetGachaByCardID(ctx, 200)
	if err != nil || byActiveWindow.ID != 100 {
		t.Fatalf("GetGachaByCardID(200) = %+v, %v", byActiveWindow, err)
	}
	byActiveWindow.Name = "mutated"
	if cached, err := cards.GetGachaByCardID(ctx, 200); err != nil || cached.Name != "Active gacha" {
		t.Fatalf("cached gacha by card = %+v, %v", cached, err)
	}
	byRecentFallback, err := cards.GetGachaByCardID(ctx, 201)
	if err != nil || byRecentFallback.ID != 102 {
		t.Fatalf("GetGachaByCardID(201) fallback = %+v, %v", byRecentFallback, err)
	}
	if _, err := cards.GetGachaByCardID(ctx, 202); err == nil {
		t.Fatal("card without a pickup gacha should return an error")
	}

	if costumes, err := cards.GetCostume3dsByCardID(ctx, 0); err != nil || costumes != nil {
		t.Fatalf("GetCostume3dsByCardID(0) = %+v, %v", costumes, err)
	}
	cardCostumes, err := cards.GetCostume3dsByCardID(ctx, 200)
	if err != nil || len(cardCostumes) != 2 || cardCostumes[0].ID != 10 || cardCostumes[1].ID != 11 {
		t.Fatalf("GetCostume3dsByCardID(200) = %+v, %v", cardCostumes, err)
	}
	cardCostumes[0].Name = "mutated"
	if cached, err := cards.GetCostume3dsByCardID(ctx, 200); err != nil || cached[0].Name != "costume" {
		t.Fatalf("cached card costumes = %+v, %v", cached, err)
	}
	if costumes, err := cards.GetCostume3dsByCardID(ctx, 202); err != nil || costumes != nil {
		t.Fatalf("card without costume links = %+v, %v", costumes, err)
	}
	if costumes, err := cards.GetCostume3dsByCardID(ctx, 203); err != nil || costumes != nil {
		t.Fatalf("card linked only to missing costumes = %+v, %v", costumes, err)
	}

	if !cardContainsPickup(&masterdata.Gacha{GachaPickups: []masterdata.GachaPickup{{CardID: 200}}}, 200) {
		t.Fatal("cardContainsPickup should find a matching pickup")
	}
	if cardContainsPickup(&masterdata.Gacha{}, 200) {
		t.Fatal("cardContainsPickup should reject a missing pickup")
	}
}
