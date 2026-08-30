package server

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/api"
	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/cache/drawingcache"
	harukiLogger "haruki-cloud/utils/logger"

	"github.com/gofiber/fiber/v3"
)

func initDrawingCacheIfConfigured(ctx context.Context, mainLogger *harukiLogger.Logger, app *fiber.App) *drawingcache.Service {
	cfg := harukiConfig.Cfg.PJSKRender.DrawingCache
	if strings.TrimSpace(cfg.StorageDir) == "" {
		return nil
	}

	service, err := drawingcache.NewService(ctx, drawingcache.Config{
		StorageDir: cfg.StorageDir,
		DBPath:     cfg.DBPath,
		GCInterval: cfg.GCInterval,
	})
	if err != nil {
		fatalStartup(mainLogger, "drawing cache initialization failed", "error_type", fmt.Sprintf("%T", err))
	}
	// Optionally gate the /cache control plane behind internal API auth. Off by
	// default; enable only once remote consumers (cache-proxy / secondary nodes)
	// send the internal token, else they get 401.
	if cfg.RequireAuth {
		app.Use(cacheRoutePrefix, api.VerifyAPIAuthorization())
		mainLogger.Info("drawing cache authorization enabled")
	}
	service.RegisterRoutes(app)

	resolved := service.Config()
	if resolved.GCInterval > 0 {
		mainLogger.Info("drawing cache API enabled",
			"route", cacheRoutePrefix,
			"gc_enabled", true,
			"gc_interval", resolved.GCInterval,
		)
	} else {
		mainLogger.Info("drawing cache API enabled",
			"route", cacheRoutePrefix,
			"gc_enabled", false,
		)
	}
	return service
}
