package sekai

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/core/upstreamerr"
	"haruki-cloud/internal/testutil"

	"github.com/go-resty/resty/v2"
)

// sekaiNode is a test Sekai API entry. While broken it closes every
// connection without an HTTP response; otherwise it answers status.
type sekaiNode struct {
	server *httptest.Server
	broken atomic.Bool
	status atomic.Int32
}

func newSekaiNode(t *testing.T, broken bool) *sekaiNode {
	t.Helper()
	node := &sekaiNode{}
	node.broken.Store(broken)
	node.status.Store(http.StatusOK)
	node.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if node.broken.Load() {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(node.status.Load()))
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(node.server.Close)
	return node
}

func (n *sekaiNode) URL() string { return n.server.URL }

// unreachableURL is an address nothing listens on (connection refused).
func unreachableURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return "http://" + addr
}

// attemptCounter counts resty attempts (retries included) per base URL.
type attemptCounter struct {
	mu   sync.Mutex
	hits map[string]int
}

func countAttempts(client *HarukiSekaiAPIClient) *attemptCounter {
	counter := &attemptCounter{hits: map[string]int{}}
	client.http.OnBeforeRequest(func(_ *resty.Client, req *resty.Request) error {
		counter.mu.Lock()
		defer counter.mu.Unlock()
		counter.hits[req.URL]++
		return nil
	})
	return counter
}

func (c *attemptCounter) of(baseURL string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for url, hits := range c.hits {
		if strings.HasPrefix(url, baseURL+"/") {
			total += hits
		}
	}
	return total
}

func TestSekaiAPIBackupGetsNoTrafficWhilePrimaryHealthy(t *testing.T) {
	primaryA := newSekaiNode(t, false)
	primaryB := newSekaiNode(t, false)
	standby := newSekaiNode(t, false)
	cases := []struct {
		name      string
		cfg       config.SekaiAPIConfig
		primaries []string
	}{
		{
			name:      "backup_base_url",
			cfg:       config.SekaiAPIConfig{BaseURL: primaryA.URL(), BackupBaseURL: standby.URL()},
			primaries: []string{primaryA.URL()},
		},
		{
			name: "targets with backup",
			cfg: config.SekaiAPIConfig{Targets: []upstream.TargetConfig{
				{Name: "a", BaseURL: primaryA.URL()},
				{Name: "standby", BaseURL: standby.URL(), Backup: true},
				{Name: "b", BaseURL: primaryB.URL()},
			}},
			primaries: []string{primaryA.URL(), primaryB.URL()},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := NewSekaiAPIClient(&tc.cfg)
			attempts := countAttempts(client)
			for range 6 {
				_, err := client.GetSystem("jp")
				testutil.Require(t, err == nil, "GetSystem: %v", err)
			}
			testutil.Require(t, attempts.of(standby.URL()) == 0, "backup got %d requests while a primary was healthy", attempts.of(standby.URL()))
			for _, url := range tc.primaries {
				testutil.Check(t, attempts.of(url) == 6/len(tc.primaries), "primaries must share idle traffic evenly, %s got %d", url, attempts.of(url))
			}
		})
	}
}

