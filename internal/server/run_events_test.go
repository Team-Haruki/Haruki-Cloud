package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"haruki-cloud/api"
	harukiConfig "haruki-cloud/config"
	pjskDB "haruki-cloud/database/pjsk"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/realtime"
	harukiLogger "haruki-cloud/utils/logger"

	"github.com/gofiber/fiber/v3"
	_ "github.com/mattn/go-sqlite3"
)

func TestResolveRole(t *testing.T) {
	cases := []struct {
		args []string
		env  string
		want string
	}{
		{nil, "", RoleAPI},
		{[]string{"events"}, "", RoleEvents},
		{[]string{" EVENTS "}, "", RoleEvents},
		{[]string{"api"}, "events", RoleAPI},
		{[]string{"-unknown"}, " events ", RoleEvents},
		{nil, "api", RoleAPI},
	}
	for _, tc := range cases {
		if got := ResolveRole(tc.args, tc.env); got != tc.want {
			t.Fatalf("ResolveRole(%v, %q) = %q, want %q", tc.args, tc.env, got, tc.want)
		}
	}
}

func TestEventStreamCloserSelection(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{}
	if closer := eventStreamCloser(nil); closer != nil {
		t.Fatalf("no events role configured = %#v", closer)
	}
	harukiConfig.Cfg.Events.InternalBaseURL = "http://events.internal:7911/"
	harukiConfig.Cfg.HarukiBotDB.InternalAPIToken = "shared"
	closer, ok := eventStreamCloser(nil).(*realtime.HTTPCloser)
	if !ok || closer.BaseURL != "http://events.internal:7911" || closer.Authorization != "Bearer shared" || closer.UserAgent != "Haruki-Cloud" {
		t.Fatalf("http closer = %#v", closer)
	}
	embedded := realtime.NewService(nil, harukiConfig.EventsConfig{}, realtime.Options{Logger: startupTestLogger(io.Discard)})
	if got := eventStreamCloser(embedded); got != embedded {
		t.Fatal("embedded service should close in process")
	}

	harukiConfig.Cfg.HMES.UserAgent = "Haruki-Cloud-HMES"
	if got := internalCallerUserAgent(); got != "Haruki-Cloud-HMES" {
		t.Fatalf("user agent = %q", got)
	}
	harukiConfig.Cfg.Backend.AcceptUserAgent = "Haruki-Internal"
	if got := internalCallerUserAgent(); got != "Haruki-Internal" {
		t.Fatalf("user agent = %q", got)
	}

	runtime := &renderapp.App{}
	wireEventStreams(runtime, embedded)
	wireEventStreams(nil, embedded)
	if runtime.EventStreams != embedded {
		t.Fatal("event streams not wired into the render runtime")
	}
}

func TestEventsListenAddrAndApp(t *testing.T) {
	if got := eventsListenAddr(harukiConfig.EventsConfig{}.WithDefaults()); got != "0.0.0.0:7911" {
		t.Fatalf("listen addr = %q", got)
	}
	if got := eventsListenAddr(harukiConfig.EventsConfig{Host: "::1", Port: 9}); got != "[::1]:9" {
		t.Fatalf("ipv6 listen addr = %q", got)
	}
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{Profile: harukiConfig.ProfileDev}
	var logs bytes.Buffer
	app := createEventsApp(startupTestLogger(&logs))
	app.Get("/panic", func(fiber.Ctx) error { panic("events panic") })
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/panic", nil))
	if err != nil || resp.StatusCode != http.StatusInternalServerError || !strings.Contains(logs.String(), "request panic recovered") {
		t.Fatalf("panic response = %v, %v; logs %s", resp, err, logs.String())
	}
	harukiConfig.Cfg.Profile = harukiConfig.ProfileProduction
	if _, err := app.Test(httptest.NewRequest(http.MethodGet, "/panic", nil)); err != nil {
		t.Fatalf("production panic: %v", err)
	}
	big := httptest.NewRequest(http.MethodPost, "/internal/events", strings.NewReader(strings.Repeat("x", eventsBodyLimit+1)))
	// fasthttp refuses the body before routing; app.Test reports that as an error.
	if resp, err := app.Test(big); err == nil && resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body accepted: %d", resp.StatusCode)
	}
}

