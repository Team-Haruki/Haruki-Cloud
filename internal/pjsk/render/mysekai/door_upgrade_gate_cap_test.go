package mysekai

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"
)

// gateMaterialGroups builds mysekaiGateMaterialGroups rows for every level in
// [1, maxLevel] of each listed gate (groupId = gateId*1000 + level).
func gateMaterialGroups(maxLevelByGate map[int]int) []map[string]any {
	rows := make([]map[string]any, 0)
	for gateID, maxLevel := range maxLevelByGate {
		for level := 1; level <= maxLevel; level++ {
			rows = append(rows, map[string]any{
				"groupId": gateID*1000 + level, "mysekaiMaterialId": 1, "quantity": level,
			})
		}
	}
	return rows
}

func newDoorUpgradeGateCapController(t *testing.T, maxLevelByGate map[int]int, userGates []map[string]any, gatesWithoutMaterials ...int) *Controller {
	t.Helper()
	masterdataDir := filepath.Join(t.TempDir(), "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	gates := make([]map[string]any, 0, len(maxLevelByGate))
	for gateID := range maxLevelByGate {
		gates = append(gates, map[string]any{"id": gateID, "assetbundleName": fmt.Sprintf("gate_%d", gateID)})
	}
	for _, gateID := range gatesWithoutMaterials {
		gates = append(gates, map[string]any{"id": gateID, "assetbundleName": fmt.Sprintf("gate_%d", gateID), "mysekaiGateType": "shuffle"})
	}
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiGates.json"), gates)
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiGateMaterialGroups.json"), gateMaterialGroups(maxLevelByGate))
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMaterials.json"), []map[string]any{
		{"id": 1, "iconAssetbundleName": "mat_1"},
	})
	userGateJSON := make([]string, 0, len(userGates))
	for _, gate := range userGates {
		userGateJSON = append(userGateJSON, fmt.Sprintf(`{"mysekaiGateId": %d, "mysekaiGateLevel": %d}`, gate["id"], gate["level"]))
	}
	mysekaiJSON := fmt.Sprintf(`{
  "updatedResources": {
    "userMysekaiMaterials": [{"mysekaiMaterialId": 1, "quantity": 5}],
    "userMysekaiGates": [%s]
  }
}`, strings.Join(userGateJSON, ","))
	return NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{
		LocalDir:      masterdataDir,
		AllowFallback: true,
	}).WithMySekaiData([]byte(mysekaiJSON))
}

func doorUpgradeLevels(gate drawing.MysekaiGateMaterials) []int {
	levels := make([]int, 0, len(gate.LevelMaterials))
	for _, level := range gate.LevelMaterials {
		levels = append(levels, level.Level)
	}
	return levels
}

func TestLoadDoorUpgradeGateMaterialsSizesEachGateFromMasterData(t *testing.T) {
	source := &sonarMasterdataSource{lists: map[string][]map[string]any{
		"mysekaiGateMaterialGroups.json": gateMaterialGroups(map[int]int{1: 70, 2: 40}),
	}}
	controller := &Controller{masterdata: source, defaultRegion: renderregion.JP}

	gates := controller.loadDoorUpgradeGateMaterials()
	if got := doorUpgradeGateMaxLevel(gates, 1); got != 70 {
		t.Fatalf("gate 1 max level = %d, want 70", got)
	}
	if got := doorUpgradeGateMaxLevel(gates, 2); got != 40 {
		t.Fatalf("gate 2 max level = %d, want 40", got)
	}
	if got := doorUpgradeGateMaxLevel(gates, 6); got != 0 {
		t.Fatalf("gate without materials max level = %d, want 0", got)
	}
	if gates[1][69][0].quantity != 70 || gates[1][40][0].quantity != 41 {
		t.Fatalf("levels above 40 must be kept: %+v", gates[1][40:])
	}
	if doorUpgradeGateIsMax(gates, 1, 40) || !doorUpgradeGateIsMax(gates, 1, 70) || !doorUpgradeGateIsMax(gates, 2, 40) {
		t.Fatal("max detection must follow the per-gate cap")
	}
	if doorUpgradeGateIsMax(gates, 6, 40) || doorUpgradeGateIsMax(gates, 6, 70) {
		t.Fatal("a gate without upgrade materials must never be reported as max")
	}
}

