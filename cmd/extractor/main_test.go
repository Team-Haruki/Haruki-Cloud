package main

import (
	"os"
	"testing"

	json "haruki-cloud/internal/jsonutil"
)

func TestMainWritesSchemaDescription(t *testing.T) {
	t.Chdir(t.TempDir())
	main()

	data, err := os.ReadFile("schema_info.json")
	if err != nil {
		t.Fatalf("read schema output: %v", err)
	}
	var tables []TableInfo
	if err := json.Unmarshal(data, &tables); err != nil {
		t.Fatalf("decode schema output: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("schema output contains no tables")
	}
	for _, table := range tables {
		if table.Name == "" || len(table.Columns) == 0 {
			t.Fatalf("invalid table description: %#v", table)
		}
	}
}
