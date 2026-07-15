package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/chartstyle"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/utils/logger"

	"golang.org/x/sync/singleflight"
)

var staticImageCacheWrites singleflight.Group

const staticImageSharedTimeout = 30 * time.Second

type staticImageFlightToken byte

type staticImageFlightResult struct {
	err        error
	operations []commandtrace.Stats
	leader     *staticImageFlightToken
}

type staticImageWriter func(context.Context, string, []byte) error

func renderMusicChartMessage(rc *RequestContext, musicCtrl *rendermusic.Controller, query rendermusic.ChartQuery) (onebot11.Message, error) {
	if rc == nil || musicCtrl == nil {
		return nil, fmt.Errorf("music chart renderer is not configured")
	}

	payload, err := musicCtrl.BuildMusicChartRequest(query)
	if err != nil {
		if ids := rendermusic.ExtractAmbiguousMusicIDs(err); len(ids) > 1 {
			return renderAmbiguousMusicIDsMessages(rc, musicCtrl, query.Region, err, ids)
		}
		return nil, err
	}

	storageRelativePath, publicRelativePath := musicChartCachePaths(query, payload)
	if cached, hit, err := cachedStaticImageMessage(rc.Ctx, rc.App, chartStaticBaseURL(rc.App), publicRelativePath, storageRelativePath); err != nil {
		return nil, err
	} else if hit {
		return cached, nil
	}

	data, err := musicCtrl.RenderMusicChartRequest(payload)
	if err != nil {
		return nil, err
	}
	return staticCachedImageMessage(rc.Ctx, data, rc.App, chartStaticBaseURL(rc.App), publicRelativePath, storageRelativePath)
}

func musicChartCachePaths(query rendermusic.ChartQuery, payload *drawing.GenerateMusicChartRequest) (string, string) {
	style := chartstyle.Normalize(query.Style)
	if style == "" {
		style = chartstyle.Default
	}

	region := regionWithDefault(query.Region)
	musicID := strings.TrimSpace(fmt.Sprint(payload.MusicID))
	if musicID == "" {
		musicID = "unknown"
	}

	difficulty := strings.ToLower(strings.TrimSpace(payload.Difficulty))
	if difficulty == "" {
		difficulty = "unknown"
	}
	variant := "no-skill"
	if payload.Skill {
		variant = "skill"
	}
	fileName := variant + ".png"

	storageRelativePath := filepath.ToSlash(filepath.Join("charts", style, region, musicID, difficulty, fileName))
	publicRelativePath := filepath.ToSlash(filepath.Join(style, region, musicID, difficulty, fileName))
	return storageRelativePath, publicRelativePath
}

