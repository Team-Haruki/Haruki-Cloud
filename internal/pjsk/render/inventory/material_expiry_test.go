package inventory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

type materialRowsStub struct {
	rows map[int]map[string]any
	ok   bool
}

func (s materialRowsStub) LoadMasterRows(context.Context, string) (map[int]map[string]any, bool) {
	return s.rows, s.ok
}

func buildMaterialExpiryRequest(t *testing.T, rows MaterialRowSource) *drawing.InventoryListRequest {
	t.Helper()
	dir := t.TempDir()
	masterDir := filepath.Join(dir, "haruki-sekai-master", "master")
	if err := os.MkdirAll(masterDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(masterDir, "materials.json"), []byte(`[
		{"id":17,"seq":1,"name":"Crystal","materialType":"common","flavorText":"crystal"},
		{"id":281,"seq":2,"name":"Old coin","materialType":"common","flavorText":"old"},
		{"id":282,"seq":3,"name":"Virtual cheer coin","materialType":"common","flavorText":"coin"}
	]`), 0o644); err != nil {
		t.Fatalf("WriteFile(materials) error = %v", err)
	}
	profile := &drawing.DetailedProfileCardRequest{ID: "1", Region: "jp", Nickname: "tester", Source: "suite"}
	raw := &snapshot.RawUserData{UserMaterials: []snapshot.RawUserMaterial{
		{MaterialID: 17, Quantity: 3}, {MaterialID: 281, Quantity: 4}, {MaterialID: 282, Quantity: 5},
	}}
	ctrl := NewController(nil, nil, nil, renderregion.JP, MasterdataOptions{LocalDir: dir})
	ctrl.now = func() time.Time { return time.UnixMilli(1_760_000_000_000) }
	req, err := ctrl.BuildListRequestFromSnapshot(Query{
		Region:       renderregion.JP,
		Profile:      profile,
		Snapshot:     &inventorySnapshotStub{raw: raw, profile: profile},
		MaterialRows: rows,
	})
	if err != nil {
		t.Fatalf("BuildListRequestFromSnapshot() error = %v", err)
	}
	return req
}

func inventoryItemsByID(req *drawing.InventoryListRequest) map[int]drawing.InventoryItem {
	items := map[int]drawing.InventoryItem{}
	for _, section := range req.Sections {
		for _, item := range section.Items {
			if item.ResourceType == "material" {
				items[item.ID] = item
			}
		}
	}
	return items
}

func TestMaterialExpiryFiltersExpiredAndMarksLimitedMaterials(t *testing.T) {
	rows := materialRowsStub{ok: true, rows: map[int]map[string]any{
		17:  {"id": int64(17), "name": "Crystal"},
		281: {"id": int64(281), "expiredAt": int64(1_750_000_000_000)},
		282: {"id": int64(282), "expiredAt": json.Number("1761231540000")},
	}}
	items := inventoryItemsByID(buildMaterialExpiryRequest(t, rows))
	if _, ok := items[281]; ok {
		t.Fatalf("expired material 281 must be filtered: %+v", items[281])
	}
	coin, ok := items[282]
	if !ok || coin.ExpiredAt == nil || *coin.ExpiredAt != 1761231540000 {
		t.Fatalf("limited material 282 = %+v", coin)
	}
	if coin.Description != "coin" {
		t.Fatalf("limited material description changed: %q", coin.Description)
	}
	if crystal := items[17]; crystal.ExpiredAt != nil || crystal.Description != "crystal" {
		t.Fatalf("material without expiry changed: %+v", crystal)
	}
}

func TestMaterialExpiryLeavesRegionsWithoutExpiryUnchanged(t *testing.T) {
	// Pre-7.0.0 regions (and a database without the column/table) carry no
	// expiredAt: every material is listed as before.
	for _, rows := range []MaterialRowSource{nil, materialRowsStub{ok: false}, materialRowsStub{ok: true, rows: map[int]map[string]any{281: {"id": int64(281)}}}} {
		items := inventoryItemsByID(buildMaterialExpiryRequest(t, rows))
		if len(items) != 3 {
			t.Fatalf("rows %#v: materials = %+v", rows, items)
		}
		for id, item := range items {
			if item.ExpiredAt != nil {
				t.Fatalf("material %d gained expiry without data: %+v", id, item)
			}
		}
	}
}
