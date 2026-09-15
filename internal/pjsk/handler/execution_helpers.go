package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"haruki-cloud/internal/onebot11"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/requestbuilder"
)

// mergeParams aliases requestbuilder.MergeParams so all bridge call sites can
// keep using the short local name while sharing one implementation.
var mergeParams = requestbuilder.MergeParams

func imageMessage(ctx context.Context, img []byte, app *renderapp.App, group string) (onebot11.Message, error) {
	tStore := time.Now()
	url, err := app.ImageCache.StoreAndGetURL(ctx, img, group)
	recordCommandStage(ctx, "image.store", time.Since(tStore))
	if err != nil {
		return nil, err
	}
	return onebot11.Message{onebot11.Image(url, "")}, nil
}

func assetImageMessage(ctx context.Context, path string, app *renderapp.App, group string) (onebot11.Message, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("asset path is empty")
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return onebot11.Message{onebot11.Image(path, "")}, nil
	}
	if app == nil {
		return nil, fmt.Errorf("image storage is not configured")
	}
	// With public asset hosts configured, send a direct URL built by the one
	// Drawing-path -> URL rule instead of relaying the bytes.
	if hosts := app.AssetHosts; hosts.Len() > 0 {
		url, err := assets.PublicAssetURL(hosts.Base(""), path)
		if err != nil {
			return nil, err
		}
		return onebot11.Message{onebot11.Image(url, "")}, nil
	}
	startedAt := time.Now()
	data, err := readAssetBytes(ctx, app, path)
	recordCommandStage(ctx, "asset.read", time.Since(startedAt))
	if err != nil {
		return nil, err
	}
	return imageMessage(ctx, data, app, group)
}

// readAssetBytes reads one asset through the app's AssetReader. Without a
// store, an absolute path (a probe already resolved it on local disk) is read
// directly, exactly as before the reader existed.
func readAssetBytes(ctx context.Context, app *renderapp.App, path string) ([]byte, error) {
	reader := assets.ReaderOr(app.AssetReader, app.Assets)
	if !reader.UsesStore() && filepath.IsAbs(path) {
		return os.ReadFile(path)
	}
	data, _, err := reader.ReadFirst(ctx, path)
	return data, err
}
