package education

import (
	"errors"
	json "haruki-cloud/internal/jsonutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
)

// JP master rows for 想いの大樹 (item 56) and ミステリアスグレープ (item 55),
// exported from the production sekai DB. Item 56's upgrade shop (shop 13)
// opens at 2026-09-30T06:00:00Z.
const (
	greatTreeShopStartAt   int64 = 1790748000000
	greatTreeBeforeOpenNow int64 = 1790724109000 // 2026-09-29T23:21:49Z
	greatTreeAfterOpenNow  int64 = greatTreeShopStartAt + 3_600_000
)

type jpAreaItemFixture struct {
	AreaItems []struct {
		ID              int    `json:"id"`
		AreaID          int    `json:"areaId"`
		Name            string `json:"name"`
		AssetbundleName string `json:"assetbundleName"`
	} `json:"areaItems"`
	AreaItemLevels []struct {
		AreaItemID            int     `json:"areaItemId"`
		Level                 int     `json:"level"`
		TargetUnit            string  `json:"targetUnit"`
		TargetCardAttr        string  `json:"targetCardAttr"`
		TargetGameCharacterID int     `json:"targetGameCharacterId"`
		Power1BonusRate       float64 `json:"power1BonusRate"`
	} `json:"areaItemLevels"`
	ResourceBoxes []struct {
		ID                 int                 `json:"id"`
		ResourceBoxPurpose string              `json:"resourceBoxPurpose"`
		ResourceBoxType    string              `json:"resourceBoxType"`
		Details            []ResourceBoxDetail `json:"details"`
	} `json:"resourceBoxes"`
	ShopItems []struct {
		ID                 int   `json:"id"`
		ShopID             int   `json:"shopId"`
		Seq                int   `json:"seq"`
		ReleaseConditionID int   `json:"releaseConditionId"`
		ResourceBoxID      int   `json:"resourceBoxId"`
		StartAt            int64 `json:"startAt"`
		Costs              []struct {
			Cost ShopItemCost `json:"cost"`
		} `json:"costs"`
	} `json:"shopItems"`
}

func jpGreatTreeSource(t *testing.T) *testSource {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "jp_area_items_55_56.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture jpAreaItemFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	source := &testSource{
		region:        renderregion.JP,
		boxes:         map[string]map[int]*ResourceBox{},
		areaItems:     map[int]*AreaItem{},
		areaLevelRows: map[int]map[int][]*AreaItemLevel{},
		shopItems:     map[int]*ShopItem{},
	}
	for _, item := range fixture.AreaItems {
		source.areaItems[item.ID] = &AreaItem{ID: item.ID, AreaID: item.AreaID, Name: item.Name, AssetbundleName: item.AssetbundleName}
	}
	for _, row := range fixture.AreaItemLevels {
		if source.areaLevelRows[row.AreaItemID] == nil {
			source.areaLevelRows[row.AreaItemID] = map[int][]*AreaItemLevel{}
		}
		source.areaLevelRows[row.AreaItemID][row.Level] = append(source.areaLevelRows[row.AreaItemID][row.Level], &AreaItemLevel{
			AreaItemID:            row.AreaItemID,
			Level:                 row.Level,
			TargetUnit:            row.TargetUnit,
			TargetCardAttr:        row.TargetCardAttr,
			TargetGameCharacterID: row.TargetGameCharacterID,
			Power1BonusRate:       row.Power1BonusRate,
		})
	}
	for _, box := range fixture.ResourceBoxes {
		if source.boxes[box.ResourceBoxPurpose] == nil {
			source.boxes[box.ResourceBoxPurpose] = map[int]*ResourceBox{}
		}
		source.boxes[box.ResourceBoxPurpose][box.ID] = &ResourceBox{
			ID:                 box.ID,
			ResourceBoxPurpose: box.ResourceBoxPurpose,
			ResourceBoxType:    box.ResourceBoxType,
			Details:            box.Details,
		}
	}
	for _, item := range fixture.ShopItems {
		shopItem := &ShopItem{
			ID:                 item.ID,
			ShopID:             item.ShopID,
			Seq:                item.Seq,
			ResourceBoxID:      item.ResourceBoxID,
			ReleaseConditionID: item.ReleaseConditionID,
			StartAt:            item.StartAt,
		}
		for _, cost := range item.Costs {
			shopItem.Costs = append(shopItem.Costs, cost.Cost)
		}
		source.shopItems[item.ResourceBoxID] = shopItem
	}
	return source
}

