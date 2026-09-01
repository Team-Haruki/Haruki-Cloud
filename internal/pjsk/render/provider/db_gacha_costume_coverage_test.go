package provider

import (
	"context"
	"encoding/json"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/testutil"
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
		{
			_, err := client.Costume3D.Create().
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
				Save(ctx)
			testutil.Require(t, !(err != nil), "create costume %d: %v", item.id, err)
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
		{
			_, err := client.Cardcostume3D.Create().
				SetCardID(link.cardID).
				SetCostume3DID(link.costumeID).
				SetServerRegion(renderregion.JP.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create card-costume link %+v: %v", link, err)
		}

	}

	costumeProvider := provider.costumes
	{
		_, err := costumeProvider.GetByID(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetByID(0) should reject a missing costume ID")
	}

	got, err := costumeProvider.GetByID(ctx, 10)
	{
		testutil.Require(t, !(err != nil), "GetByID(10) = %+v, %v", got, err)
		testutil.Require(t, !(got.ID != 10), "GetByID(10) = %+v, %v", got, err)
		testutil.Require(t, !(got.Name != "Red dress"), "GetByID(10) = %+v, %v", got, err)
	}
	{

		_, err := costumeProvider.GetByID(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "missing costume should return an error")
	}

	all, err := costumeProvider.Filter(ctx, nil)
	{
		testutil.Require(t, !(err != nil), "Filter(nil) = %+v, %v", all, err)
		testutil.Require(t, !(len(all) != 2), "Filter(nil) = %+v, %v", all, err)
		testutil.Require(t, !(all[0].ID != 11), "Filter(nil) = %+v, %v", all, err)
		testutil.Require(t, !(all[1].ID != 10), "Filter(nil) = %+v, %v", all, err)
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
	{
		testutil.Require(t, !(err != nil), "Filter(full) = %+v, %v", filtered, err)
		testutil.Require(t, !(len(filtered) != 1), "Filter(full) = %+v, %v", filtered, err)
		testutil.Require(t, !(filtered[0].ID != 11), "Filter(full) = %+v, %v", filtered, err)
	}

	withoutValidCharacterIDs, err := costumeProvider.Filter(ctx, &CostumeFilter{CharacterIDs: []int{0, -1}, Offset: 1, Limit: 1})
	{
		testutil.Require(t, !(err != nil), "Filter(invalid character IDs) = %+v, %v", withoutValidCharacterIDs, err)
		testutil.Require(t, !(len(withoutValidCharacterIDs) != 1), "Filter(invalid character IDs) = %+v, %v", withoutValidCharacterIDs, err)
		testutil.Require(t, !(withoutValidCharacterIDs[0].ID != 10), "Filter(invalid character IDs) = %+v, %v", withoutValidCharacterIDs, err)
	}
	{

		_, err := costumeProvider.GetVariants(ctx, 0, "", 0)
		testutil.RequireArgs(t, !(err == nil), "GetVariants(0) should reject a missing group ID")
	}

	variants, err := costumeProvider.GetVariants(ctx, 50, " body ", 7)
	{
		testutil.Require(t, !(err != nil), "GetVariants(50) = %+v, %v", variants, err)
		testutil.Require(t, !(len(variants) != 2), "GetVariants(50) = %+v, %v", variants, err)
		testutil.Require(t, !(variants[0].ID != 10), "GetVariants(50) = %+v, %v", variants, err)
		testutil.Require(t, !(variants[1].ID != 11), "GetVariants(50) = %+v, %v", variants, err)
	}

	missingVariants, err := costumeProvider.GetVariants(ctx, 404, "", 0)
	{
		testutil.Require(t, !(err != nil), "GetVariants(404) = %+v, %v", missingVariants, err)
		testutil.Require(t, !(len(missingVariants) != 0), "GetVariants(404) = %+v, %v", missingVariants, err)
	}
	{

		sources, err := costumeProvider.GetSourceCardIDs(ctx, nil)
		{
			testutil.Require(t, !(err != nil), "GetSourceCardIDs(nil) = %+v, %v", sources, err)
			testutil.Require(t, !(len(sources) != 0), "GetSourceCardIDs(nil) = %+v, %v", sources, err)
		}
	}
	{

		sources, err := costumeProvider.GetSourceCardIDs(ctx, []int{0, -1})
		{
			testutil.Require(t, !(err != nil), "GetSourceCardIDs(invalid) = %+v, %v", sources, err)
			testutil.Require(t, !(len(sources) != 0), "GetSourceCardIDs(invalid) = %+v, %v", sources, err)
		}
	}

	sources, err := costumeProvider.GetSourceCardIDs(ctx, []int{11, 10, 13})
	{
		testutil.Require(t, !(err != nil), "GetSourceCardIDs() = %+v, %v", sources, err)
		testutil.Require(t, !(len(sources) != 2), "GetSourceCardIDs() = %+v, %v", sources, err)
		testutil.Require(t, !(len(sources[10]) != 1), "GetSourceCardIDs() = %+v, %v", sources, err)
		testutil.Require(t, !(sources[10][0] != 200), "GetSourceCardIDs() = %+v, %v", sources, err)
		testutil.Require(t, !(sources[11][0] != 200), "GetSourceCardIDs() = %+v, %v", sources, err)
	}
	testutil.RequireArgs(t, !(cloneCostumeEntities(nil) != nil), "cloneCostumeEntities(nil) should return nil")

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
		{
			_, err := client.Gacha.Create().
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
				Save(ctx)
			testutil.Require(t, !(err != nil), "create gacha %d: %v", item.id, err)
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
		{
			_, err := client.Card.Create().
				SetGameID(item.id).
				SetCharacterID(7).
				SetCardRarityType("rarity_4").
				SetAttr("cute").
				SetPrefix("card").
				SetAssetbundleName("card_asset").
				SetReleaseAt(item.releaseAt).
				SetServerRegion(renderregion.JP.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create card %d: %v", item.id, err)
		}

	}
	for _, item := range []struct {
		id    int64
		asset string
	}{
		{id: 500, asset: "ceil_asset"},
		{id: 501, asset: ""},
	} {
		{
			_, err := client.Gachaceilitem.Create().
				SetGameID(item.id).
				SetGachaID(100).
				SetName("ceil item").
				SetAssetbundleName(item.asset).
				SetServerRegion(renderregion.JP.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create ceil item %d: %v", item.id, err)
		}

	}
	for _, link := range []struct {
		cardID, costumeID int64
	}{
		{cardID: 200, costumeID: 10},
		{cardID: 200, costumeID: 11},
		{cardID: 203, costumeID: 99},
	} {
		{
			_, err := client.Cardcostume3D.Create().
				SetCardID(link.cardID).
				SetCostume3DID(link.costumeID).
				SetServerRegion(renderregion.JP.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create card-costume link %+v: %v", link, err)
		}

	}
	for _, id := range []int64{10, 11} {
		{
			_, err := client.Costume3D.Create().
				SetGameID(id).
				SetCostume3DGroupID(50).
				SetName("costume").
				SetPartType("body").
				SetColorID(id - 9).
				SetServerRegion(renderregion.JP.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create costume %d: %v", id, err)
		}

	}

	gachas := provider.gachas
	{
		_, err := gachas.GetByID(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetByID(0) should reject a missing gacha ID")
	}

	gachaInfo, err := gachas.GetByID(ctx, 100)
	{
		testutil.Require(t, !(err != nil), "GetByID(100) = %+v, %v", gachaInfo, err)
		testutil.Require(t, !(gachaInfo.ID != 100), "GetByID(100) = %+v, %v", gachaInfo, err)
		testutil.Require(t, !(len(gachaInfo.GachaPickups) != 1), "GetByID(100) = %+v, %v", gachaInfo, err)
	}

	gachaInfo.Name = "mutated"
	{
		cached, err := gachas.GetByID(ctx, 100)
		{
			testutil.Require(t, !(err != nil), "cached GetByID(100) = %+v, %v", cached, err)
			testutil.Require(t, !(cached.Name != "Active gacha"), "cached GetByID(100) = %+v, %v", cached, err)
		}
	}
	{

		_, err := gachas.GetByID(ctx, 101)
		testutil.RequireArgs(t, !(err == nil), "malformed gacha should return a decode error")
	}
	{

		_, err := gachas.GetByID(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "missing gacha should return a query error")
	}

	all := gachas.GetAll(ctx)
	{
		testutil.Require(t, !(len(all) != 2), "GetAll() = %+v, want valid gachas sorted newest first", all)
		testutil.Require(t, !(all[0].ID != 102), "GetAll() = %+v, want valid gachas sorted newest first", all)
		testutil.Require(t, !(all[1].ID != 100), "GetAll() = %+v, want valid gachas sorted newest first", all)
	}

	all[0].Name = "mutated"
	{
		cached := gachas.GetAll(ctx)
		{
			testutil.Require(t, !(len(cached) != 2), "cached GetAll() = %+v", cached)
			testutil.Require(t, !(cached[0].Name != "Recent gacha"), "cached GetAll() = %+v", cached)
		}
	}
	{

		_, err := gachas.GetCardByID(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetCardByID(0) should reject a missing card ID")
	}

	cardInfo, err := gachas.GetCardByID(ctx, 200)
	{
		testutil.Require(t, !(err != nil), "GetCardByID(200) = %+v, %v", cardInfo, err)
		testutil.Require(t, !(cardInfo.ID != 200), "GetCardByID(200) = %+v, %v", cardInfo, err)
	}

	cardInfo.Prefix = "mutated"
	{
		cached, err := gachas.GetCardByID(ctx, 200)
		{
			testutil.Require(t, !(err != nil), "cached GetCardByID(200) = %+v, %v", cached, err)
			testutil.Require(t, !(cached.Prefix != "card"), "cached GetCardByID(200) = %+v, %v", cached, err)
		}
	}
	{

		_, err := gachas.GetCardByID(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "missing card should return a query error")
	}
	{

		_, err := gachas.GetCeilItemAssetbundleName(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetCeilItemAssetbundleName(0) should reject a missing ID")
	}

	asset, err := gachas.GetCeilItemAssetbundleName(ctx, 500)
	{
		testutil.Require(t, !(err != nil), "GetCeilItemAssetbundleName(500) = %q, %v", asset, err)
		testutil.Require(t, !(asset != "ceil_asset"), "GetCeilItemAssetbundleName(500) = %q, %v", asset, err)
	}
	{

		cached, err := gachas.GetCeilItemAssetbundleName(ctx, 500)
		{
			testutil.Require(t, !(err != nil), "cached ceil asset = %q, %v", cached, err)
			testutil.Require(t, !(cached != asset), "cached ceil asset = %q, %v", cached, err)
		}
	}
	{

		_, err := gachas.GetCeilItemAssetbundleName(ctx, 501)
		testutil.RequireArgs(t, !(err == nil), "empty ceil item asset should return an error")
	}
	{

		_, err := gachas.GetCeilItemAssetbundleName(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "missing ceil item should return a query error")
	}

	cards := provider.cards
	{
		_, err := cards.GetGachaByCardID(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetGachaByCardID(0) should reject a missing card ID")
	}

	byActiveWindow, err := cards.GetGachaByCardID(ctx, 200)
	{
		testutil.Require(t, !(err != nil), "GetGachaByCardID(200) = %+v, %v", byActiveWindow, err)
		testutil.Require(t, !(byActiveWindow.ID != 100), "GetGachaByCardID(200) = %+v, %v", byActiveWindow, err)
	}

	byActiveWindow.Name = "mutated"
	{
		cached, err := cards.GetGachaByCardID(ctx, 200)
		{
			testutil.Require(t, !(err != nil), "cached gacha by card = %+v, %v", cached, err)
			testutil.Require(t, !(cached.Name != "Active gacha"), "cached gacha by card = %+v, %v", cached, err)
		}
	}

	byRecentFallback, err := cards.GetGachaByCardID(ctx, 201)
	{
		testutil.Require(t, !(err != nil), "GetGachaByCardID(201) fallback = %+v, %v", byRecentFallback, err)
		testutil.Require(t, !(byRecentFallback.ID != 102), "GetGachaByCardID(201) fallback = %+v, %v", byRecentFallback, err)
	}
	{

		_, err := cards.GetGachaByCardID(ctx, 202)
		testutil.RequireArgs(t, !(err == nil), "card without a pickup gacha should return an error")
	}
	{

		costumes, err := cards.GetCostume3dsByCardID(ctx, 0)
		{
			testutil.Require(t, !(err != nil), "GetCostume3dsByCardID(0) = %+v, %v", costumes, err)
			testutil.Require(t, !(costumes != nil), "GetCostume3dsByCardID(0) = %+v, %v", costumes, err)
		}
	}

	cardCostumes, err := cards.GetCostume3dsByCardID(ctx, 200)
	{
		testutil.Require(t, !(err != nil), "GetCostume3dsByCardID(200) = %+v, %v", cardCostumes, err)
		testutil.Require(t, !(len(cardCostumes) != 2), "GetCostume3dsByCardID(200) = %+v, %v", cardCostumes, err)
		testutil.Require(t, !(cardCostumes[0].ID != 10), "GetCostume3dsByCardID(200) = %+v, %v", cardCostumes, err)
		testutil.Require(t, !(cardCostumes[1].ID != 11), "GetCostume3dsByCardID(200) = %+v, %v", cardCostumes, err)
	}

	cardCostumes[0].Name = "mutated"
	{
		cached, err := cards.GetCostume3dsByCardID(ctx, 200)
		{
			testutil.Require(t, !(err != nil), "cached card costumes = %+v, %v", cached, err)
			testutil.Require(t, !(cached[0].Name != "costume"), "cached card costumes = %+v, %v", cached, err)
		}
	}
	{

		costumes, err := cards.GetCostume3dsByCardID(ctx, 202)
		{
			testutil.Require(t, !(err != nil), "card without costume links = %+v, %v", costumes, err)
			testutil.Require(t, !(costumes != nil), "card without costume links = %+v, %v", costumes, err)
		}
	}
	{

		costumes, err := cards.GetCostume3dsByCardID(ctx, 203)
		{
			testutil.Require(t, !(err != nil), "card linked only to missing costumes = %+v, %v", costumes, err)
			testutil.Require(t, !(costumes != nil), "card linked only to missing costumes = %+v, %v", costumes, err)
		}
	}
	testutil.RequireArgs(t, cardContainsPickup(&masterdata.Gacha{GachaPickups: []masterdata.GachaPickup{{CardID: 200}}}, 200), "cardContainsPickup should find a matching pickup")
	testutil.RequireArgs(t, !(cardContainsPickup(&masterdata.Gacha{}, 200)), "cardContainsPickup should reject a missing pickup")

}
