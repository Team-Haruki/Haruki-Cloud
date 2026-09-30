package music

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/storage"
)

const (
	BPMIndexSchemaVersion = 1
	bpmIndexMaxCharts     = 131072
	bpmIndexMaxBytes      = 32 << 20
	bpmIndexRetryDelay    = 30 * time.Second
)

// BPMIndex contains every readable chart under both regional score prefixes.
// Complete indexes can answer missing charts without probing the object store.
type BPMIndex struct {
	SchemaVersion    int                      `json:"schema_version"`
	Region           string                   `json:"region"`
	ResourceRevision string                   `json:"resource_revision"`
	Complete         bool                     `json:"complete"`
	Prefixes         []string                 `json:"prefixes"`
	Charts           map[string]BPMIndexChart `json:"charts"`
	foldedCharts     map[string]string
}

type BPMIndexChart struct {
	MainBPM  float64         `json:"main_bpm"`
	Events   []BPMIndexEvent `json:"events"`
	BarCount int             `json:"bar_count"`
	Duration float64         `json:"duration"`
}

type BPMIndexEvent struct {
	Bar      float64 `json:"bar"`
	BPM      float64 `json:"bpm"`
	Duration float64 `json:"duration"`
}

// BPMIndexSource identifies the immutable index of the current resource
// revision. The revision may be nonempty with ok=false before an index exists.
type BPMIndexSource interface {
	BPMIndex(region string) (revision string, key storage.Key, ok bool)
}

func (c *Controller) SetBPMIndexSource(source BPMIndexSource, store storage.Store) {
	if c == nil {
		return
	}
	if source == nil || store == nil || store == storage.Disabled() {
		c.bpmIndex = nil
		return
	}
	c.bpmIndex = &bpmIndexReader{source: source, store: store, entries: make(map[string]bpmIndexEntry)}
}

type bpmIndexEntry struct {
	revision string
	key      storage.Key
	index    *BPMIndex
	retryAt  time.Time
}

type bpmIndexReader struct {
	source  BPMIndexSource
	store   storage.Store
	mu      sync.Mutex
	entries map[string]bpmIndexEntry
	flights singleflight.Group
}

// A single index flight may feed many chart workers in one command. Retain its
// trace identity only until those waiters return, not in the cached index.
type bpmIndexLoadResult struct {
	index      *BPMIndex
	operations []commandtrace.Stats
	mu         sync.Mutex
	merged     map[*commandtrace.Trace]struct{}
}

func (r *bpmIndexLoadResult) mergeOperations(ctx context.Context) {
	trace := commandtrace.FromContext(ctx)
	if r == nil || trace == nil || ctx.Err() != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, merged := r.merged[trace]; merged {
		return
	}
	if r.merged == nil {
		r.merged = make(map[*commandtrace.Trace]struct{})
	}
	r.merged[trace] = struct{}{}
	commandtrace.MergeOperations(ctx, r.operations)
}

func normalizeBPMIndexRegion(region string) (string, error) {
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		region = "jp"
	}
	switch region {
	case "jp", "en", "tw", "kr", "cn":
		return region, nil
	}
	return "", fmt.Errorf("invalid BPM index region %q", region)
}

func bpmIndexPrefixes(region string) []string {
	return []string{region + "-assets/startapp/music/music_score/", region + "-assets/ondemand/music/music_score/"}
}

func (r *bpmIndexReader) reference(region string) (string, storage.Key, bool) {
	if r == nil {
		return "", "", false
	}
	region, err := normalizeBPMIndexRegion(region)
	if err != nil {
		return "", "", false
	}
	return r.source.BPMIndex(region)
}

