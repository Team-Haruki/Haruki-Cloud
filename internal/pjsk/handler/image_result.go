package handler

import (
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
)

func (rc *RequestContext) RenderedImageMessage(image drawing.ImageResult) (onebot11.Message, error) {
	if err := rc.Ctx.Err(); err != nil {
		return nil, err
	}
	if image.FilePath() != "" {
		finish := commandtrace.MeasureOperation(rc.Ctx, "image.artifact_lookup")
		url, ok := rc.App.ImageCache.URLForFile(rc.Ctx, image.FilePath())
		finish()
		if ok {
			return onebot11.Message{onebot11.Image(url, "")}, nil
		}
	}
	data, err := image.Bytes(rc.Ctx)
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}
