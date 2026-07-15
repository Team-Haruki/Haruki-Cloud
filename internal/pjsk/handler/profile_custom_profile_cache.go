package handler

import (
	"container/list"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"

	json "github.com/bytedance/sonic"
	"golang.org/x/sync/singleflight"
)

const (
	customProfileMasterMaxFileBytes   = 64 << 20
	customProfileMasterCacheMaxItems  = 64
	customProfileMasterCacheMaxCharge = 256 << 20
	customProfileMasterMapChargeRatio = 4
)

type customProfileMasterFileID struct {
	size    int64
	modTime int64
}

type customProfileMasterCacheEntry struct {
	path   string
	fileID customProfileMasterFileID
	index  map[int]map[string]any
	charge int64
	elem   *list.Element
}

type customProfileMasterIndexCache struct {
	mu      sync.Mutex
	entries map[string]*customProfileMasterCacheEntry
	lru     *list.List
	charge  int64
	flight  singleflight.Group
}

var customProfileMasterCache = newCustomProfileMasterIndexCache()

func newCustomProfileMasterIndexCache() *customProfileMasterIndexCache {
	return &customProfileMasterIndexCache{
		entries: make(map[string]*customProfileMasterCacheEntry),
		lru:     list.New(),
	}
}

func customProfileMasterFileIdentity(info os.FileInfo) customProfileMasterFileID {
	return customProfileMasterFileID{size: info.Size(), modTime: info.ModTime().UnixNano()}
}

func (c *customProfileMasterIndexCache) get(path string, fileID customProfileMasterFileID) (map[int]map[string]any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[path]
	if entry == nil {
		return nil, false
	}
	if entry.fileID != fileID {
		c.removeLocked(entry)
		return nil, false
	}
	c.lru.MoveToFront(entry.elem)
	return entry.index, true
}

func (c *customProfileMasterIndexCache) put(path string, fileID customProfileMasterFileID, index map[int]map[string]any, sourceBytes int) {
	charge := int64(sourceBytes) * customProfileMasterMapChargeRatio
	if charge <= 0 || charge > customProfileMasterCacheMaxCharge {
		return
	}
	current, err := os.Stat(path)
	if err != nil || customProfileMasterFileIdentity(current) != fileID {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing := c.entries[path]; existing != nil {
		if existing.fileID != fileID && existing.fileID.modTime > fileID.modTime {
			return
		}
		c.removeLocked(existing)
	}
	entry := &customProfileMasterCacheEntry{
		path: path, fileID: fileID, index: index, charge: charge,
	}
	entry.elem = c.lru.PushFront(entry)
	c.entries[path] = entry
	c.charge += charge
	for len(c.entries) > customProfileMasterCacheMaxItems || c.charge > customProfileMasterCacheMaxCharge {
		back := c.lru.Back()
		if back == nil {
			break
		}
		oldest, _ := back.Value.(*customProfileMasterCacheEntry)
		if oldest == nil {
			c.lru.Remove(back)
			continue
		}
		c.removeLocked(oldest)
	}
}

func (c *customProfileMasterIndexCache) removeLocked(entry *customProfileMasterCacheEntry) {
	if entry == nil {
		return
	}
	delete(c.entries, entry.path)
	if entry.elem != nil {
		c.lru.Remove(entry.elem)
	}
	c.charge -= entry.charge
	if c.charge < 0 {
		c.charge = 0
	}
}

func (c *customProfileMasterIndexCache) load(path string, info os.FileInfo) (map[int]map[string]any, bool, error) {
	fileID := customProfileMasterFileIdentity(info)
	if index, ok := c.get(path, fileID); ok {
		return index, true, nil
	}
	flightKey := path + ":" + strconv.FormatInt(fileID.modTime, 10) + ":" + strconv.FormatInt(fileID.size, 10)
	value, err, _ := c.flight.Do(flightKey, func() (any, error) {
		if index, ok := c.get(path, fileID); ok {
			return index, nil
		}
		if fileID.size < 0 || fileID.size > customProfileMasterMaxFileBytes {
			return nil, fmt.Errorf("custom profile masterdata exceeds %d bytes", customProfileMasterMaxFileBytes)
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, customProfileMasterMaxFileBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > customProfileMasterMaxFileBytes {
			return nil, fmt.Errorf("custom profile masterdata exceeds %d bytes", customProfileMasterMaxFileBytes)
		}
		var rows []map[string]any
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, err
		}
		index := make(map[int]map[string]any, len(rows))
		for _, row := range rows {
			if id, ok := mapInt(row, "id"); ok {
				index[id] = row
			}
		}
		c.put(path, fileID, index, len(data))
		return index, nil
	})
	if err != nil {
		return nil, false, err
	}
	index, _ := value.(map[int]map[string]any)
	return index, false, nil
}

func cloneCustomProfileMasterValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, nested := range item {
			result[key] = cloneCustomProfileMasterValue(nested)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for i, nested := range item {
			result[i] = cloneCustomProfileMasterValue(nested)
		}
		return result
	case []float64:
		return append([]float64(nil), item...)
	case []string:
		return append([]string(nil), item...)
	default:
		return item
	}
}

func cloneCustomProfileMasterRow(row map[string]any) map[string]any {
	cloned, _ := cloneCustomProfileMasterValue(row).(map[string]any)
	return cloned
}
