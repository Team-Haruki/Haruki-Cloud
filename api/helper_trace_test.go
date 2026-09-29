package api

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

func TestWithCacheTraceDistinguishesColdAndCachedResponses(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	fetches := 0
	app, snapshots := responseTraceTestApp(fiber.Config{}, func(c fiber.Ctx) error {
		return WithCache(c, client, "private-cache-key", func(string) (any, error) {
			fetches++
			return fiber.Map{"value": "fresh"}, nil
		})
	})

	coldResponse, coldBody, cold := responseTraceTestRequest(t, app, snapshots)
	if coldResponse.StatusCode != http.StatusOK {
		t.Fatalf("cold status = %d", coldResponse.StatusCode)
	}
	assertResponseTrace(t, cold, 1, map[string]int{
		"api.cache_read": 1, "api.cache_write": 1, "api.data_fetch": 1,
		"response.json_encode": 1, "response.body_set": 1,
	})

	cachedResponse, cachedBody, cached := responseTraceTestRequest(t, app, snapshots)
	if cachedResponse.StatusCode != coldResponse.StatusCode || string(cachedBody) != string(coldBody) ||
		cachedResponse.Header.Get(fiber.HeaderContentType) != coldResponse.Header.Get(fiber.HeaderContentType) {
		t.Fatalf("cached response changed: status=%d body=%s headers=%v", cachedResponse.StatusCode, cachedBody, cachedResponse.Header)
	}
	if fetches != 1 {
		t.Fatalf("fetch count = %d, want 1", fetches)
	}
	assertResponseTrace(t, cached, 0, map[string]int{
		"api.cache_read": 1, "api.cache_validate": 1, "response.body_set": 1,
	})
}

func TestWithCacheTraceFailurePaths(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	closedClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	_ = closedClient.Close()

	tests := []struct {
		name       string
		client     *redis.Client
		seed       string
		fetchError bool
		bypass     bool
		wantStatus int
		wantOps    map[string]int
	}{
		{
			name: "invalid cached JSON", client: client, seed: "[]", wantStatus: 500,
			wantOps: map[string]int{"api.cache_read": 1, "api.cache_validate": 1, "response.json_encode": 1},
		},
		{
			name: "redis failure", client: closedClient, wantStatus: 500,
			wantOps: map[string]int{"api.cache_read": 1, "response.json_encode": 1},
		},
		{
			name: "fetch failure without Redis", fetchError: true, wantStatus: 500,
			wantOps: map[string]int{"api.data_fetch": 1, "response.json_encode": 1},
		},
		{
			name: "fetch bypass", bypass: true, wantStatus: 404,
			wantOps: map[string]int{"api.data_fetch": 1, "response.json_encode": 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetches := 0
			app, snapshots := responseTraceTestApp(fiber.Config{}, func(c fiber.Ctx) error {
				if tt.seed != "" {
					if err := client.Set(c.Context(), cacheKeyFromFiberCtx(c, "failure"), tt.seed, time.Minute).Err(); err != nil {
						return err
					}
				}
				return WithCache(c, tt.client, "failure", func(string) (any, error) {
					fetches++
					if tt.bypass {
						return nil, &CacheBypassError{Response: JSONResponse(c, http.StatusNotFound, "missing")}
					}
					if tt.fetchError {
						return nil, errors.New("fetch failed")
					}
					return "unexpected fetch", nil
				})
			})
			response, _, snapshot := responseTraceTestRequest(t, app, snapshots)
			if response.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, tt.wantStatus)
			}
			if tt.client != nil && fetches != 0 {
				t.Fatalf("fetch ran after cache failure: %d", fetches)
			}
			assertResponseTrace(t, snapshot, 1, tt.wantOps)
		})
	}
}

