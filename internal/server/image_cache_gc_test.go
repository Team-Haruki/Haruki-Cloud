package server

import (
	"bytes"
	"context"
	"strings"
	"testing"

	harukiConfig "haruki-cloud/config"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/imagecache"
	harukiLogger "haruki-cloud/utils/logger"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStartImageCacheGC(t *testing.T) {
	var logs bytes.Buffer
	log := harukiLogger.NewLogger("GCStart", "DEBUG", &logs)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if gc := startImageCacheGC(ctx, harukiConfig.ImageCacheConfig{}, &renderapp.App{}, log); gc != nil {
		t.Fatal("gc_enabled=false must not start GC")
	}
	if logs.Len() != 0 {
		t.Fatalf("disabled GC logged:\n%s", logs.String())
	}

	cfg := harukiConfig.ImageCacheConfig{}
	cfg.GC.Enabled = true
	if gc := startImageCacheGC(ctx, cfg, nil, log); gc != nil {
		t.Fatal("GC without a runtime must not start")
	}
	if gc := startImageCacheGC(ctx, cfg, &renderapp.App{}, log); gc != nil {
		t.Fatal("GC without an index must not start")
	}
	if strings.Count(logs.String(), "gc not started") != 2 {
		t.Fatalf("logs:\n%s", logs.String())
	}

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	runtime := &renderapp.App{ImageIndex: imagecache.NewPGStoreFromDB(db, imagecache.PGStoreOptions{})}
	if gc := startImageCacheGC(ctx, cfg, runtime, log); gc == nil {
		t.Fatal("enabled GC with an index did not start")
	}
}
