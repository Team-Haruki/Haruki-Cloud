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
	directiveCacheGroup    = "pjsk"
)

// drawingMaxCacheTTLSeconds is Drawing's X-Haruki-Cache-TTL cap (A9.3). The
// header is clamped; Cloud's own pending/legacy cache keeps the rule TTL.
const drawingMaxCacheTTLSeconds int64 = 30 * 86400

// renderDirective is the artifact directive of one render request. It travels
// on the context from the attach point down to postPrepared.
type renderDirective struct {
	Key        string // 64 hex; always set
	KeyVersion int    // 3, or 5 for api/pjsk/event/list
	APIPath    string // normalised api path
	UserID     string // "public" or a sanitised user id
	Group      string // "pjsk"
	TTLSeconds int64  // 0 = infinite; clamped only on the wire
	Store      bool   // X-Haruki-Cache-Store: 1|0
	Artifact   bool   // X-Haruki-Artifact: 1
	outcome    *renderOutcome
}

// renderOutcome is written by postPrepared only. Node carries X-Haruki-Node
// on the branches without a ref; on the ref branch it fills an empty NodeName.
type renderOutcome struct {
	Ref         *ArtifactRef
	Degraded    bool
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
	var requestCtx context.Context
	if c != nil {
		requestCtx = c.requestCtx
	}
	finishPrepare := commandtrace.MeasureOperation(requestCtx, "drawing.prepare_render")
	prepared := prepareDrawingRequestBody(endpoint, body, time.Now(), requestCtx)
	finishPrepare()
	// The same prepared body is keyed and sent, so the key describes the bytes on the wire.
	d, ok := c.newUncachedDirective(endpoint, prepared)
	if !ok {
		return c.postPrepared(endpoint, prepared)
	}
	data, err := c.WithContext(withDirective(requestCtx, d)).postPrepared(endpoint, prepared)
	if err != nil {
		return nil, err
	}
	if d.outcome.Ref != nil {
		return nil, fmt.Errorf("drawing returned an artifact ref for uncached endpoint %s", endpoint)
	}
	cacheLogger.DebugContext(requestCtx, "drawing uncached artifact-mode render",
		"upstream_path", endpoint,
		"node", d.outcome.Node,
		"degraded", d.outcome.Degraded,
	)
	return data, nil
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
			return newRenderDirective(key, policy, policy.TTL, false), true
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