func greatTreeController(t *testing.T, source *testSource, nowMs int64, ownedItems map[int]int) *Controller {
	t.Helper()
	areaItems := make([]map[string]any, 0, len(ownedItems))
	for itemID, level := range ownedItems {
		areaItems = append(areaItems, map[string]any{"areaItemId": itemID, "level": level})
	}
	snap := mustSnapshot(t, map[string]any{
		"now": nowMs,
		"userGamedata": map[string]any{
			"userId": 1001,
			"name":   "tester",
			"deck":   1,
			"coin":   1000,
		},
		"userProfile": map[string]any{"profileImageType": "normal"},
		"userDecks": []map[string]any{
			{"deckId": 1, "leader": 1, "member1": 1, "member2": 2, "member3": 3, "member4": 4, "member5": 5},
		},
		"userCards":     []map[string]any{{"cardId": 1, "level": 1}},
		"userAreas":     []map[string]any{{"areaItems": areaItems}},
		"userMaterials": []map[string]any{{"materialId": 283, "quantity": 500}},
	})
	controller := NewController(nil, nil, snap, renderregion.JP)
	controller.RegisterSource(source)
	controller.now = func() time.Time { return time.UnixMilli(nowMs) }
	return controller
}

func TestGreatTreeAreaItemAfterShopOpens(t *testing.T) {
	cases := []struct {
		name         string
		owned        map[int]int
		wantCurrent  int
		wantFirstLvl int
	}{
		{name: "not owned", owned: map[int]int{55: 20}, wantCurrent: 0, wantFirstLvl: 1},
		{name: "level 0 row", owned: map[int]int{55: 20, 56: 0}, wantCurrent: 0, wantFirstLvl: 1},
		{name: "level 1", owned: map[int]int{55: 20, 56: 1}, wantCurrent: 1, wantFirstLvl: 2},
		{name: "level 5", owned: map[int]int{56: 5}, wantCurrent: 5, wantFirstLvl: 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			controller := greatTreeController(t, jpGreatTreeSource(t), greatTreeAfterOpenNow, tc.owned)
			req, err := controller.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(AreaItemQuery{Region: renderregion.JP, AllCharacter: true})
			if err != nil {
				t.Fatalf("BuildAreaItemUpgradeMaterialsRequestFromSnapshot() error = %v", err)
			}
			if len(req.AreaItems) != 1 || req.AreaItems[0].ItemID != 56 {
				t.Fatalf("expected only item 56, got %+v", req.AreaItems)
			}
			item := req.AreaItems[0]
			if item.CurrentLevel != tc.wantCurrent {
				t.Fatalf("current level = %d, want %d", item.CurrentLevel, tc.wantCurrent)
			}
			if want := 20 - tc.wantFirstLvl + 1; len(item.Levels) != want {
				t.Fatalf("got %d level rows, want %d", len(item.Levels), want)
			}
			first := item.Levels[0]
			if first.Level != tc.wantFirstLvl || len(first.Materials) != 6 || first.MultiUnitBonus == nil {
				t.Fatalf("unexpected first level row: %+v", first)
			}
			last := item.Levels[len(item.Levels)-1]
			if last.Level != 20 || !approxEqual(last.Bonus, 10) || len(last.Materials) != 6 {
				t.Fatalf("unexpected level 20 row: %+v", last)
			}
		})
	}
}

