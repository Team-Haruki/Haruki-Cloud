package userdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func TestNewLocalFileServiceSupportsExtendedJSONExport(t *testing.T) {
	tempDir := t.TempDir()
	userJSONPath := filepath.Join(tempDir, "collections.suite.json")
	userJSON := `[{
		"_id": {"$numberLong": "GAME_USER_ID_REDACTED"},
		"server": "jp",
		"now": {"$numberLong": "1774852453796"},
		"userGamedata": {
			"userId": {"$numberLong": "GAME_USER_ID_REDACTED"},
			"name": "Deck User",
			"deck": 1,
			"rank": 123
		},
		"userProfile": {"profileImageType": "default"},
		"userDecks": [{"deckId": 1, "leader": 1001, "subLeader": 0, "member1": 0, "member2": 0, "member3": 0, "member4": 0, "member5": 0}],
		"userCards": [{"cardId": 1001, "level": 50, "masterRank": 1, "specialTrainingStatus": "not_done", "defaultImage": "normal", "episodes": []}],
		"userAreas": [],
		"userCharacters": [],
		"userHonors": []
	}]`
	if err := os.WriteFile(userJSONPath, []byte(userJSON), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}

	service := NewLocalFileService(nil, assets.NewAssetHelper(tempDir, nil), LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSONPath,
	})
	if err := service.Require(); err != nil {
		t.Fatalf("Require returned error: %v", err)
	}

	raw := service.RawData()
	if raw == nil {
		t.Fatal("expected raw data")
	}
	if raw.UserGamedata.UserID != GAME_USER_ID_REDACTED {
		t.Fatalf("unexpected user id: %d", raw.UserGamedata.UserID)
	}
	if raw.Now != 1774852453796 {
		t.Fatalf("unexpected now timestamp: %d", raw.Now)
	}
	if strings.TrimSpace(service.RawFilePath()) == "" {
		t.Fatalf("expected normalized raw file path")
	}

	rawBytes, err := service.RawBytes()
	if err != nil {
		t.Fatalf("RawBytes returned error: %v", err)
	}
	if strings.Contains(string(rawBytes), "$numberLong") {
		t.Fatalf("expected normalized raw bytes, got: %s", string(rawBytes))
	}

	var payload map[string]any
	if err := json.Unmarshal(rawBytes, &payload); err != nil {
		t.Fatalf("decode normalized raw bytes: %v", err)
	}
	if _, ok := payload["userGamedata"].(map[string]any); !ok {
		t.Fatalf("unexpected normalized payload: %+v", payload["userGamedata"])
	}

	frameBytes, err := service.RawValue("userDecks")
	if err != nil {
		t.Fatalf("RawValue returned error: %v", err)
	}

	var decks []map[string]any
	if err := json.Unmarshal(frameBytes, &decks); err != nil {
		t.Fatalf("decode raw value: %v", err)
	}
	if len(decks) != 1 {
		t.Fatalf("unexpected deck count: %d", len(decks))
	}
}

func TestMergeMySekaiDataPreservesSuiteUserGamedata(t *testing.T) {
	suiteJSON := []byte(`{
		"now": 1710000000,
		"userGamedata": {"userId": 12345004, "name": "Suite User", "deck": 1, "rank": 100, "coin": 0},
		"userProfile": {"profileImageType": "default"},
		"userDecks": [{"deckId": 1, "leader": 1001, "subLeader": 0, "member1": 0, "member2": 0, "member3": 0, "member4": 0, "member5": 0}],
		"userCards": [{"cardId": 1001, "level": 50, "masterRank": 1, "specialTrainingStatus": "not_done", "defaultImage": "normal", "episodes": []}]
	}`)
	mysekaiJSON := []byte(`{
		"now": 1710000100,
		"upload_time": 1710000200,
		"source": "toolbox_live",
		"userGamedata": {"userId": 12345000, "name": "Wrong User", "deck": 9, "rank": 1, "coin": 999},
		"updatedResources": {
			"userMysekaiGamedata": {"mysekaiRank": 12}
		}
	}`)

	merged, err := mergeMySekaiData(suiteJSON, mysekaiJSON)
	if err != nil {
		t.Fatalf("mergeMySekaiData() error = %v", err)
	}

	service, err := NewFromBytes(nil, assets.NewAssetHelper("", nil), renderregion.JP, suiteJSON, mysekaiJSON, nil)
	if err != nil {
		t.Fatalf("NewFromBytes() error = %v", err)
	}

	raw := service.RawData()
	if raw == nil {
		t.Fatal("expected merged raw data")
	}
	if raw.UserGamedata.UserID != 12345004 {
		t.Fatalf("expected suite user id to be preserved, got %d", raw.UserGamedata.UserID)
	}
	if raw.UserGamedata.Name != "Suite User" {
		t.Fatalf("expected suite user name to be preserved, got %q", raw.UserGamedata.Name)
	}
	if !strings.Contains(string(merged), `"upload_time":1710000200`) {
		t.Fatalf("expected mysekai metadata to be merged, got %s", string(merged))
	}
}
