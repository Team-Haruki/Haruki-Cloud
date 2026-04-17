package drawing

import (
	"context"

	"haruki-cloud/utils/logger"

	"github.com/go-resty/resty/v2"
)

type ClientOption func(*resty.Client)

type HarukiDrawingClient struct {
	client     *resty.Client
	baseURL    string
	cache      *RenderCacheClient
	localCache *localRenderCache
	logger     *logger.Logger
	requestCtx context.Context
}
