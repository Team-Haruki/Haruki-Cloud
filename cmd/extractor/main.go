package main

import (
	json "github.com/bytedance/sonic"
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

	bytes, err := json.MarshalIndent(tables, "", "  ")
	if err != nil {
		_, _ = os.Stderr.WriteString("failed to marshal schema info: " + err.Error() + "\n")
		os.Exit(1)
	}

	if err := os.WriteFile("schema_info.json", bytes, 0644); err != nil {
		_, _ = os.Stderr.WriteString("failed to write schema_info.json: " + err.Error() + "\n")
		os.Exit(1)
	}
}
