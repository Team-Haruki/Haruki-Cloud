package assetindex

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
	"haruki-cloud/utils/logger"
)

type node struct {
	children map[string]*node
	file     bool
}

func (n *node) insert(key string) {
	for _, p := range strings.Split(key, "/") {
		if n.children == nil {
			n.children = make(map[string]*node)
		}
		child := n.children[p]
		if child == nil {
			child = &node{}
			n.children[p] = child
		}
		n = child
	}
	n.file = true
}
func (n *node) lookup(key string) (string, bool) {
	parts := strings.Split(key, "/")
	canonical := make([]string, 0, len(parts))
	for i, p := range parts {
		eligible := func(candidate *node) bool {
			return candidate != nil && ((i == len(parts)-1 && candidate.file) || (i < len(parts)-1 && len(candidate.children) > 0))
		}
		child, ok := n.children[p]
		ok = ok && eligible(child)
		if !ok {
			best := ""
			for name, c := range n.children {
				if eligible(c) && strings.EqualFold(name, p) && (best == "" || name < best) {
					best, child = name, c
				}
			}
			if best == "" {
				return strings.Join(append(canonical, parts[i:]...), "/"), false
			}
			p = best
		}
		canonical = append(canonical, p)
		n = child
	}
	return strings.Join(canonical, "/"), n.file
}

type regionIndex struct {
	manifest          Manifest
	tree              *node
	checked           time.Time
	objects           int
	inventoryRevision string
}
type state struct{ regions map[string]*regionIndex }
type Manager struct {
	store         storage.Store
	cfg           Config
	refreshFlight singleflight.Group
	refreshMu     sync.Mutex
	refreshDone   chan struct{}
	lifecycle     context.Context
	current       atomic.Pointer[state]
	cancel        context.CancelFunc
	done          chan struct{}
	start         sync.Once
	close         sync.Once
	onChange      func()
	now           func() time.Time
}

