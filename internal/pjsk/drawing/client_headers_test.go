package drawing

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/logger"
)

type drawingResponseShape string

const (
	shapePNG         drawingResponseShape = "png"
	shapeRef         drawingResponseShape = "ref"
	shapeDegraded    drawingResponseShape = "degraded"
	shapeBadJSON     drawingResponseShape = "bad-json"
	shapeInsufficent drawingResponseShape = "insufficient"
	shapeRejected    drawingResponseShape = "rejected"
)

type artifactDrawingServer struct {
	*httptest.Server
	mu      sync.Mutex
	shape   drawingResponseShape
	headers map[string]http.Header
}

func newArtifactDrawingServer(t *testing.T) *artifactDrawingServer {
	t.Helper()
	s := &artifactDrawingServer{shape: shapePNG, headers: map[string]http.Header{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.headers[r.URL.Path] = r.Header.Clone()
		shape := s.shape
		s.mu.Unlock()
		w.Header().Set(headerNode, "cn01")
		switch shape {
		case shapeRef:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(testArtifactRefJSON(map[string]string{"node_name": ""})))
		case shapeDegraded:
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set(headerArtifactDegraded, "1")
			_, _ = w.Write([]byte("degraded-png"))
		case shapeBadJSON:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"kind":"artifact_ref","hash":"nope"}`))
		case shapeInsufficent:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"kind":"artifact_ref","detail":"data insufficient"}`))
		case shapeRejected:
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set(headerDirectiveError, headerCacheTTL)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":"invalid X-Haruki-Cache-TTL: too large","header":"X-Haruki-Cache-TTL","code":"ttl_too_large"}`))
		default:
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("plain-png"))
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *artifactDrawingServer) setShape(shape drawingResponseShape) {
	s.mu.Lock()
	s.shape = shape
	s.mu.Unlock()
}

func (s *artifactDrawingServer) lastHeaders(path string) http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.headers[path]
}

func harukiHeaders(header http.Header) map[string]string {
	out := map[string]string{}
	for name, values := range header {
		if strings.HasPrefix(strings.ToLower(name), "x-haruki-") {
			out[http.CanonicalHeaderKey(name)] = strings.Join(values, ",")
		}
	}
	return out
}

// newArtifactTestClient wires an index-mode render cache whose index always
// misses, so every call renders through Drawing.
func newArtifactTestClient(t *testing.T, server *artifactDrawingServer, cfg ArtifactConfig) *HarukiDrawingClient {
	t.Helper()
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(cfg))
	cache := NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: &fakeRenderIndex{}, Artifacts: cfg.Objects})
	client.SetRenderCache(cache)
	t.Cleanup(func() { _ = cache.Close() })
	return client.WithContext(context.Background())
}

func requireFullDirective(t *testing.T, headers map[string]string, store, ttl, version, apiPath string) {
	t.Helper()
	if len(headers) != 8 {
		t.Fatalf("headers = %v, want the eight directive headers", headers)
	}
	want := map[string]string{
		headerArtifact: "1", headerCacheStore: store, headerCacheTTL: ttl, headerCacheKeyVersion: version,
		headerCacheGroup: "pjsk", headerAPIPath: apiPath, headerUserID: "public",
	}
	for name, value := range want {
		if headers[http.CanonicalHeaderKey(name)] != value {
			t.Fatalf("%s = %q, want %q (all: %v)", name, headers[http.CanonicalHeaderKey(name)], value, headers)
		}
	}
	if !isHex64(headers[http.CanonicalHeaderKey(headerCacheKey)]) {
		t.Fatalf("cache key = %q", headers[http.CanonicalHeaderKey(headerCacheKey)])
	}
}

func TestArtifactHeadersOnCachedAllowListedRequest(t *testing.T) {
	server := newArtifactDrawingServer(t)
	client := newArtifactTestClient(t, server, ArtifactConfig{Endpoints: []string{"api/pjsk/card/box", "api/pjsk/event/list"}})
	data, err := client.GenerateCardBox(&CardBoxRequest{})
	if err != nil || string(data) != "plain-png" {
		t.Fatalf("card box = %q, %v", data, err)
	}
	ttl := strconv.FormatInt(directiveTTLSeconds(resolveRenderCacheRule("/api/pjsk/card/box").TTL, false), 10)
	requireFullDirective(t, harukiHeaders(server.lastHeaders("/api/pjsk/card/box")), "1", ttl, "3", "api/pjsk/card/box")

	if _, err := client.GenerateEventList(&EventListRequest{}); err != nil {
		t.Fatalf("event list: %v", err)
	}
	headers := harukiHeaders(server.lastHeaders("/api/pjsk/event/list"))
	if headers[headerCacheKeyVersion] != "5" || headers[headerArtifact] != "1" {
		t.Fatalf("event list headers = %v", headers)
	}

	if _, err := client.GenerateCardList(&CardListRequest{}); err != nil {
		t.Fatalf("card list: %v", err)
	}
	if headers := harukiHeaders(server.lastHeaders("/api/pjsk/card/list")); len(headers) != 0 {
		t.Fatalf("non allow-listed request sent %v", headers)
	}
}

