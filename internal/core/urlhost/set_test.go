package urlhost

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct{ at atomic.Int64 }

func newFakeClock() *fakeClock {
	c := &fakeClock{}
	c.at.Store(time.Unix(1_700_000_000, 0).UnixNano())
	return c
}

func (c *fakeClock) now() time.Time          { return time.Unix(0, c.at.Load()) }
func (c *fakeClock) advance(d time.Duration) { c.at.Add(int64(d)) }

func mustNew(t *testing.T, hosts map[string]string, opts Options) *Set {
	t.Helper()
	set, err := New(hosts, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return set
}

func TestNewOrdersByHostOrderThenName(t *testing.T) {
	set := mustNew(t, map[string]string{
		"cn01": "https://ic-cn01.example/",
		"cn09": "https://ic-cn09.example",
		"cn06": "http://ic-cn06.example/base",
	}, Options{Order: []string{"cn09", " cn09"}})
	hosts := set.Hosts()
	want := []Host{
		{Name: "cn09", BaseURL: "https://ic-cn09.example"},
		{Name: "cn01", BaseURL: "https://ic-cn01.example"},
		{Name: "cn06", BaseURL: "http://ic-cn06.example/base"},
	}
	if len(hosts) != len(want) {
		t.Fatalf("hosts = %v", hosts)
	}
	for i := range want {
		if hosts[i] != want[i] {
			t.Fatalf("hosts[%d] = %v, want %v", i, hosts[i], want[i])
		}
	}
	if set.Len() != 3 {
		t.Fatalf("Len = %d", set.Len())
	}
}

func TestNewRejectsInvalidHosts(t *testing.T) {
	cases := []struct {
		name  string
		hosts map[string]string
		opts  Options
	}{
		{"relative url", map[string]string{"a": "/images"}, Options{}},
		{"ftp url", map[string]string{"a": "ftp://x"}, Options{}},
		{"no host", map[string]string{"a": "https://"}, Options{}},
		{"empty name", map[string]string{" ": "https://x"}, Options{}},
		{"unknown order name", map[string]string{"a": "https://x"}, Options{Order: []string{"b"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.hosts, tc.opts); !errors.Is(err, ErrInvalidHost) {
				t.Fatalf("err = %v, want ErrInvalidHost", err)
			}
		})
	}
}

func TestFromListNamesAndSuffixesDuplicates(t *testing.T) {
	set, err := FromList([]string{
		"https://assets-cn09.example/",
		"",
		"https://assets-cn01.example",
		"https://assets-cn09.example/mirror",
		"https://assets-cn09.example/third",
	}, Options{})
	if err != nil {
		t.Fatalf("FromList: %v", err)
	}
	names := []string{}
	for _, h := range set.Hosts() {
		names = append(names, h.Name)
	}
	want := []string{"assets-cn09.example", "assets-cn01.example", "assets-cn09.example-2", "assets-cn09.example-3"}
	if len(names) != len(want) {
		t.Fatalf("names = %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}
	if got := set.Base(""); got != "https://assets-cn09.example" {
		t.Fatalf("first Base = %q", got)
	}
	if _, err := FromList([]string{"not a url"}, Options{}); !errors.Is(err, ErrInvalidHost) {
		t.Fatalf("invalid list err = %v", err)
	}
}

func TestSingleLegacyCompat(t *testing.T) {
	set := Single("https://assets.example/")
	if set.Len() != 1 || set.Base("") != "https://assets.example" {
		t.Fatalf("Single = %v", set.Hosts())
	}
	if url, ok := set.URL("", "/jp-assets/x.png"); !ok || url != "https://assets.example/jp-assets/x.png" {
		t.Fatalf("URL = %q %v", url, ok)
	}
	for _, raw := range []string{"", "  ", "relative/path"} {
		if s := Single(raw); s == nil || s.Len() != 0 {
			t.Fatalf("Single(%q) should be an empty non-nil set", raw)
		}
	}
}

func TestEmptyAndNilSets(t *testing.T) {
	empty := mustNew(t, nil, Options{})
	var nilSet *Set
	for _, set := range []*Set{empty, nilSet} {
		if set.Len() != 0 || set.Base("x") != "" || set.Hosts() != nil || set.Snapshot() != nil {
			t.Fatal("empty set must report nothing")
		}
		if url, ok := set.URL("", "a"); ok || url != "" {
			t.Fatalf("URL = %q %v", url, ok)
		}
		set.MarkFailure("x")
		set.Start(context.Background())
	}
}

func TestBasePrefersNamedHostAndRotates(t *testing.T) {
	set := mustNew(t, map[string]string{"a": "https://a.example", "b": "https://b.example", "c": "https://c.example"}, Options{})
	for range 3 {
		if got := set.Base("b"); got != "https://b.example" {
			t.Fatalf("preferred Base = %q", got)
		}
	}
	got := []string{set.Base(""), set.Base("unknown"), set.Base(""), set.Base("")}
	want := []string{"https://a.example", "https://b.example", "https://c.example", "https://a.example"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rotation = %v, want %v", got, want)
		}
	}
}

func TestCooldownSkipAndReadmission(t *testing.T) {
	clock := newFakeClock()
	set := mustNew(t, map[string]string{"a": "https://a.example", "b": "https://b.example"}, Options{Cooldown: time.Minute})
	set.now = clock.now

	set.MarkFailure("a")
	set.MarkFailure("unknown")
	if got := set.Base("a"); got != "https://b.example" {
		t.Fatalf("cooled preferred host must be skipped, got %q", got)
	}
	for range 4 {
		if got := set.Base(""); got != "https://b.example" {
			t.Fatalf("rotation must skip the cooled host, got %q", got)
		}
	}
	snap := set.Snapshot()
	if snap[0].Healthy || snap[0].Failures != 1 || snap[0].CooldownUntil.IsZero() || !snap[1].Healthy {
		t.Fatalf("snapshot = %+v", snap)
	}

	clock.advance(time.Minute)
	if got := set.Base("a"); got != "https://a.example" {
		t.Fatalf("host must be re-admitted after cooldown, got %q", got)
	}
}

func TestAllHostsCoolingStillReturnsURL(t *testing.T) {
	set := mustNew(t, map[string]string{"a": "https://a.example", "b": "https://b.example"}, Options{})
	set.MarkFailure("a")
	set.MarkFailure("b")
	seen := map[string]bool{}
	for range 4 {
		url, ok := set.URL("a", "x")
		if !ok || url == "" {
			t.Fatal("a configured set must never return an empty URL")
		}
		seen[url] = true
	}
	if len(seen) != 2 {
		t.Fatalf("all-down selection must still rotate, saw %v", seen)
	}
}

func TestDefaultsApplied(t *testing.T) {
	set := mustNew(t, map[string]string{"a": "https://a.example"}, Options{})
	if set.opts.ProbeInterval != DefaultProbeInterval || set.opts.ProbeTimeout != DefaultProbeTimeout ||
		set.opts.Cooldown != DefaultCooldown || set.opts.Client == nil {
		t.Fatalf("defaults = %+v", set.opts)
	}
}

func TestStartWithoutProbePathIsNoop(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer server.Close()
	set := mustNew(t, map[string]string{"a": server.URL}, Options{ProbeInterval: time.Millisecond})
	set.Start(context.Background())
	time.Sleep(20 * time.Millisecond)
	if hits.Load() != 0 {
		t.Fatalf("prober ran without a probe path: %d hits", hits.Load())
	}
}

func TestProbeMarksDownAndUp(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusServiceUnavailable)
	var methods atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods.Store(r.Method + " " + r.URL.Path)
		w.WriteHeader(int(status.Load()))
	}))
	defer server.Close()
	set := mustNew(t, map[string]string{"a": server.URL, "b": "http://127.0.0.1:1"}, Options{
		ProbePath: "/health", Client: server.Client(), ProbeTimeout: time.Second,
	})
	ctx := context.Background()

	set.probeAll(ctx)
	if got := methods.Load(); got != "HEAD /health" {
		t.Fatalf("probe request = %v", got)
	}
	snap := set.Snapshot()
	if snap[0].Healthy || snap[1].Healthy {
		t.Fatalf("5xx and transport error must mark down: %+v", snap)
	}

	status.Store(http.StatusNotFound)
	set.probeAll(ctx)
	if snap := set.Snapshot(); !snap[0].Healthy || snap[1].Healthy {
		t.Fatalf("4xx must mark up: %+v", snap)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	set.MarkFailure("a")
	set.probeAll(cancelled)
	if set.Snapshot()[0].Healthy {
		t.Fatal("a cancelled probe round must not change health")
	}
}

func TestProbeInvalidRequest(t *testing.T) {
	set := mustNew(t, map[string]string{"a": "https://a.example"}, Options{ProbePath: "%zz"})
	if set.probe(context.Background(), 0) {
		t.Fatal("an unbuildable probe request must count as down")
	}
}

func TestStartProbesUntilContextDone(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	set := mustNew(t, map[string]string{"a": server.URL}, Options{
		ProbePath: "health", ProbeInterval: 5 * time.Millisecond, Client: server.Client(),
	})
	set.MarkFailure("a")
	ctx, cancel := context.WithCancel(context.Background())
	set.Start(ctx)
	set.Start(ctx) // idempotent

	deadline := time.Now().Add(2 * time.Second)
	for hits.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hits.Load() < 2 {
		t.Fatalf("prober hits = %d", hits.Load())
	}
	if !set.Snapshot()[0].Healthy {
		t.Fatal("a 200 probe must clear the cooldown")
	}
	cancel()
	time.Sleep(20 * time.Millisecond)
	settled := hits.Load()
	time.Sleep(30 * time.Millisecond)
	if hits.Load() != settled {
		t.Fatal("prober kept running after context cancellation")
	}
}
