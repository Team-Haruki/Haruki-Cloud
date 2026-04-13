package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const largeProviderID int64 = 9007199254740993

func TestLocalMySekaiProviderPreservesLargeIntegerPrecision(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "mysekaiMusicRecords.json")
	if err := os.WriteFile(target, []byte(`[{"id":1,"externalId":9007199254740993}]`), 0o644); err != nil {
		t.Fatalf("write mysekai masterdata: %v", err)
	}

	provider := &localMySekaiProvider{store: newLocalStore(root)}
	items := provider.LoadList("mysekaiMusicRecords.json")
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
	if got, ok := interfaceToInt(items[0]["externalId"]); !ok || int64(got) != largeProviderID {
		t.Fatalf("unexpected externalId value: got=%d ok=%v", got, ok)
	}
}
