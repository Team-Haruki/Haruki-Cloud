package drawingcache

import (
	"sort"
	"sync"
	"time"
)

type cacheStatsTracker struct {
	mu        sync.RWMutex
	startedAt time.Time
	totals    cacheStatsCounter
	byPath    map[string]*cacheStatsCounter
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
		byPath:    make(map[string]*cacheStatsCounter),
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
	counter := s.byPath[apiPath]
	if counter == nil {
		counter = &cacheStatsCounter{}
		s.byPath[apiPath] = counter
	}
	apply(counter)
}

func (s *cacheStatsTracker) snapshot(filterPath string) cacheStatsSnapshot {
	filterPath = normalizeCacheStatsPath(filterPath)
	if s == nil {
		return cacheStatsSnapshot{}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := cacheStatsSnapshot{
		StartedAt: s.startedAt,
		Totals:    buildCacheStatsCounterSnapshot(s.totals),
	}

	if filterPath != "" && filterPath != "all" && filterPath != "unknown" {
		if counter := s.byPath[filterPath]; counter != nil {
			snapshot.Paths = []cachePathStatsSnapshot{{
				APIPath:                   filterPath,
				cacheStatsCounterSnapshot: buildCacheStatsCounterSnapshot(*counter),
			}}
		}
		return snapshot
	}

	snapshot.Paths = make([]cachePathStatsSnapshot, 0, len(s.byPath))
	for apiPath, counter := range s.byPath {
		snapshot.Paths = append(snapshot.Paths, cachePathStatsSnapshot{
			APIPath:                   apiPath,
			cacheStatsCounterSnapshot: buildCacheStatsCounterSnapshot(*counter),
		})
	}
	sort.Slice(snapshot.Paths, func(i, j int) bool {
		if snapshot.Paths[i].Lookups != snapshot.Paths[j].Lookups {
			return snapshot.Paths[i].Lookups > snapshot.Paths[j].Lookups
		}
		return snapshot.Paths[i].APIPath < snapshot.Paths[j].APIPath
	})
	return snapshot
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
	apiPath = normalizeAPIPath(apiPath)
	if apiPath == "" {
		return "unknown"
	}
	return apiPath
}
