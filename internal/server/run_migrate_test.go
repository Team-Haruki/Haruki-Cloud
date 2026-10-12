package server

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	harukiConfig "haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"
)

func boolPtr(v bool) *bool { return &v }

func TestInitDBClientSkipsSchemaWhenAutoMigrateDisabled(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg.Database.AutoMigrate = boolPtr(false)

	client := &startupTestClient{}
	schemaCalled := false
	got := initDBClient(context.Background(), startupTestLogger(&bytes.Buffer{}), "test",
		func() (*startupTestClient, error) { return client, nil },
		func(*startupTestClient, context.Context) error {
			schemaCalled = true
			return errors.New("schema must not run")
		},
	)
	if got != client || schemaCalled {
		t.Fatal("auto-migrate off still ran the schema callback")
	}
	if client.interceptors != 1 || client.hooks != 1 {
		t.Fatalf("tracing registration = (%d interceptors, %d hooks)", client.interceptors, client.hooks)
	}

	harukiConfig.Cfg.Database.AutoMigrate = nil
	if !autoMigrateEnabled() {
		t.Fatal("unset database.auto_migrate must default to enabled")
	}
}

func TestRunSchemaMigrationsOrderSkipAndFailure(t *testing.T) {
	var output bytes.Buffer
	logger := startupTestLogger(&output)
	var ran []string
	migration := func(name string, err error) schemaMigration {
		return schemaMigration{name: name, enabled: func() bool { return true }, run: func(context.Context, *harukiLogger.Logger) error {
			ran = append(ran, name)
			return err
		}}
	}
	skipped := schemaMigration{name: "Skipped", enabled: func() bool { return false }, run: func(context.Context, *harukiLogger.Logger) error {
		t.Fatal("disabled migration ran")
		return nil
	}}

	if err := runSchemaMigrations(context.Background(), logger, []schemaMigration{migration("A", nil), skipped, migration("B", nil)}); err != nil {
		t.Fatalf("runSchemaMigrations: %v", err)
	}
	if strings.Join(ran, ",") != "A,B" {
		t.Fatalf("ran = %v", ran)
	}
	logs := output.String()
	if !strings.Contains(logs, "database=Skipped") || !strings.Contains(logs, "database=A") {
		t.Fatalf("migration log missing entries:\n%s", logs)
	}

	ran = nil
	wantErr := errors.New("boom")
	err := runSchemaMigrations(context.Background(), logger, []schemaMigration{migration("A", wantErr), migration("B", nil)})
	if !errors.Is(err, wantErr) || !strings.HasPrefix(err.Error(), "A: ") {
		t.Fatalf("error = %v", err)
	}
	if strings.Join(ran, ",") != "A" {
		t.Fatalf("migration continued after failure: %v", ran)
	}
}

func TestSchemaMigrationsFollowConfiguredDatabases(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{}
	harukiConfig.Cfg.PJSK.Enabled = true
	harukiConfig.Cfg.Sekai.Enabled = true
	harukiConfig.Cfg.Censor.CensorDBType = "sqlite3"
	harukiConfig.Cfg.Censor.CensorDBURL = "file:censor?mode=memory"

	enabled := map[string]bool{}
	for _, migration := range schemaMigrations() {
		enabled[migration.name] = migration.enabled()
	}
	want := map[string]bool{
		"Users": false, "Chunithm main": false, "Chunithm music": false,
		"PJSK": true, "Sekai": false, "Censor": true, "Bot": true,
	}
	for name, wantEnabled := range want {
		if got, ok := enabled[name]; !ok || got != wantEnabled {
			t.Fatalf("migration %q enabled = %v (present %v), want %v", name, got, ok, wantEnabled)
		}
	}

	harukiConfig.Cfg.Sekai.AutoMigrate = true
	harukiConfig.Cfg.UsersDB.DBType = "sqlite3"
	harukiConfig.Cfg.UsersDB.DBURL = "file:users?mode=memory"
	for _, migration := range schemaMigrations() {
		if (migration.name == "Sekai" || migration.name == "Users") && !migration.enabled() {
			t.Fatalf("migration %q should be enabled", migration.name)
		}
	}
}

func TestMigrateRoleAppliesSQLiteSchemas(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{}
	harukiConfig.Cfg.Database.AutoMigrate = boolPtr(false)
	harukiConfig.Cfg.PJSK.Enabled = true
	harukiConfig.Cfg.PJSK.DBType = "sqlite3"
	harukiConfig.Cfg.PJSK.DBURL = serverCoverageSQLiteDSN(t, "migrate-pjsk")
	harukiConfig.Cfg.HarukiBotDB.DBType = "sqlite3"
	harukiConfig.Cfg.HarukiBotDB.DBURL = serverCoverageSQLiteDSN(t, "migrate-bot")

	var output bytes.Buffer
	logger := startupTestLogger(&output)
	if err := runSchemaMigrations(context.Background(), logger, schemaMigrations()); err != nil {
		t.Fatalf("runSchemaMigrations: %v\n%s", err, output.String())
	}
	for _, want := range []string{"database=PJSK", "database=Bot", "database migration applied"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("migration log missing %q:\n%s", want, output.String())
		}
	}
}
