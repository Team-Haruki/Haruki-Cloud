package mysekai

import (
	"testing"
)

func TestConvertRecipeResourceBoxesFromDatabase(t *testing.T) {
	store := newTestDBMasterdataStore(t)
	_, err := store.db.Exec(`CREATE TABLE resourceboxes (
		id INTEGER PRIMARY KEY AUTOINCREMENT, game_id INTEGER, server_region TEXT,
		resource_box_purpose TEXT, details TEXT
	);
	INSERT INTO resourceboxes (game_id, server_region, resource_box_purpose, details) VALUES
	(1, 'jp', 'mysekai_convert_consumption', '[{"resourceType":"coin","resourceQuantity":1000}]'),
	(1, 'jp', 'mysekai_convert_obtain', '[{"resourceType":"coin","resourceQuantity":5}]'),
	(1, 'en', 'mysekai_convert_obtain', '[{"resourceType":"coin","resourceQuantity":999}]');`)
	if err != nil {
		t.Fatal(err)
	}
	rows := store.loadList("resourceBoxes.json")
	if len(rows) != 2 {
		t.Fatalf("expected only JP rows, got %+v", rows)
	}
	catalog := convertRecipeCatalog{boxes: make(map[convertRecipeBoxKey]map[string]any)}
	for _, row := range rows {
		catalog.boxes[convertRecipeBoxKey{stringValue(row["resourceBoxPurpose"]), intNumber(row["id"], 0)}] = row
	}
	if got := catalog.boxResources("mysekai_convert_consumption", 1); got != "金币 × 1000" {
		t.Fatalf("inputs = %q", got)
	}
	if got := catalog.boxResources("mysekai_convert_obtain", 1); got != "金币 × 5" {
		t.Fatalf("outputs = %q", got)
	}
}
