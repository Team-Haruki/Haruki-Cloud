// Package s3 is the S3-compatible storage.Store backend (Garage). It speaks
// the five S3 verbs Cloud needs over net/http with hand-rolled SigV4 signing,
// fails over across an ordered endpoint list, and adds no dependencies.
package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/storage"
)

// Defaults applied by New to zero Config fields.
const (
	DefaultRegion           = "garage"
	DefaultDialTimeout      = 3 * time.Second
	DefaultRequestTimeout   = 10 * time.Second
	DefaultStatTimeout      = 5 * time.Second
	DefaultFailoverCooldown = 30 * time.Second
	DefaultMaxListPages     = 10000
	EndpointRoundRobin      = "round_robin"
	EndpointOrderedFailover = "ordered_failover"

	// ProxyNone ignores proxy environment variables (Garage is on the tailnet).
	ProxyNone = "none"
	// ProxyEnvironment honours HTTP_PROXY / HTTPS_PROXY / NO_PROXY.
	ProxyEnvironment = "environment"

	serviceName     = "s3"
	defaultPageSize = 1000
)

// Config configures an S3 store.
type Config struct {
	// Endpoints are absolute URLs, e.g. "http://100.64.0.11:3900".
	Endpoints     []string
	EndpointNames []string
	// EndpointPolicy defaults to round_robin; ordered_failover prefers the first healthy endpoint.
	EndpointPolicy string
	// Runtime is shared by all storage slots in a Set. A standalone client creates its own.
	Runtime *storage.IORuntime
	Slot    storage.Slot
	// MaxListPages bounds a complete listing even without a caller deadline.
	MaxListPages int
	Bucket       string
	// Root is a key prefix inside the bucket ("" for the bucket root).
	Root string
	// Region is the SigV4 scope region; Garage ignores its value.
	Region string
	// AccessKey and SecretKey are both set, or both empty for anonymous
	// (unsigned) requests.
	AccessKey string
	SecretKey string
	// PathStyle selects <endpoint>/<bucket>/<key>; false selects
	// <bucket>.<host>/<key>. Callers default it to true.
	PathStyle bool
	// DefaultACL is sent as x-amz-acl on every Put when non-empty.
	DefaultACL string
	// DialTimeout bounds connection setup; RequestTimeout bounds one Get, Put
	// or List page attempt; StatTimeout bounds one Stat or Delete attempt.
	DialTimeout    time.Duration
	RequestTimeout time.Duration
	StatTimeout    time.Duration
	// FailoverCooldown is how long a failed endpoint is skipped.
	FailoverCooldown time.Duration
	// MaxAttempts is the attempt budget per operation (default: one per
	// endpoint).
	MaxAttempts int
	// MaxObjectBytes bounds Get and Put (default storage.DefaultMaxObjectBytes).
	MaxObjectBytes int64
	// Proxy is ProxyNone (default) or ProxyEnvironment.
	Proxy string
	// Transport replaces the built transport (tests).
	Transport http.RoundTripper
}

type endpointState struct {
	url             *url.URL
	consecutiveFail atomic.Int32
	lastFailureNano atomic.Int64
	retryAfterNano  atomic.Int64
	ordinal         int
	name            string
	origin          string
}

type client struct {
	cfg        Config
	rootPrefix string // "" or "<root>/"
	endpoints  []*endpointState
	rotation   atomic.Uint32
	httpClient *http.Client
	transport  *http.Transport // the built transport; nil when Config.Transport is set
	now        func() time.Time
	pageSize   int
}

