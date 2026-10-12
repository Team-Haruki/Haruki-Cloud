package server

import (
	"context"
	botAuth "haruki-cloud/api/bot/auth"
	"os"

	harukiConfig "haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"

	botPJSK "haruki-cloud/api/bot/pjsk"
	groupGuardAPI "haruki-cloud/api/groupguard"
	trustAPI "haruki-cloud/api/trust"
	"haruki-cloud/internal/pjsk/accountdata"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func Run(ctx context.Context) {
	switch ResolveRole(os.Args[1:], os.Getenv("HARUKI_ROLE")) {
	case RoleEvents:
		RunEvents(ctx)
		return
	case RoleMigrate:
		RunMigrate(ctx)
		return
	}
	loggerWriter := setupLogging()
	mainLogger := harukiLogger.NewLogger("Main", harukiConfig.Cfg.Backend.LogLevel, loggerWriter)
	defer closeMainLogFile(mainLogger)
	defer closeAccessLogFile(mainLogger)
	logStartupInfo(mainLogger)
	timer := newStartupTimer(mainLogger)
	if !autoMigrateEnabled() {
		mainLogger.Info("database auto-migrate disabled; apply schema changes with the migrate role")
	}
	stopDiagnostics := startDiagnosticsServer(mainLogger, harukiConfig.Cfg.Diagnostics)
	defer stopDiagnostics()
	redisClient := initRedis(ctx, mainLogger)
	app := createFiberApp(mainLogger)
	timer.phase("redis")
	usersClient := initUsers(ctx, mainLogger)
	banChecker := accountdata.NewBanService(usersClient)
	banChecker.SetReadOnly(harukiConfig.Cfg.Node.ReadOnly)
	banChecker.SetAdminQQIDs(harukiConfig.Cfg.Moderation.AdminQQIDs)
	timer.phase("users_db")
	chunithmMainClient, chunithmMusicClient := initChunithmIfEnabled(ctx, mainLogger, app, redisClient)
	timer.phase("chunithm_db")
	pjskClient := initPJSKIfEnabled(ctx, mainLogger, app, redisClient)
	timer.phase("pjsk_db")
	embeddedEvents := initEmbeddedEvents(ctx, mainLogger, app, pjskClient)
	startSekaiDBRemoteSync(ctx, mainLogger)
	sekaiClient := initSekaiIfEnabled(ctx, mainLogger)
	timer.phase("sekai_db")
	renderRuntime := initPJSKRenderIfEnabled(ctx, mainLogger, sekaiClient, pjskClient)
	timer.phase("render_runtime")
	censorService := initCensorIfEnabled(ctx, mainLogger, renderRuntime)
	timer.phase("censor_db")
	configureSekaiRuntime(mainLogger, renderRuntime, pjskClient, usersClient, banChecker, censorService)
	wireEventStreams(renderRuntime, eventStreamCloser(embeddedEvents))
	wireAliasCacheInvalidation(renderRuntime, redisClient)
	startRenderHostProbers(ctx, renderRuntime)
	startStorageDiagnostics(ctx, renderRuntime, mainLogger)
	if renderRuntime != nil && renderRuntime.Bindings != nil && !harukiConfig.Cfg.Node.ReadOnly {
		go renderRuntime.Bindings.RunProfileBGCleanup(ctx)
	}
	startImageCacheGC(ctx, harukiConfig.Cfg.PJSKRender.ImageCache, renderRuntime, mainLogger)
	if renderRuntime != nil {
		groupGuardAPI.RegisterGroupGuardRoutes(app, renderRuntime.Toolbox)
	}
	validateBotAuthSecrets(mainLogger)
	noiseKeys := initNoiseKeyRing(mainLogger)
	manifestSigner := initManifestSigner(mainLogger)
	buildPolicy := initBuildPolicy(mainLogger)
	securityMonitor := initSecurityMonitor(mainLogger, redisClient)
	authIPBan := initAuthIPBan(ctx, mainLogger, app, redisClient, securityMonitor)
	timer.phase("auth_setup")
	botDBClient := initBot(ctx, mainLogger, app, redisClient, botAuth.BotAuthOptions{
		NoiseKeys:   noiseKeys,
		BanChecker:  banChecker,
		BuildPolicy: buildPolicy,
		Security:    securityMonitor,
		IPBan:       authIPBan,
	})
	timer.phase("bot_db")
	botRouteDispatchers := botPJSK.RegisterPJSKBotRoutesWithOptions(ctx, app, renderRuntime, redisClient, botDBClient, botPJSK.BotRouteOptions{
		NoiseKeys:      noiseKeys,
		ManifestSigner: manifestSigner,
		SessionPolicy:  sessionPolicyFor(buildPolicy),
		Security:       securityMonitor,
	})
	trustAPI.RegisterTrustRoutes(app, harukiConfig.Cfg.HarukiBotDB.TrustKeysetPath)

	registerImageCacheRoute(app, harukiConfig.Cfg.PJSKRender.ImageCache, renderImageHosts(renderRuntime), mainLogger)
	timer.phase("bot_routes")

	defer closeClients(redisClient, censorService, usersClient, chunithmMainClient, chunithmMusicClient, pjskClient, sekaiClient, botDBClient)
	if botRouteDispatchers != nil {
		defer botRouteDispatchers.Close()
	}
	if renderRuntime != nil {
		defer func() { _ = renderRuntime.Close() }()
	}

	if renderRuntime != nil {
		mainLogger.Info("PJSK render runtime ready")
	}
	timer.done()

	startServer(ctx, mainLogger, app)
}
