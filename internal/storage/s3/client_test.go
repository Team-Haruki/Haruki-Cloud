package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/storage"
)

func testConfig(endpoints ...string) Config {
	return Config{
		Endpoints:      endpoints,
		Bucket:         "bkt",
		PathStyle:      true,
		RequestTimeout: 2 * time.Second,
		StatTimeout:    2 * time.Second,
	}
}

func mustClient(t *testing.T, cfg Config) *client {
	t.Helper()
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	return c
}

func serve(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func xmlError(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "<Error><Code>%s</Code><Message>msg %s</Message></Error>", code, code)
}

func TestNewValidation(t *testing.T) {
	base := testConfig("http://127.0.0.1:3900")
	cases := map[string]func(*Config){
		"no endpoints":    func(c *Config) { c.Endpoints = nil },
		"bad scheme":      func(c *Config) { c.Endpoints = []string{"ftp://host"} },
		"no host":         func(c *Config) { c.Endpoints = []string{"http://"} },
		"unparseable":     func(c *Config) { c.Endpoints = []string{"http://[::1"} },
		"query":           func(c *Config) { c.Endpoints = []string{"http://host/?a=1"} },
		"userinfo":        func(c *Config) { c.Endpoints = []string{"http://u:p@host"} },
		"empty bucket":    func(c *Config) { c.Bucket = " " },
		"slash bucket":    func(c *Config) { c.Bucket = "a/b" },
		"bad root":        func(c *Config) { c.Root = "a/../../b" },
		"half keys":       func(c *Config) { c.AccessKey = "ak" },
		"half secret":     func(c *Config) { c.SecretKey = "sk" },
		"bad proxy":       func(c *Config) { c.Proxy = "socks" },
		"relative scheme": func(c *Config) { c.Endpoints = []string{"host:3900"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := base
			mutate(&cfg)
			if store, err := New(cfg); err == nil || store != nil {
				t.Fatalf("New accepted invalid config: %v", err)
			}
		})
	}
}

func TestNewDefaults(t *testing.T) {
	store, err := New(Config{Endpoints: []string{" http://a:1/base/ ", "https://b"}, Bucket: "bkt", Root: "/pjsk/x/"})
	if err != nil {
		t.Fatal(err)
	}
	c := store.(*client)
	if c.cfg.Region != DefaultRegion || c.cfg.DialTimeout != DefaultDialTimeout ||
		c.cfg.RequestTimeout != DefaultRequestTimeout || c.cfg.StatTimeout != DefaultStatTimeout ||
		c.cfg.FailoverCooldown != DefaultFailoverCooldown || c.cfg.MaxAttempts != 2 ||
		c.cfg.MaxObjectBytes != storage.DefaultMaxObjectBytes {
		t.Fatalf("defaults not applied: %+v", c.cfg)
	}
	if c.rootPrefix != "pjsk/x/" || c.endpoints[0].url.Path != "/base" {
		t.Fatalf("root %q endpoint path %q", c.rootPrefix, c.endpoints[0].url.Path)
	}
	if c.transport == nil || c.transport.Proxy != nil {
		t.Fatal("built transport must exist with Proxy == nil")
	}
}

func TestProxyModes(t *testing.T) {
	cfg := testConfig("http://a")
	cfg.Proxy = ProxyNone
	if c := mustClient(t, cfg); c.transport.Proxy != nil {
		t.Fatal("proxy none must clear Proxy")
	}
	cfg.Proxy = ProxyEnvironment
	if c := mustClient(t, cfg); c.transport.Proxy == nil {
		t.Fatal("proxy environment must keep ProxyFromEnvironment")
	}
	cfg.Transport = http.DefaultTransport
	if c := mustClient(t, cfg); c.transport != nil || c.httpClient.Transport != http.DefaultTransport {
		t.Fatal("injected transport must be used as is")
	}
}

func TestURLShapeAndKeyEscaping(t *testing.T) {
	var gotHost, gotPath, gotQuery string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotHost, gotPath, gotQuery = req.Host, req.URL.EscapedPath(), req.URL.RawQuery
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
	})
	cfg := testConfig("http://garage:3900/gw")
	cfg.Root = "root"
	cfg.Transport = transport
	c := mustClient(t, cfg)
	if _, err := c.Get(context.Background(), "a b/c+d/ü.png"); err != nil {
		t.Fatal(err)
	}
	if gotHost != "garage:3900" || gotPath != "/gw/bkt/root/a%20b/c%2Bd/%C3%BC.png" {
		t.Fatalf("path-style host=%q path=%q", gotHost, gotPath)
	}
	cfg.PathStyle = false
	c = mustClient(t, cfg)
	if _, err := c.Get(context.Background(), "k"); err != nil {
		t.Fatal(err)
	}
	if gotHost != "bkt.garage:3900" || gotPath != "/gw/root/k" {
		t.Fatalf("virtual-host host=%q path=%q", gotHost, gotPath)
	}
	_ = c.List(context.Background(), "", func(storage.Object) error { return nil })
	if gotHost != "bkt.garage:3900" || gotPath != "/gw/" || !strings.Contains(gotQuery, "prefix=root%2F") {
		t.Fatalf("virtual-host list host=%q path=%q query=%q", gotHost, gotPath, gotQuery)
	}
}

