package main

import (
	"fmt"
	"os"

	"haruki-cloud/database/sekai/migrate"
	"haruki-cloud/utils/logger"

	json "github.com/bytedance/sonic"
)

type TableInfo struct {
	Name       string     `json:"name"`
	Columns    []string   `json:"columns"`
	UniqueKeys [][]string `json:"unique_keys"`
}

func main() {
	logger.SetGlobalFileWriter(os.Stdout)
	logger.InstallStandardHandlers()
	log := logger.NewLoggerFromGlobal("Extractor")

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
		log.Error("schema extraction failed",
			"operation", "marshal",
			"error_type", fmt.Sprintf("%T", err),
		)
		os.Exit(1)
	}

	if err := os.WriteFile("schema_info.json", bytes, 0644); err != nil {
		log.Error("schema extraction failed",
			"operation", "write",
			"error_type", fmt.Sprintf("%T", err),
		)
		os.Exit(1)
	}
	log.Info("schema extraction completed", "tables", len(tables), "output_bytes", len(bytes))
}
