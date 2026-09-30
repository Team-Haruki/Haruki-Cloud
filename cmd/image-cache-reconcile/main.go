// image-cache-reconcile inspects a bounded page of indexed objects. It never
// deletes objects; repair removes dangling index references for later redraw.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/s3"
	"haruki-cloud/utils/imagecache"
	"haruki-cloud/utils/logger"
)

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("image-cache-reconcile", flag.ContinueOnError)
	configPath := flags.String("config", "haruki-cloud.yaml", "Cloud config")
	after := flags.String("after", "", "exclusive content-hash cursor from the previous page")
	limit := flags.Int("limit", 100, "maximum objects to inspect (1–1000)")
	repair := flags.Bool("repair", false, "remove index references to confirmed missing objects; requires all writers upgraded")
	timeout := flags.Duration("timeout", 2*time.Minute, "total operation deadline")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *limit < 1 || *limit > 1000 {
		return errors.New("limit must be 1–1000")
	}
	cfg, err := config.ReadConfig(*configPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.PJSKRender.ImageCache.PGURL) == "" {
		return errors.New("image cache PostgreSQL URL is not configured")
	}
	if *repair && !cfg.PJSKRender.ImageCache.GC.ObjectDeleteEnabled {
		return errors.New("repair requires gc_object_delete_enabled after all writers are upgraded")
	}
	objects, err := openReconcileObjects(cfg.PJSKRender)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(storage.WithBackgroundIO(ctx), *timeout)
	defer cancel()
	db, err := sql.Open("postgres", cfg.PJSKRender.ImageCache.PGURL)
	if err != nil {
		return err
	}
	index := imagecache.NewPGStoreFromDB(db, imagecache.PGStoreOptions{MaxOpen: 2})
	defer index.Close()
	if err = db.PingContext(ctx); err != nil {
		return err
	}
	if err = index.InspectSchema(ctx); err != nil {
		return err
	}

	hashes, err := index.ReconcileCandidates(ctx, *after, *limit)
	if err != nil {
		return err
	}
	missing := 0
	cursor := *after
	for _, hash := range hashes {
		absent, err := index.ReconcileObject(ctx, objects, hash, *repair)
		if err != nil {
			return err
		}
		if absent {
			missing++
		}
		cursor = hash
	}
	fmt.Printf("checked=%d missing=%d repair=%t next_after=%s\n", len(hashes), missing, *repair, cursor)
	return nil
}

// Candidate rows are exclusively garage objects with bucket-relative paths.
// A local override wins in the server too; inspecting it as Garage would turn
// unrelated local misses into deletions of valid remote index references.
func openReconcileObjects(cfg config.PJSKRenderConfig) (storage.Store, error) {
	if strings.TrimSpace(cfg.ImageCache.Dir) != "" {
		return nil, errors.New("image_cache.dir selects local storage; garage reconciliation requires an explicit S3 target without that override")
	}
	resolved, err := storage.Resolve(cfg.Storage.ImageCache)
	if err != nil {
		return nil, err
	}
	if resolved.Scheme != storage.SchemeS3 || resolved.Root != "" {
		return nil, errors.New("garage reconciliation requires storage.image_cache.scheme=s3 with an empty root")
	}
	stores, err := storage.BuildSet(storage.SetConfig{ImageCache: cfg.Storage.ImageCache, IO: cfg.Storage.IO}, storage.LegacyRoots{}, storage.Backends{S3: s3.Open}, logger.NewLoggerFromGlobal("ImageCacheReconcile"))
	if err != nil {
		return nil, err
	}
	return stores.ImageCache, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "image-cache-reconcile failed (%T)\n", err)
		os.Exit(1)
	}
}