func TestSignedAndAnonymousRequests(t *testing.T) {
	var auth, content atomic.Value
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
		content.Store(r.Header.Get("X-Amz-Content-Sha256"))
		w.WriteHeader(http.StatusOK)
	})
	anonymous := mustClient(t, testConfig(server.URL))
	if err := anonymous.Put(context.Background(), "k", []byte("x"), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if auth.Load().(string) != "" || content.Load().(string) != "" {
		t.Fatal("anonymous mode must not sign")
	}
	cfg := testConfig(server.URL)
	cfg.AccessKey, cfg.SecretKey = "ak", "sk"
	signed := mustClient(t, cfg)
	if err := signed.Put(context.Background(), "k", []byte("x"), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(auth.Load().(string), "AWS4-HMAC-SHA256 Credential=ak/") || content.Load().(string) != PayloadSHA256([]byte("x")) {
		t.Fatalf("signed request auth=%q", auth.Load())
	}
}

func TestGetStatuses(t *testing.T) {
	var status atomic.Int32
	var code atomic.Value
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if s := int(status.Load()); s != http.StatusOK {
			xmlError(w, s, code.Load().(string))
			return
		}
		_, _ = io.WriteString(w, "payload")
	})
	c := mustClient(t, testConfig(server.URL))
	ctx := context.Background()

	status.Store(http.StatusOK)
	if data, err := c.Get(ctx, "k"); err != nil || string(data) != "payload" {
		t.Fatalf("Get = %q, %v", data, err)
	}
	status.Store(http.StatusNotFound)
	code.Store("NoSuchKey")
	if _, err := c.Get(ctx, "k"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("404 error = %v", err)
	}
	code.Store("NoSuchBucket")
	var respErr *ResponseError
	if _, err := c.Get(ctx, "k"); !errors.As(err, &respErr) || errors.Is(err, storage.ErrNotExist) || respErr.Code != "NoSuchBucket" {
		t.Fatalf("NoSuchBucket error = %v", err)
	}
	status.Store(http.StatusForbidden)
	code.Store("AccessDenied")
	_, err := c.Get(ctx, "k")
	if !errors.As(err, &respErr) || respErr.StatusCode != 403 || respErr.Code != "AccessDenied" || respErr.Op != "get" {
		t.Fatalf("403 error = %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "HTTP 403 AccessDenied: msg AccessDenied") {
		t.Fatalf("error text = %q", msg)
	}
	for _, s := range []int{http.StatusInternalServerError, http.StatusServiceUnavailable} {
		status.Store(int32(s))
		code.Store("SlowDown")
		if _, err := c.Get(ctx, "k"); !errors.As(err, &respErr) || respErr.StatusCode != s || !strings.Contains(err.Error(), "after 1 attempts") {
			t.Fatalf("%d error = %v", s, err)
		}
	}
	if _, err := c.Get(ctx, "../x"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("invalid key error = %v", err)
	}
}

func TestResponseErrorWithoutBody(t *testing.T) {
	err := &ResponseError{Op: "stat", Key: "k", Endpoint: "http://a", StatusCode: 500}
	if err.Error() != `s3: stat "k" at http://a: HTTP 500` {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestGetTooLarge(t *testing.T) {
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bkt/declared" {
			_, _ = w.Write(bytes.Repeat([]byte("a"), 32))
			return
		}
		w.(http.Flusher).Flush() // chunked: no Content-Length
		_, _ = w.Write(bytes.Repeat([]byte("a"), 32))
	})
	cfg := testConfig(server.URL)
	cfg.MaxObjectBytes = 16
	c := mustClient(t, cfg)
	for _, key := range []storage.Key{"declared", "chunked"} {
		if _, err := c.Get(context.Background(), key); !errors.Is(err, storage.ErrTooLarge) {
			t.Fatalf("Get(%s) error = %v", key, err)
		}
	}
	if got := len(c.endpoints); got != 1 || c.endpoints[0].consecutiveFail.Load() != 0 {
		t.Fatal("ErrTooLarge must not mark the endpoint failed")
	}
}