func New(store storage.Store, cfg Config, onChange func()) *Manager {
	if !cfg.Enabled || store == nil || store == storage.Disabled() {
		return nil
	}
	lifecycle, cancel := context.WithCancel(context.Background())
	return &Manager{store: store, cfg: cfg.defaults(), onChange: onChange, now: time.Now, done: make(chan struct{}), lifecycle: lifecycle, cancel: cancel}
}
func (m *Manager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	m.start.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		if ctx.Err() != nil {
			m.cancel()
		}
		stop := context.AfterFunc(ctx, m.cancel)
		go func() {
			defer close(m.done)
			defer stop()
			ctx := m.lifecycle
			for {
				_ = m.Refresh(ctx)
				timer := time.NewTimer(m.cfg.PollInterval)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}()
	})
}
func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.close.Do(func() {
		m.cancel()
		m.start.Do(func() { close(m.done) })
		<-m.done
		// Refresh callers may stop waiting before their shared body exits.
		m.refreshMu.Lock()
		active := m.refreshDone
		m.refreshMu.Unlock()
		if active != nil {
			<-active
		}
	})
}
func (m *Manager) Refresh(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.lifecycle.Err(); err != nil {
		return err
	}
	result := m.refreshFlight.DoChan("refresh", func() (any, error) {
		m.refreshMu.Lock()
		if err := m.lifecycle.Err(); err != nil {
			m.refreshMu.Unlock()
			return nil, err
		}
		done := make(chan struct{})
		m.refreshDone = done
		m.refreshMu.Unlock()
		defer close(done)
		return nil, m.refresh()
	})
	select {
	case completed := <-result:
		return completed.Err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) refresh() error {
	ctx, cancel := context.WithTimeout(storage.WithBackgroundIO(m.lifecycle), m.cfg.Timeout)
	defer cancel()
	finish := commandtrace.MeasureOperation(ctx, "asset.index_refresh")
	defer finish()
	next := &state{regions: make(map[string]*regionIndex)}
	previous := m.current.Load()
	if previous != nil {
		for r, v := range previous.regions {
			next.regions[r] = v
		}
	}
	var errs []error
	changed := false
	for _, r := range Regions {
		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
		data, err := m.store.Get(ctx, PointerKey(r))
		if errors.Is(err, storage.ErrNotExist) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var manifest Manifest
		if err = json.Unmarshal(data, &manifest); err != nil {
			errs = append(errs, err)
			continue
		}
		if manifest.Region != r {
			errs = append(errs, fmt.Errorf("asset index: region mismatch"))
			continue
		}
		old := next.regions[r]
		// Compare full pointer semantics: BPM publication can change without assets changing.
		if old != nil && sameManifest(old.manifest, manifest) {
			copy := *old
			copy.checked = m.now()
			next.regions[r] = &copy
			continue
		}
		loaded, err := loadRegion(ctx, m.store, manifest, m.cfg.MaxObjects)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		loaded.checked = m.now()
		next.regions[r] = loaded
		changed = true
	}
	m.current.Store(next)
	if changed && m.onChange != nil {
		m.onChange()
	}
	if len(errs) > 0 {
		logger.WarnContext(ctx, "asset index refresh incomplete", "failed_regions", len(errs))
	}
	return errors.Join(errs...)
}
func sameManifest(a, b Manifest) bool {
	a.PublishedAt = time.Time{}
	b.PublishedAt = time.Time{}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func loadRegion(ctx context.Context, store storage.Store, m Manifest, maxObjects int) (*regionIndex, error) {
	if m.Version != Version || !validRegion(m.Region) || !m.Complete || !validDigest(m.Revision) || len(m.Shards) == 0 || len(m.Shards) > 4096 {
		return nil, fmt.Errorf("asset index: invalid complete manifest")
	}
	regionPrefix := m.Region + "-assets/"
	result := &regionIndex{manifest: m, tree: &node{}}
	seen := make(map[storage.Key]struct{})
	objects := make([]Object, 0)
	prefixes := make(map[string]struct{})
	for _, ref := range m.Shards {
		if !strings.HasPrefix(ref.Prefix, regionPrefix) || !strings.HasSuffix(ref.Prefix, "/") {
			return nil, fmt.Errorf("asset index: invalid shard prefix")
		}
		if _, duplicate := prefixes[ref.Prefix]; duplicate {
			return nil, fmt.Errorf("asset index: duplicate shard prefix")
		}
		prefixes[ref.Prefix] = struct{}{}
		if err := validateBlob(ref.Blob, Root+m.Region+"/shards/"); err != nil {
			return nil, err
		}
		data, err := readBlob(ctx, store, ref.Blob)
		if err != nil {
			return nil, err
		}
		var shard Shard
		if err = json.Unmarshal(data, &shard); err != nil {
			return nil, err
		}
		if shard.Version != Version || shard.Region != m.Region || shard.Prefix != ref.Prefix {
			return nil, fmt.Errorf("asset index: shard mismatch")
		}
		for _, o := range shard.Objects {
			if len(seen) >= maxObjects {
				return nil, fmt.Errorf("asset index: object budget exceeded")
			}
			key, err := storage.CleanKey(string(o.Key))
			if err != nil || key != o.Key || !strings.HasPrefix(string(key), ref.Prefix) {
				return nil, fmt.Errorf("asset index: invalid object")
			}
			if _, exists := seen[key]; exists {
				return nil, fmt.Errorf("asset index: duplicate object")
			}
			seen[key] = struct{}{}
			objects = append(objects, o)
			result.tree.insert(string(key))
		}
	}
	if len(seen) == 0 || manifestRevision(m.Shards) != m.Revision {
		return nil, fmt.Errorf("asset index: inventory revision mismatch")
	}
	if m.BPM != nil {
		if err := validateBlob(*m.BPM, "indexes/bpm/v1/"+m.Region+"/"); err != nil {
			return nil, err
		}
		if m.BPM.Revision != m.Revision {
			return nil, fmt.Errorf("asset index: BPM revision mismatch")
		}
		if _, err := readBlob(ctx, store, *m.BPM); err != nil {
			return nil, err
		}
	}
	result.objects = len(seen)
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	inventory, err := json.Marshal(objects)
	if err != nil {
		return nil, err
	}
	result.inventoryRevision = digest(inventory)
	return result, nil
}
func (m *Manager) Lookup(key storage.Key) (storage.Key, bool, bool) {
	if m == nil {
		return "", false, false
	}
	s := m.current.Load()
	if s == nil {
		return "", false, false
	}
	region, _, ok := strings.Cut(string(key), "-assets/")
	if !ok {
		return "", false, false
	}
	idx := s.regions[region]
	if idx == nil || m.now().Sub(idx.checked) > m.cfg.MaxStale {
		return "", false, false
	}
	resolved, found := idx.tree.lookup(string(key))
	return storage.Key(resolved), found, true
}
func (m *Manager) BPMIndex(region string) (string, storage.Key, bool) {
	if m == nil {
		return "", "", false
	}
	s := m.current.Load()
	if s == nil {
		return "", "", false
	}
	idx := s.regions[region]
	if idx == nil {
		return "", "", false
	}
	if idx.manifest.BPM == nil || m.now().Sub(idx.checked) > m.cfg.MaxStale {
		return idx.manifest.Revision, "", false
	}
	return idx.manifest.Revision, idx.manifest.BPM.Key, true
}
func (m *Manager) Revision() string {
	if m == nil {
		return ""
	}
	return m.revision(m.current.Load(), m.now())
}

func (m *Manager) revision(s *state, now time.Time) string {
	if s == nil || len(s.regions) == 0 {
		return ""
	}
	tokens := make([]string, 0, len(s.regions))
	for r, v := range s.regions {
		revision := v.manifest.Revision
		if now.Sub(v.checked) > m.cfg.MaxStale {
			revision = "stale:" + revision
		}
		tokens = append(tokens, r+":"+revision)
	}
	sort.Strings(tokens)
	return digest([]byte(strings.Join(tokens, "\n")))
}

// SnapshotForPayload binds the Drawing namespace and Cloud cache dependencies
// to one immutable publication, even if a refresh finishes concurrently.
func (m *Manager) SnapshotForPayload(payload any) (assetRevision, scopedRevision string) {
	if m == nil {
		return "", ""
	}
	state, now := m.current.Load(), m.now()
	return m.revision(state, now), m.revisionForPayload(state, payload, now)
}

// RevisionForPayload scopes invalidation to the resource domains actually used.
func (m *Manager) RevisionForPayload(payload any) string {
	if m == nil {
		return ""
	}
	return m.revisionForPayload(m.current.Load(), payload, m.now())
}

func (m *Manager) revisionForPayload(s *state, payload any, now time.Time) string {
	if s == nil {
		return ""
	}
	tokens := map[string]struct{}{}
	var visit func(any, int)
	visit = func(value any, depth int) {
		if depth > 64 {
			return
		}
		switch v := value.(type) {
		case map[string]any:
			for _, x := range v {
				visit(x, depth+1)
			}
		case []any:
			for _, x := range v {
				visit(x, depth+1)
			}
		case string:
			for region, idx := range s.regions {
				marker := region + "-assets/"
				pos := strings.Index(v, marker)
				if pos < 0 {
					continue
				}
				key, _ := idx.tree.lookup(v[pos:])
				best := ""
				rev := idx.manifest.Revision
				for _, shard := range idx.manifest.Shards {
					if strings.HasPrefix(key, shard.Prefix) && len(shard.Prefix) > len(best) {
						best, rev = shard.Prefix, shard.SHA256
					}
				}
				if now.Sub(idx.checked) > m.cfg.MaxStale {
					rev = "stale:" + rev
				}
				tokens[region+":"+best+":"+rev] = struct{}{}
			}
		}
	}
	visit(payload, 0)
	if len(tokens) == 0 {
		return ""
	}
	sorted := make([]string, 0, len(tokens))
	for t := range tokens {
		sorted = append(sorted, t)
	}
	sort.Strings(sorted)
	return digest([]byte(strings.Join(sorted, "\n")))
}
