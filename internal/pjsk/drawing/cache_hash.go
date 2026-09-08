package drawing

import (
	"math"

	json "haruki-cloud/internal/jsonutil"

	"github.com/mitchellh/hashstructure/v2"
)

// Hash implements hashstructure.Hashable for the normalized JSON subtree.
// The surrounding key struct still uses hashstructure, including its type and
// field names. Keep FormatV2's exact FNV-1/ordering/type semantics so persisted
// cache keys remain valid; non-JSON types use the reference implementation.
type renderCacheHashPayload struct{ value any }

func (p renderCacheHashPayload) Hash() (uint64, error) {
	return hashRenderCacheJSON(p.value)
}

const (
	renderHashOffset uint64 = 14695981039346656037
	renderHashPrime  uint64 = 1099511628211
)

func renderHashWord(seed, word uint64) uint64 {
	for range 8 {
		seed = seed*renderHashPrime ^ uint64(byte(word))
		word >>= 8
	}
	return seed
}

func renderHashString(value string) uint64 {
	hash := renderHashOffset
	for i := 0; i < len(value); i++ {
		hash = hash*renderHashPrime ^ uint64(value[i])
	}
	return hash
}

func renderHashPair(a, b uint64) uint64 {
	return renderHashWord(renderHashWord(renderHashOffset, a), b)
}

func hashRenderCacheJSON(value any) (uint64, error) {
	switch v := value.(type) {
	case nil:
		return renderHashWord(renderHashOffset, 0), nil
	case string:
		return renderHashString(v), nil
	case json.Number:
		return renderHashString(string(v)), nil
	case bool:
		var b uint64
		if v {
			b = 1
		}
		seed := renderHashOffset
		return seed*renderHashPrime ^ b, nil
	case float64:
		return renderHashWord(renderHashOffset, math.Float64bits(v)), nil
	case int:
		return renderHashWord(renderHashOffset, uint64(v)), nil
	case int64:
		return renderHashWord(renderHashOffset, uint64(v)), nil
	case uint64:
		return renderHashWord(renderHashOffset, v), nil
	case map[string]any:
		var hash uint64
		for key, child := range v {
			current, err := hashRenderCacheJSON(child)
			if err != nil {
				return 0, err
			}
			hash ^= renderHashPair(renderHashString(key), current)
		}
		return renderHashWord(renderHashOffset, hash), nil
	case []any:
		var hash uint64
		for _, child := range v {
			current, err := hashRenderCacheJSON(child)
			if err != nil {
				return 0, err
			}
			hash = renderHashPair(hash, current)
		}
		return hash, nil
	default:
		return hashstructure.Hash(value, hashstructure.FormatV2, nil)
	}
}
