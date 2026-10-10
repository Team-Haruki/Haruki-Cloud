package realtime

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/config"
	pjskdb "haruki-cloud/database/pjsk"
	harukiLogger "haruki-cloud/utils/logger"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gofiber/fiber/v3"
	_ "github.com/mattn/go-sqlite3"
)

const testIngestToken = "ingest-secret"

func testIngestHash() string {
	sum := sha256.Sum256([]byte(testIngestToken))
	return hex.EncodeToString(sum[:])
}

// openTestDB returns a migrated in-memory PJSK database behind a single
// connection, so concurrent stream, ingest and sweep calls serialise.
func openTestDB(t *testing.T) *pjskdb.Client {
	t.Helper()
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:realtime_%d?mode=memory&_fk=1", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	client := pjskdb.NewClient(pjskdb.Driver(entsql.OpenDB(dialect.SQLite, db)))
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

type fakeClock struct {
	offset atomic.Int64
}

func (c *fakeClock) Now() time.Time {
	return time.Now().Add(time.Duration(c.offset.Load()))
}

func (c *fakeClock) Advance(d time.Duration) {
	c.offset.Add(int64(d))
}

// logBuffer is a concurrency-safe log sink for assertions.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type testEnv struct {
	t        *testing.T
	db       *pjskdb.Client
	svc      *Service
	app      *fiber.App
	base     string
	clock    *fakeClock
	logs     *logBuffer
	readOnly atomic.Bool
}

func testConfig() config.EventsConfig {
	return config.EventsConfig{
		IngestTokenSHA256: testIngestHash(),
		HeartbeatInterval: time.Hour,
		ClientRetry:       1500 * time.Millisecond,
		SweepInterval:     time.Hour,
		GCInterval:        time.Hour,
	}
}

func newTestEnv(t *testing.T, cfg config.EventsConfig) *testEnv {
	t.Helper()
	return newTestEnvWithDB(t, cfg, openTestDB(t))
}

