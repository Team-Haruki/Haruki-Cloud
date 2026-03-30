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

	var payload map[string]interface{}
	if err := json.Unmarshal(rawBytes, &payload); err != nil {
		t.Fatalf("decode normalized raw bytes: %v", err)
	}
	if _, ok := payload["userGamedata"].(map[string]interface{}); !ok {
		t.Fatalf("unexpected normalized payload: %+v", payload["userGamedata"])
	}
}
