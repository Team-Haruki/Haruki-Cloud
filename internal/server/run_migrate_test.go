package server

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	harukiConfig "haruki-cloud/config"
	usersDB "haruki-cloud/database/users"
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

func TestMigrateRoleAppliesEveryConfiguredSQLiteSchema(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{}
	harukiConfig.Cfg.UsersDB.DBType = "sqlite3"
	harukiConfig.Cfg.UsersDB.DBURL = serverCoverageSQLiteDSN(t, "migrate-users")
	harukiConfig.Cfg.Chunithm.Enabled = true
	harukiConfig.Cfg.Chunithm.BindingDBType = "sqlite3"
	harukiConfig.Cfg.Chunithm.BindingDBURL = serverCoverageSQLiteDSN(t, "migrate-chunithm-main")
	harukiConfig.Cfg.Chunithm.MusicDBType = "sqlite3"
	harukiConfig.Cfg.Chunithm.MusicDBURL = serverCoverageSQLiteDSN(t, "migrate-chunithm-music")
	harukiConfig.Cfg.PJSK.Enabled = true
	harukiConfig.Cfg.PJSK.DBType = "sqlite3"
	harukiConfig.Cfg.PJSK.DBURL = serverCoverageSQLiteDSN(t, "migrate-pjsk")
	harukiConfig.Cfg.Sekai.Enabled = true
	harukiConfig.Cfg.Sekai.AutoMigrate = true
	harukiConfig.Cfg.Sekai.DBType = "sqlite3"
	harukiConfig.Cfg.Sekai.DBURL = serverCoverageSQLiteDSN(t, "migrate-sekai")
	harukiConfig.Cfg.Censor.CensorDBType = "sqlite3"
	harukiConfig.Cfg.Censor.CensorDBURL = serverCoverageSQLiteDSN(t, "migrate-censor")
	harukiConfig.Cfg.HarukiBotDB.DBType = "sqlite3"
	harukiConfig.Cfg.HarukiBotDB.DBURL = serverCoverageSQLiteDSN(t, "migrate-bot")

	var output bytes.Buffer
	if err := runSchemaMigrations(context.Background(), startupTestLogger(&output), schemaMigrations()); err != nil {
		t.Fatalf("runSchemaMigrations: %v\n%s", err, output.String())
	}
	for _, name := range []string{"Users", "Chunithm main", "Chunithm music", "PJSK", "Sekai", "Censor", "Bot"} {
		if !strings.Contains(output.String(), "database migration applied") || !strings.Contains(output.String(), name) {
			t.Fatalf("migration of %q not logged:\n%s", name, output.String())
		}
	}
}

func TestSchemaMigrationsSurfaceConnectAndSchemaFailures(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{}
	logger := startupTestLogger(&bytes.Buffer{})
	ctx := context.Background()

	// An unsupported driver fails at connect for every open helper.
	harukiConfig.Cfg.UsersDB.DBType = "missing-driver"
	harukiConfig.Cfg.UsersDB.DBURL = "dsn"
	harukiConfig.Cfg.Chunithm.BindingDBType = "missing-driver"
	harukiConfig.Cfg.Chunithm.MusicDBType = "missing-driver"
	harukiConfig.Cfg.PJSK.DBType = "missing-driver"
	harukiConfig.Cfg.Sekai.DBType = "missing-driver"
	harukiConfig.Cfg.Censor.CensorDBType = "missing-driver"
	harukiConfig.Cfg.HarukiBotDB.DBType = "missing-driver"
	if _, err := openUsersClient(); err == nil {
		t.Fatal("users: unsupported driver accepted")
	}
	if _, err := openChunithmMainClient(); err == nil {
		t.Fatal("chunithm main: unsupported driver accepted")
	}
	if _, err := openChunithmMusicClient(); err == nil {
		t.Fatal("chunithm music: unsupported driver accepted")
	}
	if _, _, err := openPJSKClient(); err == nil {
		t.Fatal("pjsk: unsupported driver accepted")
	}
	if _, err := openSekaiClient(); err == nil {
		t.Fatal("sekai: unsupported driver accepted")
	}
	if _, err := openCensorClient(); err == nil {
		t.Fatal("censor: unsupported driver accepted")
	}
	if _, err := openBotClient(); err == nil {
		t.Fatal("bot: unsupported driver accepted")
	}
	if err := migratePJSK(ctx, logger); err == nil || !strings.HasPrefix(err.Error(), "connect: ") {
		t.Fatalf("migratePJSK connect error = %v", err)
	}
	run := migrateClient(openUsersClient, func(context.Context, *harukiLogger.Logger, *usersDB.Client) error { return nil })
	if err := run(ctx, logger); err == nil || !strings.HasPrefix(err.Error(), "connect: ") {
		t.Fatalf("migrateClient connect error = %v", err)
	}

	// A migration step failure is reported as a schema error and the client is closed.
	client := &startupTestClient{}
	wantErr := errors.New("create failed")
	run = migrateClient(func() (*startupTestClient, error) { return client, nil },
		func(context.Context, *harukiLogger.Logger, *startupTestClient) error { return wantErr })
	if err := run(ctx, logger); !errors.Is(err, wantErr) || !strings.HasPrefix(err.Error(), "schema: ") {
		t.Fatalf("migrateClient schema error = %v", err)
	}
	if !client.closed {
		t.Fatal("client was not closed after a failed migration")
	}

	// Schema.Create on a closed PJSK client fails inside migratePJSKSchema.
	harukiConfig.Cfg.PJSK.DBType = "sqlite3"
	harukiConfig.Cfg.PJSK.DBURL = serverCoverageSQLiteDSN(t, "migrate-pjsk-closed")
	pjskClient, drv, err := openPJSKClient()
	if err != nil {
		t.Fatal(err)
	}
	_ = pjskClient.Close()
	if err := migratePJSKSchema(ctx, logger, pjskClient, drv); err == nil {
		t.Fatal("Schema.Create on a closed client succeeded")
	}
	harukiConfig.Cfg.PJSK.DBURL = "file:" + t.TempDir() + "/missing/pjsk.db?mode=ro"
	if err := migratePJSK(ctx, logger); err == nil {
		t.Fatal("migratePJSK against an unopenable database succeeded")
	}
}