func TestGetTruncatedBodyIsRetried(t *testing.T) {
	var calls atomic.Int32
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Content-Length", "100")
			_, _ = io.WriteString(w, "short")
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		_, _ = io.WriteString(w, "full")
	})
	cfg := testConfig(server.URL)
	cfg.MaxAttempts = 2
	c := mustClient(t, cfg)
	if data, err := c.Get(context.Background(), "k"); err != nil || string(data) != "full" {
		t.Fatalf("Get = %q, %v", data, err)
	}
}

func TestPutHeadersAndErrors(t *testing.T) {
	var mu sync.Mutex
	var headers http.Header
	var body []byte
	var fail atomic.Bool
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		headers = r.Header.Clone()
		body, _ = io.ReadAll(r.Body)
		mu.Unlock()
		if fail.Load() {
			xmlError(w, http.StatusBadRequest, "InvalidArgument")
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	cfg := testConfig(server.URL)
	cfg.DefaultACL = "public-read"
	c := mustClient(t, cfg)
	ctx := context.Background()
	png := []byte("\x89PNG\r\n\x1a\n0000")
	if err := c.Put(ctx, "a.png", png, storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if headers.Get("Content-Type") != "image/png" || headers.Get("Cache-Control") != "" ||
		headers.Get("X-Amz-Acl") != "public-read" || !bytes.Equal(body, png) {
		t.Fatalf("headers = %v body=%q", headers, body)
	}
	mu.Unlock()
	if err := c.Put(ctx, "a.json", nil, storage.PutOptions{ContentType: "application/json", CacheControl: "no-cache"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if headers.Get("Content-Type") != "application/json" || headers.Get("Cache-Control") != "no-cache" ||
		headers.Get("Content-Length") != "0" || len(body) != 0 {
		t.Fatalf("headers = %v body=%q", headers, body)
	}
	mu.Unlock()
	fail.Store(true)
	var respErr *ResponseError
	if err := c.Put(ctx, "a", []byte("x"), storage.PutOptions{}); !errors.As(err, &respErr) || respErr.Code != "InvalidArgument" {
		t.Fatalf("Put error = %v", err)
	}
	cfg.MaxObjectBytes = 1
	small := mustClient(t, cfg)
	if err := small.Put(ctx, "a", []byte("xx"), storage.PutOptions{}); !errors.Is(err, storage.ErrTooLarge) {
		t.Fatalf("oversize Put error = %v", err)
	}
	if err := small.Put(ctx, "", []byte("x"), storage.PutOptions{}); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("invalid key Put error = %v", err)
	}
}

func TestStatParsing(t *testing.T) {
	var mode atomic.Value
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %s", r.Method)
		}
		switch mode.Load().(string) {
		case "ok":
			w.Header().Set("Content-Length", "42")
			w.Header().Set("Last-Modified", "Sun, 13 Sep 2026 08:00:00 GMT")
			w.Header().Set("ETag", `"abc"`)
		case "bad-time":
			w.Header().Set("Content-Length", "7")
			w.Header().Set("Last-Modified", "yesterday")
		case "missing":
			w.WriteHeader(http.StatusNotFound)
			return
		case "down":
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	c := mustClient(t, testConfig(server.URL))
	ctx := context.Background()
	mode.Store("ok")
	object, err := c.Stat(ctx, "dir/k")
	want := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	if err != nil || object.Key != "dir/k" || object.Size != 42 || object.ETag != "abc" || !object.ModTime.Equal(want) {
		t.Fatalf("Stat = %+v, %v", object, err)
	}
	mode.Store("bad-time")
	if object, err = c.Stat(ctx, "k"); err != nil || object.Size != 7 || !object.ModTime.IsZero() {
		t.Fatalf("Stat malformed Last-Modified = %+v, %v", object, err)
	}
	badLength := &http.Response{Header: http.Header{"Content-Length": {"-"}}}
	if _, err = statObject(badLength, "k"); err == nil || !strings.Contains(err.Error(), "Content-Length") {
		t.Fatalf("Stat malformed Content-Length error = %v", err)
	}
	mode.Store("missing")
	if _, err = c.Stat(ctx, "k"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("Stat 404 error = %v", err)
	}
	mode.Store("down")
	var respErr *ResponseError
	if _, err = c.Stat(ctx, "k"); !errors.As(err, &respErr) || respErr.StatusCode != http.StatusBadGateway || respErr.Code != "" {
		t.Fatalf("Stat 502 error = %v", err)
	}
	if _, err = c.Stat(ctx, "a//b"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("Stat invalid key error = %v", err)
	}
}

func TestDeleteStatuses(t *testing.T) {
	var status atomic.Int32
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		xmlError(w, int(status.Load()), "X")
	})
	c := mustClient(t, testConfig(server.URL))
	for _, s := range []int{http.StatusOK, http.StatusNoContent, http.StatusNotFound} {
		status.Store(int32(s))
		if err := c.Delete(context.Background(), "k"); err != nil {
			t.Fatalf("Delete %d error = %v", s, err)
		}
	}
	status.Store(http.StatusForbidden)
	if err := c.Delete(context.Background(), "k"); err == nil {
		t.Fatal("Delete 403 must fail")
	}
	if err := c.Delete(context.Background(), "../k"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("Delete invalid key error = %v", err)
	}
}

const listPage1 = `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
<IsTruncated>true</IsTruncated><NextContinuationToken>tok/2 +</NextContinuationToken>
<Contents><Key>root/a</Key><LastModified>2026-09-13T08:00:00.000Z</LastModified><ETag>&quot;e1&quot;</ETag><Size>3</Size></Contents>
<Contents><Key>other/x</Key><Size>1</Size></Contents>
<Contents><Key>root/bad//key</Key><Size>1</Size></Contents>
</ListBucketResult>`

const listPage2 = `<ListBucketResult><IsTruncated>false</IsTruncated>
<Contents><Key>root/b/c</Key><LastModified>garbage</LastModified><Size>5</Size></Contents>
</ListBucketResult>`

func TestListPaginationAndFiltering(t *testing.T) {
	var queries []string
	var mu sync.Mutex
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.RawQuery)
		mu.Unlock()
		if r.URL.Path != "/bkt" {
			t.Errorf("list path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("continuation-token") == "tok/2 +" {
			_, _ = io.WriteString(w, listPage2)
			return
		}
		_, _ = io.WriteString(w, listPage1)
	})
	cfg := testConfig(server.URL)
	cfg.Root = "root"
	c := mustClient(t, cfg)
	c.pageSize = 2
	var objects []storage.Object
	err := c.List(context.Background(), "", func(object storage.Object) error {
		objects = append(objects, object)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 2 || objects[0].Key != "a" || objects[0].Size != 3 || objects[0].ETag != "e1" ||
		objects[0].ModTime.IsZero() || objects[1].Key != "b/c" || !objects[1].ModTime.IsZero() {
		t.Fatalf("objects = %+v", objects)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(queries) != 2 || !strings.Contains(queries[0], "list-type=2") || !strings.Contains(queries[0], "max-keys=2") ||
		!strings.Contains(queries[0], "prefix=root%2F") || !strings.Contains(queries[1], "continuation-token=tok%2F2%20%2B") {
		t.Fatalf("queries = %v", queries)
	}
}

func TestListErrors(t *testing.T) {
	var body atomic.Value
	var status atomic.Int32
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(status.Load()))
		_, _ = io.WriteString(w, body.Load().(string))
	})
	c := mustClient(t, testConfig(server.URL))
	ctx := context.Background()
	noop := func(storage.Object) error { return nil }

	status.Store(http.StatusOK)
	body.Store(`<ListBucketResult><IsTruncated>true</IsTruncated></ListBucketResult>`)
	if err := c.List(ctx, "p/", noop); err == nil || !strings.Contains(err.Error(), "continuation token") {
		t.Fatalf("truncated without token error = %v", err)
	}
	body.Store(`<nope`)
	if err := c.List(ctx, "p/", noop); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("malformed XML error = %v", err)
	}
	status.Store(http.StatusForbidden)
	body.Store(`<Error><Code>AccessDenied</Code></Error>`)
	var respErr *ResponseError
	if err := c.List(ctx, "p/", noop); !errors.As(err, &respErr) || respErr.Code != "AccessDenied" || respErr.Op != "list" {
		t.Fatalf("403 error = %v", err)
	}
	status.Store(http.StatusOK)
	body.Store(`<ListBucketResult><Contents><Key>p/1</Key></Contents><Contents><Key>p/2</Key></Contents></ListBucketResult>`)
	stop := errors.New("stop")
	visits := 0
	if err := c.List(ctx, "p/", func(storage.Object) error { visits++; return stop }); !errors.Is(err, stop) || visits != 1 {
		t.Fatalf("abort error = %v visits=%d", err, visits)
	}
	if err := c.List(ctx, "../", noop); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("invalid prefix error = %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	visits = 0
	err := c.List(canceled, "p/", func(storage.Object) error { visits++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || visits != 1 {
		t.Fatalf("cancel during emit error = %v visits=%d", err, visits)
	}
}

func TestContextCancellationMidRequest(t *testing.T) {
	release := make(chan struct{})
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	defer close(release)
	c := mustClient(t, testConfig(server.URL, server.URL))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	_, err := c.Get(ctx, "k")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Get error = %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation was not prompt")
	}
	for _, ep := range c.endpoints {
		if ep.consecutiveFail.Load() != 0 {
			t.Fatal("caller cancellation must not mark endpoints failed")
		}
	}
	for name, op := range map[string]func(context.Context) error{
		"get":    func(ctx context.Context) error { _, err := c.Get(ctx, "k"); return err },
		"put":    func(ctx context.Context) error { return c.Put(ctx, "k", []byte("x"), storage.PutOptions{}) },
		"stat":   func(ctx context.Context) error { _, err := c.Stat(ctx, "k"); return err },
		"delete": func(ctx context.Context) error { return c.Delete(ctx, "k") },
		"list":   func(ctx context.Context) error { return c.List(ctx, "", func(storage.Object) error { return nil }) },
	} {
		done, cancelDone := context.WithCancel(context.Background())
		cancelDone()
		if err := op(done); !errors.Is(err, context.Canceled) {
			t.Fatalf("%s with canceled ctx error = %v", name, err)
		}
	}
}

func TestRequestBuildFailure(t *testing.T) {
	c := mustClient(t, testConfig("http://a"))
	ep := c.endpoints[0]
	if _, err := c.send(context.Background(), ep, "BAD METHOD", "k", "", nil, nil); err == nil {
		t.Fatal("send with an invalid method must fail")
	}
}

const listDirPage1 = `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
<IsTruncated>true</IsTruncated><NextContinuationToken>root/d/b/</NextContinuationToken>
<Contents><Key>root/d/</Key><Size>0</Size></Contents>
<Contents><Key>root/d/a</Key><LastModified>2026-09-13T08:00:00.000Z</LastModified><ETag>&quot;e1&quot;</ETag><Size>3</Size></Contents>
<CommonPrefixes><Prefix>root/d/b/</Prefix></CommonPrefixes>
</ListBucketResult>`

const listDirPage2 = `<ListBucketResult><IsTruncated>false</IsTruncated>
<Contents><Key>root/d/c</Key><Size>5</Size></Contents>
<Contents><Key>root/d/ignored/slash</Key><Size>1</Size></Contents>
<Contents><Key>other/x</Key><Size>1</Size></Contents>
<CommonPrefixes><Prefix>root/d/e/</Prefix></CommonPrefixes>
<CommonPrefixes><Prefix>root/d/</Prefix></CommonPrefixes>
<CommonPrefixes><Prefix>elsewhere/</Prefix></CommonPrefixes>
</ListBucketResult>`

// The continuation token of the first page lands on a common prefix, as it
// does when a page boundary falls between two sub-directories.
func TestListDirPagesAcrossCommonPrefixes(t *testing.T) {
	var queries []string
	var mu sync.Mutex
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.RawQuery)
		mu.Unlock()
		if r.URL.Query().Get("continuation-token") == "root/d/b/" {
			_, _ = io.WriteString(w, listDirPage2)
			return
		}
		_, _ = io.WriteString(w, listDirPage1)
	})
	cfg := testConfig(server.URL)
	cfg.Root = "root"
	c := mustClient(t, cfg)
	c.pageSize = 2
	var objects, dirs []string
	err := c.ListDir(context.Background(), "d", func(entry storage.DirEntry) error {
		if entry.Dir {
			if entry.Object != (storage.Object{}) {
				t.Errorf("dir %q carries an object", entry.Name)
			}
			dirs = append(dirs, entry.Name)
			return nil
		}
		if entry.Object.Key != storage.Key("d/"+entry.Name) {
			t.Errorf("object %q has key %q", entry.Name, entry.Object.Key)
		}
		objects = append(objects, entry.Name)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(objects, ",") != "a,c" || strings.Join(dirs, ",") != "b,e" {
		t.Fatalf("objects = %v, dirs = %v", objects, dirs)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(queries) != 2 {
		t.Fatalf("queries = %v", queries)
	}
	first, _ := url.ParseQuery(queries[0])
	second, _ := url.ParseQuery(queries[1])
	if first.Get("delimiter") != "/" || first.Get("prefix") != "root/d/" || first.Get("max-keys") != "2" || first.Get("continuation-token") != "" ||
		second.Get("continuation-token") != "root/d/b/" || second.Get("delimiter") != "/" {
		t.Fatalf("queries = %v", queries)
	}
}
