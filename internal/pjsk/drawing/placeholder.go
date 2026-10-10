package drawing

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"haruki-cloud/internal/observability/commandtrace"
)

// headerRenderMissingAssets is how many missing-asset placeholders ("?"
// images) Drawing drew into this render. Drawing sends it only when the count
// is positive, next to X-Haruki-Cache-Store: 0; an older Drawing never sends
// it, so such renders stay uncached as before.
const headerRenderMissingAssets = "X-Haruki-Render-Missing-Assets"

// defaultPlaceholderCacheTTL is pjsk_render.drawing_cache.placeholder_ttl
// when unset.
const defaultPlaceholderCacheTTL = time.Hour

// placeholderRenderCacheMaxEntries and placeholderRenderCacheMaxBytes bound
// the in-process cache of flagged renders in index mode.
const (
	placeholderRenderCacheMaxEntries = 256
	placeholderRenderCacheMaxBytes   = 64 << 20
)

// effectivePlaceholderTTL resolves the configured placeholder TTL: 0 is the
// default, a negative value disables caching flagged renders.
func effectivePlaceholderTTL(configured time.Duration) time.Duration {
	switch {
	case configured < 0:
		return 0
	case configured == 0:
		return defaultPlaceholderCacheTTL
	default:
		return configured
	}
}

// placeholderEntryTTL is the fixed lifetime of one flagged render: the
// placeholder TTL, capped by the endpoint's own TTL. An infinite endpoint gets
// the placeholder TTL, never an infinite entry. 0 means do not cache.
func placeholderEntryTTL(ruleTTL time.Duration, infinite bool, placeholderTTL time.Duration) time.Duration {
	if placeholderTTL <= 0 {
		return 0
	}
	if !infinite && ruleTTL > 0 && ruleTTL < placeholderTTL {
		return ruleTTL
	}
	return placeholderTTL
}

// WithPlaceholderCacheTTL sets pjsk_render.drawing_cache.placeholder_ttl for
// the in-process render cache used without a render index.
func WithPlaceholderCacheTTL(configured time.Duration) ClientOption {
	return func(_ *resty.Client, client *HarukiDrawingClient) {
		if client != nil && client.localCache != nil {
			client.localCache.placeholderTTL = effectivePlaceholderTTL(configured)
		}
	}
}

// parseRenderMissingAssets reads X-Haruki-Render-Missing-Assets; anything but
// a positive integer counts as no placeholder.
func parseRenderMissingAssets(value string) int64 {
	count, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || count < 0 {
		return 0
	}
	return count
}

func markRenderPlaceholders(ctx context.Context, count int64) {
	if ctx == nil || count <= 0 {
		return
	}
	state, ok := ctx.Value(responseCacheabilityKey{}).(*responseCacheability)
	if !ok {
		return
	}
	for {
		current := state.placeholders.Load()
		if count <= current || state.placeholders.CompareAndSwap(current, count) {
			return
		}
	}
}

// renderPlaceholders is the placeholder count Drawing reported for the render
// made under ctx (0 when none, or when the response was not observed).
func renderPlaceholders(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	state, _ := ctx.Value(responseCacheabilityKey{}).(*responseCacheability)
	if state == nil {
		return 0
	}
	return state.placeholders.Load()
}

// notePlaceholderRender records a render Drawing drew with placeholders, so
// the operator can see which routes show "?" images and how often.
func (c *HarukiDrawingClient) notePlaceholderRender(endpoint string, d *renderDirective, node string, count int64) {
	if count <= 0 {
		return
	}
	markRenderPlaceholders(c.requestCtx, count)
	commandtrace.RecordOperation(c.requestCtx, "drawing.placeholder_render", 0)
	apiPath := ""
	if d != nil {
		apiPath = d.APIPath
	}
	c.logger.InfoContext(c.requestCtx, "drawing render drew missing-asset placeholders",
		"upstream", "drawing",
		"upstream_path", endpoint,
		"api_path", apiPath,
		"node", node,
		"missing_assets", count,
		"metric", "drawing_placeholder_render",
	)
}
