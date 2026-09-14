// Package urlhost selects one public base URL out of a set of per-node hosts.
// Cloud never fetches these URLs itself (the QQ client does), so health is
// driven by explicit MarkFailure calls and, optionally, a HEAD prober.
package urlhost

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Defaults applied to zero Options fields.
const (
	DefaultProbeInterval = 30 * time.Second
	DefaultProbeTimeout  = 3 * time.Second
	DefaultCooldown      = 60 * time.Second
)

// Host is one named public base URL.
type Host struct{ Name, BaseURL string }

// Options tunes selection and probing. The zero value selects in name order
// and never starts a prober.
type Options struct {
	Order         []string      // preferred order; empty -> name sort (New) or list order (FromList)
	ProbePath     string        // "" -> no prober goroutine at all
	ProbeInterval time.Duration // default 30s
	ProbeTimeout  time.Duration // default 3s
	Cooldown      time.Duration // default 60s
	Client        *http.Client  // tests inject
}

// HostStatus is one row of Snapshot.
type HostStatus struct {
	Name          string
	BaseURL       string
	Healthy       bool
	Failures      uint64
	CooldownUntil time.Time
}

type hostState struct {
	cooldownUntil atomic.Int64 // unix nanos; 0 = healthy
	failures      atomic.Uint64
}

// Set is an immutable host list with per-host health. A nil *Set behaves as
// an empty set.
type Set struct {
	hosts  []Host // in selection order
	byName map[string]int
	state  []hostState
	rr     atomic.Uint32
	opts   Options
	now    func() time.Time
	start  sync.Once
}

// ErrInvalidHost is returned for a host whose base URL is not an absolute
// http(s) URL or whose name is empty or unknown.
var ErrInvalidHost = errors.New("urlhost: invalid host")

// New builds a set of named hosts (image_cache.hosts). Names listed in
// opts.Order come first, in that order; the remaining hosts follow in name
// order. An empty map yields an empty set.
func New(hosts map[string]string, opts Options) (*Set, error) {
	names := make([]string, 0, len(hosts))
	for name := range hosts {
		names = append(names, name)
	}
	slices.Sort(names)
	ordered := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range opts.Order {
		name = strings.TrimSpace(name)
		if _, ok := hosts[name]; !ok {
			return nil, fmt.Errorf("%w: host_order names unknown host %q", ErrInvalidHost, name)
		}
		if !seen[name] {
			seen[name] = true
			ordered = append(ordered, name)
		}
	}
	for _, name := range names {
		if !seen[name] {
			ordered = append(ordered, name)
		}
	}
	list := make([]Host, 0, len(ordered))
	for _, name := range ordered {
		list = append(list, Host{Name: name, BaseURL: hosts[name]})
	}
	return build(list, opts)
}

// FromList builds an ordered set of unnamed hosts (asset_dirs.assets_base_urls).
// Each host is named by its URL host; duplicates are suffixed -2, -3, ...
func FromList(urls []string, opts Options) (*Set, error) {
	list := make([]Host, 0, len(urls))
	counts := make(map[string]int, len(urls))
	for _, raw := range urls {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parsed, err := parseBase(raw)
		if err != nil {
			return nil, err
		}
		name := parsed.Host
		counts[name]++
		if n := counts[name]; n > 1 {
			name += "-" + strconv.Itoa(n)
		}
		list = append(list, Host{Name: name, BaseURL: raw})
	}
	return build(list, opts)
}

// Single wraps one legacy base URL. An empty or invalid URL yields an empty set.
func Single(baseURL string) *Set {
	set, err := FromList([]string{baseURL}, Options{})
	if err != nil {
		set, _ = build(nil, Options{})
	}
	return set
}

func build(list []Host, opts Options) (*Set, error) {
	if opts.ProbeInterval <= 0 {
		opts.ProbeInterval = DefaultProbeInterval
	}
	if opts.ProbeTimeout <= 0 {
		opts.ProbeTimeout = DefaultProbeTimeout
	}
	if opts.Cooldown <= 0 {
		opts.Cooldown = DefaultCooldown
	}
	if opts.Client == nil {
		opts.Client = &http.Client{}
	}
	opts.Order = nil
	set := &Set{byName: make(map[string]int, len(list)), opts: opts, now: time.Now}
	for _, host := range list {
		name := strings.TrimSpace(host.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: empty host name", ErrInvalidHost)
		}
		if _, err := parseBase(host.BaseURL); err != nil {
			return nil, fmt.Errorf("%w (host %q)", err, name)
		}
		set.byName[name] = len(set.hosts)
		set.hosts = append(set.hosts, Host{Name: name, BaseURL: strings.TrimRight(strings.TrimSpace(host.BaseURL), "/")})
	}
	set.state = make([]hostState, len(set.hosts))
	return set, nil
}

