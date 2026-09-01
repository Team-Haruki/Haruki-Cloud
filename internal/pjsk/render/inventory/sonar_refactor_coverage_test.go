package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func TestInventoryRefactorFallbackItemsAndInvalidEntries(t *testing.T) {
	controller := &Controller{}
	raw := &snapshot.RawUserData{
		UserMaterials:            []snapshot.RawUserMaterial{{MaterialID: 0, Quantity: 1}, {MaterialID: 11, Quantity: 1}},
		UserGachaTickets:         []snapshot.RawUserGachaTicket{{GachaTicketID: 0, Quantity: 1}, {GachaTicketID: 12, Quantity: 1}},
		UserPracticeTickets:      []snapshot.RawUserPracticeTicket{{PracticeTicketID: 0, Quantity: 1}, {PracticeTicketID: 13, Quantity: 1}},
		UserSkillPracticeTickets: []snapshot.RawUserSkillPracticeTicket{{SkillPracticeTicketID: 0, Quantity: 1}, {SkillPracticeTicketID: 14, Quantity: 1}},
		UserGachaCeilItems:       []snapshot.RawUserGachaCeilItem{{GachaCeilItemID: 0, Quantity: 1}, {GachaCeilItemID: 15, Quantity: 1}},
		UserMysekaiMaterials:     []snapshot.RawUserMysekaiMaterial{{MysekaiMaterialID: 0, Quantity: 1}, {MysekaiMaterialID: 16, Quantity: 1}},
		UserBoostItems:           []snapshot.RawUserBoostItem{{BoostItemID: 0, Quantity: 1}, {BoostItemID: 17, Quantity: 1}},
	}
	items := controller.inventoryItems(renderregion.JP, raw, emptyRegionMasterdata())
	if len(items) != 8 {
		t.Fatalf("inventory items = %d, want coin plus seven fallbacks", len(items))
	}
	for _, want := range []string{"材料 11", "招募券 12", "练习乐谱 13", "技能升级乐谱 14", "招募贴纸 15", "MySekai 材料 16", "演出能量道具 17"} {
		found := false
		for _, item := range items {
			found = found || item.Name == want
		}
		if !found {
			t.Errorf("fallback item %q missing: %#v", want, items)
		}
	}
	if got := controller.inventoryRegion(renderregion.Unknown); got != renderregion.JP {
		t.Fatalf("fallback region = %q", got)
	}
}

func TestInventoryRefactorRequestErrors(t *testing.T) {
	controller := NewController(nil, nil, nil, renderregion.JP, MasterdataOptions{})
	raw := &snapshot.RawUserData{}
	if _, err := controller.BuildListRequestFromSnapshot(Query{Snapshot: &inventorySnapshotStub{raw: raw}}); err == nil || !strings.Contains(err.Error(), "profile") {
		t.Fatalf("missing profile error = %v", err)
	}
	profile := &drawing.DetailedProfileCardRequest{}
	if _, err := controller.BuildListRequestFromSnapshot(Query{Filter: FilterJewel, Snapshot: &inventorySnapshotStub{raw: raw, profile: profile}}); err == nil || !strings.Contains(err.Error(), "no inventory") {
		t.Fatalf("empty filtered inventory error = %v", err)
	}
	if _, err := controller.inventorySnapshot(&inventorySnapshotStub{}); err == nil || !strings.Contains(err.Error(), "raw data") {
		t.Fatalf("missing raw data error = %v", err)
	}
}

func TestInventoryRefactorMasterdataBranches(t *testing.T) {
	if got := (*masterdataStore)(nil).forRegion(renderregion.JP); got == nil {
		t.Fatal("nil store returned nil masterdata")
	}
	(*masterdataStore)(nil).resetCache()
	if candidates := masterdataFileCandidates(" ", renderregion.JP, "materials.json"); candidates != nil {
		t.Fatalf("blank-dir candidates = %#v", candidates)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "materials.json"), []byte(`[{"id":0,"name":"skip"},{"id":9,"name":"keep"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "invalid.json"), []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := make(map[int]materialMeta)
	loadIndexedMasterdata(dir, renderregion.JP, "materials.json", destination, func(item materialMeta) int { return item.ID })
	if len(destination) != 1 || destination[9].Name != "keep" {
		t.Fatalf("indexed masterdata = %#v", destination)
	}
	var invalid []materialMeta
	if err := loadMasterdataFile(dir, renderregion.JP, "invalid.json", &invalid); err == nil {
		t.Fatal("invalid masterdata JSON was accepted")
	}
}

func TestInventoryRefactorRemainingUtilityBranches(t *testing.T) {
	controller := &Controller{}
	for _, resource := range []string{"boost_item", "practice_ticket", "skill_practice_ticket"} {
		if got := controller.inventoryIconPath(renderregion.JP, resource, 0); got != "" {
			t.Errorf("invalid %s icon = %q", resource, got)
		}
	}
	items := []drawing.InventoryItem{
		{ID: 2, Name: "b", Category: "currency", Quantity: 1, Seq: 1},
		{ID: 1, Name: "c", Category: "currency", Quantity: 1, Seq: 1},
		{ID: 1, Name: "a", Category: "currency", Quantity: 1, Seq: 1},
	}
	sections := buildInventorySections(items)
	if len(sections) != 1 || sections[0].Items[0].Name != "a" || sections[0].Items[2].ID != 2 {
		t.Fatalf("tie-break sorting = %#v", sections)
	}
	if got := normalizeFilter(Filter("future-filter")); got != Filter("future-filter") {
		t.Fatalf("unknown filter = %q", got)
	}
}
