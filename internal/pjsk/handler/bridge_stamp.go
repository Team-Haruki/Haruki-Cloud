package handler

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/stamp"
)

func executeStamp(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App.Stamps == nil {
		return nil, fmt.Errorf("stamp service unavailable: sekai client not configured")
	}
	region := renderregion.Value(rc.Cmd.Region)
	switch rc.Cmd.Mode {
	case "stamp-list":
		q := stamp.ListQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		_ = resolveStampCharacterSelection(rc.Ctx, rc.App, &q, rc.Cmd.Query)
		if message, ok, directErr := resolveDirectStampImage(rc.App, q); ok {
			return message, directErr
		}
		if q.All {
			images, renderErr := rc.App.Stamps.RenderStampListPages(q)
			if renderErr != nil {
				return nil, renderErr
			}
			message = make(onebot11.Message, 0, len(images))
			for _, img := range images {
				segment, imageErr := imageMessage(img, rc.App, BotModulePJSK)
				if imageErr != nil {
					return nil, imageErr
				}
				message = append(message, segment...)
			}
			if len(message) == 0 {
				return nil, fmt.Errorf("stamp all mode did not produce any images")
			}
			return message, nil
		}
		data, renderErr := rc.App.Stamps.RenderStampList(q)
		if renderErr != nil {
			return nil, renderErr
		}
		return imageMessage(data, rc.App, BotModulePJSK)
	default:
		return nil, unsupportedModeError("stamp", rc.Cmd.Mode)
	}
}

func resolveDirectStampImage(app *renderapp.App, query stamp.ListQuery) (onebot11.Message, bool, error) {
	if app == nil || app.Stamps == nil {
		return nil, false, nil
	}
	if strings.TrimSpace(app.Config.AssetsBaseURL) == "" {
		return nil, false, nil
	}
	if query.All || len(query.IDs) != 1 {
		return nil, false, nil
	}
	req, err := app.Stamps.BuildStampListRequest(query)
	if err != nil {
		return nil, true, err
	}
	if req == nil || len(req.Stamps) != 1 {
		return nil, false, nil
	}
	imagePath := strings.TrimSpace(req.Stamps[0].ImagePath)
	if imagePath == "" {
		return nil, true, fmt.Errorf("stamp %d image path is empty", query.IDs[0])
	}
	message, imageErr := assetImageMessage(imagePath, app, BotModulePJSK)
	if imageErr != nil {
		return nil, false, nil
	}
	return message, true, nil
}

func resolveStampCharacterSelection(ctx context.Context, app *renderapp.App, query *stamp.ListQuery, rawQuery string) error {
	if query == nil || len(query.CharacterIDs) > 0 {
		return nil
	}
	if strings.TrimSpace(rawQuery) == "" {
		return nil
	}
	characterID, err := resolveGameCharacterIDByQuery(ctx, app, query.Region, rawQuery, "stamp")
	if err != nil || characterID <= 0 {
		return nil
	}
	query.CharacterIDs = []int{characterID}
	return nil
}
