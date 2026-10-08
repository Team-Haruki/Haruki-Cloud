package drawing

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/observability/commandtrace"

	"github.com/go-resty/resty/v2"
)

// C13 request/response headers of the artifact directive.
const (
	headerArtifact         = "X-Haruki-Artifact"
	headerCacheKey         = "X-Haruki-Cache-Key"
	headerCacheTTL         = "X-Haruki-Cache-TTL"
	headerCacheKeyVersion  = "X-Haruki-Cache-Key-Version"
	headerCacheStore       = "X-Haruki-Cache-Store"
	headerCacheGroup       = "X-Haruki-Cache-Group"
	headerAPIPath          = "X-Haruki-Api-Path"
	headerUserID           = "X-Haruki-User-Id"
	headerArtifactDegraded = "X-Haruki-Artifact-Degraded"
	headerNode             = "X-Haruki-Node"
	headerDirectiveError   = "X-Haruki-Directive-Error"
	// headerArtifactMode selects Drawing's store-ref mode. It is a separate
	// header because an older Drawing rejects any X-Haruki-Artifact value but
	// 0/1 with a 400, while it ignores this header and answers Cache-Store: 0
	// with bytes, which Cloud already handles.
	headerArtifactMode   = "X-Haruki-Artifact-Mode"
	artifactModeStoreRef = "store-ref"
	directiveCacheGroup  = "pjsk"
)

// drawingMaxCacheTTLSeconds is Drawing's X-Haruki-Cache-TTL cap (A9.3). The
// header is clamped; Cloud's own pending/legacy cache keeps the rule TTL.
const drawingMaxCacheTTLSeconds int64 = 30 * 86400

// renderDirective is the artifact directive of one render request. It travels
// on the context from the attach point down to postPrepared.
type renderDirective struct {
	Key        string // 64 hex; always set
	KeyVersion int    // legacy 3/5; versioned resource/renderer keys use 6
	APIPath    string // normalised api path
	UserID     string // "public" or a sanitised user id
	Group      string // "pjsk"
	TTLSeconds int64  // 0 = infinite; clamped only on the wire
	Store      bool   // X-Haruki-Cache-Store: 1|0
	Artifact   bool   // X-Haruki-Artifact: 1
	// StoreRef asks for a store-ref (X-Haruki-Artifact-Mode: store-ref); only
	// sent with Store false, and only by callers that accept a ref result.
	StoreRef bool
	outcome  *renderOutcome
}

// renderOutcome is written by postPrepared only. Node carries X-Haruki-Node
// on the branches without a ref; on the ref branch it fills an empty NodeName.
// StoreRef marks Ref as a store-ref whose row Cloud has recorded: it is never
// a render cache entry.
type renderOutcome struct {
	Ref         *ArtifactRef
	StoreRef    bool
	Degraded    bool
	NoStore     bool
	ContentType string
	Node        string
}

type directiveCtxKey struct{}

func withDirective(ctx context.Context, d *renderDirective) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, directiveCtxKey{}, d)
}

func directiveFrom(ctx context.Context) (*renderDirective, bool) {
	if ctx == nil {
		return nil, false
	}
	d, ok := ctx.Value(directiveCtxKey{}).(*renderDirective)
	return d, ok && d != nil
}

// renderCacheKeyVersionFor mirrors buildRenderCacheKey's version choice so the
// protected helper stays untouched.
func renderCacheKeyVersionFor(apiPath string) int {
	if strings.TrimSpace(apiPath) == "api/pjsk/event/list" {
		return renderCacheEventListKeyVersion
	}
	return renderCacheKeyVersion
}

