package handler

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	"haruki-cloud/internal/pjsk/render/gacha"
	"haruki-cloud/utils/usererror"
)

var (
	reGachaCardFilter = regexp.MustCompile(`(?i)\bcard(\d+)\b`)
	reGachaPageP      = regexp.MustCompile(`(?i)\bp(\d+)\b`)
	reGachaPageCN     = regexp.MustCompile(`(\d+)页`)
)

func (sekaiHandlers) GachaHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "gacha",
		Commands: []string{
			"/pjsk gacha", "/卡池列表", "/卡池一览", "/卡池", "/查卡池",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			return resolveGachaDetailOrList(ctx)
		},
	}, executeGacha)
}

func resolveGachaDetailOrList(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
	args := strings.TrimSpace(ctx.GetArgs())
	if args == "" {
		return makeCommandRequestWithParams(ctx, parser.ModuleGacha, gachaListCommand, map[string]any{
			"include_past": true,
		}), nil
	}

	if params, ok, err := parseSingleGachaQuery(args); err != nil {
		return nil, gachaSearchUsageError(ctx.originalTriggerCmd)
	} else if ok {
		return makeCommandRequestWithParams(ctx, parser.ModuleGacha, "gacha-detail", params), nil
	}

	params, remaining := parseMultiGachaQuery(args)
	if remaining != "" {
		return nil, gachaSearchUsageError(ctx.originalTriggerCmd)
	}
	return makeCommandRequestWithParams(ctx, parser.ModuleGacha, gachaListCommand, params), nil
}

func gachaSearchUsageError(trigger string) error {
	return usererror.Unrecognized()
}

func parseSingleGachaQuery(args string) (map[string]any, bool, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil, false, nil
	}

	lower := strings.ToLower(args)
	if strings.HasPrefix(lower, "event") {
		eventText := strings.TrimSpace(args[len("event"):])
		eventID, err := strconv.Atoi(eventText)
		if err != nil || eventID <= 0 {
			return nil, false, usererror.Unrecognized()
		}
		return map[string]any{"event_id": eventID}, true, nil
	}

	value, err := strconv.Atoi(args)
	if err != nil {
		return nil, false, nil
	}
	switch {
	case value > 0:
		return map[string]any{"gacha_id": value}, true, nil
	case value < 0:
		return map[string]any{"neg_index": value}, true, nil
	default:
		return nil, false, usererror.Unrecognized()
	}
}

func parseMultiGachaQuery(args string) (map[string]any, string) {
	remaining := strings.TrimSpace(args)
	params := map[string]any{
		"include_past": true,
	}

	if strings.Contains(remaining, "复刻") {
		remaining = strings.TrimSpace(strings.ReplaceAll(remaining, "复刻", ""))
		params["is_rerelease"] = true
	}
	if strings.Contains(remaining, "回响") {
		remaining = strings.TrimSpace(strings.ReplaceAll(remaining, "回响", ""))
		params["is_recall"] = true
	}
	if strings.Contains(remaining, "当前") {
		remaining = strings.TrimSpace(strings.ReplaceAll(remaining, "当前", ""))
		params["only_current"] = true
	}

	if yearResult := rendercard.NewExtractor(nil).ExtractYear(remaining); yearResult.Found {
		params["year"] = yearResult.Value
		remaining = yearResult.Remaining
	}

	if matches := reGachaCardFilter.FindStringSubmatch(remaining); len(matches) > 1 {
		cardID, _ := strconv.Atoi(matches[1])
		params["card_id"] = cardID
		remaining = strings.TrimSpace(reGachaCardFilter.ReplaceAllString(remaining, ""))
	}

	if matches := reGachaPageP.FindStringSubmatch(remaining); len(matches) > 1 {
		page, _ := strconv.Atoi(matches[1])
		params["page"] = page
		remaining = strings.TrimSpace(reGachaPageP.ReplaceAllString(remaining, ""))
	}
	if matches := reGachaPageCN.FindStringSubmatch(remaining); len(matches) > 1 {
		page, _ := strconv.Atoi(matches[1])
		params["page"] = page
		remaining = strings.TrimSpace(reGachaPageCN.ReplaceAllString(remaining, ""))
	}

	return params, strings.TrimSpace(remaining)
}

func executeGacha(rc *RequestContext) (message onebot11.Message, err error) {
	defer func() {
		err = normalizeGachaUserFacingError(err)
	}()

	if rc.App.Gachas == nil {
		return nil, usererror.Misconfigured(errors.New("gacha service unavailable: sekai client not configured"))
	}
	gachaCtrl := rc.App.Gachas.WithContext(rc.Ctx)
	var data drawing.ImageResult
	region := renderregion.Value(rc.Cmd.Region)
	switch rc.Cmd.Mode {
	case "gacha", gachaListCommand:
		q := gacha.ListQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = gachaCtrl.RenderGachaListImage(q)
	case "gacha-detail":
		q := gacha.DetailQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = gachaCtrl.RenderGachaDetailImage(q)
	default:
		return nil, unsupportedModeError("gacha", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return rc.RenderedImageMessage(data)
}
