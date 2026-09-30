package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
)

func TestCallerDeadlineReservesFailoverTime(t *testing.T) {
	first, second := newCountingServer(t), newCountingServer(t)
	first.delay.Store(int64(time.Second))
	cfg := testConfig(first.URL, second.URL)
	cfg.RequestTimeout = 10 * time.Second
	c := mustClient(t, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	data, err := c.Get(ctx, "key")
	if err != nil || string(data) != second.URL {
		t.Fatalf("Get=%q,%v", data, err)
	}
	if first.calls.Load() != 1 || second.calls.Load() != 1 || c.endpoints[0].consecutiveFail.Load() != 1 {
		t.Fatalf("calls=%d/%d failed=%d", first.calls.Load(), second.calls.Load(), c.endpoints[0].consecutiveFail.Load())
	}
}

func TestUserCancellationDoesNotPenalizeEndpoint(t *testing.T) {
	started := make(chan struct{})
	server := serve(t, func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() })
	c := mustClient(t, testConfig(server.URL))
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := c.Get(ctx, "key"); result <- err }()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	if c.endpoints[0].consecutiveFail.Load() != 0 {
		t.Fatal("caller cancellation poisoned endpoint health")
	}
}

func TestQueuedStorageCancellationSendsNoRequest(t *testing.T) {
	server := newCountingServer(t)
	runtime := storage.NewIORuntime(storage.IOConfig{MaxConcurrent: 1, MaxPerOrigin: 1})
	cfg := testConfig(server.URL)
	cfg.Runtime = runtime
	c := mustClient(t, cfg)
	release, err := runtime.Acquire(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.Get(ctx, "key"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v", err)
	}
	if server.calls.Load() != 0 || c.endpoints[0].consecutiveFail.Load() != 0 {
		t.Fatal("queued request sent or endpoint penalized")
	}
}

func Test429FailsOverAndHonorsRetryAfterOnLaterOperations(t *testing.T) {
	var calls atomic.Int32
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "300")
		xmlError(w, http.StatusTooManyRequests, "SlowDown")
	})
	second := newCountingServer(t)
	cfg := testConfig(server.URL, second.URL)
	cfg.EndpointPolicy = EndpointOrderedFailover
	c := mustClient(t, cfg)
	for range 2 {
		if got := getFrom(t, c); got != second.URL {
			t.Fatalf("served by %s", got)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("throttled endpoint received %d requests", calls.Load())
	}
	// Even when no healthy alternative exists, a subsequent operation must
	// not resend before Retry-After or sleep for five minutes.
	cfg = testConfig(server.URL)
	cfg.MaxAttempts = 2
	c = mustClient(t, cfg)
	before := calls.Load()
	if _, err := c.Get(context.Background(), "key"); err == nil {
		t.Fatal("expected throttling error")
	}
	if _, err := c.Get(context.Background(), "key"); err == nil {
		t.Fatal("expected deferred retry error")
	}
	if calls.Load() != before+1 {
		t.Fatalf("Retry-After ignored: calls=%d want %d", calls.Load(), before+1)
	}
}

func TestRetryWaitIsCancelable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := waitRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("retry wait ignored cancellation")
	}
}

func TestListRejectsRepeatedAndCyclicTokens(t *testing.T) {
	for _, dir := range []bool{false, true} {
		for _, cycle := range []bool{false, true} {
			t.Run(fmt.Sprintf("dir_%v_cycle_%v", dir, cycle), func(t *testing.T) {
				var calls atomic.Int32
				server := serve(t, func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					token := "A"
					if cycle && r.URL.Query().Get("continuation-token") == "A" {
						token = "B"
					}
					fmt.Fprintf(w, "<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>%s</NextContinuationToken></ListBucketResult>", token)
				})
				c := mustClient(t, testConfig(server.URL))
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				var err error
				if dir {
					err = c.ListDir(ctx, "p", func(storage.DirEntry) error { return nil })
				} else {
					err = c.List(ctx, "p", func(storage.Object) error { return nil })
				}
				if err == nil || !strings.Contains(err.Error(), "repeated continuation token") {
					t.Fatalf("error=%v", err)
				}
				want := int32(2)
				if cycle {
					want = 3
				}
				if calls.Load() != want {
					t.Fatalf("calls=%d want %d", calls.Load(), want)
				}
			})
		}
	}
}

