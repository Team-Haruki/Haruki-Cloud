package mysekai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
)

const largeMySekaiID int64 = 9007199254740993

func TestDecodeSnapshotPreservesLargeIntegerPrecision(t *testing.T) {
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{AllowFallback: true}).WithMySekaiData([]byte(`{
  "updatedResources": {
    "userMysekaiPhotos": [
      {"seq": 1, "obtainedAt": 9007199254740993, "imagePath": "photos/one"}
    ]
  }
}`))

	merged, _, err := controller.decodeSnapshot("jp")
	if err != nil {
		t.Fatalf("decodeSnapshot() error = %v", err)
	}

	photos := nestedList(merged, "userMysekaiPhotos")
	if len(photos) != 1 {
		t.Fatalf("expected 1 photo, got %+v", photos)
	}
	photo, ok := photos[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected photo payload: %#v", photos[0])
	}

	raw, ok := photo["obtainedAt"].(json.Number)
	if !ok {
		t.Fatalf("expected obtainedAt to stay as json.Number, got %T", photo["obtainedAt"])
	}
	if raw.String() != "9007199254740993" {
		t.Fatalf("unexpected obtainedAt json.Number: %s", raw.String())
	}
	if got := int64Number(photo["obtainedAt"], 0); got != largeMySekaiID {
		t.Fatalf("unexpected obtainedAt value: got=%d want=%d", got, largeMySekaiID)
	}
}

func TestLocalMasterdataStorePreservesLargeIntegerPrecision(t *testing.T) {
	root := t.TempDir()
	writeTestJSON(t, filepath.Join(root, "mysekaiMusicRecords.json"), []map[string]any{
		{"id": 1, "externalId": largeMySekaiID},
	})

	store := newLocalMasterdataStore(root)
	items := store.loadList("mysekaiMusicRecords.json")
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %+v", items)
	}

	raw, ok := items[0]["externalId"].(json.Number)
	if !ok {
		t.Fatalf("expected externalId to stay as json.Number, got %T", items[0]["externalId"])
	}
	if raw.String() != "9007199254740993" {
		t.Fatalf("unexpected externalId json.Number: %s", raw.String())
	}
	if got := int64Number(items[0]["externalId"], 0); got != largeMySekaiID {
		t.Fatalf("unexpected externalId value: got=%d want=%d", got, largeMySekaiID)
	}
}

func TestNormalizeValuePreservesLargeIntegerPrecisionInJSONB(t *testing.T) {
	parsed := normalizeValue([]byte(`{"thumbnailId":9007199254740993}`), nil, 0)
	payload, ok := parsed.(map[string]any)
	if !ok {
		t.Fatalf("expected JSON object, got %T", parsed)
	}

	raw, ok := payload["thumbnailId"].(json.Number)
	if !ok {
		t.Fatalf("expected thumbnailId to stay as json.Number, got %T", payload["thumbnailId"])
	}
	if raw.String() != "9007199254740993" {
		t.Fatalf("unexpected thumbnailId json.Number: %s", raw.String())
	}
	if got := int64Number(payload["thumbnailId"], 0); got != largeMySekaiID {
		t.Fatalf("unexpected thumbnailId value: got=%d want=%d", got, largeMySekaiID)
	}
}

func TestDBMasterdataFileMapIncludesMysekaiPhenomena(t *testing.T) {
	if got := fileToTable["mysekaiPhenomenas.json"]; got != "mysekaiphenomenas" {
		t.Fatalf("unexpected db table mapping for mysekaiPhenomenas.json: %q", got)
	}
	if got := fileToTable["mysekaiPhenomenaBackgroundColors.json"]; got != "mysekaiphenomenabackgroundcolors" {
		t.Fatalf("unexpected db table mapping for mysekaiPhenomenaBackgroundColors.json: %q", got)
	}
}

func TestLocalMasterdataStoreLoadObjectPreservesLargeIntegerPrecision(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "mysekai", "system", "fixture_reaction_data", "fixture_reaction_data.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir fixture reaction dir: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{"FixturerRactions":[{"FixtureId":9007199254740993}]}`), 0o644); err != nil {
		t.Fatalf("write fixture reaction json: %v", err)
	}

	store := newLocalMasterdataStore(root)
	var parsed map[string]any
	if !store.loadObject("mysekai/system/fixture_reaction_data/fixture_reaction_data.json", &parsed) {
		t.Fatal("loadObject() returned false")
	}

	items, ok := parsed["FixturerRactions"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("unexpected FixturerRactions payload: %#v", parsed["FixturerRactions"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected fixture reaction item: %#v", items[0])
	}
	raw, ok := item["FixtureId"].(json.Number)
	if !ok {
		t.Fatalf("expected FixtureId to stay as json.Number, got %T", item["FixtureId"])
	}
	if raw.String() != "9007199254740993" {
		t.Fatalf("unexpected FixtureId json.Number: %s", raw.String())
	}
}