func ingestHashFor(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// TestEmbeddedEventsRoutesPrecedeInternalGroup checks that the ingest route
// on the main app answers to the ingest token although a later /internal
// group demands the shared token.
func TestEmbeddedEventsRoutesPrecedeInternalGroup(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{}
	harukiConfig.Cfg.HarukiBotDB.InternalAPIToken = "shared"
	harukiConfig.Cfg.Events.IngestTokenSHA256 = ingestHashFor("ingest")
	var logs bytes.Buffer
	logger := startupTestLogger(&logs)
	app := fiber.New()

	if initEmbeddedEvents(context.Background(), logger, app, nil) != nil {
		t.Fatal("embedded events started while disabled")
	}
	harukiConfig.Cfg.Events.Embedded = true
	if initEmbeddedEvents(context.Background(), logger, app, nil) != nil || !strings.Contains(logs.String(), "realtime_embedded_disabled") {
		t.Fatal("embedded events without a database")
	}

	client := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:embedded_events_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := initEmbeddedEvents(ctx, logger, app, client)
	if service == nil {
		t.Fatal("embedded events not started")
	}
	app.Group("/internal", api.VerifyAPIAuthorization()).Get("/other", func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })

	request := func(method, path, auth, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return resp.StatusCode
	}
	// 409: authorized by the ingest token, rejected only for the unknown subscription.
	if status := request(http.MethodPost, realtime.RouteIngest, "Bearer ingest", `{"event_id":"e","subscription_id":"1","subscription_version":"v1"}`); status != http.StatusConflict {
		t.Fatalf("embedded ingest = %d, want 409", status)
	}
	if status := request(http.MethodGet, realtime.RouteHealth, "", ""); status != http.StatusOK {
		t.Fatalf("embedded healthz = %d", status)
	}
	if status := request(http.MethodPost, "/internal/subscriptions/1/close?subscription_version=v1", "", ""); status != http.StatusUnauthorized {
		t.Fatalf("embedded close without token = %d", status)
	}
	if status := request(http.MethodPost, "/internal/subscriptions/1/close?subscription_version=v1", "Bearer shared", ""); status != http.StatusOK {
		t.Fatalf("embedded close = %d", status)
	}
	if status := request(http.MethodGet, "/internal/other", "Bearer ingest", ""); status != http.StatusUnauthorized {
		t.Fatalf("other internal route accepted the ingest token: %d", status)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestRunEventsServesUntilCancelled(t *testing.T) {
	preserveServerConfig(t)
	defer func() {
		harukiLogger.SetGlobalFileWriter(io.Discard)
		harukiLogger.SetCommandWriter(nil)
	}()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pjsk.db")
	dsn := "file:" + dbPath + "?_fk=1"
	migrated := pjskenttest.Open(t, "sqlite3", dsn)
	_ = migrated.Close()

	port := freePort(t)
	configBody := fmt.Sprintf(`profile: dev
backend:
  log_level: INFO
pjsk:
  enabled: true
  db_type: sqlite3
  db_url: %q
events:
  host: 127.0.0.1
  port: %d
  ingest_token_sha256: %q
`, dsn, port, ingestHashFor("ingest"))
	configPath := filepath.Join(dir, "events.yaml")
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARUKI_CONFIG_PATH", configPath)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunEvents(ctx)
		close(done)
	}()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(base + realtime.RouteHealth)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("events role did not come up: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	resp, err := http.Get(base + realtime.RouteSSE + "?subscription_id=1&subscription_version=v1&token=v1.x")
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sse for an unknown subscription = %v, %v", resp, err)
	}
	_ = resp.Body.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("events role did not stop")
	}
}

func TestOpenEventsPJSKClientUsesConfiguredDatabase(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{}
	harukiConfig.Cfg.PJSK.Enabled = true
	harukiConfig.Cfg.PJSK.DBType = "sqlite3"
	harukiConfig.Cfg.PJSK.DBURL = fmt.Sprintf("file:events_open_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	client := openEventsPJSKClient(startupTestLogger(io.Discard))
	defer client.Close()
	var _ *pjskDB.Client = client
	// The events role never migrates: the table does not exist yet.
	if err := realtime.NewStore(client).Ping(context.Background()); err == nil {
		t.Fatal("events role created the schema")
	}
}
