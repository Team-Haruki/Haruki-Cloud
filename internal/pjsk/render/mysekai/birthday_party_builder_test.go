package mysekai

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

// JP birthdayParties ids 6/7 (Rin and Len, run together) and 1 (Haruka).
const (
	testRinLenStart    = int64(1766502000000) // 2025-12-23 15:00 UTC
	testRinLenBirthday = int64(1766761200000) // 2025-12-26 15:00 UTC
	testRinLenClose    = int64(1767020399000) // 2025-12-29 14:59:59 UTC
	testHarukaStart    = int64(1759330800000)
	testHarukaBirthday = int64(1759590000000)
	testHarukaClose    = int64(1759849199000)
)

func writeBirthdayPartyMasterdata(t *testing.T, withRewards bool) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "masterdata")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	writeTestJSON(t, filepath.Join(dir, "birthdayParties.json"), []map[string]any{
		{"id": 1, "gameCharacterUnitId": 6, "startAt": testHarukaStart, "birthdayStartAt": testHarukaBirthday, "closedAt": testHarukaClose, "assetbundleName": "haruka_2025"},
		{"id": 7, "gameCharacterUnitId": 39, "startAt": testRinLenStart, "birthdayStartAt": testRinLenBirthday, "closedAt": testRinLenClose, "assetbundleName": "len_2025"},
		{"id": 6, "gameCharacterUnitId": 33, "startAt": testRinLenStart, "birthdayStartAt": testRinLenBirthday, "closedAt": testRinLenClose, "assetbundleName": "rin_2025"},
	})
	if withRewards {
		rewards := []map[string]any{}
		for _, partyID := range []int{1, 6, 7} {
			for i, requirement := range []int{2, 3, 4, 380, 400} {
				rewards = append(rewards, map[string]any{
					"id": partyID*100 + i, "birthdayPartyId": partyID, "requirement": requirement, "resourceBoxId": 100,
				})
			}
		}
		writeTestJSON(t, filepath.Join(dir, "birthdayPartyDeliveryTotalRewards.json"), rewards)
	}
	writeTestJSON(t, filepath.Join(dir, "gameCharacterUnits.json"), []map[string]any{
		{"id": 6, "gameCharacterId": 6, "unit": "idol", "colorCode": "#99ccff"},
		{"id": 33, "gameCharacterId": 22, "unit": "idol"},
		{"id": 39, "gameCharacterId": 23, "unit": "street"},
	})
	writeTestJSON(t, filepath.Join(dir, "gameCharacters.json"), []map[string]any{
		{"id": 6, "firstName": "桐谷", "givenName": "遥"},
		{"id": 22, "givenName": "鏡音リン"},
		{"id": 23, "givenName": "鏡音レン"},
	})
	return dir
}

func buildBirthdayPartyRequest(t *testing.T, masterdataDir, mysekaiJSON string, now int64) *drawing.MysekaiResourceRequest {
	t.Helper()
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}).WithMySekaiData([]byte(mysekaiJSON))
	req, err := controller.BuildResourceRequest(ResourceQuery{Region: "jp", Profile: &drawing.ProfileCardRequest{}, NowMillis: now})
	if err != nil {
		t.Fatalf("BuildResourceRequest() error = %v", err)
	}
	return req
}

func userBirthdayPartiesJSON(entries string) string {
	return `{"updatedResources": {"userBirthdayParties": [` + entries + `]}}`
}

func TestBuildResourceRequestShowsRunningBirthdayPartyLevel(t *testing.T) {
	dir := writeBirthdayPartyMasterdata(t, true)
	data := userBirthdayPartiesJSON(`{"birthdayPartyId": 1, "deliveryTotalPoint": 302300, "obtainedMysekaiMaterialCount": 30, "droppedMysekaiMaterialCount": 0}`)

	req := buildBirthdayPartyRequest(t, dir, data, testHarukaStart+1000)
	if len(req.BirthdayParties) != 1 {
		t.Fatalf("birthday parties = %+v, want one", req.BirthdayParties)
	}
	got := req.BirthdayParties[0]
	want := drawing.MysekaiBirthdayPartyProgress{
		BirthdayPartyID:   1,
		CharacterUnitID:   6,
		CharacterName:     "桐谷遥",
		CharacterIconPath: got.CharacterIconPath,
		CharacterColor:    "#99ccff",
		Level:             30,
		MaxLevel:          400,
		DropEndAt:         testHarukaBirthday,
		WateringEndAt:     testHarukaClose,
	}
	if got != want {
		t.Fatalf("birthday party = %+v, want %+v", got, want)
	}
	if !strings.HasSuffix(got.CharacterIconPath, "chara_icon/hrk.png") {
		t.Fatalf("character icon = %q", got.CharacterIconPath)
	}
}

