package server

import (
	"context"
	"errors"
	"os"
	"testing"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func TestProfileBGStorageFromUserUploadSlot(t *testing.T) {
	ctx := context.Background()
	memory := storagetest.NewMemory()
	const imgPath = "user_upload/profile_bg/jp/uid_1_abcd1234.jpg"
	if err := memory.Put(ctx, imgPath, []byte("jpeg"), storage.PutOptions{}); err != nil {
		t.Fatalf("seed profile background: %v", err)
	}
	runtime := &renderapp.App{Stores: storage.Set{UserUpload: memory}}

	bgStore := profileBGStorageFor(runtime)
	path := imgPath
	if err := bgStore.DeleteProfileBackground(ctx, &drawing.ProfileBgSettings{ImgPath: &path}); err != nil {
		t.Fatalf("DeleteProfileBackground: %v", err)
	}
	if _, err := memory.Stat(ctx, imgPath); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("profile background still on the user_upload slot: %v", err)
	}
}

// TestNoWritesUnderWorkingDirectory is the E1 gate: with asset_dirs.primary
// blank and every storage slot unset, a profile background save reports
// storage.ErrNotConfigured instead of landing under the process working
// directory.
func TestNoWritesUnderWorkingDirectory(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	cfg := harukiConfig.PJSKRenderConfig{}
	cfg.AssetDirs.Primary = ""
	stores, err := buildRenderStores(cfg, nil)
	if err != nil {
		t.Fatalf("buildRenderStores: %v", err)
	}
	runtime := &renderapp.App{Stores: stores}

	_, err = profileBGStorageFor(runtime).SaveProfileBackground(context.Background(), "jp", "1", "https://images.example/bg.png")
	if !errors.Is(err, storage.ErrNotConfigured) {
		t.Fatalf("SaveProfileBackground error = %v, want storage.ErrNotConfigured", err)
	}
	if _, err := (&renderapp.App{}).Stores.Normalized().Static.Stat(context.Background(), "static_images/pjsk_3d_preview/x.png"); !errors.Is(err, storage.ErrNotConfigured) {
		t.Fatalf("unset static slot Stat error = %v, want storage.ErrNotConfigured", err)
	}

	entries, err := os.ReadDir(cwd)
	if err != nil {
		t.Fatalf("read working directory: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("writes landed under the working directory: %v", names)
	}
}
