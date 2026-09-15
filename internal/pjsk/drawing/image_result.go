package drawing

import (
	"context"
	"fmt"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
)

// ImageResult is a rendered image: its bytes, or an artifact ref whose bytes
// are read back only when a caller actually needs them.
type ImageResult struct {
	data    []byte
	ref     *ArtifactRef
	fetcher *artifactFetcher
}

func ImageBytes(data []byte) ImageResult { return ImageResult{data: data} }

// ImageArtifact wraps a ref without a byte reader: URL emission works, Bytes
// reports ErrArtifactBytesUnavailable.
func ImageArtifact(ref *ArtifactRef) ImageResult { return ImageResult{ref: ref} }

// Ref returns the artifact ref of a result Drawing stored, or nil.
func (r ImageResult) Ref() *ArtifactRef { return r.ref }

func (r ImageResult) Bytes(ctx context.Context) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.ref != nil && r.data == nil {
		return r.fetcher.fetch(ctx, r.ref)
	}
	return r.data, nil
}

func (c *RenderCacheClient) RenderImageSharedContext(ctx context.Context, endpoint string, request any, render func(context.Context) ([]byte, error)) (ImageResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil {
		data, err := render(ctx)
		return ImageBytes(data), err
	}
	policy, key, ok := resolveRenderCachePolicyKey(ctx, endpoint, request)
	if !ok {
		data, err := render(ctx)
		return ImageBytes(data), err
	}
	return c.renderRemoteImageFlight(ctx, endpoint, key, policy, render)
}

func (c *HarukiDrawingClient) cachedPostImage(endpoint string, body any) (ImageResult, error) {
	if c == nil {
		return ImageResult{}, fmt.Errorf("drawing client is not configured")
	}
	ctx := c.withArtifactMode(c.requestCtx, endpoint)
	if ctx == nil {
		ctx = context.Background()
	}
	finish := commandtrace.MeasureOperation(ctx, "drawing.prepare_cache")
	prepared := prepareDrawingRequestBody(endpoint, body, time.Now(), ctx)
	finish()
	render := func(renderCtx context.Context) ([]byte, error) {
		active := c.WithContext(renderCtx)
		return active.renderWithPermit(endpoint, prepared, func(request any) ([]byte, error) { return active.postPrepared(endpoint, request) })
	}
	request := preparedRenderCachePayload{payload: prepared}
	if c.cache != nil {
		return c.cache.RenderImageSharedContext(ctx, endpoint, request, render)
	}
	if c.localCache != nil {
		data, err := c.localCache.RenderSharedContext(ctx, endpoint, request, render)
		return ImageBytes(data), err
	}
	data, err := render(ctx)
	return ImageBytes(data), err
}
