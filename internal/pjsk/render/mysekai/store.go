package mysekai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type localMasterdataStore struct {
	dirs []string

	mu       sync.Mutex
	lists    map[string][]map[string]any
	mapsByID map[string]map[int]map[string]any
}

func newLocalMasterdataStore(dirs ...string) *localMasterdataStore {
	cleaned := make([]string, 0, len(dirs))
	seen := make(map[string]struct{}, len(dirs))
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		dir = filepath.Clean(dir)
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		cleaned = append(cleaned, dir)
	}

	return &localMasterdataStore{
		dirs:     cleaned,
		lists:    make(map[string][]map[string]any),
		mapsByID: make(map[string]map[int]map[string]any),
	}
}

func (s *localMasterdataStore) Configured() bool {
	return s != nil && len(s.dirs) > 0
}

func (s *localMasterdataStore) loadList(filename string) []map[string]any {
	if s == nil || !s.Configured() {
		return nil
	}

	s.mu.Lock()
	if cached, ok := s.lists[filename]; ok {
		s.mu.Unlock()
		return cached
	}
	s.mu.Unlock()

	var data []byte
	found := false
	for _, dir := range s.dirs {
		content, err := os.ReadFile(filepath.Join(dir, filepath.Clean(filename)))
		if err != nil {
			continue
		}
		data = content
		found = true
		break
	}
	if !found {
		return nil
	}
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil
	}

	s.mu.Lock()
	s.lists[filename] = items
	s.mu.Unlock()
	return items
}

func (s *localMasterdataStore) loadMapByID(filename string) map[int]map[string]any {
	if s == nil || !s.Configured() {
		return map[int]map[string]any{}
	}

	s.mu.Lock()
	if cached, ok := s.mapsByID[filename]; ok {
		s.mu.Unlock()
		return cached
	}
	s.mu.Unlock()

	items := s.loadList(filename)
	result := make(map[int]map[string]any, len(items))
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

func (s *localMasterdataStore) loadObject(filename string, target any) bool {
	if s == nil || !s.Configured() {
		return false
	}

	for _, dir := range s.dirs {
		data, err := os.ReadFile(filepath.Join(dir, filepath.Clean(filename)))
		if err != nil {
			continue
		}
		return json.Unmarshal(data, target) == nil
	}
	return false
}