func TestListPageLimit(t *testing.T) {
	var calls atomic.Int32
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		page := calls.Add(1)
		fmt.Fprintf(w, "<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>%d</NextContinuationToken></ListBucketResult>", page)
	})
	cfg := testConfig(server.URL)
	cfg.MaxListPages = 2
	c := mustClient(t, cfg)
	if err := c.List(context.Background(), "", func(storage.Object) error { return nil }); err == nil || !strings.Contains(err.Error(), "page limit") {
		t.Fatalf("error=%v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestListMetricsDistinguishOperationPagesAndAttempts(t *testing.T) {
	response := func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("continuation-token")
		if token == "two" {
			io.WriteString(w, "<ListBucketResult/>")
			return
		}
		next := "one"
		if token == "one" {
			next = "two"
		}
		fmt.Fprintf(w, "<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>%s</NextContinuationToken></ListBucketResult>", next)
	}
	first := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("continuation-token") == "one" {
			xmlError(w, 503, "Unavailable")
			return
		}
		response(w, r)
	})
	second := serve(t, response)
	cfg := testConfig(first.URL, second.URL)
	cfg.EndpointPolicy = EndpointOrderedFailover
	c := mustClient(t, cfg)
	ctx, trace := commandtrace.WithTrace(context.Background())
	if err := c.List(ctx, "secret-user-path", func(storage.Object) error { return nil }); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, metric := range trace.Snapshot().Operations {
		counts[metric.Name] = metric.Count
	}
	for name, want := range map[string]int{"storage.list.operation": 1, "storage.list.page": 3, "storage.list.attempt": 4, "storage.list.retry": 1, "storage.list.status_2xx": 3, "storage.list.status_5xx": 1} {
		if counts[name] != want {
			t.Errorf("%s count=%d want %d", name, counts[name], want)
		}
	}
	if counts["storage.list.bytes_received.total"] == 0 {
		t.Error("missing received byte count")
	}
	for _, metric := range c.cfg.Runtime.Stats() {
		if strings.Contains(metric.Name, "secret") || strings.Contains(metric.Name, first.URL) {
			t.Errorf("sensitive metric label=%s", metric.Name)
		}
	}
}

