package handler

import (
	"context"
	"fmt"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/stamp"
	"strconv"
	"strings"

	"haruki-cloud/internal/onebot11"
)

func (sekaiHandlers) StampHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "stamp",
		Commands: []string{
			"/贴纸", "/查贴纸", "/pjsk贴纸", "/pjsk表情", "/pjsk stamp", "/pjsk bq", "/stamp",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if page, remaining, ok := parseStampPageWithRemaining(args); ok {
				ctx.SetArgs(remaining)
				return makeCommandRequestWithParams(ctx, parser.ModuleStamp, stampListCommand, map[string]any{
					"page": page,
				}), nil
			}
			if parseStampAll(args) {
				ctx.SetArgs("")
				return makeCommandRequestWithParams(ctx, parser.ModuleStamp, stampListCommand, map[string]any{
					"all": true,
				}), nil
			}
			if params := parseStampIDs(args); len(params) > 0 {
				ctx.SetArgs("")
				return makeCommandRequestWithParams(ctx, parser.ModuleStamp, stampListCommand, map[string]any{
					"ids": params,
				}), nil
			}
			return makeCommandRequest(ctx, parser.ModuleStamp, stampListCommand), nil
		},
	}, executeStamp)
}

func parseStampIDs(args string) []int {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return nil
	}
	ids := make([]int, 0, len(fields))
	for _, field := range fields {
		id, err := strconv.Atoi(field)
		if err != nil || id <= 0 {
			return nil
		}
		ids = append(ids, id)
	}
	return ids
}

func parseStampPageWithRemaining(args string) (int, string, bool) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) < 2 {
		return 0, "", false
	}

	for i := 0; i < len(fields)-1; i++ {
		switch strings.ToLower(fields[i]) {
		case "page", "p", "页":
		default:
			continue
		}

		page, err := strconv.Atoi(fields[i+1])
		if err != nil || page <= 0 {
			return 0, "", false
		}

		remainingFields := make([]string, 0, len(fields)-2)
		remainingFields = append(remainingFields, fields[:i]...)
		remainingFields = append(remainingFields, fields[i+2:]...)
		return page, strings.Join(remainingFields, " "), true
	}

	return 0, "", false
}

func parseStampAll(args string) bool {
	value := strings.TrimSpace(strings.ToLower(args))
	switch value {
	case "all", "全部", "所有":
		return true
	default:
		return false
	}
}

func executeStamp(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App.Stamps == nil {
		return nil, fmt.Errorf("stamp service unavailable: sekai client not configured")
	}
	stampCtrl := rc.App.Stamps.WithContext(rc.Ctx)
	region := renderregion.Value(rc.Cmd.Region)
	switch rc.Cmd.Mode {
	case stampListCommand:
		return executeStampList(rc, stampCtrl, region)
	default:
		return nil, unsupportedModeError("stamp", rc.Cmd.Mode)
	}
}

func executeStampList(rc *RequestContext, stampCtrl *stamp.Controller, region renderregion.Value) (onebot11.Message, error) {
	q := stamp.ListQuery{Region: region}
	mergeParams(rc.Cmd.Params, &q)
	resolveStampCharacterSelection(rc.Ctx, rc.App, &q, rc.Cmd.Query)
	if message, ok, err := resolveDirectStampImage(rc.Ctx, stampCtrl, rc.App, q); ok {
		return message, err
	}
	if q.All {
		return renderAllStampPages(rc, stampCtrl, q)
	}
	data, err := stampCtrl.RenderStampList(q)
	if err != nil {
		return nil, err
	}
	return imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
}

func renderAllStampPages(rc *RequestContext, stampCtrl *stamp.Controller, q stamp.ListQuery) (onebot11.Message, error) {
	images, err := stampCtrl.RenderStampListPages(q)
	if err != nil {
		return nil, err
	}
	message := make(onebot11.Message, 0, len(images))
	for _, img := range images {
		segment, imageErr := imageMessage(rc.Ctx, img, rc.App, BotModulePJSK)
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

func resolveDirectStampImage(ctx context.Context, stampCtrl *stamp.Controller, app *renderapp.App, query stamp.ListQuery) (onebot11.Message, bool, error) {
	if app == nil || stampCtrl == nil {
		return nil, false, nil
	}
	if strings.TrimSpace(app.Config.AssetsBaseURL) == "" {
		return nil, false, nil
	}
	if query.All || len(query.IDs) != 1 {
		return nil, false, nil
	}
	req, err := stampCtrl.BuildStampListRequest(query)
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
	message, imageErr := assetImageMessage(ctx, imagePath, app, BotModulePJSK)
	if imageErr != nil {
		return nil, false, nil
	}
	return message, true, nil
}

func resolveStampCharacterSelection(ctx context.Context, app *renderapp.App, query *stamp.ListQuery, rawQuery string) {
	if query == nil || len(query.CharacterIDs) > 0 {
		return
	}
	if strings.TrimSpace(rawQuery) == "" {
		return
	}
	characterID, err := resolveGameCharacterIDByQuery(ctx, app, query.Region, rawQuery, "stamp")
	if err != nil || characterID <= 0 {
		return
	}
	query.CharacterIDs = []int{characterID}
}
