package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/utils/imagecache"
)

func TestChartCachePathAndLocationEdges(t *testing.T) {
	storage, public := musicChartCachePaths(rendermusic.ChartQuery{}, &drawing.GenerateMusicChartRequest{})
	if !strings.Contains(storage, "charts/black/jp/<nil>/unknown/no-skill.png") || strings.HasPrefix(public, "charts/") {
		t.Fatalf("default chart paths = %q, %q", storage, public)
	}
	storage, public = musicChartCachePaths(
		rendermusic.ChartQuery{Style: "compact", Region: "cn"},
		&drawing.GenerateMusicChartRequest{MusicID: "", Difficulty: " MASTER ", Skill: true},
	)
	if !strings.Contains(storage, "/unknown/master/skill.png") || !strings.Contains(public, "black/cn/") {
		t.Fatalf("custom chart paths = %q, %q", storage, public)
	}

	if got := chartStaticBaseURL(nil); got != "" {
		t.Fatalf("nil chart base URL = %q", got)
	}
	if got := chartStaticBaseURL(&renderapp.App{Config: renderapp.Config{ChartsBaseURL: " https://charts.test/ "}}); got != "https://charts.test" {
		t.Fatalf("explicit chart base URL = %q", got)
	}
	if got := chartStaticBaseURL(&renderapp.App{Config: renderapp.Config{ImageCacheURI: " https://images.test/ "}}); got != "https://images.test/charts" {
		t.Fatalf("fallback chart base URL = %q", got)
	}
	if got := chartStaticBaseURL(&renderapp.App{}); got != "" {
		t.Fatalf("empty chart base URL = %q", got)
	}

	for _, path := range []string{"", ".", "..", "../escape", filepath.Join(string(filepath.Separator), "absolute")} {
		if _, err := normalizeStaticImageRelativePath(path); err == nil {
			t.Fatalf("unsafe relative path %q accepted", path)
		}
	}
	if got, err := normalizeStaticImageRelativePath(" a/../b/image.png "); err != nil || got != "b/image.png" {
		t.Fatalf("normalized relative path = %q, %v", got, err)
	}

	dir := t.TempDir()
	app := &renderapp.App{Config: renderapp.Config{ImageCacheDir: dir}}
	if _, _, err := resolveStaticImageLocation(nil, "https://x", "p.png", "s.png"); err == nil {
		t.Fatal("nil static image app accepted")
	}
	if _, _, err := resolveStaticImageLocation(app, "", "p.png", "s.png"); err == nil {
		t.Fatal("empty static image base URL accepted")
	}
	if _, _, err := resolveStaticImageLocation(app, "https://x", "../p.png", "s.png"); err == nil {
		t.Fatal("unsafe public path accepted")
	}
	if _, _, err := resolveStaticImageLocation(app, "https://x", "p.png", "../s.png"); err == nil {
		t.Fatal("unsafe storage path accepted")
	}
	url, target, err := resolveStaticImageLocation(app, "https://x", "p.png", "nested/s.png")
	if err != nil || url != "https://x/p.png" || target != filepath.Join(dir, "nested", "s.png") {
		t.Fatalf("resolved static location = %q, %q, %v", url, target, err)
	}
}

func TestChartCacheReadWriteErrorEdges(t *testing.T) {
	ctx := context.Background()
	if _, _, err := cachedStaticImageMessage(context.Background(), nil, "", "p.png", "s.png"); err != nil {
		t.Fatalf("unconfigured cache lookup should be a miss: %v", err)
	}
	dir := t.TempDir()
	app := &renderapp.App{Config: renderapp.Config{ImageCacheDir: dir}}
	if message, hit, err := cachedStaticImageMessage(ctx, app, "https://x", "p.png", "missing.png"); err != nil || hit || message != nil {
		t.Fatalf("cache miss = %#v, %v, %v", message, hit, err)
	}

	rootFile := filepath.Join(t.TempDir(), "root-file")
	if err := os.WriteFile(rootFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	badRoot := &renderapp.App{Config: renderapp.Config{ImageCacheDir: rootFile}}
	if _, _, err := cachedStaticImageMessage(ctx, badRoot, "https://x", "p.png", "child.png"); err == nil {
		t.Fatal("cache stat error was ignored")
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := staticCachedImageMessageWithWriter(canceled, []byte("x"), app, "https://x", "p.png", "s.png", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled cache store = %v", err)
	}
	if _, err := staticCachedImageMessageWithWriter(ctx, []byte("x"), nil, "", "p.png", "s.png", nil); err == nil {
		t.Fatal("unconfigured cache store accepted")
	}

	cache := imagecache.New("https://cache.test", t.TempDir())
	t.Cleanup(func() { _ = cache.Close() })
	fallback := &renderapp.App{ImageCache: cache}
	if message, err := staticCachedImageMessageWithWriter(ctx, []byte("x"), fallback, "", "p.png", "s.png", nil); err != nil || len(message) != 1 {
		t.Fatalf("image cache fallback = %#v, %v", message, err)
	}

	wantErr := errors.New("write failed")
	writer := func(context.Context, string, []byte) error { return wantErr }
	if _, err := staticCachedImageMessageWithWriter(ctx, []byte("x"), app, "https://x", "write-error.png", "write-error.png", writer); !errors.Is(err, wantErr) {
		t.Fatalf("writer error = %v", err)
	}
	if err := writeStaticImageAtomically(canceled, filepath.Join(dir, "cancel.png"), []byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("atomic canceled write = %v", err)
	}
	if err := writeStaticImageAtomically(ctx, filepath.Join(dir, "missing", "file.png"), []byte("x")); err == nil {
		t.Fatal("atomic write with missing parent succeeded")
	}
}
