// asset-index bootstraps a resource catalog. Normal publication belongs to the
// region-locked Asset-Updater job; pause that job before a manual publication.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/pjsk/render/assetindex"
	"haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/s3"
	"haruki-cloud/utils/logger"
)

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("asset-index", flag.ContinueOnError)
	configPath := flags.String("config", "haruki-cloud.yaml", "Cloud config with assets storage (write credentials only when publishing)")
	region := flags.String("region", "", "jp, en, tw, kr, or cn")
	publish := flags.Bool("publish", false, "publish shards, BPM metadata and pointer; pause the region updater first")
	timeout := flags.Duration("timeout", 30*time.Minute, "total operation deadline")
	if err := flags.Parse(args); err != nil {
		return err
	}
	valid := false
	for _, r := range assetindex.Regions {
		valid = valid || *region == r
	}
	if !valid {
		return errors.New("region must be jp, en, tw, kr, or cn")
	}
	cfg, err := config.ReadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	// Open only assets; unrelated databases and writable slots are not needed.
	stores, err := storage.BuildSet(storage.SetConfig{Assets: cfg.PJSKRender.Storage.Assets, IO: cfg.PJSKRender.Storage.IO}, storage.LegacyRoots{AssetPrimary: cfg.PJSKRender.AssetDirs.Primary}, storage.Backends{S3: s3.Open}, logger.NewLoggerFromGlobal("AssetIndex"))
	if err != nil {
		return err
	}
	if stores.Assets == storage.Disabled() {
		return errors.New("assets storage is disabled")
	}
	ctx, cancel := context.WithTimeout(storage.WithBackgroundIO(ctx), *timeout)
	defer cancel()
	if !*publish {
		objects, _, err := assetindex.Inventory(ctx, stores.Assets, *region)
		if err != nil {
			return err
		}
		fmt.Printf("region=%s objects=%d publish=false\n", *region, len(objects))
		return nil
	}
	manifest, err := assetindex.Prepare(ctx, stores.Assets, *region)
	if err != nil {
		return err
	}
	bpm, err := music.BuildBPMIndex(ctx, stores.Assets, *region, manifest.Revision)
	if err != nil {
		return err
	}
	key, hash, err := music.PublishBPMIndex(ctx, stores.Assets, bpm)
	if err != nil {
		return err
	}
	manifest.BPM = &assetindex.Blob{Key: key, SHA256: hash, Revision: manifest.Revision}
	if err := assetindex.Commit(ctx, stores.Assets, manifest); err != nil {
		return err
	}
	fmt.Printf("region=%s revision=%s shards=%d publish=true\n", *region, manifest.Revision, len(manifest.Shards))
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		// Upstream errors may contain signed URLs or credentials. Detailed values
		// belong in a secured debugger, never a bootstrap job's shared stdout.
		fmt.Fprintf(os.Stderr, "asset-index failed (%T)\n", err)
		os.Exit(1)
	}
}
