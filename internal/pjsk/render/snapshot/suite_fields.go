package snapshot

import (
	"fmt"
	"slices"
	"strings"
)

// The field set is also the cache namespace: the same upload_time does not
// make two different projections interchangeable.
func normalizeSuiteFields(fields []string) ([]string, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(fields)+2)
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" || strings.ContainsAny(field, ",.$") {
			return nil, fmt.Errorf("snapshot: invalid Suite field %q", field)
		}
		normalized = append(normalized, field)
	}
	// Identity is required by the snapshot factory. The timestamp allows one-hop
	// conditional reads and guarantees an object response (single-key reads do not).
	normalized = append(normalized, "userGamedata", "upload_time")
	slices.Sort(normalized)
	return slices.Compact(normalized), nil
}