func cachedStaticImageMessage(ctx context.Context, app *renderapp.App, baseURL string, publicRelativePath string, storageRelativePath string) (onebot11.Message, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	finishLookup := commandtrace.MeasureOperation(ctx, "image.static_cache_lookup")
	defer finishLookup()
	url, targetPath, err := resolveStaticImageLocation(app, baseURL, publicRelativePath, storageRelativePath)
	if err != nil {
		return nil, false, nil
	}
	if _, err := os.Stat(targetPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return onebot11.Message{onebot11.Image(url, "")}, true, nil
}

func staticCachedImageMessage(ctx context.Context, data []byte, app *renderapp.App, baseURL string, publicRelativePath string, storageRelativePath string) (onebot11.Message, error) {
	return staticCachedImageMessageWithWriter(ctx, data, app, baseURL, publicRelativePath, storageRelativePath, writeStaticImageAtomically)
}

func staticCachedImageMessageWithWriter(ctx context.Context, data []byte, app *renderapp.App, baseURL string, publicRelativePath string, storageRelativePath string, write staticImageWriter) (onebot11.Message, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	finishStore := commandtrace.MeasureOperation(ctx, "image.static_cache_store")
	defer finishStore()
	url, targetPath, err := resolveStaticImageLocation(app, baseURL, publicRelativePath, storageRelativePath)
	if err != nil {
		if app == nil || app.ImageCache == nil {
			return nil, err
		}
		return imageMessage(ctx, data, app, BotModulePJSK)
	}

	finishCopy := commandtrace.MeasureOperation(ctx, "image.static_cache_copy")
	ownedData := bytes.Clone(data)
	finishCopy()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if write == nil {
		write = writeStaticImageAtomically
	}
	callerToken := new(staticImageFlightToken)
	resultCh := staticImageCacheWrites.DoChan(targetPath, func() (any, error) {
		sharedBase, cancel := staticImageSharedContext()
		defer cancel()
		sharedCtx, trace := commandtrace.WithNewTrace(sharedBase)
		complete := func(err error) staticImageFlightResult {
			return staticImageFlightResult{
				err:        err,
				operations: trace.Snapshot().Operations,
				leader:     callerToken,
			}
		}

		finishStat := commandtrace.MeasureOperation(sharedCtx, "image.static_cache_stat")
		if _, statErr := os.Stat(targetPath); statErr == nil {
			finishStat()
			return complete(nil), nil
		} else if !errors.Is(statErr, os.ErrNotExist) {
			finishStat()
			return complete(statErr), nil
		}
		finishStat()

		finishMkdir := commandtrace.MeasureOperation(sharedCtx, "image.static_cache_mkdir")
		if mkdirErr := os.MkdirAll(filepath.Dir(targetPath), 0o755); mkdirErr != nil {
			finishMkdir()
			return complete(mkdirErr), nil
		}
		finishMkdir()
		return complete(write(sharedCtx, targetPath, ownedData)), nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		resolved, ok := result.Val.(staticImageFlightResult)
		if !ok {
			return nil, fmt.Errorf("static image cache returned unexpected shared result %T", result.Val)
		}
		commandtrace.MergeOperations(ctx, resolved.operations)
		if resolved.leader != callerToken {
			commandtrace.RecordOperation(ctx, "image.static_cache_shared", 0)
		}
		if resolved.err != nil {
			return nil, resolved.err
		}
	}
	return onebot11.Message{onebot11.Image(url, "")}, nil
}

func staticImageSharedContext() (context.Context, context.CancelFunc) {
	shared := logger.WithContextAttrs(context.Background(), slog.Bool("shared_work", true))
	return context.WithTimeout(shared, staticImageSharedTimeout)
}

func writeStaticImageAtomically(ctx context.Context, targetPath string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	finishWrite := commandtrace.MeasureOperation(ctx, "image.static_cache_write")
	tmp, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		finishWrite()
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		finishWrite()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		finishWrite()
		return err
	}
	if err := tmp.Close(); err != nil {
		finishWrite()
		return err
	}
	finishWrite()
	if err := ctx.Err(); err != nil {
		return err
	}
	finishRename := commandtrace.MeasureOperation(ctx, "image.static_cache_rename")
	err = os.Rename(tmpName, targetPath)
	finishRename()
	return err
}

func chartStaticBaseURL(app *renderapp.App) string {
	if app == nil {
		return ""
	}
	baseURL := strings.TrimRight(strings.TrimSpace(app.Config.ChartsBaseURL), "/")
	if baseURL != "" {
		return baseURL
	}
	imageCacheBaseURL := strings.TrimRight(strings.TrimSpace(app.Config.ImageCacheURI), "/")
	if imageCacheBaseURL == "" {
		return ""
	}
	return imageCacheBaseURL + "/charts"
}

func resolveStaticImageLocation(app *renderapp.App, baseURL string, publicRelativePath string, storageRelativePath string) (string, string, error) {
	if app == nil {
		return "", "", fmt.Errorf("image storage is not configured")
	}

	rootDir := strings.TrimSpace(app.Config.ImageCacheDir)
	if baseURL == "" || rootDir == "" {
		return "", "", fmt.Errorf("static image cache is not configured")
	}

	cleanPublicRelative, err := normalizeStaticImageRelativePath(publicRelativePath)
	if err != nil {
		return "", "", err
	}
	cleanStorageRelative, err := normalizeStaticImageRelativePath(storageRelativePath)
	if err != nil {
		return "", "", err
	}
	return baseURL + "/" + cleanPublicRelative, filepath.Join(rootDir, filepath.FromSlash(cleanStorageRelative)), nil
}

func normalizeStaticImageRelativePath(relativePath string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(relativePath))
	if clean == "." || clean == "" {
		return "", fmt.Errorf("relative path is empty")
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("relative path must stay inside cache root")
	}
	return filepath.ToSlash(clean), nil
}