func (r *bpmIndexReader) lookup(ctx context.Context, region, revision string, key storage.Key, candidates []string) (*parsedChartBPM, bool, bool) {
	if r == nil || revision == "" || key == "" {
		return nil, false, false
	}
	index := r.load(ctx, region, revision, key)
	if index == nil {
		return nil, false, false
	}
	for _, candidate := range candidates {
		objectKey, err := assets.ObjectKey(candidate)
		if err != nil {
			return nil, false, false
		}
		canonicalKnown := false
		if metadata, ok := r.source.(assets.MetadataIndex); ok {
			if resolved, found, authoritative := metadata.Lookup(objectKey); authoritative && found {
				objectKey, canonicalKnown = resolved, true
			}
		}
		chart, found := index.Charts[string(objectKey)]
		if !found && !canonicalKnown {
			if folded, ok := index.foldedCharts[strings.ToLower(string(objectKey))]; ok {
				chart, found = index.Charts[folded]
			}
		}
		if found {
			commandtrace.RecordOperation(ctx, "music.bpm_index_hit", 0)
			return chart.parsed(), true, true
		}
		// A metadata index resolving an object absent from the BPM index may
		// indicate an asset publication transition. Read it instead of turning
		// a known object into an authoritative missing chart.
		if canonicalKnown {
			return nil, false, false
		}
		covered := false
		for _, prefix := range index.Prefixes {
			covered = covered || strings.HasPrefix(string(objectKey), prefix)
		}
		// A local legacy candidate is outside a regional index. Preserve its
		// precedence instead of claiming that it is missing.
		if !covered {
			return nil, false, false
		}
	}
	commandtrace.RecordOperation(ctx, "music.bpm_index_missing", 0)
	return nil, false, true
}

func (r *bpmIndexReader) cached(region, revision string, key storage.Key) (*BPMIndex, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[region]
	if !ok || entry.revision != revision || entry.key != key {
		return nil, false
	}
	return entry.index, entry.index != nil || time.Now().Before(entry.retryAt)
}

func (r *bpmIndexReader) load(ctx context.Context, region, revision string, key storage.Key) *BPMIndex {
	if index, ok := r.cached(region, revision, key); ok {
		return index
	}
	finish := commandtrace.MeasureOperation(ctx, "music.bpm_index_wait")
	defer finish()
	flight := r.flights.DoChan(region+"\x00"+revision+"\x00"+string(key), func() (any, error) {
		if index, ok := r.cached(region, revision, key); ok {
			return &bpmIndexLoadResult{index: index}, nil
		}
		shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		shared, trace := commandtrace.WithNewTrace(shared)
		data, err := r.store.Get(shared, key)
		var index *BPMIndex
		if err == nil {
			index, err = decodeBPMIndex(data, region, revision, key)
		}
		if err == nil {
			err = shared.Err()
		}
		entry := bpmIndexEntry{revision: revision, key: key, index: index}
		if err != nil {
			entry.index = nil
			entry.retryAt = time.Now().Add(bpmIndexRetryDelay)
		}
		currentRevision, currentKey, currentOK := r.source.BPMIndex(region)
		if currentOK && currentRevision == revision && currentKey == key {
			r.mu.Lock()
			r.entries[region] = entry
			r.mu.Unlock()
		}
		return &bpmIndexLoadResult{index: entry.index, operations: trace.Snapshot().Operations}, err
	})
	select {
	case <-ctx.Done():
		return nil
	case result := <-flight:
		if ctx.Err() != nil {
			return nil
		}
		loaded, _ := result.Val.(*bpmIndexLoadResult)
		loaded.mergeOperations(ctx)
		if result.Err != nil {
			commandtrace.RecordOperation(ctx, "music.bpm_index_unavailable", 0)
			return nil
		}
		return loaded.index
	}
}

