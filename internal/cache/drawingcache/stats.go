package drawingcache

import (
	"container/list"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	cacheStatsMaxTrackedPaths = 64
	cacheStatsPathUnknown     = "unknown"
	cacheStatsPathOther       = "other"
)

var cacheStatsAllowedDomains = map[string]struct{}{
	"card":      {},
	"chart":     {},
	"costume":   {},
	"deck":      {},
	"education": {},
	"event":     {},
	"gacha":     {},
	"help":      {},
	"honor":     {},
	"inventory": {},
	"misc":      {},
	"music":     {},
	"mysekai":   {},
	"profile":   {},
	"score":     {},
	"sk":        {},
	"stamp":     {},
	"vlive":     {},
}

type cacheStatsTracker struct {
	mu        sync.RWMutex
	startedAt time.Time
	totals    cacheStatsCounter
	byPath    map[string]*cacheStatsPathEntry
	lru       *list.List
	unknown   cacheStatsCounter
	other     cacheStatsCounter
	maxPaths  int
}

type cacheStatsPathEntry struct {
	counter cacheStatsCounter
	element *list.Element
}

type cacheStatsCounter struct {
	Hits         int64 `json:"hits"`
	Misses       int64 `json:"misses"`
	Expired      int64 `json:"expired"`
	MissingFiles int64 `json:"missing_files"`
	Stores       int64 `json:"stores"`
}

type cacheStatsSnapshot struct {
	StartedAt time.Time                 `json:"started_at"`
	Totals    cacheStatsCounterSnapshot `json:"totals"`
	Paths     []cachePathStatsSnapshot  `json:"paths"`
}

type cacheStatsCounterSnapshot struct {
	Hits         int64   `json:"hits"`
	Misses       int64   `json:"misses"`
	Expired      int64   `json:"expired"`
	MissingFiles int64   `json:"missing_files"`
	Stores       int64   `json:"stores"`
	Lookups      int64   `json:"lookups"`
	HitRate      float64 `json:"hit_rate"`
}

type cachePathStatsSnapshot struct {
	APIPath string `json:"api_path"`
	cacheStatsCounterSnapshot
}

type cacheMissReason string

const (
	cacheMissReasonLookup      cacheMissReason = "lookup"
	cacheMissReasonExpired     cacheMissReason = "expired"
	cacheMissReasonMissingFile cacheMissReason = "missing_file"
)

func newCacheStatsTracker(now func() time.Time) *cacheStatsTracker {
	if now == nil {
		now = time.Now
	}
	return &cacheStatsTracker{
		startedAt: now().UTC(),
		byPath:    make(map[string]*cacheStatsPathEntry),
		lru:       list.New(),
		maxPaths:  cacheStatsMaxTrackedPaths,
	}
}

func (s *cacheStatsTracker) recordHit(apiPath string) {
	s.record(apiPath, func(counter *cacheStatsCounter) {
		counter.Hits++
	})
}

func (s *cacheStatsTracker) recordMiss(apiPath string, reason cacheMissReason) {
	s.record(apiPath, func(counter *cacheStatsCounter) {
		counter.Misses++
		switch reason {
		case cacheMissReasonExpired:
			counter.Expired++
		case cacheMissReasonMissingFile:
			counter.MissingFiles++
		}
	})
}

func (s *cacheStatsTracker) recordStore(apiPath string) {
	s.record(apiPath, func(counter *cacheStatsCounter) {
		counter.Stores++
	})
}

func (s *cacheStatsTracker) record(apiPath string, apply func(counter *cacheStatsCounter)) {
	if s == nil || apply == nil {
		return
	}
	apiPath = normalizeCacheStatsPath(apiPath)
	s.mu.Lock()
	defer s.mu.Unlock()
	apply(&s.totals)
	s.ensureInitializedLocked()
	s.recordPathLocked(apiPath, apply)
}

func (s *cacheStatsTracker) ensureInitializedLocked() {
	if s.byPath == nil {
		s.byPath = make(map[string]*cacheStatsPathEntry)
	}
	if s.lru == nil {
		s.lru = list.New()
	}
	if s.maxPaths <= 0 {
		s.maxPaths = cacheStatsMaxTrackedPaths
	}
}

func (s *cacheStatsTracker) recordPathLocked(apiPath string, apply func(counter *cacheStatsCounter)) {
	switch apiPath {
	case cacheStatsPathUnknown:
		apply(&s.unknown)
		return
	case cacheStatsPathOther:
		apply(&s.other)
		return
	}

	if entry := s.byPath[apiPath]; entry != nil {
		apply(&entry.counter)
		s.lru.MoveToFront(entry.element)
		return
	}

	for len(s.byPath) >= s.maxPaths {
		oldest := s.lru.Back()
		if oldest == nil {
			break
		}
		oldPath, _ := oldest.Value.(string)
		oldEntry := s.byPath[oldPath]
		delete(s.byPath, oldPath)
		s.lru.Remove(oldest)
		if oldEntry != nil {
			mergeCacheStatsCounter(&s.other, oldEntry.counter)
		}
	}

	entry := &cacheStatsPathEntry{}
	entry.element = s.lru.PushFront(apiPath)
	apply(&entry.counter)
	s.byPath[apiPath] = entry
}

