package handler

import (
	"context"
	"encoding/base64"

	"haruki-cloud/internal/observability/commandtrace"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

// RenderedImageMessage emits a rendered image: an artifact ref as a public
// image-cache URL (preferring the rendering node), an inline result (a
// drawing_artifact.no_store_paths render) as base64 bytes, anything else as
// bytes stored in the image cache.
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
	if image.Inline() {
		return inlineImageMessage(ctx, data), nil
	}
	return imageMessage(ctx, data, app, BotModulePJSK)
}

// inlineImageMessage sends the bytes themselves (base64://) instead of an
// image-cache URL: no upload, more bytes on the bot link.
func inlineImageMessage(ctx context.Context, data []byte) onebot11.Message {
	finish := commandtrace.MeasureOperation(ctx, "image.inline")
	defer finish()
	return onebot11.Message{onebot11.Image("base64://"+base64.StdEncoding.EncodeToString(data), "")}
}
