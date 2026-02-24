package main

import (
	"encoding/json"
	"os"

	"haruki-cloud/database/sekai/migrate"
)

type TableInfo struct {
	Name       string     `json:"name"`
	Columns    []string   `json:"columns"`
	UniqueKeys [][]string `json:"unique_keys"`
}

func main() {
	var tables []TableInfo
	for _, table := range migrate.Tables {
		var cols []string
		for _, col := range table.Columns {
			cols = append(cols, col.Name+":"+col.Type.String())
		}

		var uniqueKeys [][]string
		for _, idx := range table.Indexes {
			if idx.Unique {
				var idxCols []string
				for _, c := range idx.Columns {
					idxCols = append(idxCols, c.Name)
				}
				uniqueKeys = append(uniqueKeys, idxCols)
			}
		}

		tables = append(tables, TableInfo{
			Name:       table.Name,
			Columns:    cols,
			UniqueKeys: uniqueKeys,
		})
	}

	bytes, _ := json.MarshalIndent(tables, "", "  ")
	os.WriteFile("schema_info.json", bytes, 0644)
}
