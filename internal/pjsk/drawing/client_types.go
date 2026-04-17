package drawing

import (
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
}