func TestSekaiAPIGetFailsOverOnTransportFailure(t *testing.T) {
	cases := []struct {
		name    string
		primary func(t *testing.T) string
	}{
		{"connection closed without response", func(t *testing.T) string { return newSekaiNode(t, true).URL() }},
		{"connection refused", unreachableURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			primary := tc.primary(t)
			standby := newSekaiNode(t, false)
			// Default retry policy (1 s first backoff): the failover must not
			// wait for same-URL retries.
			client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: primary, BackupBaseURL: standby.URL()})
			attempts := countAttempts(client)

			started := time.Now()
			_, err := client.GetSystem("jp")
			elapsed := time.Since(started)
			testutil.Require(t, err == nil, "GET must succeed on the backup, got %v", err)
			testutil.Check(t, attempts.of(primary) == 1, "primary must be tried once, got %d", attempts.of(primary))
			testutil.Check(t, attempts.of(standby.URL()) == 1, "backup must answer within the same call, got %d", attempts.of(standby.URL()))
			testutil.Check(t, elapsed < 900*time.Millisecond, "failover waited for same-URL retries: %v", elapsed)
			testutil.Check(t, !client.health.Healthy("sekai-api-1"), "primary must be marked down")

			_, err = client.WithContext(t.Context()).GetSystem("jp")
			testutil.Require(t, err == nil, "second GET: %v", err)
			testutil.Check(t, attempts.of(primary) == 1, "a down primary must be skipped, attempts = %d", attempts.of(primary))
			testutil.Check(t, attempts.of(standby.URL()) == 2, "second call must go straight to the backup, got %d", attempts.of(standby.URL()))
		})
	}
}

func TestSekaiAPIHTTPErrorsDoNotFailOver(t *testing.T) {
	cases := []struct {
		status int
		check  func(error) bool
	}{
		{http.StatusServiceUnavailable, func(err error) bool { return errors.Is(err, ErrServerMaintenance) }},
		{http.StatusInternalServerError, func(err error) bool {
			apiErr, ok := errors.AsType[*APIError](err)
			return ok && apiErr.StatusCode == http.StatusInternalServerError
		}},
		{http.StatusBadGateway, func(err error) bool { _, ok := errors.AsType[*APIError](err); return ok }},
		{http.StatusNotFound, func(err error) bool { return errors.Is(err, ErrUserNotFound) }},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			primary := newSekaiNode(t, false)
			primary.status.Store(int32(tc.status))
			standby := newSekaiNode(t, false)
			client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: primary.URL(), BackupBaseURL: standby.URL(), Retry: fastRetry})
			attempts := countAttempts(client)

			for range 2 {
				_, err := client.GetSystem("jp")
				testutil.Require(t, tc.check(err), "unexpected error for %d: %v", tc.status, err)
			}
			testutil.Check(t, attempts.of(standby.URL()) == 0, "HTTP %d must not fail over, backup got %d", tc.status, attempts.of(standby.URL()))
			testutil.Check(t, client.health.Healthy("sekai-api-1"), "HTTP %d must not mark the primary down", tc.status)
		})
	}
}

func TestSekaiAPIPostIsNotReplayedButMarksDown(t *testing.T) {
	primary := newSekaiNode(t, true)
	standby := newSekaiNode(t, false)
	client := NewSekaiAPIClient(&config.SekaiAPIConfig{
		Retry: fastRetry,
		Targets: []upstream.TargetConfig{
			{Name: "primary", BaseURL: primary.URL()},
			{Name: "standby", BaseURL: standby.URL(), Backup: true},
		},
	})
	attempts := countAttempts(client)

	_, err := client.EnterMySekaiHousingCompetitionEntry("jp", 1, 2, 3, false)
	_, transport := errors.AsType[*upstreamerr.TransportError](err)
	testutil.Require(t, transport, "POST must fail with a transport error, got %v", err)
	testutil.Check(t, attempts.of(standby.URL()) == 0, "POST must not be replayed on the backup, got %d", attempts.of(standby.URL()))
	testutil.Check(t, !client.health.Healthy("primary"), "a transport failure on POST must mark the target down")

	primaryAttempts := attempts.of(primary.URL())
	_, err = client.GetSystem("jp")
	testutil.Require(t, err == nil, "next GET: %v", err)
	testutil.Check(t, attempts.of(primary.URL()) == primaryAttempts, "next call must skip the down primary")
	testutil.Check(t, attempts.of(standby.URL()) == 1, "next call must go to the backup, got %d", attempts.of(standby.URL()))
}

