package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	pjskenttest "haruki-cloud/database/pjsk/enttest"
	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/pjsk/meta"
	renderregion "haruki-cloud/internal/pjsk/region"

	_ "github.com/mattn/go-sqlite3"
)

func TestNewBuildsNilDatabaseRuntime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assetDir := t.TempDir()
	loader := meta.NewLoader(nil)

	runtime := New(nil, nil, Config{
		InitContext:             ctx,
		DefaultRegion:           renderregion.Unknown,
		MetaLoader:              loader,
		AssetPrimaryDir:         assetDir,
		DrawingBaseURL:          "http://127.0.0.1:1",
		DrawingTimeout:          time.Second,
		DrawingRetryCount:       2,
		DrawingSKMaxConcurrency: 1,
		DrawingSKAcquireTimeout: time.Millisecond,
		DrawingMaxConcurrency:   2,
		ReadOnly:                true,
	})
	if runtime == nil {
		t.Fatal("New returned nil")
	}
	if runtime.Config.DefaultRegion != renderregion.JP {
		t.Fatalf("default region = %s", runtime.Config.DefaultRegion)
	}
	if runtime.MetaLoader != loader || runtime.Provider != nil || len(runtime.Providers) != 0 {
		t.Fatalf("unexpected provider state: provider=%#v providers=%#v", runtime.Provider, runtime.Providers)
	}
	if runtime.Decks == nil || runtime.Edu == nil || runtime.Inventory == nil || runtime.Misc == nil ||
		runtime.MySekai == nil || runtime.Score == nil || runtime.SK == nil {
		t.Fatalf("runtime omitted always-available controllers: %#v", runtime)
	}
	if runtime.Cards != nil || runtime.Music != nil || runtime.Aliases != nil {
		t.Fatalf("database controllers unexpectedly initialized: %#v", runtime)
	}
	roots := runtime.AssetRoots()
	if len(roots) != 1 || roots[0] != assetDir {
		t.Fatalf("asset roots = %#v", roots)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := (*App)(nil).Close(); err != nil || (*App)(nil).AssetRoots() != nil {
		t.Fatalf("nil app helpers = %v", err)
	}
}

func TestNewBuildsRegionalDatabaseRuntimeAndFallbacks(t *testing.T) {
	suffix := time.Now().UnixNano()
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:app_sekai_%d?mode=memory&cache=shared&_fk=1", suffix))
	pjskClient := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:app_pjsk_%d?mode=memory&cache=shared&_fk=1", suffix))

	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	userSnapshot := filepath.Join(root, "user.json")
	if err := os.WriteFile(userSnapshot, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runtime := New(sekaiClient, pjskClient, Config{
		InitContext:   ctx,
		DefaultRegion: renderregion.CN,
		MetaLoader:    meta.NewLoader(nil),
		LocalMasterdata: LocalMasterdataConfig{
			Enabled:       true,
			AllowFallback: true,
			AllowLeaks:    true,
			Dir:           masterdataDir,
		},
		UserSnapshot: UserSnapshotConfig{
			Provider:      "toolbox",
			AllowFallback: true,
			UserJSON:      userSnapshot,
		},
	})
	if runtime.Provider == nil || len(runtime.Providers) != 5 {
		t.Fatalf("regional providers = %d, primary=%#v", len(runtime.Providers), runtime.Provider)
	}
	for _, region := range []renderregion.Value{renderregion.JP, renderregion.CN, renderregion.TW, renderregion.KR, renderregion.EN} {
		if runtime.Providers[region] == nil {
			t.Errorf("provider for %s is nil", region)
		}
	}
	if runtime.Cards == nil || runtime.Costumes == nil || runtime.Events == nil || runtime.Gachas == nil ||
		runtime.Honors == nil || runtime.Music == nil || runtime.Profiles == nil || runtime.Stamps == nil ||
		runtime.VLive == nil || runtime.Aliases == nil || runtime.Snapshots == nil {
		t.Fatalf("database runtime omitted controllers: %#v", runtime)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close database runtime: %v", err)
	}
}

func TestFallbackFeatureGates(t *testing.T) {
	if shouldEnableLocalMasterdataFallback(Config{}) {
		t.Fatal("disabled local masterdata fallback was enabled")
	}
	if shouldEnableLocalMasterdataFallback(Config{LocalMasterdata: LocalMasterdataConfig{Enabled: true, Dir: "/tmp"}}) {
		t.Fatal("local masterdata without an allowed use was enabled")
	}
	if shouldEnableLocalMasterdataFallback(Config{LocalMasterdata: LocalMasterdataConfig{Enabled: true, AllowFallback: true, Dir: " "}}) {
		t.Fatal("blank local masterdata directory was enabled")
	}
	if !shouldEnableLocalMasterdataFallback(Config{LocalMasterdata: LocalMasterdataConfig{Enabled: true, AllowLeaks: true, Dir: "/tmp"}}) {
		t.Fatal("explicit local masterdata leak fallback was disabled")
	}

	base := Config{UserSnapshot: UserSnapshotConfig{AllowFallback: true, UserJSON: "/tmp/user.json"}}
	for _, providerName := range []string{"", "local_file", "toolbox", "internal_cloud", " TOOLBOX "} {
		cfg := base
		cfg.UserSnapshot.Provider = providerName
		if !shouldEnableLocalSnapshotFallback(cfg) {
			t.Errorf("snapshot provider %q was disabled", providerName)
		}
	}
	for _, cfg := range []Config{
		{},
		{UserSnapshot: UserSnapshotConfig{AllowFallback: true}},
		{UserSnapshot: UserSnapshotConfig{AllowFallback: true, UserJSON: "/tmp/user.json", Provider: "remote"}},
	} {
		if shouldEnableLocalSnapshotFallback(cfg) {
			t.Fatalf("invalid snapshot fallback was enabled: %#v", cfg.UserSnapshot)
		}
	}
}
