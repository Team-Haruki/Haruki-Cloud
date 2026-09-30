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
// image-cache URL (preferring the rendering node), anything else (including a
// drawing_artifact.no_store_paths render) as bytes stored in the image cache
// and sent as a URL. Never inline: a bot v2 response is one Noise message
// capped at 65535 bytes, far below a rendered image as base64.
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

// inlineImageMessage sends the bytes themselves (base64://). Only the help
// fallback without an image cache (dev setups) uses it: a bot v2 response is
// one Noise message capped at 65535 bytes.
func inlineImageMessage(ctx context.Context, data []byte) onebot11.Message {
	finish := commandtrace.MeasureOperation(ctx, "image.inline")
	defer finish()
	return onebot11.Message{onebot11.Image("base64://"+base64.StdEncoding.EncodeToString(data), "")}
}
