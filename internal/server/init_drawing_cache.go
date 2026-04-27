package server

import (
	"context"
	"os"
	"strings"

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
		mainLogger.Errorf("Failed to initialize drawing cache service: %v", err)
		os.Exit(1)
	}
	service.RegisterRoutes(app)

	resolved := service.Config()
	if resolved.GCInterval > 0 {
		mainLogger.Infof("Drawing cache API enabled at /cache (storage=%s db=%s gc=%s)", resolved.StorageDir, resolved.DBPath, resolved.GCInterval)
	} else {
		mainLogger.Infof("Drawing cache API enabled at /cache (storage=%s db=%s gc=disabled)", resolved.StorageDir, resolved.DBPath)
	}
	return service
}
