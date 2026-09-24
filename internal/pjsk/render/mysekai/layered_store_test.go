package mysekai

import (
	"context"
	"testing"
)

type layeredTestContextKey string

type layeredTestSource struct {
	lists     map[string][]map[string]any
	maps      map[string]map[int]map[string]any
	object    bool
	resets    int
	closes    int
	listCalls int
	mapCalls  int
	ctx       context.Context
}

func (s *layeredTestSource) Configured() bool { return s != nil }
func (s *layeredTestSource) loadList(filename string) []map[string]any {
	s.listCalls++
	return s.lists[filename]
}
func (s *layeredTestSource) loadMapByID(filename string) map[int]map[string]any {
	s.mapCalls++
	if rows, ok := s.maps[filename]; ok {
		return rows
	}
	return map[int]map[string]any{}
}
func (s *layeredTestSource) loadObject(string, any) bool { return s.object }
func (s *layeredTestSource) WithContext(ctx context.Context) masterdataSource {
	clone := *s
	clone.ctx = ctx
	return &clone
}
func (s *layeredTestSource) resetCache() { s.resets++ }
func (s *layeredTestSource) Close()      { s.closes++ }

func TestLayeredMasterdataSourcePrefersDatabaseAndFallsBackWhenEmpty(t *testing.T) {
	primary := &layeredTestSource{
		lists: map[string][]map[string]any{"mysekaiGates.json": {{"id": 1, "assetbundleName": "db"}}, "mysekaiGateSkins.json": {}},
		maps:  map[string]map[int]map[string]any{"mysekaiGates.json": {1: {"assetbundleName": "db"}}, "mysekaiGateSkins.json": {}},
	}
	fallback := &layeredTestSource{
		lists:  map[string][]map[string]any{"mysekaiGates.json": {{"id": 1, "assetbundleName": "local"}}, "mysekaiGateSkins.json": {{"id": 5, "mysekaiGateSkinType": "unit", "mysekaiGateSkinTypeId": 2}}},
		maps:   map[string]map[int]map[string]any{"mysekaiGates.json": {1: {"assetbundleName": "local"}}, "mysekaiGateSkins.json": {5: {"mysekaiGateSkinType": "unit", "mysekaiGateSkinTypeId": 2}}},
		object: true,
	}
	source := newLayeredMasterdataSource(primary, fallback)
	if _, ok := source.(*layeredMasterdataSource); !ok {
		t.Fatalf("layered source type = %T", source)
	}
	if got := stringValue(source.loadMapByID("mysekaiGates.json")[1]["assetbundleName"]); got != "db" {
		t.Fatalf("database rows must win: %q", got)
	}
	if got := stringValue(source.loadMapByID("mysekaiGateSkins.json")[5]["mysekaiGateSkinType"]); got != "unit" {
		t.Fatalf("empty database table must fall back to local: %q", got)
	}
	if got := source.loadList("mysekaiGateSkins.json"); len(got) != 1 {
		t.Fatalf("empty database list must fall back to local: %+v", got)
	}
	if got := source.loadList("missing.json"); got != nil {
		t.Fatalf("unserved list = %+v", got)
	}
	if !source.loadObject("fixture_reaction_data.json", nil) {
		t.Fatal("object fallback was not consulted")
	}

	bound := bindMasterdataContext(source, context.WithValue(context.Background(), layeredTestContextKey("k"), "x")).(*layeredMasterdataSource)
	if bound.primary.(*layeredTestSource).ctx == nil || bound.fallback.(*layeredTestSource).ctx == nil {
		t.Fatal("WithContext did not bind both layers")
	}
	source.(resettableMasterdataSource).resetCache()
	source.(interface{ Close() }).Close()
	if primary.resets != 1 || fallback.resets != 1 || primary.closes != 1 || fallback.closes != 1 {
		t.Fatalf("reset/close fan-out = primary(%d,%d) fallback(%d,%d)", primary.resets, primary.closes, fallback.resets, fallback.closes)
	}

	if got := newLayeredMasterdataSource(primary, nil); got != primary {
		t.Fatalf("without a fallback the primary is used directly: %T", got)
	}
	if got := newLayeredMasterdataSource(nil, fallback); got != fallback {
		t.Fatalf("without a primary the fallback is used directly: %T", got)
	}
}

func TestLayeredMasterdataSourceQueriesPrimaryOnceOnMiss(t *testing.T) {
	primary := &layeredTestSource{
		lists: map[string][]map[string]any{"mysekaiGateSkins.json": {}},
		maps:  map[string]map[int]map[string]any{"mysekaiGateSkins.json": {}},
	}
	fallback := &layeredTestSource{}
	source := newLayeredMasterdataSource(primary, fallback)

	if got := source.loadList("mysekaiGateSkins.json"); got == nil || len(got) != 0 {
		t.Fatalf("served empty table must stay served: %#v", got)
	}
	if got := source.loadList("missing.json"); got != nil {
		t.Fatalf("unserved table = %#v", got)
	}
	if got := source.loadMapByID("mysekaiGateSkins.json"); len(got) != 0 {
		t.Fatalf("served empty map = %#v", got)
	}
	if got := source.loadMapByID("missing.json"); len(got) != 0 {
		t.Fatalf("unserved map = %#v", got)
	}
	if primary.listCalls != 2 || primary.mapCalls != 2 {
		t.Fatalf("primary was queried list=%d map=%d times on misses, want once per lookup", primary.listCalls, primary.mapCalls)
	}
	if fallback.listCalls != 2 || fallback.mapCalls != 2 {
		t.Fatalf("fallback was queried list=%d map=%d times on misses, want once per lookup", fallback.listCalls, fallback.mapCalls)
	}
}

func TestResolverBuildsLayeredSourceOnlyWithFallbackFlag(t *testing.T) {
	root := t.TempDir()
	withFlag := newMasterdataResolver(MasterdataOptions{LocalDir: root, AllowFallback: true})
	if _, ok := withFlag.build(context.Background(), "jp").(*localMasterdataStore); !ok {
		t.Fatalf("no DSN with fallback should build a local store")
	}
	withoutFlag := newMasterdataResolver(MasterdataOptions{LocalDir: root})
	if got := withoutFlag.build(context.Background(), "jp"); got != nil {
		t.Fatalf("no DSN without fallback must build nothing, got %T", got)
	}
}
