package drawing

import (
	"context"
	"sync/atomic"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/httpcoding"
	"haruki-cloud/utils/logger"

	"github.com/go-resty/resty/v2"
)

type ClientOption func(*resty.Client, *HarukiDrawingClient)

type HarukiDrawingClient struct {
	versions   *cacheVersions
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
	// coding remembers which Drawing nodes take zstd request bodies; shared
	// by clones. nil sends identity.
	coding *httpcoding.Negotiator
	// health holds the nodes skipped after a connection failure; shared by
	// clones.
	health *nodeHealth
}
