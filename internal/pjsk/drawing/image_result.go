package drawing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
)

// ImageResult keeps a cached file lazy until a caller actually needs its bytes.
// File paths are validated against the render cache root before being exposed.
type ImageResult struct {
	data     []byte
	filePath string
	cache    *RenderCacheClient
	fallback func(context.Context) ([]byte, error)
}

func ImageBytes(data []byte) ImageResult { return ImageResult{data: data} }

func (r ImageResult) FilePath() string { return r.filePath }

func (r ImageResult) Bytes(ctx context.Context) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.filePath == "" {
		return r.data, nil
	}
	data, err := r.cache.readSharedImage(ctx, r.filePath)
	if err != nil && r.fallback != nil {
		return r.fallback(ctx)
	}
	return data, err
}

func (c *RenderCacheClient) cachedFile(candidate string) (ImageResult, error) {
	resolved, err := resolveContainedCacheFile(c.storageDir, candidate)
	if err != nil {
		return ImageResult{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return ImageResult{}, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > drawingMaxResponseBytes {
		return ImageResult{}, fmt.Errorf("invalid render cache image file")
	}
	absolute, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return ImageResult{}, err
	}
	return ImageResult{filePath: absolute, cache: c}, nil
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
	return c.renderRemoteImageFlight(ctx, endpoint, key, policy, render, false)
}

func (c *HarukiDrawingClient) cachedPostImage(endpoint string, body any) (ImageResult, error) {
	if c == nil {
		return ImageResult{}, fmt.Errorf("drawing client is not configured")
	}
	ctx := c.requestCtx
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

func (c *RenderCacheClient) readSharedImage(ctx context.Context, path string) ([]byte, error) {
	finish := commandtrace.MeasureOperation(ctx, "drawing.cache_read_wait")
	defer finish()
	caller := new(renderFlightToken)
	result := c.readFlight.DoChan(path, func() (any, error) {
		completed := runSharedRenderFlight(ctx, func(sharedCtx context.Context) ([]byte, error) {
			finish := commandtrace.MeasureOperation(sharedCtx, "drawing.cache_read")
			defer finish()
			return c.readCacheFile(path)
		})
		completed.leader = caller
		return completed, nil
	})
	return waitForRenderFlight(ctx, result, caller, "file")
}