func TestGreatTreeAreaItemBeforeShopOpens(t *testing.T) {
	// The production failure: JP 7.0.0 master data had item 56, but its
	// upgrade shop items all start in the future. A user who does not own
	// the tree yet ended with no area items and a generic failure.
	controller := greatTreeController(t, jpGreatTreeSource(t), greatTreeBeforeOpenNow, map[int]int{55: 20})
	_, err := controller.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(AreaItemQuery{Region: renderregion.JP, AllCharacter: true})
	var notReleased *AreaItemNotReleasedError
	if !errors.As(err, &notReleased) || !errors.Is(err, ErrAreaItemNotReleased) {
		t.Fatalf("expected AreaItemNotReleasedError, got %T %v", err, err)
	}
	if notReleased.OpensAtMs != greatTreeShopStartAt {
		t.Fatalf("OpensAtMs = %d, want %d", notReleased.OpensAtMs, greatTreeShopStartAt)
	}

	// Other items are unaffected by the unreleased tree.
	req, err := controller.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(AreaItemQuery{Region: renderregion.JP})
	if err != nil {
		t.Fatalf("unfiltered query error = %v", err)
	}
	if len(req.AreaItems) != 1 || req.AreaItems[0].ItemID != 55 || req.AreaItems[0].CurrentLevel != 20 {
		t.Fatalf("unfiltered query changed: %+v", req.AreaItems)
	}
}

func TestGreatTreeAreaItemFullIgnoresShopOpenTime(t *testing.T) {
	controller := NewController(nil, nil, nil, renderregion.JP)
	controller.RegisterSource(jpGreatTreeSource(t))
	req, err := controller.BuildAreaItemUpgradeMaterialsRequestFull(AreaItemQuery{Region: renderregion.JP, AllCharacter: true})
	if err != nil {
		t.Fatalf("BuildAreaItemUpgradeMaterialsRequestFull() error = %v", err)
	}
	if len(req.AreaItems) != 1 || req.AreaItems[0].ItemID != 56 || len(req.AreaItems[0].Levels) != 20 {
		t.Fatalf("unexpected full payload: %+v", req.AreaItems)
	}
	if got := req.AreaItems[0].Levels[0].Materials; len(got) != 6 || got[2].MaterialID != 283 || got[2].Quantity != 300 {
		t.Fatalf("unexpected level 1 materials: %+v", got)
	}
}

func TestAreaItemWithoutAnyShopItemsReportsNothingToShow(t *testing.T) {
	// A matched item the user does not own and that has no shop rows at all
	// yields ErrAreaItemNotReleased without an opening time.
	source := jpGreatTreeSource(t)
	source.shopItems = map[int]*ShopItem{}
	controller := greatTreeController(t, source, greatTreeAfterOpenNow, map[int]int{55: 20})
	_, err := controller.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(AreaItemQuery{Region: renderregion.JP, AllCharacter: true})
	var notReleased *AreaItemNotReleasedError
	if !errors.As(err, &notReleased) || notReleased.OpensAtMs != 0 {
		t.Fatalf("expected AreaItemNotReleasedError without opening time, got %T %v", err, err)
	}
}

func TestGreatTreeReleaseUsesQueryTimeWithOldSnapshot(t *testing.T) {
	for _, queryTime := range []int64{greatTreeShopStartAt - 1, greatTreeShopStartAt, greatTreeAfterOpenNow} {
		t.Run(time.UnixMilli(queryTime).UTC().Format(time.RFC3339Nano), func(t *testing.T) {
			controller := greatTreeController(t, jpGreatTreeSource(t), greatTreeBeforeOpenNow, map[int]int{55: 20})
			controller.now = func() time.Time { return time.UnixMilli(queryTime) }
			req, err := controller.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(AreaItemQuery{Region: renderregion.JP, AllCharacter: true})
			if queryTime < greatTreeShopStartAt {
				var notReleased *AreaItemNotReleasedError
				if !errors.As(err, &notReleased) || notReleased.OpensAtMs != greatTreeShopStartAt {
					t.Fatalf("expected future opening, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(req.AreaItems) != 1 || req.AreaItems[0].ItemID != 56 || len(req.AreaItems[0].Levels) != 20 {
				t.Fatalf("unexpected unlocked tree: %+v", req.AreaItems)
			}
		})
	}
}

func TestGreatTreeFutureSnapshotDoesNotUnlockEarly(t *testing.T) {
	controller := greatTreeController(t, jpGreatTreeSource(t), greatTreeAfterOpenNow, map[int]int{55: 20})
	controller.now = func() time.Time { return time.UnixMilli(greatTreeBeforeOpenNow) }
	_, err := controller.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(AreaItemQuery{Region: renderregion.JP, AllCharacter: true})
	if !errors.Is(err, ErrAreaItemNotReleased) {
		t.Fatalf("expected unreleased tree, got %v", err)
	}
}
