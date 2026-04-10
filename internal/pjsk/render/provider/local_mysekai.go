package provider

import (
	"encoding/json"
)

// ===========================================================================
// localMySekaiProvider
// ===========================================================================

type localMySekaiProvider struct {
	store *localStore
}

func (p *localMySekaiProvider) Configured() bool {
	return true
}

func (p *localMySekaiProvider) LoadList(filename string) []map[string]any {
	data, err := p.store.readFile(filename)
	if err != nil {
		return nil
	}
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil
	}
	return items
}

func (p *localMySekaiProvider) LoadMapByID(filename string) map[int]map[string]any {
	items := p.LoadList(filename)
	if items == nil {
		return nil
	}
	result := make(map[int]map[string]any, len(items))
	for _, item := range items {
		if id, ok := interfaceToInt(item["id"]); ok {
			result[id] = item
		}
	}
	return result
}

func (p *localMySekaiProvider) LoadObject(filename string, target any) bool {
	data, err := p.store.readFile(filename)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, target) == nil
}
