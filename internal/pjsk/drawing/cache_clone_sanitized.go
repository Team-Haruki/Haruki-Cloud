package drawing

// Clone and sanitize in one traversal. Ignored subtrees are never cloned, and
// retained maps/slices remain owned by the policy: render preparation hooks
// may modify the original request after the cache key has been computed.
func cloneSanitizedRenderCacheNode(node any, path []string, rule renderCacheRule) any {
	switch value := node.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(value))
		for key, child := range value {
			childPath := append(path, key)
			if !shouldIgnoreRenderCachePath(rule, childPath) {
				cloned[key] = cloneSanitizedRenderCacheNode(child, childPath, rule)
			}
		}
		return cloned
	case []any:
		cloned := make([]any, len(value))
		for i, child := range value {
			cloned[i] = cloneSanitizedRenderCacheNode(child, append(path, "*"), rule)
		}
		return cloned
	default:
		if bucket := renderCacheBucketFor(rule, path); bucket > 0 {
			if bucketed, ok := bucketRenderCacheValue(value, bucket); ok {
				return bucketed
			}
		}
		return value
	}
}
