package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/onebot11"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	renderstamp "haruki-cloud/internal/pjsk/render/stamp"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/imagecache"
)

func imageSegmentFile(t *testing.T, message onebot11.Message) string {
	t.Helper()
	if len(message) != 1 || message[0].Type != onebot11.TypeImage {
		t.Fatalf("message = %+v", message)
	}
	data, ok := message[0].Data.(onebot11.ImageData)
	if !ok {
		t.Fatalf("image data = %#v", message[0].Data)
	}
	return data.File
}

func TestAssetImageMessageUsesPublicAssetURL(t *testing.T) {
	hosts, err := urlhost.FromList([]string{"https://assets-cn09.example/", "https://assets-cn01.example"}, urlhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	app := &renderapp.App{AssetHosts: hosts}
	ctx := context.Background()
	seen := map[string]bool{}
	for range 2 {
		message, err := assetImageMessage(ctx, "/srv/assets/jp-assets/startapp/music/jacket/j/j.png", app, BotModulePJSK)
		if err != nil {
			t.Fatal(err)
		}
		seen[imageSegmentFile(t, message)] = true
	}
	if !seen["https://assets-cn09.example/jp-assets/startapp/music/jacket/j/j.png"] || !seen["https://assets-cn01.example/jp-assets/startapp/music/jacket/j/j.png"] {
		t.Fatalf("host rotation urls = %v", seen)
	}
	if _, err := assetImageMessage(ctx, "../etc/passwd", app, BotModulePJSK); err == nil {
		t.Fatal("traversal path must be rejected")
	}
	message, err := assetImageMessage(ctx, "https://cdn.example/x.png", nil, BotModulePJSK)
	if err != nil || imageSegmentFile(t, message) != "https://cdn.example/x.png" {
		t.Fatalf("passthrough = %+v, %v", message, err)
	}
	if _, err := assetImageMessage(ctx, "asset/jp-assets/x.png", nil, BotModulePJSK); err == nil {
		t.Fatal("nil app must fail")
	}
	if _, err := assetImageMessage(ctx, " ", app, BotModulePJSK); err == nil {
		t.Fatal("empty path must fail")
	}
}

func TestAssetImageMessageReadsBytesThroughReader(t *testing.T) {
	ctx := context.Background()
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/stamp/a/a.png": []byte("png")})
	app := &renderapp.App{
		AssetReader: assets.NewAssetReader(nil, memory),
		ImageCache:  imagecache.New("https://image-cache.test", t.TempDir()),
	}
	message, err := assetImageMessage(ctx, "asset/jp-assets/startapp/stamp/a/a.png", app, BotModulePJSK)
	if err != nil || !strings.HasPrefix(imageSegmentFile(t, message), "https://image-cache.test/") {
		t.Fatalf("store bytes message = %+v, %v", message, err)
	}
	if _, err := assetImageMessage(ctx, "asset/jp-assets/startapp/stamp/b/b.png", app, BotModulePJSK); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("store miss = %v", err)
	}

	root := t.TempDir()
	full := filepath.Join(root, "jp-assets", "startapp", "stamp", "c", "c.png")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := &renderapp.App{Assets: assets.NewAssetHelper(root, nil), ImageCache: imagecache.New("https://image-cache.test", t.TempDir())}
	for _, path := range []string{full, "asset/jp-assets/startapp/stamp/c/c.png"} {
		if _, err := assetImageMessage(ctx, path, legacy, BotModulePJSK); err != nil {
			t.Fatalf("legacy read %q: %v", path, err)
		}
	}
}

func TestDirectStampURLPath(t *testing.T) {
	cases := []struct {
		region renderregion.Value
		in     string
		want   string
	}{
		{"", "stamp/a/a.png", "asset/jp-assets/startapp/stamp/a/a.png"},
		{renderregion.TW, "/stamp/a/a.png", "asset/tw-assets/startapp/stamp/a/a.png"},
		{renderregion.EN, "jp-assets/ondemand/stamp/a/a.png", "jp-assets/ondemand/stamp/a/a.png"},
		{renderregion.JP, "https://cdn.example/a.png", "https://cdn.example/a.png"},
	}
	for _, tc := range cases {
		if got := directStampURLPath(tc.region, tc.in); got != tc.want {
			t.Fatalf("directStampURLPath(%q, %q) = %q, want %q", tc.region, tc.in, got, tc.want)
		}
	}
}

// The stamp Drawing field keeps the controller's string while the direct-send
// URL uses the startapp-first candidate.
func TestResolveDirectStampImageUsesStartappURL(t *testing.T) {
	ctx, controller, app, _ := newStampExecutionFixture(t)
	req, err := controller.BuildStampListRequest(renderstamp.ListQuery{IDs: []int{1}, Region: renderregion.JP})
	if err != nil || req == nil || len(req.Stamps) != 1 || req.Stamps[0].ImagePath != "stamp/stamp_a/stamp_a.png" {
		t.Fatalf("drawing request = %+v, %v", req, err)
	}
	message, ok, err := resolveDirectStampImage(ctx, controller, app, renderstamp.ListQuery{IDs: []int{1}, Region: renderregion.JP})
	if !ok || err != nil {
		t.Fatalf("direct stamp = %v %v", ok, err)
	}
	if got := imageSegmentFile(t, message); got != "https://cdn.example/jp-assets/startapp/stamp/stamp_a/stamp_a.png" {
		t.Fatalf("direct stamp url = %q", got)
	}
}
