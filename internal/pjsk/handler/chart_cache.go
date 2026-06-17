package handler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/chartstyle"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
)

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
	if cached, hit, err := cachedStaticImageMessage(rc.App, chartStaticBaseURL(rc.App), publicRelativePath, storageRelativePath); err != nil {
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

func cachedStaticImageMessage(app *renderapp.App, baseURL string, publicRelativePath string, storageRelativePath string) (onebot11.Message, bool, error) {
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
	url, targetPath, err := resolveStaticImageLocation(app, baseURL, publicRelativePath, storageRelativePath)
	if err != nil {
		if app == nil || app.ImageCache == nil {
			return nil, err
		}
		return imageMessage(ctx, data, app, BotModulePJSK)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return nil, err
	}
	if writeErr := os.WriteFile(targetPath, data, 0o644); writeErr != nil {
		return nil, writeErr
	}
	return onebot11.Message{onebot11.Image(url, "")}, nil
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