func TestBuildResourceRequestKeepsBirthdayPartyLevelAboveTarget(t *testing.T) {
	dir := writeBirthdayPartyMasterdata(t, true)
	data := userBirthdayPartiesJSON(`{"birthdayPartyId": 1, "deliveryTotalPoint": 5234100, "obtainedMysekaiMaterialCount": 523}`)

	req := buildBirthdayPartyRequest(t, dir, data, testHarukaClose-1)
	if len(req.BirthdayParties) != 1 || req.BirthdayParties[0].Level != 523 || req.BirthdayParties[0].MaxLevel != 400 {
		t.Fatalf("birthday parties = %+v, want Lv.523/400", req.BirthdayParties)
	}
}

func TestBuildResourceRequestBirthdayPartyWindow(t *testing.T) {
	dir := writeBirthdayPartyMasterdata(t, true)
	data := userBirthdayPartiesJSON(`{"birthdayPartyId": 1, "obtainedMysekaiMaterialCount": 30}`)

	for name, now := range map[string]int64{
		"before start":  testHarukaStart - 1,
		"at closed":     testHarukaClose,
		"after closed":  testHarukaClose + 86_400_000,
		"between gaps":  testRinLenClose + 1,
		"zero snapshot": 1,
	} {
		t.Run(name, func(t *testing.T) {
			if got := buildBirthdayPartyRequest(t, dir, data, now).BirthdayParties; got != nil {
				t.Fatalf("birthday parties = %+v, want none", got)
			}
		})
	}
	if got := buildBirthdayPartyRequest(t, dir, data, testHarukaStart).BirthdayParties; len(got) != 1 {
		t.Fatalf("party must be shown from startAt, got %+v", got)
	}
}

func TestBuildResourceRequestShowsConcurrentBirthdayPartiesInOrder(t *testing.T) {
	dir := writeBirthdayPartyMasterdata(t, true)
	// Len has no record yet: the party is running, so it shows Lv.0.
	data := userBirthdayPartiesJSON(`{"birthdayPartyId": 6, "obtainedMysekaiMaterialCount": 12}`)

	got := buildBirthdayPartyRequest(t, dir, data, testRinLenStart+1).BirthdayParties
	if len(got) != 2 {
		t.Fatalf("birthday parties = %+v, want two", got)
	}
	if got[0].BirthdayPartyID != 6 || got[0].Level != 12 || got[0].CharacterName != "鏡音リン" || !strings.HasSuffix(got[0].CharacterIconPath, "chara_icon/rin.png") {
		t.Fatalf("first party = %+v", got[0])
	}
	if got[1].BirthdayPartyID != 7 || got[1].Level != 0 || got[1].MaxLevel != 400 || !strings.HasSuffix(got[1].CharacterIconPath, "chara_icon/len.png") {
		t.Fatalf("second party = %+v", got[1])
	}
}

func TestBuildResourceRequestOmitsBirthdayPartyWithoutData(t *testing.T) {
	now := testHarukaStart + 1
	withRewards := writeBirthdayPartyMasterdata(t, true)
	if got := buildBirthdayPartyRequest(t, withRewards, `{"updatedResources": {}}`, now).BirthdayParties; got != nil {
		t.Fatalf("a snapshot without userBirthdayParties must not draw the block, got %+v", got)
	}

	withoutRewards := writeBirthdayPartyMasterdata(t, false)
	data := userBirthdayPartiesJSON(`{"birthdayPartyId": 1, "obtainedMysekaiMaterialCount": 30}`)
	if got := buildBirthdayPartyRequest(t, withoutRewards, data, now).BirthdayParties; got != nil {
		t.Fatalf("a region without delivery total rewards must not draw the block, got %+v", got)
	}

	empty := filepath.Join(t.TempDir(), "masterdata")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	req := buildBirthdayPartyRequest(t, empty, data, now)
	if req.BirthdayParties != nil {
		t.Fatalf("a region without birthdayParties must not draw the block, got %+v", req.BirthdayParties)
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "birthday_parties") {
		t.Fatalf("payload must not carry birthday_parties outside a party: %s", body)
	}
}

