package handler

import (
	"context"
	"haruki-cloud/internal/observability/commandtrace"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

// RenderedImageMessage emits a rendered image: an artifact ref as a public
// image-cache URL (preferring the rendering node), anything else as bytes.
func (rc *RequestContext) RenderedImageMessage(image drawing.ImageResult) (onebot11.Message, error) {
	return renderedImageMessage(rc.Ctx, image, rc.App)
}

func renderedImageMessage(ctx context.Context, image drawing.ImageResult, app *renderapp.App) (onebot11.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ref := image.Ref(); ref != nil && app != nil {
		finishURL := commandtrace.MeasureOperation(ctx, "image.result_url")
		url, ok := app.ImageHosts.URL(ref.NodeName, ref.EscapedCDNPath())
		if ok {
			message := onebot11.Message{onebot11.Image(url, "")}
			finishURL()
			return message, nil
		}
		finishURL()
	}
	data, err := image.Bytes(ctx)
	if err != nil {
		return nil, err
	}
	return imageMessage(ctx, data, app, BotModulePJSK)
}