func TestSekaiAPIAllTargetsDownStillAttempts(t *testing.T) {
	t.Run("down but reachable", func(t *testing.T) {
		primary := newSekaiNode(t, false)
		standby := newSekaiNode(t, false)
		client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: primary.URL(), BackupBaseURL: standby.URL()})
		client.health.MarkDown("sekai-api-1")
		client.health.MarkDown(config.SekaiAPIBackupTargetName)
		attempts := countAttempts(client)

		_, err := client.GetSystem("jp")
		testutil.Require(t, err == nil, "GET with every target down must still be attempted: %v", err)
		testutil.Check(t, attempts.of(primary.URL()) == 1 && attempts.of(standby.URL()) == 0, "the non-backup target goes first, attempts primary=%d backup=%d", attempts.of(primary.URL()), attempts.of(standby.URL()))
		testutil.Check(t, client.health.Healthy("sekai-api-1"), "a success must clear the down state")
	})

	t.Run("both unreachable", func(t *testing.T) {
		primary := newSekaiNode(t, true)
		standby := newSekaiNode(t, true)
		client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: primary.URL(), BackupBaseURL: standby.URL(), Retry: fastRetry})
		attempts := countAttempts(client)

		_, err := client.GetSystem("jp")
		_, transport := errors.AsType[*upstreamerr.TransportError](err)
		testutil.Require(t, transport, "expected a transport error, got %v", err)
		testutil.Check(t, attempts.of(primary.URL()) == 1, "primary is tried once before moving on, got %d", attempts.of(primary.URL()))
		testutil.Check(t, attempts.of(standby.URL()) == 5, "the last target keeps the same-URL retry policy, got %d", attempts.of(standby.URL()))

		_, err = client.GetSystem("jp")
		testutil.Require(t, err != nil, "second call must fail too")
		testutil.Check(t, attempts.of(primary.URL()) == 2, "with every target down the primary is still attempted first, got %d", attempts.of(primary.URL()))
	})
}

func TestSekaiAPITargetRecoversAfterWindow(t *testing.T) {
	primary := newSekaiNode(t, true)
	standby := newSekaiNode(t, false)
	client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: primary.URL(), BackupBaseURL: standby.URL()})
	clock := testutil.NewFakeClock()
	client.health = upstream.NewHealth(sekaiTargetDownWindow, clock.Now, logSekaiTargetHealth)
	attempts := countAttempts(client)

	_, err := client.GetSystem("jp")
	testutil.Require(t, err == nil, "first GET: %v", err)
	primary.broken.Store(false)

	clock.Advance(sekaiTargetDownWindow - time.Second)
	_, err = client.GetSystem("jp")
	testutil.Require(t, err == nil, "GET inside the window: %v", err)
	testutil.Check(t, attempts.of(primary.URL()) == 1, "primary must be skipped inside the window, got %d", attempts.of(primary.URL()))

	clock.Advance(time.Second)
	for range 3 {
		_, err = client.GetSystem("jp")
		testutil.Require(t, err == nil, "GET after the window: %v", err)
	}
	testutil.Check(t, attempts.of(primary.URL()) == 4, "primary must take traffic again after the window, got %d", attempts.of(primary.URL()))
	testutil.Check(t, attempts.of(standby.URL()) == 2, "backup must stop getting traffic once the primary is back, got %d", attempts.of(standby.URL()))
}

