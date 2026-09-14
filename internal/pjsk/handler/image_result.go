package handler

import (
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
)

// RenderedImageMessage emits a rendered image: an artifact ref as a public
// image-cache URL (preferring the rendering node), a legacy cached file as its
// image-cache URL, and anything else as bytes.
func (rc *RequestContext) RenderedImageMessage(image drawing.ImageResult) (onebot11.Message, error) {
	if err := rc.Ctx.Err(); err != nil {
		return nil, err
	}
	if ref := image.Ref(); ref != nil && rc.App != nil {
		if url, ok := rc.App.ImageHosts.URL(ref.NodeName, ref.EscapedCDNPath()); ok {
			return onebot11.Message{onebot11.Image(url, "")}, nil
		}
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
