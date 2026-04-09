package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMigrationDBConfigPrefersEnv(t *testing.T) {
	t.Setenv("HARUKI_SEKAI_DB_URL", "postgres://env-dsn")
	t.Setenv("HARUKI_SEKAI_DB_TYPE", "postgres")
	t.Setenv("HARUKI_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))

	cfg, err := resolveMigrationDBConfig()
	if err != nil {
		t.Fatalf("resolveMigrationDBConfig() error = %v", err)
	}
	if cfg.dsn != "postgres://env-dsn" || cfg.source != "env:HARUKI_SEKAI_DB_URL" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestResolveMigrationDBConfigFallsBackToConfigFile(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "haruki-db-configs.yaml")
	content := []byte("sekai:\n  db_type: postgres\n  db_url: host=localhost port=5432 user=test password=test dbname=test sslmode=disable\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("HARUKI_SEKAI_DB_URL", "")
	t.Setenv("HARUKI_SEKAI_DSN", "")
	t.Setenv("HARUKI_CONFIG_PATH", configPath)

	cfg, err := resolveMigrationDBConfig()
	if err != nil {
		t.Fatalf("resolveMigrationDBConfig() error = %v", err)
	}
	if cfg.driver != "postgres" || cfg.source != configPath {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestResolveMigrationDBConfigReturnsErrorWithoutSource(t *testing.T) {
	t.Setenv("HARUKI_SEKAI_DB_URL", "")
	t.Setenv("HARUKI_SEKAI_DSN", "")
	t.Setenv("HARUKI_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))

	if _, err := resolveMigrationDBConfig(); err == nil {
		t.Fatalf("expected error when no migration config source is available")
	}
}
