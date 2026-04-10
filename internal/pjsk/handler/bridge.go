package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/logger"
)

// Execute routes a ResolvedCommand to the corresponding execution controller
// and returns a OneBot message or an error.
func Execute(ctx context.Context, resolved *parser.ResolvedCommand, app *renderapp.App) (message onebot11.Message, err error) {
	if resolved == nil {
		return nil, fmt.Errorf("bridge: nil resolved command")
	}
	if app == nil {
		return nil, fmt.Errorf("bridge: nil render app")
	}

	// Check requester ban before dispatching.
	if platform := strings.TrimSpace(resolved.RequesterPlatform); platform != "" {
		if userID := strings.TrimSpace(resolved.RequesterUserID); userID != "" {
			if err := app.BanChecker.CheckBan(ctx, platform, userID, resolved.Module); err != nil {
				message = append(message, onebot11.Text(err.Error()))
				return message, nil
			}
		}
	}

	// When no region was explicitly specified (no prefix like /jp, no -r flag),
	// resolve it from the user's global default binding so e.g. a TW player
	// doesn't always get JP results when typing bare commands.
	resolved.Region = resolveRegionFromDefaultBinding(ctx, resolved, app)

	// Create request context for functions that support it.
	rc := NewRequestContext(ctx, resolved, app)

	switch resolved.Module {
	case parser.ModuleCard:
		message, err = executeCard(rc)
	case parser.ModuleEvent:
		message, err = executeEvent(rc)
	case parser.ModuleMusic:
		message, err = executeMusic(rc)
	case parser.ModuleAlias:
		message, err = executeAlias(rc)
	case parser.ModuleGacha:
		message, err = executeGacha(rc)
	case parser.ModuleDeck:
		message, err = executeDeck(rc)
	case parser.ModuleEducation:
		message, err = executeEducation(rc)
	case parser.ModuleSK:
		message, err = executeSK(rc)
	case parser.ModuleScore:
		message, err = executeScore(rc)
	case parser.ModuleProfile:
		message, err = executeProfile(rc)
	case parser.ModuleArrest:
		message, err = executeArrest(rc)
	case parser.ModuleRegTime:
		message, err = executeRegTime(rc)
	case parser.ModuleCheckData:
		message, err = executeCheckData(rc)
	case parser.ModuleMysekai:
		message, err = executeMysekai(rc)
	case parser.ModuleStamp:
		message, err = executeStamp(rc)
	case parser.ModuleMisc:
		message, err = executeMisc(rc)
	case parser.ModuleVLive:
		message, err = executeVLive(rc)

	default:
		return nil, fmt.Errorf("bridge: unsupported module %v", resolved.Module)
	}
	if err != nil {
		return nil, err
	}
	return message, nil
}

func imageMessage(ctx context.Context, img []byte, app *renderapp.App, group string) (onebot11.Message, error) {
	url, err := app.ImageCache.StoreAndGetURL(ctx, img, group)
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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return imageMessage(ctx, data, app, group)
}

// mergeParams unmarshals the JSON params from ResolvedCommand into the target struct,
// allowing handler-set fields to override defaults. Fields not present in params
// remain at their zero/pre-set values.
func mergeParams(params json.RawMessage, target any) {
	if len(params) == 0 {
		return
	}
	if err := json.Unmarshal(params, target); err != nil {
		logger.Warnf("bridge: failed to parse command params into %T: %v (raw_len=%d)", target, err, len(params))
	}
}