func TestBuildSetSharesAdmissionAndConnectionPools(t *testing.T) {
	var active, peak, background, backgroundPeak atomic.Int32
	updatePeak := func(peak *atomic.Int32, current int32) {
		for {
			old := peak.Load()
			if old >= current || peak.CompareAndSwap(old, current) {
				return
			}
		}
	}
	handler := func(w http.ResponseWriter, r *http.Request) {
		now := active.Add(1)
		updatePeak(&peak, now)
		defer active.Add(-1)
		bg := strings.Contains(r.URL.Path, "/bg/")
		if bg {
			now := background.Add(1)
			updatePeak(&backgroundPeak, now)
			defer background.Add(-1)
		}
		time.Sleep(5 * time.Millisecond)
		io.WriteString(w, "data")
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()
	provider := storage.ProviderConfig{Scheme: "s3", Bucket: "bucket", Endpoint: server.URL}
	set, err := storage.BuildSet(storage.SetConfig{IO: storage.IOConfig{MaxConcurrent: 3, MaxPerOrigin: 2, MaxBackground: 1}, Assets: provider, Cache: provider, ImageCache: provider}, storage.LegacyRoots{}, storage.Backends{S3: Open}, nil)
	if err != nil {
		t.Fatal(err)
	}
	stores := []storage.Store{set.Assets, set.Cache, set.ImageCache}
	if set.Assets.(*client).transport != set.Cache.(*client).transport {
		t.Fatal("same options did not share connections")
	}
	var wg sync.WaitGroup
	for i := range 48 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			key := storage.Key(fmt.Sprintf("fg/%d", i))
			if i%3 == 0 {
				ctx = storage.WithBackgroundIO(ctx)
				key = storage.Key(fmt.Sprintf("bg/%d", i))
			}
			if _, err := stores[i%len(stores)].Get(ctx, key); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if peak.Load() > 2 || peak.Load() < 2 {
		t.Errorf("shared origin peak=%d want 2", peak.Load())
	}
	if backgroundPeak.Load() != 1 {
		t.Errorf("background peak=%d want 1", backgroundPeak.Load())
	}
	if len(set.Runtime.Stats()) == 0 {
		t.Fatal("missing process metrics")
	}
}

func TestOrderedEndpointPolicyKeepsPrimary(t *testing.T) {
	first, second := newCountingServer(t), newCountingServer(t)
	cfg := testConfig(first.URL, second.URL)
	cfg.EndpointPolicy = EndpointOrderedFailover
	c := mustClient(t, cfg)
	for range 5 {
		if got := getFrom(t, c); got != first.URL {
			t.Fatalf("served by %s", got)
		}
	}
	if second.calls.Load() != 0 {
		t.Fatal("ordered policy rotated endpoints")
	}
}

func TestBuildSetGlobalLimitAcrossDifferentOrigins(t *testing.T) {
	var active, peak atomic.Int32
	handler := func(w http.ResponseWriter, r *http.Request) {
		now := active.Add(1)
		for {
			old := peak.Load()
			if old >= now || peak.CompareAndSwap(old, now) {
				break
			}
		}
		defer active.Add(-1)
		time.Sleep(5 * time.Millisecond)
		io.WriteString(w, "data")
	}
	first, second := serve(t, handler), serve(t, handler)
	set, err := storage.BuildSet(storage.SetConfig{
		IO:     storage.IOConfig{MaxConcurrent: 3, MaxPerOrigin: 3},
		Assets: storage.ProviderConfig{Scheme: "s3", Bucket: "a", Endpoint: first.URL},
		Cache:  storage.ProviderConfig{Scheme: "s3", Bucket: "b", Endpoint: second.URL},
	}, storage.LegacyRoots{}, storage.Backends{S3: Open}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store := set.Assets
			if i%2 == 0 {
				store = set.Cache
			}
			if _, err := store.Get(context.Background(), "key"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if peak.Load() != 3 {
		t.Fatalf("peak across origins=%d want 3", peak.Load())
	}
}

func BenchmarkS3MetricsAndAdmission(b *testing.B) {
	c, err := newClient(Config{Endpoints: []string{"http://storage.test"}, Bucket: "b", PathStyle: true, Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data")), Header: make(http.Header), ContentLength: 4, Request: req}, nil
	})})
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.Get(ctx, "key"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestEndpointOriginCanonicalization(t *testing.T) {
	for _, pair := range [][2]string{{"http://EXAMPLE.test:80/path", "http://example.test"}, {"https://EXAMPLE.test:443", "https://example.test"}, {"http://[::1]:80/a", "http://[::1]/b"}} {
		endpoints, err := parseEndpoints(pair[:])
		if err != nil {
			t.Fatal(err)
		}
		if endpoints[0].origin != endpoints[1].origin {
			t.Fatalf("equivalent origins differ: %q/%q", endpoints[0].origin, endpoints[1].origin)
		}
	}
}

func TestResolvedStorageRuntimeAndPolicyOptions(t *testing.T) {
	runtime := storage.NewIORuntime(storage.IOConfig{})
	resolved, err := storage.Resolve(storage.ProviderConfig{Scheme: "s3", Endpoint: "http://example.test", Bucket: "b", Options: map[string]string{storage.OptionEndpointPolicy: EndpointOrderedFailover, storage.OptionMaxListPages: "7"}})
	if err != nil {
		t.Fatal(err)
	}
	resolved.Runtime = runtime
	resolved.Slot = storage.SlotAssets
	cfg, err := ConfigFromResolved(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runtime != runtime || cfg.Slot != storage.SlotAssets || cfg.EndpointPolicy != EndpointOrderedFailover || cfg.MaxListPages != 7 {
		t.Fatal("runtime or typed options not propagated")
	}
	cfg.EndpointPolicy = "invalid"
	if _, err := newClient(cfg); err == nil {
		t.Fatal("invalid policy accepted")
	}
	resolved.Options[storage.OptionMaxListPages] = "-1"
	if _, err := ConfigFromResolved(resolved); err == nil {
		t.Fatal("negative page limit accepted")
	}
}

func TestShortCallerDeadlineLeavesTimeAfterRetryBackoff(t *testing.T) {
	var attempts atomic.Int32
	cfg := testConfig("http://first.test", "http://second.test")
	cfg.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts.Add(1)
		if req.URL.Host == "first.test" {
			<-req.Context().Done()
			return nil, req.Context().Err()
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: 2, Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
	})
	c := mustClient(t, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	data, err := c.Get(ctx, "key")
	if err != nil || string(data) != "ok" || attempts.Load() != 2 {
		t.Fatalf("Get=%q,%v attempts=%d", data, err, attempts.Load())
	}
}

type interruptedErrorBody struct{}

func (interruptedErrorBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (interruptedErrorBody) Close() error             { return nil }

func TestBroken404ErrorBodyDoesNotBecomeMissing(t *testing.T) {
	for _, body := range []func() io.ReadCloser{
		func() io.ReadCloser { return interruptedErrorBody{} },
		func() io.ReadCloser { return io.NopCloser(strings.NewReader("<Error><Code>NoSuchBucket</Code>")) },
	} {
		var attempts atomic.Int32
		cfg := testConfig("http://first.test", "http://second.test")
		cfg.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts.Add(1)
			if req.URL.Host == "first.test" {
				return &http.Response{StatusCode: 404, Header: make(http.Header), Body: body(), Request: req}, nil
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: 2, Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
		})
		c := mustClient(t, cfg)
		data, err := c.Get(context.Background(), "key")
		if err != nil || string(data) != "ok" || attempts.Load() != 2 {
			t.Fatalf("Get=%q,%v missing=%v attempts=%d", data, err, errors.Is(err, storage.ErrNotExist), attempts.Load())
		}
	}
	cfg := testConfig("http://only.test")
	cfg.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 404, Header: make(http.Header), Body: interruptedErrorBody{}, Request: req}, nil
	})
	c := mustClient(t, cfg)
	if _, err := c.Get(context.Background(), "key"); errors.Is(err, storage.ErrNotExist) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated404 error=%v", err)
	}
}

