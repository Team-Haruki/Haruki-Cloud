package drawing

import (
	"context"
	"fmt"

	"haruki-cloud/internal/observability/commandtrace"
)

// ImageResult is a rendered image: its bytes, or an artifact ref whose bytes
// are read back only when a caller actually needs them.
type ImageResult struct {
	data    []byte
	ref     *ArtifactRef
	fetcher *artifactFetcher
	inline  bool
}

func ImageBytes(data []byte) ImageResult { return ImageResult{data: data} }

// ImageInlineBytes wraps bytes that should reach the bot as-is (see Inline).
func ImageInlineBytes(data []byte) ImageResult { return ImageResult{data: data, inline: true} }

// ImageArtifact wraps a ref without a byte reader: URL emission works, Bytes
// reports ErrArtifactBytesUnavailable.
func ImageArtifact(ref *ArtifactRef) ImageResult { return ImageResult{ref: ref} }

// Ref returns the artifact ref of a result Drawing stored, or nil.
func (r ImageResult) Ref() *ArtifactRef { return r.ref }

// Inline reports bytes rendered for a drawing_artifact.no_store_paths path:
// nothing was stored, so they are meant to reach the bot as-is instead of
// being uploaded to the image cache for a URL.
func (r ImageResult) Inline() bool { return r.inline && r.ref == nil }

func (r ImageResult) Bytes(ctx context.Context) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	finish := commandtrace.MeasureOperation(ctx, "image.result_bytes")
	defer finish()
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
	return c.renderImageWithCacheRequestAndPrepare(endpoint, body, body, nil, func(renderCtx context.Context, prepared any) ([]byte, error) {
		return c.WithContext(renderCtx).postPrepared(endpoint, prepared)
	}, true)
}
