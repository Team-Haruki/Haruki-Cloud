package mysekai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type localMasterdataStore struct {
	dir string

	mu       sync.Mutex
	lists    map[string][]map[string]interface{}
	mapsByID map[string]map[int]map[string]interface{}
}

func newLocalMasterdataStore(dir string) *localMasterdataStore {
	return &localMasterdataStore{
		dir:      filepath.Clean(dir),
		lists:    make(map[string][]map[string]interface{}),
		mapsByID: make(map[string]map[int]map[string]interface{}),
	}
}

func (s *localMasterdataStore) Configured() bool {
	return s != nil && s.dir != "" && s.dir != "."
}

func (s *localMasterdataStore) loadList(filename string) []map[string]interface{} {
	if s == nil || !s.Configured() {
		return nil
	}

	s.mu.Lock()
	if cached, ok := s.lists[filename]; ok {
		s.mu.Unlock()
		return cached
	}
	s.mu.Unlock()

	data, err := os.ReadFile(filepath.Join(s.dir, filepath.Clean(filename)))
	if err != nil {
		return nil
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil
	}

	s.mu.Lock()
	s.lists[filename] = items
	s.mu.Unlock()
	return items
}

func (s *localMasterdataStore) loadMapByID(filename string) map[int]map[string]interface{} {
	if s == nil || !s.Configured() {
		return map[int]map[string]interface{}{}
	}

	s.mu.Lock()
	if cached, ok := s.mapsByID[filename]; ok {
		s.mu.Unlock()
		return cached
	}
	s.mu.Unlock()

	items := s.loadList(filename)
	result := make(map[int]map[string]interface{}, len(items))
	for _, item := range items {
		id := intNumber(item["id"], 0)
		if id == 0 {
			continue
		}
		result[id] = item
	}

	s.mu.Lock()
	s.mapsByID[filename] = result
	s.mu.Unlock()
	return result
}

func (s *localMasterdataStore) loadObject(filename string, target interface{}) bool {
	if s == nil || !s.Configured() {
		return false
	}
	data, err := os.ReadFile(filepath.Join(s.dir, filepath.Clean(filename)))
	if err != nil {
		return false
	}
	return json.Unmarshal(data, target) == nil
}