func TestBuildDoorUpgradeRequestJP70CapShowsLevels41To70(t *testing.T) {
	controller := newDoorUpgradeGateCapController(t, map[int]int{1: 70, 2: 70},
		[]map[string]any{{"id": 1, "level": 40}, {"id": 2, "level": 70}})

	// Auto-select: gate 2 is complete at 70, gate 1 at 40 still has 30 levels.
	req, err := controller.BuildDoorUpgradeRequest(DoorUpgradeQuery{Region: "jp", Profile: &drawing.ProfileCardRequest{}})
	if err != nil {
		t.Fatalf("BuildDoorUpgradeRequest() error = %v", err)
	}
	if len(req.GateMaterials) != 1 || req.GateMaterials[0].ID != 1 {
		t.Fatalf("expected gate 1 to be auto-selected, got %+v", req.GateMaterials)
	}
	levels := doorUpgradeLevels(req.GateMaterials[0])
	if len(levels) != 30 || levels[0] != 41 || levels[29] != 70 {
		t.Fatalf("expected levels 41..70, got %v", levels)
	}

	// Level 40 is no longer "max" for JP data when queried explicitly.
	if _, err := controller.BuildDoorUpgradeRequest(DoorUpgradeQuery{Region: "jp", Query: "1", Profile: &drawing.ProfileCardRequest{}}); err != nil {
		t.Fatalf("gate 1 at level 40 must be upgradable on a 70-cap region: %v", err)
	}

	// Level 70 is max.
	if _, err := controller.BuildDoorUpgradeRequest(DoorUpgradeQuery{Region: "jp", Query: "2", Profile: &drawing.ProfileCardRequest{}}); err == nil || testutil.MessageID(err) != "mysekai.gate.max_level" {
		t.Fatalf("expected gate 2 at level 70 to be reported as max, got err=%v", err)
	}

	// ShowFull lists every level from 1 to the derived cap.
	full, err := controller.BuildDoorUpgradeRequest(DoorUpgradeQuery{Region: "jp", ShowFull: new(true)})
	if err != nil {
		t.Fatalf("BuildDoorUpgradeRequest(full) error = %v", err)
	}
	if len(full.GateMaterials) != 2 || len(full.GateMaterials[0].LevelMaterials) != 70 || len(full.GateMaterials[1].LevelMaterials) != 70 {
		t.Fatalf("expected 70 levels per gate on full view, got %+v", full.GateMaterials)
	}
}

func TestBuildDoorUpgradeRequest40CapRegionUnchanged(t *testing.T) {
	controller := newDoorUpgradeGateCapController(t, map[int]int{1: 40, 2: 40},
		[]map[string]any{{"id": 1, "level": 40}, {"id": 2, "level": 39}})

	req, err := controller.BuildDoorUpgradeRequest(DoorUpgradeQuery{Region: "jp", Profile: &drawing.ProfileCardRequest{}})
	if err != nil {
		t.Fatalf("BuildDoorUpgradeRequest() error = %v", err)
	}
	if len(req.GateMaterials) != 1 || req.GateMaterials[0].ID != 2 {
		t.Fatalf("expected gate 2 (39/40) to be auto-selected, got %+v", req.GateMaterials)
	}
	if levels := doorUpgradeLevels(req.GateMaterials[0]); len(levels) != 1 || levels[0] != 40 {
		t.Fatalf("expected only level 40 to remain, got %v", levels)
	}

	if _, err := controller.BuildDoorUpgradeRequest(DoorUpgradeQuery{Region: "jp", Query: "1", Profile: &drawing.ProfileCardRequest{}}); err == nil || testutil.MessageID(err) != "mysekai.gate.max_level" {
		t.Fatalf("expected gate 1 at 40 to stay max on a 40-cap region, got err=%v", err)
	}
}

