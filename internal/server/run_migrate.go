package server

import (
	"context"
	"fmt"
	"os"
	"time"

	harukiConfig "haruki-cloud/config"
	botDB "haruki-cloud/database/bot"
	censorDB "haruki-cloud/database/censor"
	chunithmMainDB "haruki-cloud/database/chunithm/maindb"
	chunithmMusicDB "haruki-cloud/database/chunithm/music"
	sekaiDB "haruki-cloud/database/sekai"
	usersDB "haruki-cloud/database/users"
	harukiLogger "haruki-cloud/utils/logger"
)

// RoleMigrate applies the ent schemas and exits (`haruki-cloud migrate`). It
// is how schema changes reach a deployment that runs the API role with
// database.auto_migrate off: run it once per deploy before the new API
// container starts.
const RoleMigrate = "migrate"

// schemaMigration is one database's migration as the migrate role runs it:
// open, migrate, close. The enabled check mirrors the API role's init.
type schemaMigration struct {
	name    string
	enabled func() bool
	run     func(ctx context.Context, logger *harukiLogger.Logger) error
}

// schemaMigrations lists every database the API role would auto-migrate, in
// its startup order, with the same open helpers and data steps.
func schemaMigrations() []schemaMigration {
	return []schemaMigration{
		{name: "Users", enabled: usersDBConfigured, run: migrateClient(openUsersClient, func(ctx context.Context, _ *harukiLogger.Logger, c *usersDB.Client) error {
			return c.Schema.Create(ctx)
		})},
		{name: "Chunithm main", enabled: func() bool { return harukiConfig.Cfg.Chunithm.Enabled }, run: migrateClient(openChunithmMainClient, func(ctx context.Context, _ *harukiLogger.Logger, c *chunithmMainDB.Client) error {
			return c.Schema.Create(ctx)
		})},
		{name: "Chunithm music", enabled: func() bool { return harukiConfig.Cfg.Chunithm.Enabled }, run: migrateClient(openChunithmMusicClient, func(ctx context.Context, _ *harukiLogger.Logger, c *chunithmMusicDB.Client) error {
			return c.Schema.Create(ctx)
		})},
		{name: "PJSK", enabled: func() bool { return harukiConfig.Cfg.PJSK.Enabled }, run: migratePJSK},
		{name: "Sekai", enabled: func() bool { return harukiConfig.Cfg.Sekai.Enabled && harukiConfig.Cfg.Sekai.AutoMigrate }, run: migrateClient(openSekaiClient, func(ctx context.Context, _ *harukiLogger.Logger, c *sekaiDB.Client) error {
			return c.Schema.Create(ctx)
		})},
		{name: "Censor", enabled: censorDBConfigured, run: migrateClient(openCensorClient, func(ctx context.Context, _ *harukiLogger.Logger, c *censorDB.Client) error {
			return c.Schema.Create(ctx)
		})},
		{name: "Bot", enabled: func() bool { return true }, run: migrateClient(openBotClient, func(ctx context.Context, _ *harukiLogger.Logger, c *botDB.Client) error {
			return c.Schema.Create(ctx)
		})},
	}
}

func migrateClient[T interface{ Close() error }](open func() (T, error), migrate func(context.Context, *harukiLogger.Logger, T) error) func(context.Context, *harukiLogger.Logger) error {
	return func(ctx context.Context, logger *harukiLogger.Logger) error {
		client, err := open()
		if err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		defer func() { _ = client.Close() }()
		if err := migrate(ctx, logger, client); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
		return nil
	}
}

func migratePJSK(ctx context.Context, logger *harukiLogger.Logger) error {
	client, drv, err := openPJSKClient()
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = client.Close() }()
	if err := migratePJSKSchema(ctx, logger, client, drv); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	return nil
}

func openSekaiClient() (*sekaiDB.Client, error) {
	cfg := harukiConfig.Cfg.Sekai
	drv, err := openEntDriver("sekai", cfg.DBType, cfg.DBURL, cfg.DBPool())
	if err != nil {
		return nil, err
	}
	return sekaiDB.NewClient(sekaiDB.Driver(drv)), nil
}

// runSchemaMigrations applies every enabled migration in order and returns
// the first failure. Each database logs its own elapsed time.
func runSchemaMigrations(ctx context.Context, logger *harukiLogger.Logger, migrations []schemaMigration) error {
	for _, migration := range migrations {
		if !migration.enabled() {
			logger.Info("database migration skipped", "database", migration.name, "reason", "not configured")
			continue
		}
		startedAt := time.Now()
		if err := migration.run(ctx, logger); err != nil {
			return fmt.Errorf("%s: %w", migration.name, err)
		}
		logger.Info("database migration applied", "database", migration.name, "elapsed_ms", time.Since(startedAt).Milliseconds())
	}
	return nil
}

// RunMigrate boots the migrate role: it applies the schemas the API role
// would auto-migrate, then exits 0, or exits 1 on the first failure. The
// API role's own auto-migrate setting is irrelevant here.
func RunMigrate(ctx context.Context) {
	ctx = ensureContext(ctx)
	loggerWriter := setupLogging()
	mainLogger := harukiLogger.NewLogger("Migrate", harukiConfig.Cfg.Backend.LogLevel, loggerWriter)
	logStartupInfo(mainLogger)

	startedAt := time.Now()
	if err := runSchemaMigrations(ctx, mainLogger, schemaMigrations()); err != nil {
		fatalStartup(mainLogger, "database migration failed", "error", err.Error())
	}
	mainLogger.Info("database migrations complete", "elapsed_ms", time.Since(startedAt).Milliseconds())
	closeMainLogFile(mainLogger)
	os.Exit(0)
}