func (s *cacheStatsTracker) snapshot(filterPath string) cacheStatsSnapshot {
	if s == nil {
		return cacheStatsSnapshot{}
	}
	filterAll := strings.TrimSpace(filterPath) == "" || strings.EqualFold(strings.TrimSpace(filterPath), "all")
	filterPath = normalizeCacheStatsPath(filterPath)

	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := cacheStatsSnapshot{
		StartedAt: s.startedAt,
		Totals:    buildCacheStatsCounterSnapshot(s.totals),
	}

	if !filterAll {
		counter, ok := s.counterForPathLocked(filterPath)
		if ok {
			snapshot.Paths = []cachePathStatsSnapshot{{
				APIPath:                   filterPath,
				cacheStatsCounterSnapshot: buildCacheStatsCounterSnapshot(counter),
			}}
		}
		return snapshot
	}

	snapshot.Paths = make([]cachePathStatsSnapshot, 0, len(s.byPath)+2)
	for apiPath, entry := range s.byPath {
		snapshot.Paths = append(snapshot.Paths, cachePathStatsSnapshot{
			APIPath:                   apiPath,
			cacheStatsCounterSnapshot: buildCacheStatsCounterSnapshot(entry.counter),
		})
	}
	appendReservedCacheStatsPath(&snapshot.Paths, cacheStatsPathUnknown, s.unknown)
	appendReservedCacheStatsPath(&snapshot.Paths, cacheStatsPathOther, s.other)
	sort.Slice(snapshot.Paths, func(i, j int) bool {
		if snapshot.Paths[i].Lookups != snapshot.Paths[j].Lookups {
			return snapshot.Paths[i].Lookups > snapshot.Paths[j].Lookups
		}
		return snapshot.Paths[i].APIPath < snapshot.Paths[j].APIPath
	})
	return snapshot
}

func (s *cacheStatsTracker) counterForPathLocked(apiPath string) (cacheStatsCounter, bool) {
	switch apiPath {
	case cacheStatsPathUnknown:
		return s.unknown, !cacheStatsCounterEmpty(s.unknown)
	case cacheStatsPathOther:
		return s.other, !cacheStatsCounterEmpty(s.other)
	default:
		entry := s.byPath[apiPath]
		if entry == nil {
			return cacheStatsCounter{}, false
		}
		return entry.counter, true
	}
}

func appendReservedCacheStatsPath(paths *[]cachePathStatsSnapshot, apiPath string, counter cacheStatsCounter) {
	if cacheStatsCounterEmpty(counter) {
		return
	}
	*paths = append(*paths, cachePathStatsSnapshot{
		APIPath:                   apiPath,
		cacheStatsCounterSnapshot: buildCacheStatsCounterSnapshot(counter),
	})
}

func cacheStatsCounterEmpty(counter cacheStatsCounter) bool {
	return counter == (cacheStatsCounter{})
}

func mergeCacheStatsCounter(dst *cacheStatsCounter, src cacheStatsCounter) {
	if dst == nil {
		return
	}
	dst.Hits += src.Hits
	dst.Misses += src.Misses
	dst.Expired += src.Expired
	dst.MissingFiles += src.MissingFiles
	dst.Stores += src.Stores
}

func buildCacheStatsCounterSnapshot(counter cacheStatsCounter) cacheStatsCounterSnapshot {
	lookups := counter.Hits + counter.Misses
	hitRate := 0.0
	if lookups > 0 {
		hitRate = float64(counter.Hits) / float64(lookups)
	}
	return cacheStatsCounterSnapshot{
		Hits:         counter.Hits,
		Misses:       counter.Misses,
		Expired:      counter.Expired,
		MissingFiles: counter.MissingFiles,
		Stores:       counter.Stores,
		Lookups:      lookups,
		HitRate:      hitRate,
	}
}

func normalizeCacheStatsPath(apiPath string) string {
	raw := strings.TrimSpace(apiPath)
	if raw == "" {
		return cacheStatsPathUnknown
	}
	if len(raw) > maxCacheAPIPathBytes {
		return cacheStatsPathOther
	}
	if strings.EqualFold(raw, cacheStatsPathUnknown) {
		return cacheStatsPathUnknown
	}
	if strings.EqualFold(raw, cacheStatsPathOther) {
		return cacheStatsPathOther
	}
	apiPath = normalizeAPIPath(apiPath)
	if apiPath == "" {
		return cacheStatsPathOther
	}
	parts := strings.Split(apiPath, "/")
	if len(parts) < 3 || parts[0] != "api" || parts[1] != "pjsk" {
		return cacheStatsPathOther
	}
	if _, allowed := cacheStatsAllowedDomains[parts[2]]; !allowed {
		return cacheStatsPathOther
	}
	return apiPath
}