func TestResponseEncodingTracePreservesErrorsAndTransport(t *testing.T) {
	encodeErr := errors.New("test JSON encoding failure")
	closedClient := redis.NewClient(&redis.Options{})
	_ = closedClient.Close()
	tests := []struct {
		name       string
		cfg        fiber.Config
		handler    fiber.Handler
		wantStatus int
		wantType   string
		wantOps    map[string]int
	}{
		{
			name: "JSON", handler: func(c fiber.Ctx) error { return JSONResponse(c, 201, "created", "data") },
			wantStatus: 201, wantType: fiber.MIMEApplicationJSONCharsetUTF8,
			wantOps: map[string]int{"response.json_encode": 1},
		},
		{
			name: "JSON encoding failure", cfg: fiber.Config{JSONEncoder: func(any) ([]byte, error) { return nil, encodeErr }},
			handler: func(c fiber.Ctx) error {
				err := JSONResponse(c, 201, "created", "data")
				if !errors.Is(err, encodeErr) {
					return c.Status(http.StatusTeapot).SendString("JSONResponse changed encoder error")
				}
				return err
			},
			wantStatus: 500, wantOps: map[string]int{"response.json_encode": 1},
		},
		{
			name: "cached JSON encoding failure", cfg: fiber.Config{JSONEncoder: func(any) ([]byte, error) { return nil, encodeErr }},
			handler: func(c fiber.Ctx) error {
				err := CachedJSONResponse(c.Context(), c, nil, time.Minute, "key", 200, "ok", "data")
				if !errors.Is(err, encodeErr) {
					return c.Status(http.StatusTeapot).SendString("CachedJSONResponse changed encoder error")
				}
				return err
			},
			wantStatus: 500, wantOps: map[string]int{"response.json_encode": 1},
		},
		{
			name: "best-effort cache write failure",
			handler: func(c fiber.Ctx) error {
				return CachedJSONResponse(c.Context(), c, closedClient, time.Minute, "key", 200, "ok", "data")
			},
			wantStatus: 200, wantType: ContentTypeJSON,
			wantOps: map[string]int{"response.json_encode": 1, "api.cache_write": 1, "response.body_set": 1},
		},
		{
			name: "MsgPack", handler: func(c fiber.Ctx) error { return MsgPackResponse(c, 202, "packed", "data") },
			wantStatus: 202, wantType: ContentTypeMsgPack,
			wantOps: map[string]int{"response.msgpack_encode": 1, "response.body_set": 1},
		},
		{
			name: "MsgPack encoding failure", handler: func(c fiber.Ctx) error { return MsgPackResponse(c, 202, "packed", make(chan int)) },
			wantStatus: 500, wantOps: map[string]int{"response.msgpack_encode": 1, "response.body_set": 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, snapshots := responseTraceTestApp(tt.cfg, tt.handler)
			response, _, snapshot := responseTraceTestRequest(t, app, snapshots)
			if response.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, tt.wantStatus)
			}
			if tt.wantType != "" && response.Header.Get(fiber.HeaderContentType) != tt.wantType {
				t.Fatalf("content type = %q, want %q", response.Header.Get(fiber.HeaderContentType), tt.wantType)
			}
			assertResponseTrace(t, snapshot, 1, tt.wantOps)
		})
	}
}

func responseTraceTestApp(cfg fiber.Config, handler fiber.Handler) (*fiber.App, <-chan commandtrace.Snapshot) {
	snapshots := make(chan commandtrace.Snapshot, 1)
	app := fiber.New(cfg)
	app.Use(func(c fiber.Ctx) error {
		ctx, trace := commandtrace.WithTrace(c.Context())
		c.SetContext(ctx)
		err := c.Next()
		snapshots <- trace.Snapshot()
		return err
	})
	app.Get("/trace/:id", handler)
	return app, snapshots
}

func responseTraceTestRequest(t *testing.T, app *fiber.App, snapshots <-chan commandtrace.Snapshot) (*http.Response, []byte, commandtrace.Snapshot) {
	t.Helper()
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/trace/private-id?secret=value", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return response, body, <-snapshots
}

func assertResponseTrace(t *testing.T, snapshot commandtrace.Snapshot, phaseCount int, wantOps map[string]int) {
	t.Helper()
	if phaseCount == 0 {
		if len(snapshot.Phases) != 0 {
			t.Fatalf("unexpected phases: %+v", snapshot.Phases)
		}
	} else if len(snapshot.Phases) != 1 || snapshot.Phases[0].Name != "response_encode" || snapshot.Phases[0].Count != phaseCount {
		t.Fatalf("phases = %+v, want response_encode count %d", snapshot.Phases, phaseCount)
	}
	gotOps := make(map[string]int, len(snapshot.Operations))
	for _, op := range snapshot.Operations {
		gotOps[op.Name] = op.Count
	}
	if !reflect.DeepEqual(gotOps, wantOps) {
		t.Fatalf("operations = %v, want %v", gotOps, wantOps)
	}
}