// New validates cfg, applies defaults and returns the store.
func New(cfg Config) (storage.Store, error) {
	c, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func newClient(cfg Config) (*client, error) {
	endpoints, err := parseEndpoints(cfg.Endpoints)
	if err != nil {
		return nil, err
	}
	names, err := storage.NormalizeEndpointNames(cfg.EndpointNames, len(endpoints))
	if err != nil {
		return nil, err
	}
	for i, name := range names {
		endpoints[i].name = name
	}
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	if cfg.Bucket == "" || strings.ContainsAny(cfg.Bucket, "/\\") {
		return nil, fmt.Errorf("s3: invalid bucket %q", cfg.Bucket)
	}
	rootPrefix, err := parseRoot(cfg.Root)
	if err != nil {
		return nil, err
	}
	if (cfg.AccessKey == "") != (cfg.SecretKey == "") {
		return nil, errors.New("s3: access key and secret key must be set together")
	}
	applyDefaults(&cfg, len(endpoints))
	if cfg.EndpointPolicy != EndpointRoundRobin && cfg.EndpointPolicy != EndpointOrderedFailover {
		return nil, fmt.Errorf("s3: invalid endpoint policy %q", cfg.EndpointPolicy)
	}
	if cfg.Runtime == nil {
		cfg.Runtime = storage.NewIORuntime(storage.IOConfig{})
	}
	c := &client{
		cfg:        cfg,
		rootPrefix: rootPrefix,
		endpoints:  endpoints,
		now:        time.Now,
		pageSize:   defaultPageSize,
	}
	transport := cfg.Transport
	if transport == nil {
		if cfg.Proxy != "" && cfg.Proxy != ProxyNone && cfg.Proxy != ProxyEnvironment {
			return nil, fmt.Errorf("s3: invalid proxy mode %q (want %q or %q)", cfg.Proxy, ProxyNone, ProxyEnvironment)
		}
		proxy := cfg.Proxy
		if proxy == "" {
			proxy = ProxyNone
		}
		built := cfg.Runtime.SharedTransport(fmt.Sprintf("s3:%d:%s", cfg.DialTimeout, proxy), func() *http.Transport {
			transport := upstream.NewTunedTransport(upstream.TunedTransportConfig{DialTimeout: cfg.DialTimeout})
			if proxy == ProxyNone {
				transport.Proxy = nil
			}
			return transport
		})
		c.transport = built
		transport = built
	}
	c.httpClient = &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return c, nil
}

func parseEndpoints(raw []string) ([]*endpointState, error) {
	if len(raw) == 0 {
		return nil, errors.New("s3: no endpoints configured")
	}
	endpoints := make([]*endpointState, 0, len(raw))
	for index, value := range raw {
		parsed, err := url.Parse(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("s3: invalid endpoint %q: %w", value, err)
		}
		if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
			parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
			return nil, fmt.Errorf("s3: endpoint %q must be an absolute http(s) URL without query, fragment or userinfo", value)
		}
		parsed.Path = strings.TrimRight(parsed.Path, "/")
		parsed.RawPath = ""
		endpoints = append(endpoints, &endpointState{url: parsed, ordinal: index + 1, origin: endpointOrigin(parsed)})
	}
	return endpoints, nil
}

func endpointOrigin(endpoint *url.URL) string {
	host := strings.ToLower(endpoint.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := endpoint.Port()
	if port != "" && !(endpoint.Scheme == "http" && port == "80") && !(endpoint.Scheme == "https" && port == "443") {
		host += ":" + port
	}
	return endpoint.Scheme + "://" + host
}

func parseRoot(raw string) (string, error) {
	root := strings.Trim(strings.TrimSpace(raw), "/")
	if root == "" {
		return "", nil
	}
	cleaned, err := storage.CleanKey(root)
	if err != nil {
		return "", fmt.Errorf("s3: invalid root %q: %w", raw, err)
	}
	return string(cleaned) + "/", nil
}

func applyDefaults(cfg *Config, endpointCount int) {
	if cfg.Region = strings.TrimSpace(cfg.Region); cfg.Region == "" {
		cfg.Region = DefaultRegion
	}
	cfg.DialTimeout = durationOr(cfg.DialTimeout, DefaultDialTimeout)
	cfg.RequestTimeout = durationOr(cfg.RequestTimeout, DefaultRequestTimeout)
	cfg.StatTimeout = durationOr(cfg.StatTimeout, DefaultStatTimeout)
	cfg.FailoverCooldown = durationOr(cfg.FailoverCooldown, DefaultFailoverCooldown)
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = endpointCount
	}
	if cfg.MaxObjectBytes <= 0 {
		cfg.MaxObjectBytes = storage.DefaultMaxObjectBytes
	}
	cfg.Proxy = strings.TrimSpace(cfg.Proxy)
	if cfg.EndpointPolicy = strings.TrimSpace(cfg.EndpointPolicy); cfg.EndpointPolicy == "" {
		cfg.EndpointPolicy = EndpointRoundRobin
	}
	if cfg.MaxListPages <= 0 {
		cfg.MaxListPages = DefaultMaxListPages
	}
}