// directiveTTLSeconds encodes a TTL like the legacy /cache form field: 0 is
// infinite, anything else is rounded up to at least one second.
func directiveTTLSeconds(ttl time.Duration, infinite bool) int64 {
	if infinite {
		return 0
	}
	seconds := int64(math.Ceil(ttl.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return seconds
}

func newRenderDirective(key string, policy renderCachePolicy, ttl time.Duration, store bool) *renderDirective {
	return &renderDirective{
		Key:        key,
		KeyVersion: renderCacheKeyVersionFor(policy.APIPath),
		APIPath:    policy.APIPath,
		UserID:     policy.UserID,
		Group:      directiveCacheGroup,
		TTLSeconds: directiveTTLSeconds(ttl, policy.Infinite),
		Store:      store,
		Artifact:   true,
		outcome:    &renderOutcome{},
	}
}

// wireTTLSeconds is the clamped X-Haruki-Cache-TTL value.
func (d *renderDirective) wireTTLSeconds() int64 {
	return min(d.TTLSeconds, drawingMaxCacheTTLSeconds)
}

func (d *renderDirective) apply(request *resty.Request) {
	store := "0"
	if d.Store {
		store = "1"
	}
	request.SetHeader(headerArtifact, "1").
		SetHeader(headerCacheKey, d.Key).
		SetHeader(headerCacheTTL, strconv.FormatInt(d.wireTTLSeconds(), 10)).
		SetHeader(headerCacheKeyVersion, strconv.Itoa(d.KeyVersion)).
		SetHeader(headerCacheStore, store).
		SetHeader(headerCacheGroup, d.Group).
		SetHeader(headerAPIPath, d.APIPath).
		SetHeader(headerUserID, d.UserID)
	if d.StoreRef && !d.Store {
		request.SetHeader(headerArtifactMode, artifactModeStoreRef)
	}
}

// activeDirective returns the context directive when artifact mode allows its
// api path. postPrepared is the single authority on writing headers.
func (c *HarukiDrawingClient) activeDirective() *renderDirective {
	d, ok := directiveFrom(c.requestCtx)
	if !ok || !d.Artifact || d.Key == "" || d.outcome == nil || c.artifact == nil || !c.artifact.allow.has(d.APIPath) {
		return nil
	}
	return d
}

// postUncached is attach point B: the deliberately uncached endpoints send
// the full directive with X-Haruki-Cache-Store: 0 when allow-listed.
func (c *HarukiDrawingClient) postUncached(endpoint string, body any) ([]byte, error) {
	image, err := c.postUncachedResult(endpoint, body, false)
	if err != nil {
		return nil, err
	}
	return image.data, nil
}

// postUncachedImage is postUncached for image callers: on a store-ref path it
// asks Drawing for a store-ref and returns the ref instead of bytes.
func (c *HarukiDrawingClient) postUncachedImage(endpoint string, body any) (ImageResult, error) {
	return c.postUncachedResult(endpoint, body, true)
}

func (c *HarukiDrawingClient) postUncachedResult(endpoint string, body any, acceptRef bool) (ImageResult, error) {
	var requestCtx context.Context
	if c != nil {
		requestCtx = c.requestCtx
	}
	finishPrepare := commandtrace.MeasureOperation(requestCtx, "drawing.prepare_render")
	prepared := prepareDrawingRequestBody(endpoint, body, time.Now(), requestCtx)
	if c != nil {
		requestCtx = attachVersions(requestCtx, c.versions.snapshot(prepared))
	}
	finishPrepare()
	// The same prepared body is keyed and sent, so the key describes the bytes on the wire.
	d, ok := c.WithContext(requestCtx).newUncachedDirective(endpoint, prepared)
	if !ok {
		data, err := c.WithContext(requestCtx).postPrepared(endpoint, prepared)
		return ImageBytes(data), err
	}
	d.StoreRef = acceptRef && c.artifact.storeRefFor(d.APIPath)
	data, err := c.WithContext(withDirective(requestCtx, d)).postPrepared(endpoint, prepared)
	if err != nil {
		return ImageResult{}, err
	}
	cacheLogger.DebugContext(requestCtx, "drawing uncached artifact-mode render",
		"upstream_path", endpoint,
		"node", d.outcome.Node,
		"degraded", d.outcome.Degraded,
		"store_ref", d.outcome.StoreRef,
	)
	if d.outcome.Ref != nil {
		if d.outcome.StoreRef {
			return ImageResult{ref: d.outcome.Ref, fetcher: c.artifact.fetcher}, nil
		}
		return ImageResult{}, fmt.Errorf("drawing returned an artifact ref for uncached endpoint %s", endpoint)
	}
	return ImageBytes(data), nil
}

// newUncachedDirective keys an uncached endpoint. Any failure drops the
// directive entirely: Cloud never sends X-Haruki-Artifact without a key.
func (c *HarukiDrawingClient) newUncachedDirective(endpoint string, prepared any) (*renderDirective, bool) {
	if c == nil || !c.artifact.allowsEndpoint(endpoint) {
		return nil, false
	}
	policy, err := buildDirectivePolicy(endpoint, prepared)
	if err == nil {
		var key string
		if key, err = buildRenderCacheKey(policy); err == nil {
			return newRenderDirective(versionedCacheKey(c.requestCtx, key), policy, policy.TTL, false), true
		}
	}
	cacheLogger.DebugContext(c.requestCtx, "drawing directive dropped: request is not keyable",
		"upstream_path", endpoint, "error", err)
	return nil, false
}

// directiveRenderCacheRule is resolveRenderCacheRule without the
// renderCacheDisabledEndpoints check; for an enabled endpoint it is identical.
func directiveRenderCacheRule(endpointPath string) renderCacheRule {
	rule := cloneRenderCacheRule(defaultRenderCacheRule)
	if specific, ok := renderCacheRules[strings.TrimSpace(endpointPath)]; ok {
		rule = mergeRenderCacheRule(rule, specific)
	}
	rule.Enabled = true
	return rule
}

// buildDirectivePolicy is buildRenderCachePolicy's prepared-payload branch
// with directiveRenderCacheRule in place of the disabled-endpoint bail. It
// never mutates prepared.
func buildDirectivePolicy(endpoint string, prepared any) (renderCachePolicy, error) {
	parsed, err := parseRenderCacheEndpoint(endpoint)
	if err != nil {
		return renderCachePolicy{}, err
	}
	payload := prepared
	switch payload.(type) {
	case map[string]any, []any:
	default:
		if payload, err = normalizeRenderCachePayload(prepared); err != nil {
			return renderCachePolicy{}, err
		}
	}
	rule := adjustRenderCacheRuleForPayload(parsed.Path, payload, directiveRenderCacheRule(parsed.Path))
	var path [16]string
	params := cloneSanitizedRenderCacheNode(payload, path[:0], rule)
	userID := normalizeRenderCacheUserID(extractRenderCacheUserID(params))
	apiPath := buildRenderCacheAPIPath(parsed, userID, params)
	if apiPath == "" {
		return renderCachePolicy{}, fmt.Errorf("cache api path is empty")
	}
	return renderCachePolicy{
		Endpoint: parsed.Normalized,
		APIPath:  apiPath,
		UserID:   userID,
		Params:   params,
		TTL:      rule.TTL,
		Infinite: rule.Infinite,
	}, nil
}
