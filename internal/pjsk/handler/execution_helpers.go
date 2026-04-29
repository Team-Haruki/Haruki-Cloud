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
	"haruki-cloud/internal/pjsk/requestbuilder"
)

// mergeParams aliases requestbuilder.MergeParams so all bridge call sites can
// keep using the short local name while sharing one implementation.
var mergeParams = requestbuilder.MergeParams

func imageMessage(ctx context.Context, img []byte, app *renderapp.App, group string) (onebot11.Message, error) {
	tStore := time.Now()
	url, err := app.ImageCache.StoreAndGetURL(ctx, img, group)
	recordCommandStage(ctx, "image_store", time.Since(tStore))
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
	// When a CDN base URL is configured, build a direct URL by extracting the
	// "{region}-assets/..." portion of the path (e.g. "jp-assets/startapp/...").
	if app != nil {
		if base := strings.TrimRight(app.Config.AssetsBaseURL, "/"); base != "" {
			rel := filepath.ToSlash(path)
			if idx := strings.Index(rel, "-assets/"); idx > 0 {
				start := strings.LastIndex(rel[:idx], "/") + 1
				rel = rel[start:]
			}
			return onebot11.Message{onebot11.Image(base+"/"+rel, "")}, nil
		}
	}
	if app == nil {
		return nil, fmt.Errorf("image storage is not configured")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return imageMessage(ctx, data, app, group)
}