func durationOr(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

// resolve cleans key and returns it with its full object name in the bucket.
func (c *client) resolve(key storage.Key) (storage.Key, string, error) {
	cleaned, err := storage.CleanKey(string(key))
	if err != nil {
		return "", "", err
	}
	return cleaned, c.rootPrefix + string(cleaned), nil
}

// order returns the endpoints for one operation in rotation order, starting
// at the next rotation slot. Endpoints inside their failure cooldown are
// skipped to the end of the list, so they are only tried once every healthy
// endpoint has failed (and first-in-rotation when all are cooling down).
func (c *client) order() []*endpointState {
	n := uint32(len(c.endpoints))
	var start uint32
	if c.cfg.EndpointPolicy == EndpointRoundRobin {
		start = (c.rotation.Add(1) - 1) % n
	}
	now := c.now().UnixNano()
	healthy := make([]*endpointState, 0, n)
	var cooling []*endpointState
	for i := range n {
		ep := c.endpoints[(start+i)%n]
		if c.coolingDown(ep, now) {
			cooling = append(cooling, ep)
		} else {
			healthy = append(healthy, ep)
		}
	}
	return append(healthy, cooling...)
}

func (c *client) coolingDown(ep *endpointState, now int64) bool {
	if ep.consecutiveFail.Load() < 1 {
		return false
	}
	return time.Duration(now-ep.lastFailureNano.Load()) < c.cfg.FailoverCooldown
}

func (c *client) markFailure(ep *endpointState) {
	ep.lastFailureNano.Store(c.now().UnixNano())
	ep.consecutiveFail.Add(1)
}

// attemptFunc performs one attempt against ep. A nil error is success; retry
// reports whether the error is a node failure worth the next endpoint.
type attemptFunc func(ctx context.Context, ep *endpointState) (retry bool, err error)

func (c *client) run(ctx context.Context, op string, key storage.Key, timeout time.Duration, attempt attemptFunc) error {
	order := c.order()
	var lastErr error
	var lastEndpoint string
	for i := range c.cfg.MaxAttempts {
		if err := ctx.Err(); err != nil {
			return err
		}
		ep := order[i%len(order)]
		delay := time.Duration(0)
		if i > 0 {
			delay = retryDelay(i)
			if deadline, ok := ctx.Deadline(); ok {
				delay = min(delay, max(0, time.Until(deadline)/4))
			}
		}
		if notBefore := ep.retryAfterNano.Load(); notBefore > 0 {
			delay = max(delay, time.Until(time.Unix(0, notBefore)))
		}
		// Do not hammer an endpoint that requested a long pause or turn a
		// deadline-free call into an unbounded sleep. Other endpoints may
		// still be usable; a later operation can retry this one.
		if delay > 2*time.Second {
			if lastErr == nil {
				lastErr = errors.New("s3: endpoint Retry-After exceeds retry budget")
			}
			lastEndpoint = ep.url.String()
			c.record(ctx, ep, op, "retry_deferred", 0, 0)
			continue
		}
		if delay > 0 {
			started := time.Now()
			if err := waitRetry(ctx, delay); err != nil {
				return err
			}
			c.record(ctx, ep, op, "retry_wait", time.Since(started), 0)
		}
		if i > 0 {
			c.record(ctx, ep, op, "retry", 0, 0)
		}
		queued := time.Now()
		release, err := c.cfg.Runtime.Acquire(ctx, ep.origin)
		c.record(ctx, ep, op, "queue", time.Since(queued), 0)
		if err != nil {
			c.record(ctx, ep, op, "queue_canceled", 0, 0)
			return err
		}
		if err := ctx.Err(); err != nil {
			release()
			return err
		}
		budget := timeout
		if deadline, ok := ctx.Deadline(); ok {
			// Reserve a share of the remaining end-to-end budget for every
			// permitted attempt, so a slow first node cannot starve failover.
			budget = min(budget, time.Until(deadline)/time.Duration(c.cfg.MaxAttempts-i))
		}
		attemptCtx, cancel := context.WithTimeout(ctx, budget)
		attemptCtx = context.WithValue(attemptCtx, attemptMetricKey{}, attemptMetric{op: op, ep: ep})
		started := time.Now()
		retry, err := attempt(attemptCtx, ep)
		attemptErr := attemptCtx.Err()
		cancel()
		release()
		c.record(ctx, ep, op, "attempt", time.Since(started), 0)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("s3: %s %q: %w", op, key, ctxErr)
		}
		if err == nil || !retry {
			ep.consecutiveFail.Store(0)
			ep.retryAfterNano.Store(0)
			return err
		}
		if errors.Is(attemptErr, context.DeadlineExceeded) {
			c.record(ctx, ep, op, "attempt_timeout", 0, 0)
		}
		c.markFailure(ep)
		var responseErr *ResponseError
		if errors.As(err, &responseErr) && responseErr.RetryAfter > 0 {
			ep.retryAfterNano.Store(time.Now().Add(responseErr.RetryAfter).UnixNano())
		}
		lastErr, lastEndpoint = err, ep.url.String()
	}
	return fmt.Errorf("s3: %s %q failed after %d attempts, last endpoint %s: %w",
		op, key, c.cfg.MaxAttempts, lastEndpoint, lastErr)
}