func TestSekaiAPISingleTargetBehaviourUnchanged(t *testing.T) {
	cases := []struct {
		name string
		cfg  func(url string) config.SekaiAPIConfig
	}{
		{"base_url", func(url string) config.SekaiAPIConfig { return config.SekaiAPIConfig{BaseURL: url} }},
		{"one target", func(url string) config.SekaiAPIConfig {
			return config.SekaiAPIConfig{Targets: []upstream.TargetConfig{{Name: "only", BaseURL: url}}}
		}},
		{"targets win over backup_base_url", func(url string) config.SekaiAPIConfig {
			return config.SekaiAPIConfig{BackupBaseURL: "http://standby.invalid", Targets: []upstream.TargetConfig{{BaseURL: url}}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := unreachableURL(t)
			cfg := tc.cfg(url)
			cfg.Retry = fastRetry
			client := NewSekaiAPIClient(&cfg)
			testutil.Require(t, client.pool.Size() == 1 && client.health == nil, "single target must not track health (size %d)", client.pool.Size())
			attempts := countAttempts(client)

			for call := 1; call <= 2; call++ {
				_, err := client.GetSystem("jp")
				_, transport := errors.AsType[*upstreamerr.TransportError](err)
				testutil.Require(t, transport, "expected a transport error, got %v", err)
				testutil.Check(t, attempts.of(url) == 5*call, "single target keeps all same-URL retries, got %d after %d calls", attempts.of(url), call)
			}
		})
	}
}

func TestSekaiAPICallerCancellationDoesNotMarkDown(t *testing.T) {
	primary := newSekaiNode(t, false)
	standby := newSekaiNode(t, false)
	client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: primary.URL(), BackupBaseURL: standby.URL()})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := client.WithContext(ctx).GetSystem("jp")
	testutil.Require(t, errors.Is(err, context.Canceled), "expected the caller's cancellation, got %v", err)
	testutil.Check(t, client.health.Healthy("sekai-api-1"), "a cancelled caller must not mark the target down")
}

func TestSekaiAPISlowTargetIsNotMarkedDown(t *testing.T) {
	var primaryHits atomic.Int32
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(slow.Close)
	standby := newSekaiNode(t, false)
	client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: slow.URL, BackupBaseURL: standby.URL(), Retry: fastRetry})
	client.http.SetTimeout(50 * time.Millisecond)
	attempts := countAttempts(client)

	_, err := client.GetSystem("jp")
	_, transport := errors.AsType[*upstreamerr.TransportError](err)
	testutil.Require(t, transport, "expected a timeout, got %v", err)
	testutil.Check(t, attempts.of(slow.URL) == 5, "a response timeout keeps the same-URL retries, got %d attempts", attempts.of(slow.URL))
	testutil.Check(t, attempts.of(standby.URL()) == 0, "a slow primary must not push traffic to the backup, got %d", attempts.of(standby.URL()))
	testutil.Check(t, client.health.Healthy("sekai-api-1"), "a response timeout must not mark the target down")

	_, _ = client.GetSystem("jp")
	testutil.Check(t, attempts.of(standby.URL()) == 0, "the next call must still go to the primary, backup got %d", attempts.of(standby.URL()))
}

func TestSekaiAPIConnectTimeoutFailsOver(t *testing.T) {
	standby := newSekaiNode(t, false)
	const darkHost = "dark.sekai.invalid:80"
	client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: "http://" + darkHost, BackupBaseURL: standby.URL()})
	transport := upstream.NewTunedTransport(upstream.TunedTransportConfig{DialTimeout: sekaiDialTimeout})
	dial := transport.DialContext
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == darkHost {
			return nil, &net.OpError{Op: "dial", Net: network, Err: timeoutError{}}
		}
		return dial(ctx, network, addr)
	}
	client.http.SetTransport(transport)
	attempts := countAttempts(client)

	started := time.Now()
	_, err := client.GetSystem("jp")
	testutil.Require(t, err == nil, "GET must fail over to the backup, got %v", err)
	testutil.Check(t, time.Since(started) < 900*time.Millisecond, "connect timeout must not wait for same-URL retries")
	testutil.Check(t, attempts.of("http://"+darkHost) == 1 && attempts.of(standby.URL()) == 1, "dark=%d backup=%d", attempts.of("http://"+darkHost), attempts.of(standby.URL()))
	testutil.Check(t, !client.health.Healthy("sekai-api-1"), "a connect timeout must mark the target down")
}

