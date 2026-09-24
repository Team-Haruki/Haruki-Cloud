package mysekai

import "context"

// layeredMasterdataSource serves the database first and falls back to the
// local masterdata files for a table the database serves empty or cannot
// serve (a table that exists but has not been ingested yet, or is missing).
// It is only built when the local fallback flag is on; without it the
// database store is used alone and an empty table stays empty.
type layeredMasterdataSource struct {
	primary  masterdataSource
	fallback masterdataSource
}

func newLayeredMasterdataSource(primary, fallback masterdataSource) masterdataSource {
	if primary == nil || !primary.Configured() {
		return fallback
	}
	if fallback == nil || !fallback.Configured() {
		return primary
	}
	return &layeredMasterdataSource{primary: primary, fallback: fallback}
}

func (s *layeredMasterdataSource) Configured() bool {
	return s != nil && s.primary != nil && s.primary.Configured()
}

// loadList queries the primary once: its rows win, else the fallback's,
// else the primary's (served empty or unserved) result is returned as is.
func (s *layeredMasterdataSource) loadList(filename string) []map[string]any {
	if s == nil {
		return nil
	}
	primary := s.primary.loadList(filename)
	if len(primary) > 0 {
		return primary
	}
	if items := s.fallback.loadList(filename); len(items) > 0 {
		return items
	}
	return primary
}

func (s *layeredMasterdataSource) loadMapByID(filename string) map[int]map[string]any {
	if s == nil {
		return map[int]map[string]any{}
	}
	primary := s.primary.loadMapByID(filename)
	if len(primary) > 0 {
		return primary
	}
	if items := s.fallback.loadMapByID(filename); len(items) > 0 {
		return items
	}
	return primary
}

func (s *layeredMasterdataSource) loadObject(filename string, target any) bool {
	if s == nil {
		return false
	}
	return s.primary.loadObject(filename, target) || s.fallback.loadObject(filename, target)
}

func (s *layeredMasterdataSource) WithContext(ctx context.Context) masterdataSource {
	if s == nil {
		return nil
	}
	return &layeredMasterdataSource{
		primary:  bindMasterdataContext(s.primary, ctx),
		fallback: bindMasterdataContext(s.fallback, ctx),
	}
}

func (s *layeredMasterdataSource) resetCache() {
	if s == nil {
		return
	}
	for _, source := range []masterdataSource{s.primary, s.fallback} {
		if resetter, ok := source.(resettableMasterdataSource); ok {
			resetter.resetCache()
		}
	}
}

func (s *layeredMasterdataSource) Close() {
	if s == nil {
		return
	}
	for _, source := range []masterdataSource{s.primary, s.fallback} {
		if closer, ok := source.(interface{ Close() }); ok {
			closer.Close()
		}
	}
}