func retryDelay(attempt int) time.Duration {
	ceiling := min(250*time.Millisecond, 25*time.Millisecond*time.Duration(1<<min(attempt-1, 4)))
	return ceiling/2 + time.Duration(rand.Int64N(int64(ceiling/2)+1))
}

func waitRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// requestURL builds the URL for objectName ("" addresses the bucket).
func (c *client) requestURL(ep *endpointState, objectName, rawQuery string) *url.URL {
	u := *ep.url
	if c.cfg.PathStyle {
		u.Path += "/" + c.cfg.Bucket
		if objectName != "" {
			u.Path += "/" + objectName
		}
	} else {
		u.Host = c.cfg.Bucket + "." + u.Host
		u.Path += "/" + objectName
	}
	u.RawPath = encodePath(u.Path)
	u.RawQuery = rawQuery
	return &u
}

// send performs one signed request. A transport error is always retryable.
func (c *client) send(ctx context.Context, ep *endpointState, method, objectName, rawQuery string, body []byte, header http.Header) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.requestURL(ep, objectName, rawQuery).String(), reader)
	if err != nil {
		return nil, err
	}
	for name, values := range header {
		req.Header[name] = values
	}
	if c.cfg.AccessKey != "" {
		Sign(req, PayloadSHA256(body), c.cfg.AccessKey, c.cfg.SecretKey, c.cfg.Region, serviceName, c.now())
	}
	return c.sendObserved(ctx, ep, req)
}

// statusError converts a non-success response; 429 and 5xx are retryable.
func statusError(resp *http.Response, op string, key storage.Key, ep *endpointState) (bool, error) {
	respErr := &ResponseError{Op: op, Key: key, Endpoint: ep.url.String(), StatusCode: resp.StatusCode, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())}
	if resp.Request == nil || resp.Request.Method != http.MethodHead {
		respErr.Code, respErr.Message, respErr.bodyErr = decodeError(resp.Body)
	}
	retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError
	return retry || (resp.StatusCode == http.StatusNotFound && respErr.bodyErr != nil), respErr
}

func drainClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, errorBodyLimit))
	_ = resp.Body.Close()
}

func (c *client) Get(ctx context.Context, key storage.Key) ([]byte, error) {
	started := time.Now()
	defer func() { c.record(ctx, nil, "get", "operation", time.Since(started), 0) }()
	cleaned, objectName, err := c.resolve(key)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var data []byte
	err = c.run(ctx, "get", cleaned, c.cfg.RequestTimeout, func(attemptCtx context.Context, ep *endpointState) (bool, error) {
		resp, err := c.send(attemptCtx, ep, http.MethodGet, objectName, "", nil, nil)
		if err != nil {
			return true, err
		}
		defer drainClose(resp)
		switch resp.StatusCode {
		case http.StatusOK:
			data, err = c.readObject(resp, cleaned)
			return err != nil && !errors.Is(err, storage.ErrTooLarge), err
		case http.StatusNotFound:
			retry, respErr := statusError(resp, "get", cleaned, ep)
			if response := respErr.(*ResponseError); response.Code == "NoSuchBucket" || response.bodyErr != nil {
				return retry, respErr
			}
			return false, storage.NotExistError("get", cleaned)
		default:
			return statusError(resp, "get", cleaned, ep)
		}
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (c *client) readObject(resp *http.Response, key storage.Key) ([]byte, error) {
	limit := c.cfg.MaxObjectBytes
	if resp.ContentLength > limit {
		return nil, storage.TooLargeError("get", key, resp.ContentLength, limit)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("s3: read %q: %w", key, err)
	}
	if int64(len(data)) > limit {
		return nil, storage.TooLargeError("get", key, int64(len(data)), limit)
	}
	return data, nil
}

func (c *client) Put(ctx context.Context, key storage.Key, data []byte, opts storage.PutOptions) error {
	started := time.Now()
	defer func() { c.record(ctx, nil, "put", "operation", time.Since(started), 0) }()
	cleaned, objectName, err := c.resolve(key)
	if err != nil {
		return err
	}
	if int64(len(data)) > c.cfg.MaxObjectBytes {
		return storage.TooLargeError("put", cleaned, int64(len(data)), c.cfg.MaxObjectBytes)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if data == nil {
		data = []byte{}
	}
	header := http.Header{}
	contentType := opts.ContentType
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	header.Set("Content-Type", contentType)
	if opts.CacheControl != "" {
		header.Set("Cache-Control", opts.CacheControl)
	}
	if c.cfg.DefaultACL != "" {
		header.Set("X-Amz-Acl", c.cfg.DefaultACL)
	}
	var writerNode string
	err = c.run(ctx, "put", cleaned, c.cfg.RequestTimeout, func(attemptCtx context.Context, ep *endpointState) (bool, error) {
		resp, err := c.send(attemptCtx, ep, http.MethodPut, objectName, "", data, header)
		if err != nil {
			return true, err
		}
		defer drainClose(resp)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			writerNode = ep.name
			return false, nil
		}
		return statusError(resp, "put", cleaned, ep)
	})
	if err == nil {
		storage.ReportWriteReceipt(ctx, storage.WriteReceipt{WriterNode: writerNode, WrittenAt: c.now().UTC()})
	}
	return err
}

func (c *client) Stat(ctx context.Context, key storage.Key) (storage.Object, error) {
	started := time.Now()
	defer func() { c.record(ctx, nil, "stat", "operation", time.Since(started), 0) }()
	cleaned, objectName, err := c.resolve(key)
	if err != nil {
		return storage.Object{}, err
	}
	if err := ctx.Err(); err != nil {
		return storage.Object{}, err
	}
	var object storage.Object
	err = c.run(ctx, "stat", cleaned, c.cfg.StatTimeout, func(attemptCtx context.Context, ep *endpointState) (bool, error) {
		resp, err := c.send(attemptCtx, ep, http.MethodHead, objectName, "", nil, nil)
		if err != nil {
			return true, err
		}
		defer drainClose(resp)
		switch resp.StatusCode {
		case http.StatusOK:
			object, err = statObject(resp, cleaned)
			return false, err
		case http.StatusNotFound:
			return false, storage.NotExistError("stat", cleaned)
		default:
			return statusError(resp, "stat", cleaned, ep)
		}
	})
	if err != nil {
		return storage.Object{}, err
	}
	return object, nil
}

// statObject reads HEAD metadata. A malformed Last-Modified leaves ModTime
// zero rather than failing the existence check.
func statObject(resp *http.Response, key storage.Key) (storage.Object, error) {
	size, err := strconv.ParseInt(strings.TrimSpace(resp.Header.Get("Content-Length")), 10, 64)
	if err != nil || size < 0 {
		return storage.Object{}, fmt.Errorf("s3: stat %q: malformed Content-Length %q", key, resp.Header.Get("Content-Length"))
	}
	object := storage.Object{Key: key, Size: size, ETag: trimETag(resp.Header.Get("ETag"))}
	if modTime, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		object.ModTime = modTime
	}
	return object, nil
}

func (c *client) Delete(ctx context.Context, key storage.Key) error {
	started := time.Now()
	defer func() { c.record(ctx, nil, "delete", "operation", time.Since(started), 0) }()
	cleaned, objectName, err := c.resolve(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.run(ctx, "delete", cleaned, c.cfg.StatTimeout, func(attemptCtx context.Context, ep *endpointState) (bool, error) {
		resp, err := c.send(attemptCtx, ep, http.MethodDelete, objectName, "", nil, nil)
		if err != nil {
			return true, err
		}
		defer drainClose(resp)
		switch resp.StatusCode {
		case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
			return false, nil
		default:
			return statusError(resp, "delete", cleaned, ep)
		}
	})
}

func (c *client) List(ctx context.Context, prefix storage.Key, fn func(storage.Object) error) error {
	started := time.Now()
	defer func() { c.record(ctx, nil, "list", "operation", time.Since(started), 0) }()
	cleaned, err := storage.CleanPrefix(string(prefix))
	if err != nil {
		return err
	}
	fullPrefix := c.rootPrefix + string(cleaned)
	token := ""
	seenTokens := make(map[string]struct{})
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber >= c.cfg.MaxListPages {
			return fmt.Errorf("s3: list page limit %d exceeded", c.cfg.MaxListPages)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		page, err := c.listPage(ctx, cleaned, fullPrefix, token, "")
		if err != nil {
			return err
		}
		if err := c.emit(ctx, page.Contents, fn); err != nil {
			return err
		}
		if !page.IsTruncated {
			return nil
		}
		if page.NextContinuationToken == "" {
			return fmt.Errorf("s3: list %q: truncated page without continuation token", cleaned)
		}
		if _, seen := seenTokens[page.NextContinuationToken]; seen {
			return errors.New("s3: repeated continuation token")
		}
		seenTokens[page.NextContinuationToken] = struct{}{}
		token = page.NextContinuationToken
	}
}

// ListDir is ListObjectsV2 with delimiter "/": objects directly under the
// prefix arrive as Contents, sub-prefixes as CommonPrefixes.
func (c *client) ListDir(ctx context.Context, prefix storage.Key, fn func(storage.DirEntry) error) error {
	started := time.Now()
	defer func() { c.record(ctx, nil, "listdir", "operation", time.Since(started), 0) }()
	cleaned, err := storage.CleanDirPrefix(string(prefix))
	if err != nil {
		return err
	}
	fullPrefix := c.rootPrefix + string(cleaned)
	token := ""
	seenTokens := make(map[string]struct{})
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber >= c.cfg.MaxListPages {
			return fmt.Errorf("s3: list page limit %d exceeded", c.cfg.MaxListPages)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		page, err := c.listPage(ctx, cleaned, fullPrefix, token, "/")
		if err != nil {
			return err
		}
		if err := c.emitDir(ctx, fullPrefix, page, fn); err != nil {
			return err
		}
		if !page.IsTruncated {
			return nil
		}
		if page.NextContinuationToken == "" {
			return fmt.Errorf("s3: listdir %q: truncated page without continuation token", cleaned)
		}
		if _, seen := seenTokens[page.NextContinuationToken]; seen {
			return errors.New("s3: repeated continuation token")
		}
		seenTokens[page.NextContinuationToken] = struct{}{}
		token = page.NextContinuationToken
	}
}

func (c *client) listPage(ctx context.Context, prefix storage.Key, fullPrefix, token, delimiter string) (listBucketResult, error) {
	op := "list"
	if delimiter != "" {
		op = "listdir"
	}
	started := time.Now()
	defer func() { c.record(ctx, nil, op, "page", time.Since(started), 0) }()
	query := "list-type=2&max-keys=" + strconv.Itoa(c.pageSize) + "&prefix=" + encodeQueryComponent(fullPrefix)
	if delimiter != "" {
		query += "&delimiter=" + encodeQueryComponent(delimiter)
	}
	if token != "" {
		query += "&continuation-token=" + encodeQueryComponent(token)
	}
	var page listBucketResult
	err := c.run(ctx, op, prefix, c.cfg.RequestTimeout, func(attemptCtx context.Context, ep *endpointState) (bool, error) {
		resp, err := c.send(attemptCtx, ep, http.MethodGet, "", query, nil, nil)
		if err != nil {
			return true, err
		}
		defer drainClose(resp)
		if resp.StatusCode != http.StatusOK {
			return statusError(resp, "list", prefix, ep)
		}
		decodeStarted := time.Now()
		page, err = decodeList(resp.Body)
		c.record(ctx, ep, op, "decode", time.Since(decodeStarted), 0)
		return err != nil, err
	})
	return page, err
}

// emit hands listed objects to fn with the root prefix removed. Keys outside
// the root or not in CleanKey form (written by another tool) are skipped: they
// could not be addressed through this Store anyway.
func (c *client) emit(ctx context.Context, entries []listEntry, fn func(storage.Object) error) error {
	for _, entry := range entries {
		rel, ok := strings.CutPrefix(entry.Key, c.rootPrefix)
		if !ok {
			continue
		}
		if cleaned, err := storage.CleanKey(rel); err != nil || string(cleaned) != rel {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		object := storage.Object{
			Key:     storage.Key(rel),
			Size:    entry.Size,
			ModTime: parseListTime(entry.LastModified),
			ETag:    trimETag(entry.ETag),
		}
		if err := fn(object); err != nil {
			return err
		}
	}
	return nil
}

// emitDir hands one delimiter page to fn. Objects whose name still contains
// a "/" (a server ignoring the delimiter), the prefix marker itself and keys
// not in CleanKey form are skipped.
func (c *client) emitDir(ctx context.Context, fullPrefix string, page listBucketResult, fn func(storage.DirEntry) error) error {
	for _, entry := range page.Contents {
		name, ok := strings.CutPrefix(entry.Key, fullPrefix)
		if !ok || name == "" || strings.Contains(name, "/") {
			continue
		}
		rel := strings.TrimPrefix(entry.Key, c.rootPrefix)
		if cleaned, err := storage.CleanKey(rel); err != nil || string(cleaned) != rel {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		object := storage.Object{
			Key:     storage.Key(rel),
			Size:    entry.Size,
			ModTime: parseListTime(entry.LastModified),
			ETag:    trimETag(entry.ETag),
		}
		if err := fn(storage.DirEntry{Name: name, Object: object}); err != nil {
			return err
		}
	}
	for _, common := range page.CommonPrefixes {
		rel, ok := strings.CutPrefix(common.Prefix, fullPrefix)
		if !ok {
			continue
		}
		name := strings.TrimSuffix(rel, "/")
		if name == "" || strings.Contains(name, "/") {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(storage.DirEntry{Name: name, Dir: true}); err != nil {
			return err
		}
	}
	return nil
}
