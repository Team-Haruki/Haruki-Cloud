package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/database/sekai"
	"log"

	_ "github.com/lib/pq"
)

type migrateDBConfig struct {
	driver string
	dsn    string
	source string
}

func resolveMigrationDBConfig() (migrateDBConfig, error) {
	if dsn := strings.TrimSpace(os.Getenv("HARUKI_SEKAI_DB_URL")); dsn != "" {
		driver := strings.TrimSpace(os.Getenv("HARUKI_SEKAI_DB_TYPE"))
		if driver == "" {
			driver = "postgres"
		}
		return migrateDBConfig{driver: driver, dsn: dsn, source: "env:HARUKI_SEKAI_DB_URL"}, nil
	}
	if dsn := strings.TrimSpace(os.Getenv("HARUKI_SEKAI_DSN")); dsn != "" {
		driver := strings.TrimSpace(os.Getenv("HARUKI_SEKAI_DB_TYPE"))
		if driver == "" {
			driver = "postgres"
		}
		return migrateDBConfig{driver: driver, dsn: dsn, source: "env:HARUKI_SEKAI_DSN"}, nil
	}

	configPath := strings.TrimSpace(os.Getenv("HARUKI_CONFIG_PATH"))
	if configPath == "" {
		configPath = "haruki-cloud.yaml"
	}
	if _, err := os.Stat(configPath); err == nil {
		cfg, err := harukiConfig.ReadConfig(configPath)
		if err != nil {
			return migrateDBConfig{}, err
		}
		driver := strings.TrimSpace(cfg.Sekai.DBType)
		if driver == "" {
			driver = "postgres"
		}
		dsn := strings.TrimSpace(cfg.Sekai.DBURL)
		if dsn == "" {
			return migrateDBConfig{}, fmt.Errorf("sekai.db_url is empty in %s", configPath)
		}
		return migrateDBConfig{driver: driver, dsn: dsn, source: configPath}, nil
	}

	return migrateDBConfig{}, fmt.Errorf("sekai migration config is missing: set HARUKI_SEKAI_DB_URL/HARUKI_SEKAI_DSN or provide %s", configPath)
}

func main() {
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbCfg, err := resolveMigrationDBConfig()
	if err != nil {
		log.Fatalf("failed to resolve migration config: %v", err)
	}

	client, err := sekai.Open(dbCfg.driver, dbCfg.dsn)
	if err != nil {
		log.Fatalf("failed opening connection to %s: %v", dbCfg.driver, err)
	}
	defer client.Close()

	log.Printf("running sekai migration using %s", dbCfg.source)
	// Run the auto migration tool.
	if err := client.Schema.Create(rootCtx); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}
	log.Println("migration complete")
}