func TestIsNoRouteFailure(t *testing.T) {
	answered := &resty.Response{RawResponse: &http.Response{StatusCode: http.StatusOK}}
	cases := []struct {
		name string
		resp *resty.Response
		err  error
		want bool
	}{
		{"no error", nil, nil, false},
		{"connect timeout", nil, &url.Error{Op: "Get", Err: &net.OpError{Op: "dial", Err: timeoutError{}}}, true},
		{"refused", nil, &url.Error{Op: "Get", Err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}}, true},
		{"unknown host", nil, &net.OpError{Op: "dial", Err: &net.DNSError{Err: "no such host", IsNotFound: true}}, true},
		{"reset before response", nil, &url.Error{Op: "Get", Err: &net.OpError{Op: "read", Err: syscall.ECONNRESET}}, true},
		{"EOF before response", nil, &url.Error{Op: "Get", Err: io.EOF}, true},
		{"response timeout", nil, &url.Error{Op: "Get", Err: &net.OpError{Op: "read", Err: timeoutError{}}}, false},
		{"client timeout", nil, &url.Error{Op: "Get", Err: context.DeadlineExceeded}, false},
		{"caller cancelled", nil, context.Canceled, false},
		{"body cut after headers", answered, io.ErrUnexpectedEOF, false},
		{"tls failure", nil, errors.New("tls: bad certificate"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Check(t, isNoRouteFailure(tc.resp, tc.err) == tc.want, "isNoRouteFailure(%v) = %v", tc.err, !tc.want)
		})
	}
}

func TestSekaiAPIQueuedRequestSkipsPrimaryMarkedDownWhileWaiting(t *testing.T) {
	inFlight := make(chan struct{})
	fail := make(chan struct{})
	var once sync.Once
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		once.Do(func() { close(inFlight) })
		<-fail
		if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(primary.Close)
	standby := newSekaiNode(t, false)
	client := NewSekaiAPIClient(&config.SekaiAPIConfig{Targets: []upstream.TargetConfig{
		{Name: "primary", BaseURL: primary.URL, Concurrency: 1},
		{Name: "standby", BaseURL: standby.URL(), Backup: true},
	}})
	attempts := countAttempts(client)

	firstErr := make(chan error, 1)
	go func() { _, err := client.GetSystem("jp"); firstErr <- err }()
	<-inFlight

	type result struct {
		body []byte
		err  error
	}
	second := make(chan result, 1)
	go func() {
		body, err := client.EnterMySekaiHousingCompetitionEntry("jp", 1, 2, 3, false)
		second <- result{body, err}
	}()
	// Give the POST time to queue behind the primary's only slot. If it is
	// slower than this it sees the primary down at pick time, which must end
	// the same way.
	time.Sleep(50 * time.Millisecond)
	close(fail)

	testutil.Require(t, <-firstErr == nil, "first GET must fail over to the backup")
	got := <-second
	testutil.Require(t, got.err == nil, "queued POST must go to the backup, got %v", got.err)
	testutil.Check(t, attempts.of(primary.URL) == 1, "the queued request must not dial the down primary, primary attempts = %d", attempts.of(primary.URL))
	testutil.Check(t, attempts.of(standby.URL()) == 2, "backup must serve the failed-over GET and the queued POST, got %d", attempts.of(standby.URL()))
}

func TestSekaiAPIDuplicateTargetNamesStillFailOver(t *testing.T) {
	primary := newSekaiNode(t, true)
	standby := newSekaiNode(t, false)
	client := NewSekaiAPIClient(&config.SekaiAPIConfig{Targets: []upstream.TargetConfig{
		{Name: "entry", BaseURL: primary.URL()},
		{Name: "entry", BaseURL: standby.URL(), Backup: true},
	}})

	_, err := client.GetSystem("jp")
	testutil.Require(t, err == nil, "GET must fail over to the same-named backup, got %v", err)
	testutil.Check(t, !client.health.Healthy("entry") && client.health.Healthy("entry#2"), "health must be tracked per renamed target")
}