func decodeBPMIndex(data []byte, region, revision string, key storage.Key) (*BPMIndex, error) {
	if len(data) > bpmIndexMaxBytes {
		return nil, fmt.Errorf("BPM index exceeds %d bytes", bpmIndexMaxBytes)
	}
	digest := sha256.Sum256(data)
	wantKey := storage.Key("indexes/bpm/v1/" + region + "/" + hex.EncodeToString(digest[:]) + ".json")
	if key != wantKey {
		return nil, fmt.Errorf("BPM index content hash does not match its key")
	}
	var index BPMIndex
	if err := jsonutil.UnmarshalUniqueNames(data, &index); err != nil {
		return nil, fmt.Errorf("decode BPM index: %w", err)
	}
	if index.Region != region || index.ResourceRevision != revision {
		return nil, fmt.Errorf("BPM index resource revision does not match")
	}
	if err := index.validate(); err != nil {
		return nil, err
	}
	// Score prefixes and numeric directories are fixed; only the filename
	// can vary in case. Exact matches win, then lexical spelling resolves
	// collisions deterministically like the resource metadata index.
	keys := make([]string, 0, len(index.Charts))
	for key := range index.Charts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	index.foldedCharts = make(map[string]string, len(keys))
	for _, key := range keys {
		folded := strings.ToLower(key)
		if _, ok := index.foldedCharts[folded]; !ok {
			index.foldedCharts[folded] = key
		}
	}
	return &index, nil
}

func (index *BPMIndex) validate() error {
	if index == nil {
		return fmt.Errorf("BPM index is nil")
	}
	region, err := normalizeBPMIndexRegion(index.Region)
	if err != nil || region != index.Region {
		return fmt.Errorf("invalid BPM index region")
	}
	if index.SchemaVersion != BPMIndexSchemaVersion || !index.Complete || strings.TrimSpace(index.ResourceRevision) == "" {
		return fmt.Errorf("BPM index has unsupported schema or is incomplete")
	}
	if !slices.Equal(index.Prefixes, bpmIndexPrefixes(region)) {
		return fmt.Errorf("BPM index must cover both score prefixes")
	}
	if index.Charts == nil || len(index.Charts) > bpmIndexMaxCharts {
		return fmt.Errorf("invalid BPM index chart count")
	}
	for key, chart := range index.Charts {
		if !bpmIndexScoreKey(region, storage.Key(key)) {
			return fmt.Errorf("BPM index contains an invalid score key")
		}
		if err := chart.validate(); err != nil {
			return err
		}
	}
	return nil
}

func bpmIndexScoreKey(region string, key storage.Key) bool {
	for _, prefix := range bpmIndexPrefixes(region) {
		if !strings.HasPrefix(string(key), prefix) {
			continue
		}
		rest := strings.TrimPrefix(string(key), prefix)
		parts := strings.Split(rest, "/")
		if len(parts) != 2 || !strings.HasSuffix(parts[0], "_01") {
			return false
		}
		id := strings.TrimSuffix(parts[0], "_01")
		if id == "" {
			return false
		}
		for _, c := range id {
			if c < '0' || c > '9' {
				return false
			}
		}
		filename := strings.ToLower(path.Base(rest))
		difficulty := strings.TrimSuffix(filename, ".txt")
		return strings.HasSuffix(filename, ".txt") && difficultyOrder(difficulty) < 99
	}
	return false
}

func (chart BPMIndexChart) validate() error {
	if !finitePositive(chart.MainBPM) || chart.BarCount < 0 || !finiteNonNegative(chart.Duration) || len(chart.Events) == 0 {
		return fmt.Errorf("BPM index contains invalid chart metadata")
	}
	previous := -1.0
	for _, event := range chart.Events {
		if !finiteNonNegative(event.Bar) || event.Bar < previous || !finitePositive(event.BPM) || !finiteNonNegative(event.Duration) {
			return fmt.Errorf("BPM index contains invalid BPM events")
		}
		previous = event.Bar
	}
	return nil
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
func finiteNonNegative(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (chart BPMIndexChart) parsed() *parsedChartBPM {
	parsed := &parsedChartBPM{MainBPM: chart.MainBPM, BarCount: chart.BarCount, Duration: chart.Duration, Events: make([]BPMEvent, len(chart.Events))}
	for i, event := range chart.Events {
		parsed.Events[i] = BPMEvent(event)
	}
	return parsed
}

func indexedChart(parsed *parsedChartBPM) BPMIndexChart {
	chart := BPMIndexChart{MainBPM: parsed.MainBPM, BarCount: parsed.BarCount, Duration: parsed.Duration, Events: make([]BPMIndexEvent, len(parsed.Events))}
	for i, event := range parsed.Events {
		chart.Events[i] = BPMIndexEvent(event)
	}
	return chart
}
