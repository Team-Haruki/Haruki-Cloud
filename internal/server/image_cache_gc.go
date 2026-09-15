package server

import (
	"context"

	harukiConfig "haruki-cloud/config"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/imagecache"
	harukiLogger "haruki-cloud/utils/logger"
)

// startImageCacheGC starts the image cache GC with the run context when
// image_cache.gc_enabled is set and the index is open. It returns the
// collector (nil when not started).
func startImageCacheGC(ctx context.Context, cfg harukiConfig.ImageCacheConfig, runtime *renderapp.App, log *harukiLogger.Logger) *imagecache.GC {
	if !cfg.GC.Enabled {
		return nil
	}
	if runtime == nil || runtime.ImageIndex == nil {
		log.Warn("image cache gc enabled but the image cache index is not available; gc not started")
		return nil
	}
	gc := imagecache.NewGC(runtime.ImageIndex, runtime.Stores.Normalized().ImageCache, imagecache.GCConfig{
		Enabled:             true,
		DryRun:              cfg.GC.DryRunEnabled(),
		Interval:            cfg.GC.Interval,
		Batch:               cfg.GC.Batch,
		ObjectRetentionDays: cfg.GC.ObjectRetentionDays,
	}, harukiLogger.NewLoggerFromGlobal("ImageCacheGC"))
	gc.Start(ctx)
	return gc
}
