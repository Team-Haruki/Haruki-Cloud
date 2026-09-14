package drawing

import (
	"context"
	"sync/atomic"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/utils/logger"

	"github.com/go-resty/resty/v2"
)

type ClientOption func(*resty.Client, *HarukiDrawingClient)

type HarukiDrawingClient struct {
	client     *resty.Client
	baseURL    string
	pool       *upstream.Pool
	cache      *RenderCacheClient
	localCache *localRenderCache
	limiter    *drawingLimiter
	logger     *logger.Logger
	requestCtx context.Context
	// artifact is nil unless drawing_artifact.endpoints is non-empty.
	artifact *artifactSettings
	// directiveRejected counts drawing_directive_rejected; shared by clones.
	directiveRejected *atomic.Int64
}
