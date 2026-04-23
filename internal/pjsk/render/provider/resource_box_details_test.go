package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalEducationProviderLoadsStandaloneResourceBoxDetails(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "resourceBoxes.json", `[
		{"id":1,"resourceBoxPurpose":"challenge_live_high_score","resourceBoxType":"expand"},
		{"id":2,"resourceBoxPurpose":"shop_item","resourceBoxType":"expand"}
	]`)
	writeTestFile(t, root, "resourceBoxDetails.json", `[
		{"resourceBoxId":1,"resourceBoxPurpose":"challenge_live_high_score","resourceType":"jewel","resourceQuantity":100},
		{"resourceBoxId":1,"resourceBoxPurpose":"challenge_live_high_score","resourceType":"material","resourceId":15,"resourceQuantity":10},
		{"resourceBoxId":2,"resourceBoxPurpose":"shop_item","resourceType":"area_item","resourceId":101,"resourceLevel":3,"resourceQuantity":1}
	]`)

	provider := &localEducationProvider{store: newLocalStore(root)}
	box := provider.GetResourceBoxByPurpose(context.TODO(), "challenge_live_high_score", 1)
	if box == nil {
		t.Fatal("expected resource box")
	}
	if len(box.Details) != 2 {
		t.Fatalf("expected 2 details, got %+v", box.Details)
	}
	if got := box.Details[0]; got.ResourceType != "jewel" || got.ResourceQuantity != 100 {
		t.Fatalf("unexpected first detail: %+v", got)
	}

	shopBox := provider.GetResourceBoxByPurpose(context.TODO(), "shop_item", 2)
	if shopBox == nil || len(shopBox.Details) != 1 {
		t.Fatalf("expected shop item detail, got %+v", shopBox)
	}
	if got := shopBox.Details[0]; got.ResourceType != "area_item" || got.ResourceID != 101 || got.ResourceLevel != 3 {
		t.Fatalf("unexpected shop detail: %+v", got)
	}
}

func TestLocalEducationProviderLoadsCompactResourceBoxDetailsFallback(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "resourceBoxes.json", `[
		{"id":4,"resourceBoxPurpose":"challenge_live_high_score","resourceBoxType":"expand"}
	]`)
	writeTestFile(t, root, "compactResourceBoxDetails.json", `{
		"__ENUM__":{
			"resourceBoxPurpose":["challenge_live_high_score"],
			"resourceType":["jewel","material"]
		},
		"resourceBoxId":[4,4],
		"resourceBoxPurpose":[0,0],
		"resourceId":[null,15],
		"resourceLevel":[null,null],
		"resourceQuantity":[250,10],
		"resourceType":[0,1]
	}`)

	provider := &localEducationProvider{store: newLocalStore(root)}
	box := provider.GetResourceBoxByPurpose(context.TODO(), "challenge_live_high_score", 4)
	if box == nil {
		t.Fatal("expected compact resource box")
	}
	if len(box.Details) != 2 {
		t.Fatalf("expected 2 compact details, got %+v", box.Details)
	}
	if got := box.Details[1]; got.ResourceType != "material" || got.ResourceID != 15 || got.ResourceQuantity != 10 {
		t.Fatalf("unexpected compact detail: %+v", got)
	}
}

func writeTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
