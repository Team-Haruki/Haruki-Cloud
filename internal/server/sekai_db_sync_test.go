package server

import (
	"strings"
	"testing"
	"time"

	harukiConfig "haruki-cloud/config"
)

func TestBuildSekaiDBSyncSettingsDefaults(t *testing.T) {
	original := harukiConfig.Cfg
	t.Cleanup(func() {
		harukiConfig.Cfg = original
	})

	harukiConfig.Cfg.Sekai.DBType = "postgres"
	harukiConfig.Cfg.Sekai.DBURL = "host=local port=5432 user=sekai dbname=local sslmode=disable"
	harukiConfig.Cfg.Sekai.RemoteSync.SourceDBURL = "host=remote port=5432 user=sekai dbname=source sslmode=disable"

	got, err := buildSekaiDBSyncSettings()
	if err != nil {
		t.Fatalf("build settings: %v", err)
	}
	if got.PgDumpPath != "pg_dump" {
		t.Fatalf("unexpected pg_dump path: %q", got.PgDumpPath)
	}
	if got.PgRestorePath != "pg_restore" {
		t.Fatalf("unexpected pg_restore path: %q", got.PgRestorePath)
	}
	if got.Timeout != defaultSekaiDBSyncTimeout {
		t.Fatalf("unexpected timeout: %v", got.Timeout)
	}
}

func TestBuildSekaiDBSyncSettingsRejectsUnsupportedTarget(t *testing.T) {
	original := harukiConfig.Cfg
	t.Cleanup(func() {
		harukiConfig.Cfg = original
	})

	harukiConfig.Cfg.Sekai.DBType = "sqlite"
	harukiConfig.Cfg.Sekai.DBURL = "file:test.db"
	harukiConfig.Cfg.Sekai.RemoteSync.SourceDBURL = "host=remote port=5432 user=sekai dbname=source sslmode=disable"

	_, err := buildSekaiDBSyncSettings()
	if err == nil || !strings.Contains(err.Error(), "only postgres") {
		t.Fatalf("expected unsupported target error, got %v", err)
	}
}

func TestBuildSekaiDBSyncSettingsRejectsSameSourceAndTarget(t *testing.T) {
	original := harukiConfig.Cfg
	t.Cleanup(func() {
		harukiConfig.Cfg = original
	})

	dsn := "host=same port=5432 user=sekai dbname=haruki_sekai sslmode=disable"
	harukiConfig.Cfg.Sekai.DBType = "postgres"
	harukiConfig.Cfg.Sekai.DBURL = dsn
	harukiConfig.Cfg.Sekai.RemoteSync.SourceDBURL = dsn
	harukiConfig.Cfg.Sekai.RemoteSync.Timeout = 2 * time.Minute

	_, err := buildSekaiDBSyncSettings()
	if err == nil || !strings.Contains(err.Error(), "different databases") {
		t.Fatalf("expected same database error, got %v", err)
	}
}