func parseBase(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("%w: base URL %q must be an absolute http(s) URL", ErrInvalidHost, raw)
	}
	return parsed, nil
}

// Len reports the number of hosts.
func (s *Set) Len() int {
	if s == nil {
		return 0
	}
	return len(s.hosts)
}

// Hosts returns a copy of the hosts in selection order.
func (s *Set) Hosts() []Host {
	if s == nil {
		return nil
	}
	return slices.Clone(s.hosts)
}

// Base picks a base URL: the host named prefer when it is configured and not
// cooling down, otherwise the next healthy host in rotation, otherwise (every
// host cooling down) the next host in rotation. It returns "" only for an
// empty set.
func (s *Set) Base(prefer string) string {
	if idx, ok := s.pick(prefer); ok {
		return s.hosts[idx].BaseURL
	}
	return ""
}

// URL joins the selected base with relPath, which must already be escaped.
func (s *Set) URL(prefer, relPath string) (string, bool) {
	base := s.Base(prefer)
	if base == "" {
		return "", false
	}
	return base + "/" + strings.TrimLeft(relPath, "/"), true
}

func (s *Set) pick(prefer string) (int, bool) {
	if s.Len() == 0 {
		return 0, false
	}
	now := s.now().UnixNano()
	if idx, ok := s.byName[strings.TrimSpace(prefer)]; ok && s.healthy(idx, now) {
		return idx, true
	}
	n := len(s.hosts)
	start := int(s.rr.Add(1)-1) % n
	for i := range n {
		if idx := (start + i) % n; s.healthy(idx, now) {
			return idx, true
		}
	}
	return start, true
}

func (s *Set) healthy(idx int, now int64) bool {
	until := s.state[idx].cooldownUntil.Load()
	return until == 0 || now >= until
}

// MarkFailure puts the named host into cooldown. Unknown names are ignored.
func (s *Set) MarkFailure(name string) {
	if s == nil {
		return
	}
	idx, ok := s.byName[strings.TrimSpace(name)]
	if !ok {
		return
	}
	s.state[idx].failures.Add(1)
	s.state[idx].cooldownUntil.Store(s.now().Add(s.opts.Cooldown).UnixNano())
}

func (s *Set) markHealthy(idx int) {
	s.state[idx].cooldownUntil.Store(0)
}

// Start launches the prober bound to ctx. It is a no-op for an empty set,
// when ProbePath is "", and on every call after the first.
func (s *Set) Start(ctx context.Context) {
	if s.Len() == 0 || strings.TrimSpace(s.opts.ProbePath) == "" {
		return
	}
	s.start.Do(func() {
		go s.probeLoop(ctx)
	})
}

func (s *Set) probeLoop(ctx context.Context) {
	ticker := time.NewTicker(s.opts.ProbeInterval)
	defer ticker.Stop()
	for {
		s.probeAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Set) probeAll(ctx context.Context) {
	for idx := range s.hosts {
		if ctx.Err() != nil {
			return
		}
		if s.probe(ctx, idx) {
			s.markHealthy(idx)
		} else {
			s.MarkFailure(s.hosts[idx].Name)
		}
	}
}

// probe reports whether the host answered below 500: Caddy answering at all
// is what matters, so 4xx counts as up.
func (s *Set) probe(ctx context.Context, idx int) bool {
	ctx, cancel := context.WithTimeout(ctx, s.opts.ProbeTimeout)
	defer cancel()
	target := s.hosts[idx].BaseURL + "/" + strings.TrimLeft(s.opts.ProbePath, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return false
	}
	resp, err := s.opts.Client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode < http.StatusInternalServerError
}

// Snapshot reports every host's health in selection order.
func (s *Set) Snapshot() []HostStatus {
	if s.Len() == 0 {
		return nil
	}
	now := s.now().UnixNano()
	out := make([]HostStatus, len(s.hosts))
	for idx, host := range s.hosts {
		status := HostStatus{Name: host.Name, BaseURL: host.BaseURL, Healthy: s.healthy(idx, now), Failures: s.state[idx].failures.Load()}
		if until := s.state[idx].cooldownUntil.Load(); until != 0 {
			status.CooldownUntil = time.Unix(0, until)
		}
		out[idx] = status
	}
	return out
}