func TestArtifactHeadersImageEndpointAndInfiniteTTL(t *testing.T) {
	server := newArtifactDrawingServer(t)
	client := newArtifactTestClient(t, server, ArtifactConfig{Endpoints: []string{"*"}})
	image, err := client.GenerateCardBoxImage(&CardBoxRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if data, err := image.Bytes(t.Context()); err != nil || string(data) != "plain-png" {
		t.Fatalf("image = %q, %v", data, err)
	}
	if headers := harukiHeaders(server.lastHeaders("/api/pjsk/card/box")); headers[headerCacheStore] != "1" || len(headers) != 8 {
		t.Fatalf("image headers = %v", headers)
	}
	if _, err := client.GenerateStampList(&StampListRequest{}); err != nil {
		t.Fatal(err)
	}
	if rule := resolveRenderCacheRule("/api/pjsk/stamp/list"); rule.Infinite {
		if headers := harukiHeaders(server.lastHeaders("/api/pjsk/stamp/list")); headers[headerCacheTTL] != "0" {
			t.Fatalf("infinite ttl headers = %v", headers)
		}
	}
}

func TestArtifactHeadersAbsentWithEmptyAllowList(t *testing.T) {
	server := newArtifactDrawingServer(t)
	client := newArtifactTestClient(t, server, ArtifactConfig{})
	server.setShape(shapeDegraded)
	calls := map[string]func() ([]byte, error){
		"/api/pjsk/card/box":        func() ([]byte, error) { return client.GenerateCardBox(&CardBoxRequest{}) },
		"/api/pjsk/event/detail":    func() ([]byte, error) { return client.GenerateEventDetail(&EventDetailRequest{}) },
		"/api/pjsk/misc/alias-list": func() ([]byte, error) { return client.GenerateAliasList(&AliasListRequest{}) },
	}
	for path, call := range calls {
		data, err := call()
		if err != nil || string(data) != "degraded-png" {
			t.Fatalf("%s = %q, %v", path, data, err)
		}
		if headers := harukiHeaders(server.lastHeaders(path)); len(headers) != 0 {
			t.Fatalf("%s sent %v with an empty allow-list", path, headers)
		}
	}
}

func TestArtifactHeadersOnUncachedCallSites(t *testing.T) {
	server := newArtifactDrawingServer(t)
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(ArtifactConfig{Endpoints: []string{"api/pjsk/event/detail", "api/pjsk/misc/alias-list"}})).WithContext(context.Background())
	if data, err := client.GenerateEventDetail(&EventDetailRequest{}); err != nil || string(data) != "plain-png" {
		t.Fatalf("event detail = %q, %v", data, err)
	}
	requireFullDirective(t, harukiHeaders(server.lastHeaders("/api/pjsk/event/detail")), "0", "86400", "3", "api/pjsk/event/detail")
	server.setShape(shapeDegraded)
	if data, err := client.GenerateAliasList(&AliasListRequest{}); err != nil || string(data) != "degraded-png" {
		t.Fatalf("alias list = %q, %v", data, err)
	}
	requireFullDirective(t, harukiHeaders(server.lastHeaders("/api/pjsk/misc/alias-list")), "0", "0", "3", "api/pjsk/misc/alias-list")

	server.setShape(shapeRef)
	if data, err := client.GenerateAliasList(&AliasListRequest{}); err == nil || data != nil || !strings.Contains(err.Error(), "uncached endpoint /api/pjsk/misc/alias-list") {
		t.Fatalf("Store: 0 answered with a ref = %q, %v", data, err)
	}
	server.setShape(shapeBadJSON)
	if _, err := client.GenerateEventDetail(&EventDetailRequest{}); !errors.Is(err, errDrawingBadArtifactRef) {
		t.Fatalf("malformed artifact JSON err = %v", err)
	}

	plain := NewHarukiDrawingClient(server.URL, WithArtifactConfig(ArtifactConfig{Endpoints: []string{"api/pjsk/card/box"}})).WithContext(context.Background())
	server.setShape(shapePNG)
	if _, err := plain.GenerateEventDetail(&EventDetailRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := plain.GenerateAliasList(&AliasListRequest{}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/pjsk/event/detail", "/api/pjsk/misc/alias-list"} {
		if headers := harukiHeaders(server.lastHeaders(path)); len(headers) != 0 {
			t.Fatalf("%s sent %v when not allow-listed", path, headers)
		}
	}
}

func TestArtifactRefOnCachedPathResolvesBytes(t *testing.T) {
	server := newArtifactDrawingServer(t)
	objects := storagetest.NewMemory()
	ref, err := parseArtifactRef([]byte(testArtifactRefJSON(nil)))
	if err != nil {
		t.Fatal(err)
	}
	objects.Seed(map[string][]byte{ref.CDNPath: []byte("stored-artifact")})
	client := newArtifactTestClient(t, server, ArtifactConfig{Endpoints: []string{"api/pjsk/card/box"}, Objects: objects})
	server.setShape(shapeRef)
	data, err := client.GenerateCardBox(&CardBoxRequest{})
	if err != nil || string(data) != "stored-artifact" {
		t.Fatalf("card box via ref = %q, %v", data, err)
	}

	server.setShape(shapeBadJSON)
	if _, err := client.GenerateCardList(&CardListRequest{}); err != nil {
		t.Fatalf("non allow-listed JSON body must stay bytes: %v", err)
	}
	if _, err := client.GenerateCardBox(&CardBoxRequest{Region: "tw"}); !errors.Is(err, errDrawingBadArtifactRef) {
		t.Fatalf("malformed ref err = %v", err)
	}
}

func TestArtifactRefOnCachedPathFailsWhenBytesUnavailable(t *testing.T) {
	server := newArtifactDrawingServer(t)
	client := newArtifactTestClient(t, server, ArtifactConfig{Endpoints: []string{"api/pjsk/card/box"}})
	server.setShape(shapeRef)
	if _, err := client.GenerateCardBox(&CardBoxRequest{}); !errors.Is(err, ErrArtifactBytesUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestSuccessBodyShapes(t *testing.T) {
	server := newArtifactDrawingServer(t)
	settings := newArtifactSettings(ArtifactConfig{Endpoints: []string{"api/pjsk/card/box"}})
	base := NewHarukiDrawingClient(server.URL)
	base.artifact = settings
	directive := func() *renderDirective {
		return newRenderDirective(strings.Repeat("c", 64), renderCachePolicy{APIPath: "api/pjsk/card/box", UserID: "public", TTL: time.Hour}, time.Hour, true)
	}

	d := directive()
	server.setShape(shapeRef)
	data, err := base.WithContext(withDirective(context.Background(), d)).postPrepared("/api/pjsk/card/box", map[string]any{})
	if err != nil || data != nil || d.outcome.Ref == nil || d.outcome.Ref.NodeName != "cn01" || d.outcome.Node != "cn01" {
		t.Fatalf("ref shape = %q, %v, %+v", data, err, d.outcome)
	}

	d = directive()
	server.setShape(shapeDegraded)
	data, err = base.WithContext(withDirective(context.Background(), d)).postPrepared("/api/pjsk/card/box", map[string]any{})
	if err != nil || string(data) != "degraded-png" || !d.outcome.Degraded || d.outcome.Node != "cn01" || d.outcome.ContentType != "image/png" {
		t.Fatalf("degraded shape = %q, %v, %+v", data, err, d.outcome)
	}

	d = directive()
	server.setShape(shapePNG)
	data, err = base.WithContext(withDirective(context.Background(), d)).postPrepared("/api/pjsk/card/box", map[string]any{})
	if err != nil || string(data) != "plain-png" || d.outcome.Degraded || d.outcome.Ref != nil || d.outcome.Node != "cn01" {
		t.Fatalf("png shape = %q, %v, %+v", data, err, d.outcome)
	}

	d = directive()
	server.setShape(shapeInsufficent)
	_, err = base.WithContext(withDirective(context.Background(), d)).postPrepared("/api/pjsk/card/box", map[string]any{})
	if !errors.Is(err, ErrDrawingDataInsufficient) || d.outcome.Ref != nil {
		t.Fatalf("non-2xx classification err = %v, outcome %+v", err, d.outcome)
	}

	// A directive whose api path is not allow-listed is ignored entirely.
	d = newRenderDirective(strings.Repeat("c", 64), renderCachePolicy{APIPath: "api/pjsk/card/list", UserID: "public"}, time.Hour, true)
	server.setShape(shapeRef)
	data, err = base.WithContext(withDirective(context.Background(), d)).postPrepared("/api/pjsk/card/list", map[string]any{})
	if err != nil || len(data) == 0 || d.outcome.Ref != nil || len(harukiHeaders(server.lastHeaders("/api/pjsk/card/list"))) != 0 {
		t.Fatalf("non allow-listed directive = %q, %v, %+v", data, err, d.outcome)
	}
}

func TestDegradedHeaderWithoutDirectiveDoesNotPanic(t *testing.T) {
	server := newArtifactDrawingServer(t)
	server.setShape(shapeDegraded)
	client := NewHarukiDrawingClient(server.URL).WithContext(context.Background())
	data, err := client.postPrepared("/api/pjsk/card/box", map[string]any{})
	if err != nil || string(data) != "degraded-png" {
		t.Fatalf("degraded without directive = %q, %v", data, err)
	}
}

func TestDirectiveRejectionLogsErrorAndCounts(t *testing.T) {
	server := newArtifactDrawingServer(t)
	var logs bytes.Buffer
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(ArtifactConfig{Endpoints: []string{"api/pjsk/event/detail"}}))
	client.logger = logger.NewLogger("drawing-test", "DEBUG", &logs)
	server.setShape(shapeRejected)
	_, err := client.WithContext(context.Background()).GenerateEventDetail(&EventDetailRequest{})
	if err == nil || !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("rejected directive err = %v", err)
	}
	if client.DirectiveRejectedCount() != 1 {
		t.Fatalf("drawing_directive_rejected = %d", client.DirectiveRejectedCount())
	}
	output := logs.String()
	if strings.Count(output, "drawing rejected the render cache directive") != 1 || !strings.Contains(output, "ERROR") || !strings.Contains(output, "ttl_too_large") {
		t.Fatalf("logs = %s", output)
	}
	var nilClient *HarukiDrawingClient
	if nilClient.DirectiveRejectedCount() != 0 || (&HarukiDrawingClient{}).DirectiveRejectedCount() != 0 {
		t.Fatal("unconfigured counter is non-zero")
	}
}

func TestDirectiveRejectionFromBodyAndIgnoredCases(t *testing.T) {
	var logs bytes.Buffer
	var mu sync.Mutex
	body, header, status := "", "", http.StatusBadRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if header != "" {
			w.Header().Set(headerDirectiveError, header)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(ArtifactConfig{Endpoints: []string{"api/pjsk/card/box"}}))
	client.logger = logger.NewLogger("drawing-test", "DEBUG", &logs)
	d := newRenderDirective(strings.Repeat("d", 64), renderCachePolicy{APIPath: "api/pjsk/card/box", UserID: "public"}, time.Hour, true)
	withD := client.WithContext(withDirective(context.Background(), d))
	set := func(b, h string, s int) { mu.Lock(); body, header, status = b, h, s; mu.Unlock() }

	set(`{"detail":"invalid","header":"X-Haruki-Cache-Key","code":"bad_key"}`, "", http.StatusBadRequest)
	_, _ = withD.postPrepared("/api/pjsk/card/box", map[string]any{})
	if client.DirectiveRejectedCount() != 1 {
		t.Fatalf("body-coded rejection count = %d", client.DirectiveRejectedCount())
	}
	set(strings.Repeat(" ", drawingErrorClassificationBytes+10), "", http.StatusBadRequest)
	_, _ = withD.postPrepared("/api/pjsk/card/box", map[string]any{})
	set(`{"detail":"plain bad request"}`, "", http.StatusBadRequest)
	_, _ = withD.postPrepared("/api/pjsk/card/box", map[string]any{})
	set(`{"detail":"x","header":"X-Haruki-Cache-Key","code":"bad_key"}`, "", http.StatusBadRequest)
	_, _ = client.WithContext(context.Background()).postPrepared("/api/pjsk/card/box", map[string]any{})
	set(`{"detail":"x"}`, headerCacheKey, http.StatusInternalServerError)
	_, _ = withD.postPrepared("/api/pjsk/card/box", map[string]any{})
	if client.DirectiveRejectedCount() != 1 {
		t.Fatalf("non-directive failures counted: %d", client.DirectiveRejectedCount())
	}
	set(`not-json`, headerCacheKey, http.StatusBadRequest)
	_, _ = client.WithContext(context.Background()).postPrepared("/api/pjsk/card/box", map[string]any{})
	if client.DirectiveRejectedCount() != 2 {
		t.Fatalf("header rejection without directive count = %d", client.DirectiveRejectedCount())
	}
}