func TestEmpty404RemainsMissingAnd403NeverRetries(t *testing.T) {
	for _, status := range []int{404, 403} {
		var attempts atomic.Int32
		cfg := testConfig("http://first.test", "http://second.test")
		cfg.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts.Add(1)
			body := io.ReadCloser(http.NoBody)
			if status == 403 {
				body = interruptedErrorBody{}
			}
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: body, Request: req}, nil
		})
		c := mustClient(t, cfg)
		_, err := c.Get(context.Background(), "key")
		if err == nil || attempts.Load() != 1 || (status == 404 && !errors.Is(err, storage.ErrNotExist)) {
			t.Fatalf("status=%d error=%v attempts=%d", status, err, attempts.Load())
		}
	}
}

func TestPutReceiptReportsActualSuccessfulEndpoint(t *testing.T) {
	first, second := newCountingServer(t), newCountingServer(t)
	first.status.Store(503)
	cfg := testConfig(first.URL, second.URL)
	cfg.EndpointNames = []string{"vm105", "cn06"}
	c := mustClient(t, cfg)
	var receipts []storage.WriteReceipt
	ctx := storage.WithWriteReceipt(context.Background(), func(receipt storage.WriteReceipt) { receipts = append(receipts, receipt) })
	start := time.Now()
	if err := c.Put(ctx, "image", []byte("data"), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 || receipts[0].WriterNode != "cn06" || receipts[0].WrittenAt.Before(start) || receipts[0].WrittenAt.Location() != time.UTC {
		t.Fatalf("receipts=%+v", receipts)
	}
	first.status.Store(403)
	second.status.Store(403)
	receipts = nil
	if err := c.Put(ctx, "image", []byte("data"), storage.PutOptions{}); err == nil {
		t.Fatal("expected failed Put")
	}
	if len(receipts) != 0 {
		t.Fatal("failed Put emitted success receipt")
	}
}

func TestConcurrentPutReceiptsDoNotLeakBetweenCalls(t *testing.T) {
	first, second := newCountingServer(t), newCountingServer(t)
	cfg := testConfig(first.URL, second.URL)
	cfg.EndpointNames = []string{"vm105", "cn06"}
	c := mustClient(t, cfg)
	var firstCount, secondCount atomic.Int32
	var wg sync.WaitGroup
	for i := range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var receipts []storage.WriteReceipt
			ctx := storage.WithWriteReceipt(context.Background(), func(receipt storage.WriteReceipt) { receipts = append(receipts, receipt) })
			if err := c.Put(ctx, storage.Key(fmt.Sprintf("image-%d", i)), []byte("data"), storage.PutOptions{}); err != nil {
				t.Error(err)
				return
			}
			if len(receipts) != 1 {
				t.Errorf("receipts=%+v", receipts)
				return
			}
			switch receipts[0].WriterNode {
			case "vm105":
				firstCount.Add(1)
			case "cn06":
				secondCount.Add(1)
			default:
				t.Errorf("unexpected node=%q", receipts[0].WriterNode)
			}
		}()
	}
	wg.Wait()
	if firstCount.Load() != 15 || secondCount.Load() != 15 {
		t.Fatalf("receipts=%d/%d", firstCount.Load(), secondCount.Load())
	}
}
