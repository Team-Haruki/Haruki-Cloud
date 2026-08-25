package meta

import (
	"fmt"
	"slices"
	"strings"

	json "haruki-cloud/internal/jsonutil"
)

// View is an immutable, parsed music metadata snapshot. A Loader replaces the
// whole View when a region is refreshed, so readers can retain a pointer
// without locking and will always observe a self-consistent generation.
type View struct {
	entries []Entry
	byMusic map[int][]int
	byKey   map[musicMetaKey]int
}

// Entry exposes read-only access to one music metadata row. Methods returning
// composite values clone them so callers cannot mutate the shared View.
type Entry struct {
	row        map[string]any
	musicID    int
	difficulty string
}

type musicMetaKey struct {
	musicID    int
	difficulty string
}

// Prepare parses a payload once, injects the synthetic omakase rows when
// needed, and returns both the processed bytes and their immutable index.
func Prepare(data []byte) ([]byte, *View, error) {
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, nil, fmt.Errorf("decode music metadata: %w", err)
	}

	injected := injectOmakaseRows(&rows)
	processed := slices.Clone(data)
	if injected {
		encoded, err := json.Marshal(rows)
		if err != nil {
			return nil, nil, fmt.Errorf("encode music metadata: %w", err)
		}
		processed = encoded
	}
	return processed, newView(rows), nil
}

// Parse builds an immutable index without rewriting the payload.
func Parse(data []byte) (*View, error) {
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("decode music metadata: %w", err)
	}
	return newView(rows), nil
}

func newView(rows []map[string]any) *View {
	view := &View{
		entries: make([]Entry, 0, len(rows)),
		byMusic: make(map[int][]int),
		byKey:   make(map[musicMetaKey]int),
	}
	for _, row := range rows {
		musicID := metaInt(row["music_id"])
		difficulty := normalizeMetaDifficulty(metaString(row["difficulty"]))
		entry := Entry{row: row, musicID: musicID, difficulty: difficulty}
		index := len(view.entries)
		view.entries = append(view.entries, entry)
		if musicID <= 0 {
			continue
		}
		view.byMusic[musicID] = append(view.byMusic[musicID], index)
		if difficulty != "" {
			key := musicMetaKey{musicID: musicID, difficulty: difficulty}
			if _, exists := view.byKey[key]; !exists {
				view.byKey[key] = index
			}
		}
	}
	return view
}

// Find returns one indexed row without scanning the full payload.
func (v *View) Find(musicID int, difficulty string) (Entry, bool) {
	if v == nil || musicID <= 0 {
		return Entry{}, false
	}
	index, ok := v.byKey[musicMetaKey{
		musicID:    musicID,
		difficulty: normalizeMetaDifficulty(difficulty),
	}]
	if !ok {
		return Entry{}, false
	}
	return v.entries[index], true
}

// FindAll returns lightweight read-only handles for one music ID.
func (v *View) FindAll(musicID int) []Entry {
	if v == nil || musicID <= 0 {
		return nil
	}
	indexes := v.byMusic[musicID]
	entries := make([]Entry, 0, len(indexes))
	for _, index := range indexes {
		entries = append(entries, v.entries[index])
	}
	return entries
}

// Range visits the immutable entries without allocating a full row slice.
func (v *View) Range(visit func(Entry) bool) {
	if v == nil || visit == nil {
		return
	}
	for _, entry := range v.entries {
		if !visit(entry) {
			return
		}
	}
}

func (e Entry) MusicID() int {
	return e.musicID
}

func (e Entry) Difficulty() string {
	return e.difficulty
}

func (e Entry) String(key string) string {
	return metaString(e.row[key])
}

func (e Entry) Float(key string) float64 {
	return metaFloat(e.row[key])
}

func (e Entry) Int(key string) int {
	return int(e.Float(key))
}

func (e Entry) FloatSlice(key string) []float64 {
	switch values := e.row[key].(type) {
	case []float64:
		return slices.Clone(values)
	case []any:
		result := make([]float64, 0, len(values))
		for _, value := range values {
			result = append(result, metaFloat(value))
		}
		return result
	default:
		return nil
	}
}

// Map returns a deep copy suitable for mutation or serialization by callers.
func (e Entry) Map() map[string]any {
	cloned, _ := cloneMetaValue(e.row).(map[string]any)
	return cloned
}

func cloneMetaValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, nested := range item {
			result[key] = cloneMetaValue(nested)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for i, nested := range item {
			result[i] = cloneMetaValue(nested)
		}
		return result
	case []float64:
		return slices.Clone(item)
	case []string:
		return slices.Clone(item)
	default:
		return item
	}
}

func normalizeMetaDifficulty(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func metaString(value any) string {
	text, _ := value.(string)
	return text
}

func metaFloat(value any) float64 {
	switch item := value.(type) {
	case float64:
		return item
	case float32:
		return float64(item)
	case int:
		return float64(item)
	case int64:
		return float64(item)
	default:
		return 0
	}
}

func metaInt(value any) int {
	return int(metaFloat(value))
}