func newTestEnvWithDB(t *testing.T, cfg config.EventsConfig, db *pjskdb.Client) *testEnv {
	t.Helper()
	env := &testEnv{t: t, db: db, clock: &fakeClock{}, logs: &logBuffer{}}
	env.svc = NewService(db, cfg, Options{
		Logger:   harukiLogger.NewLogger("Events", "DEBUG", env.logs),
		ReadOnly: env.readOnly.Load,
		Now:      env.clock.Now,
	})
	env.app = fiber.New()
	env.svc.Register(env.app, func(c fiber.Ctx) error {
		if c.Get(fiber.HeaderAuthorization) != "Bearer internal" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		return c.Next()
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = env.app.Listener(ln, fiber.ListenConfig{DisableStartupMessage: true}) }()
	env.base = "http://" + ln.Addr().String()
	t.Cleanup(func() {
		env.svc.Shutdown()
		_ = env.app.ShutdownWithTimeout(5 * time.Second)
	})
	return env
}

type subscriptionSpec struct {
	version   string
	active    bool
	expiresIn time.Duration
}

func (env *testEnv) createSubscription(spec subscriptionSpec) *pjskdb.MysekaiBirthdaySubscription {
	env.t.Helper()
	if spec.version == "" {
		spec.version = "v1"
	}
	if spec.expiresIn == 0 {
		spec.expiresIn = time.Hour
	}
	sub, err := env.db.MysekaiBirthdaySubscription.Create().
		SetRegion("jp").
		SetUID(fmt.Sprint(time.Now().UnixNano())).
		SetPlatform("qq").
		SetPlatformUserID("user-1").
		SetPlatformGroupID("group-1").
		SetCloudBotID("cloud-1").
		SetSelfID("self-1").
		SetMaterials([]string{"diamond"}).
		SetToken(spec.version + ".secret").
		SetActive(spec.active).
		SetExpiresAt(time.Now().Add(spec.expiresIn)).
		Save(context.Background())
	if err != nil {
		env.t.Fatalf("create subscription: %v", err)
	}
	return sub
}

func (env *testEnv) activeSubscription() *pjskdb.MysekaiBirthdaySubscription {
	return env.createSubscription(subscriptionSpec{active: true})
}

func (env *testEnv) post(path, authorization, body string) (int, string) {
	env.t.Helper()
	req, err := http.NewRequest(http.MethodPost, env.base+path, strings.NewReader(body))
	if err != nil {
		env.t.Fatalf("request: %v", err)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		env.t.Fatalf("post %s: %v", path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func (env *testEnv) ingest(sub *pjskdb.MysekaiBirthdaySubscription, version, eventID, payloadRef string) int {
	env.t.Helper()
	status, _ := env.post(RouteIngest, "Bearer "+testIngestToken, fmt.Sprintf(
		`{"event_id":%q,"subscription_id":"%d","subscription_version":%q,"payload_ref":%q,"empty_result":false}`,
		eventID, sub.ID, version, payloadRef))
	return status
}

func (env *testEnv) get(path string) (int, string) {
	env.t.Helper()
	resp, err := http.Get(env.base + path)
	if err != nil {
		env.t.Fatalf("get %s: %v", path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func streamPath(sub *pjskdb.MysekaiBirthdaySubscription, version string) string {
	return fmt.Sprintf("%s?subscription_id=%d&subscription_version=%s&token=%s.secret", RouteSSE, sub.ID, version, version)
}

// frame is one parsed SSE frame.
type frame struct {
	id      string
	event   string
	data    string
	retry   string
	comment string
}

type sseClient struct {
	t      *testing.T
	resp   *http.Response
	frames chan frame
	done   chan struct{}
	cancel context.CancelFunc
}

func (env *testEnv) connect(path string, headers map[string]string) *sseClient {
	env.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.base+path, nil)
	if err != nil {
		cancel()
		env.t.Fatalf("request: %v", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		env.t.Fatalf("connect: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		_ = resp.Body.Close()
		env.t.Fatalf("connect status = %d", resp.StatusCode)
	}
	client := &sseClient{t: env.t, resp: resp, frames: make(chan frame, 64), done: make(chan struct{}), cancel: cancel}
	go client.read()
	env.t.Cleanup(client.close)
	return client
}

func (c *sseClient) read() {
	defer close(c.done)
	reader := bufio.NewReader(c.resp.Body)
	var current frame
	started := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if started {
				c.frames <- current
			}
			current = frame{}
			started = false
			continue
		}
		started = true
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "":
			current.comment = value
		case "id":
			current.id = value
		case "event":
			current.event = value
		case "data":
			current.data = value
		case "retry":
			current.retry = value
		}
	}
}

func (c *sseClient) close() {
	c.cancel()
	_ = c.resp.Body.Close()
}

// next returns the next frame that is not a heartbeat or the initial retry.
func (c *sseClient) next(timeout time.Duration) (frame, bool) {
	c.t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case f := <-c.frames:
			if f.event == "" && f.data == "" {
				continue
			}
			return f, true
		case <-deadline:
			return frame{}, false
		}
	}
}

func (c *sseClient) mustNext() frame {
	c.t.Helper()
	f, ok := c.next(3 * time.Second)
	if !ok {
		c.t.Fatal("timed out waiting for an SSE event")
	}
	return f
}

func (c *sseClient) expectNone(wait time.Duration) {
	c.t.Helper()
	if f, ok := c.next(wait); ok {
		c.t.Fatalf("unexpected SSE event %+v", f)
	}
}

// rawFrame returns the next frame of any kind, the initial retry included.
func (c *sseClient) rawFrame(timeout time.Duration) (frame, bool) {
	select {
	case f := <-c.frames:
		return f, true
	case <-time.After(timeout):
		return frame{}, false
	}
}

func (c *sseClient) waitClosed(timeout time.Duration) bool {
	select {
	case <-c.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