func TestBuildDoorUpgradeRequestIgnoresGateWithoutMaterials(t *testing.T) {
	// JP 7.0.0 gate 6 ("shuffle", unit none) has no material groups; a user
	// payload containing it must neither panic nor be auto-selected.
	controller := newDoorUpgradeGateCapController(t, map[int]int{1: 40, 2: 40},
		[]map[string]any{{"id": 1, "level": 40}, {"id": 2, "level": 40}, {"id": 6, "level": 12}})

	req, err := controller.BuildDoorUpgradeRequest(DoorUpgradeQuery{Region: "jp", Profile: &drawing.ProfileCardRequest{}})
	if err != nil {
		t.Fatalf("BuildDoorUpgradeRequest() error = %v", err)
	}
	if len(req.GateMaterials) != 2 {
		t.Fatalf("expected the two known gates when nothing is upgradable, got %+v", req.GateMaterials)
	}
	for _, gate := range req.GateMaterials {
		if gate.ID == 6 {
			t.Fatalf("gate 6 must not be rendered without material data: %+v", req.GateMaterials)
		}
		if len(gate.LevelMaterials) != 0 {
			t.Fatalf("gate %d at cap must have no remaining levels: %+v", gate.ID, gate.LevelMaterials)
		}
	}

	// Explicitly querying gate 6 falls back to the full list instead of failing.
	req, err = controller.BuildDoorUpgradeRequest(DoorUpgradeQuery{Region: "jp", Query: "6", Profile: &drawing.ProfileCardRequest{}})
	if err != nil {
		t.Fatalf("BuildDoorUpgradeRequest(query=6) error = %v", err)
	}
	if len(req.GateMaterials) != 2 {
		t.Fatalf("expected fallback to all gates for gate 6, got %+v", req.GateMaterials)
	}
}

func TestLowestIncompleteDoorUpgradeGateUsesPerGateCap(t *testing.T) {
	gates := map[int][][]doorUpgradeMaterial{
		1: make([][]doorUpgradeMaterial, 70),
		2: make([][]doorUpgradeMaterial, 40),
	}
	if got := lowestIncompleteDoorUpgradeGate(gates, map[int]int{1: 40, 2: 40, 6: 50}); got != 1 {
		t.Fatalf("expected gate 1 (40/70) to be selected, got %d", got)
	}
	if got := lowestIncompleteDoorUpgradeGate(gates, map[int]int{1: 70, 2: 40, 6: 50}); got != 0 {
		t.Fatalf("expected no selectable gate when unit gates are capped, got %d", got)
	}
	if got := lowestIncompleteDoorUpgradeGate(gates, map[int]int{1: 30, 2: 39}); got != 2 {
		t.Fatalf("expected the highest incomplete gate, got %d", got)
	}
}

func TestBuildDoorUpgradeRequestGateWithoutMaterialsReportsIt(t *testing.T) {
	// JP 7.0.0 lists shuffle gate 6 in mysekaiGates without material groups.
	controller := newDoorUpgradeGateCapController(t, map[int]int{1: 70, 2: 70},
		[]map[string]any{{"id": 1, "level": 40}, {"id": 6, "level": 1}}, 6)
	_, err := controller.BuildDoorUpgradeRequest(DoorUpgradeQuery{Region: "jp", Query: "6", Profile: &drawing.ProfileCardRequest{}})
	if err == nil || testutil.MessageID(err) != "mysekai.gate.no_materials" {
		t.Fatalf("expected gate 6 to be reported as having no materials, got err=%v", err)
	}
}

func TestBuildDoorUpgradeRequestUnknownGateKeepsAllGatesFallback(t *testing.T) {
	// A region without gate 6 (pre-7.0.0 data) keeps the old behaviour for an
	// id it does not know: every gate is listed.
	controller := newDoorUpgradeGateCapController(t, map[int]int{1: 40, 2: 40},
		[]map[string]any{{"id": 1, "level": 10}, {"id": 2, "level": 10}})
	req, err := controller.BuildDoorUpgradeRequest(DoorUpgradeQuery{Region: "jp", Query: "6", Profile: &drawing.ProfileCardRequest{}})
	if err != nil {
		t.Fatalf("BuildDoorUpgradeRequest() error = %v", err)
	}
	if len(req.GateMaterials) != 2 {
		t.Fatalf("expected both gates for an unknown id, got %+v", req.GateMaterials)
	}
}