func TestBuildResourceRequestReadsTopLevelUserBirthdayParties(t *testing.T) {
	merged := map[string]any{
		"userBirthdayParties": []any{map[string]any{"birthdayPartyId": json.Number("1"), "obtainedMysekaiMaterialCount": json.Number("401")}},
	}
	parties, ok := userBirthdayParties(merged)
	if !ok || intNumber(parties[1]["obtainedMysekaiMaterialCount"], 0) != 401 {
		t.Fatalf("userBirthdayParties() = %+v, %v", parties, ok)
	}
	if _, ok := userBirthdayParties(map[string]any{"userBirthdayParties": nil}); ok {
		t.Fatal("a null list carries no data")
	}
}

func TestBirthdayPartyCharacterNameJoinsLatinNames(t *testing.T) {
	dir := writeBirthdayPartyMasterdata(t, true)
	writeTestJSON(t, filepath.Join(dir, "gameCharacters.json"), []map[string]any{
		{"id": 6, "firstName": "Kiritani", "givenName": "Haruka"},
	})
	controller := NewController(nil, nil, renderregion.EN, nil, MasterdataOptions{LocalDir: dir, AllowFallback: true})
	if got := controller.birthdayPartyCharacterName(6); got != "Kiritani Haruka" {
		t.Fatalf("character name = %q", got)
	}
}

func TestDBMasterdataStoreServesBirthdayPartyTables(t *testing.T) {
	store := newTestDBMasterdataStore(t)
	for _, stmt := range []string{
		`CREATE TABLE birthdayparties (id INTEGER PRIMARY KEY AUTOINCREMENT, game_id INTEGER, server_region TEXT, game_character_unit_id INTEGER, start_at INTEGER, birthday_start_at INTEGER, closed_at INTEGER)`,
		`INSERT INTO birthdayparties (game_id, server_region, game_character_unit_id, start_at, birthday_start_at, closed_at) VALUES (1, 'jp', 6, 1759330800000, 1759590000000, 1759849199000), (1, 'cn', 6, 1, 2, 3)`,
		`CREATE TABLE birthdaypartydeliverytotalrewards (id INTEGER PRIMARY KEY AUTOINCREMENT, game_id INTEGER, server_region TEXT, birthday_party_id INTEGER, requirement INTEGER)`,
		`INSERT INTO birthdaypartydeliverytotalrewards (game_id, server_region, birthday_party_id, requirement) VALUES (45, 'jp', 1, 380), (46, 'jp', 1, 400)`,
	} {
		if _, err := store.db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	parties := store.loadMapByID("birthdayParties.json")
	if len(parties) != 1 || intNumber(parties[1]["gameCharacterUnitId"], 0) != 6 || int64Number(parties[1]["closedAt"], 0) != testHarukaClose ||
		int64Number(parties[1]["birthdayStartAt"], 0) != testHarukaBirthday {
		t.Fatalf("birthday parties = %+v", parties)
	}
	if got := birthdayPartyMaxLevels(store.loadList("birthdayPartyDeliveryTotalRewards.json")); got[1] != 400 {
		t.Fatalf("max levels = %+v", got)
	}
}

func TestBuildResourceRequestCarriesBirthdayPartyDropAndWateringEnds(t *testing.T) {
	dir := writeBirthdayPartyMasterdata(t, true)
	data := userBirthdayPartiesJSON(`{"birthdayPartyId": 6, "obtainedMysekaiMaterialCount": 12}`)

	// After the birthday the 露滴 have stopped dropping, but the end times are
	// still sent: the drawer compares them with the render time.
	for _, now := range []int64{testRinLenStart, testRinLenBirthday + 1} {
		got := buildBirthdayPartyRequest(t, dir, data, now).BirthdayParties
		if len(got) != 2 {
			t.Fatalf("birthday parties = %+v, want two", got)
		}
		for _, party := range got {
			if party.DropEndAt != testRinLenBirthday || party.WateringEndAt != testRinLenClose {
				t.Fatalf("party %d drop/watering ends = %d/%d, want %d/%d",
					party.BirthdayPartyID, party.DropEndAt, party.WateringEndAt, testRinLenBirthday, testRinLenClose)
			}
		}
	}
}

func TestBirthdayPartyDropEndIsOmittedWithoutBirthdayStartAt(t *testing.T) {
	body, err := json.Marshal(drawing.MysekaiBirthdayPartyProgress{BirthdayPartyID: 1, CharacterUnitID: 6, WateringEndAt: testHarukaClose})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), "drop_end_at") || !strings.Contains(string(body), `"watering_end_at":1759849199000`) {
		t.Fatalf("payload = %s", body)
	}
}
