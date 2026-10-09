package handler

import (
	"errors"
	"strconv"
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	rendercostume "haruki-cloud/internal/pjsk/render/costume"
	"haruki-cloud/utils/usererror"
)

func (sekaiHandlers) CostumeDetailHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path:     "costume/detail",
		Commands: []string{"/查服装"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			fields := strings.Fields(ctx.GetArgs())
			if len(fields) < 1 || len(fields) > 2 {
				return nil, usererror.Unrecognized()
			}
			cardID, err := strconv.Atoi(fields[0])
			if err != nil || cardID <= 0 {
				return nil, usererror.Unrecognized()
			}
			color := 0
			if len(fields) == 2 {
				color, err = strconv.Atoi(fields[1])
				if err != nil || color <= 0 {
					return nil, usererror.Unrecognized()
				}
			}
			return makeCostumeDetailCommandRequest(ctx, rendercostume.Query{CardID: cardID, ColorPosition: color, ExpectedPartType: "body"}), nil
		},
	}, executeCostume)
}

func makeCostumeDetailCommandRequest(ctx HarrukiSekaiHandlerContext, query rendercostume.Query) *CommandRequest {
	query.Region = ctx.Region().String()
	request := makeCommandRequestWithParams(ctx, parser.ModuleCostume, "costume-detail", query)
	request.Query = query.Query
	return request
}

func executeCostume(rc *RequestContext) (onebot11.Message, error) {
	if rc.App.Costumes == nil {
		return nil, usererror.Misconfigured(errors.New("costume service unavailable: sekai client not configured"))
	}
	if rc.Cmd.Mode != "costume-detail" {
		return nil, unsupportedModeError("costume", rc.Cmd.Mode)
	}
	q := rendercostume.Query{}
	mergeParams(rc.Cmd.Params, &q)
	if q.CardID <= 0 || q.ColorPosition < 0 {
		return nil, usererror.Unrecognized()
	}
	q = rendercostume.Query{CardID: q.CardID, ColorPosition: q.ColorPosition, Region: rc.Cmd.Region, ExpectedPartType: "body"}
	data, err := rc.App.Costumes.WithContext(rc.Ctx).RenderCostumeDetailImage(q)
	if err != nil {
		return nil, normalizeCostume3DError(err)
	}
	return rc.RenderedImageMessage(data)
}
