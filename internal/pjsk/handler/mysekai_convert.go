package handler

import (
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
)

const mySekaiConvertRecipeCommand = "mysekai-convert-recipes"

func (sekaiHandlers) MysekaiConvertRecipesHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "mysekai/convert-recipes",
		Commands: []string{
			"/烤森合成表", "/烤森转换器", "/mysekai合成表", "/mysekai convert",
		},
		ParseUIDArg: new(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			query := rendermysekai.ConvertRecipeQuery{Query: strings.Join(strings.Fields(ctx.GetArgs()), " ")}
			return makeCommandRequestWithParams(ctx, parser.ModuleMysekai, mySekaiConvertRecipeCommand, query), nil
		},
	}, executeMysekai)
}

func executeMysekaiConvertRecipes(rc *RequestContext, region string) (onebot11.Message, error) {
	query := rendermysekai.ConvertRecipeQuery{}
	mergeParams(rc.Cmd.Params, &query)
	query.Region = regionWithDefault(region)
	finishBuild := measurePayloadBuild(rc.Ctx)
	payload, err := rc.App.MySekai.WithContext(rc.Ctx).BuildConvertRecipeRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	if rc.App.Drawing == nil {
		return onebot11.Message{onebot11.Text(payload.Text)}, nil
	}
	image, err := rc.App.Drawing.WithContext(rc.Ctx).GenerateMysekaiConvertRecipesImage(payload)
	if err != nil {
		return onebot11.Message{onebot11.Text(payload.Text)}, nil
	}
	if rc.App.ImageCache != nil || (image.Ref() != nil && rc.App.ImageHosts.Len() > 0) {
		return mysekaiRenderedImageResult(rc, image, nil)
	}
	data, err := image.Bytes(rc.Ctx)
	if err != nil {
		return onebot11.Message{onebot11.Text(payload.Text)}, nil
	}
	return inlineImageMessage(rc.Ctx, data), nil
}
