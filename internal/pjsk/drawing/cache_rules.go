package drawing

import (
	"strconv"
	"strings"
	"time"
)

type renderCacheRule struct {
	Enabled          bool
	TTL              time.Duration
	IgnoreFieldNames map[string]struct{}
	IgnorePaths      map[string]struct{}
	BucketFieldNames map[string]time.Duration
	BucketPaths      map[string]time.Duration
}

var (
	defaultRenderCacheRule = renderCacheRule{
		Enabled:          true,
		IgnoreFieldNames: renderCacheStringSet("dt"),
	}

	skRenderCacheBucket = 10 * time.Second

	renderCacheRules = map[string]renderCacheRule{
		"/api/pjsk/deck/recommend": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("model_name", "cost_times", "wait_times", "profile.update_time"),
		},
		"/api/pjsk/event/record": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("user_info.update_time"),
		},
		"/api/pjsk/music/list": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.update_time"),
		},
		"/api/pjsk/music/progress": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.data_sources.*.update_time"),
		},
		"/api/pjsk/music/rewards/detail": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.data_sources.*.update_time"),
		},
		"/api/pjsk/music/rewards/basic": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.data_sources.*.update_time"),
		},
		"/api/pjsk/education/challenge-live": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.update_time", "profile.data_sources.*.update_time"),
		},
		"/api/pjsk/education/power-bonus": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.update_time", "profile.data_sources.*.update_time"),
		},
		"/api/pjsk/education/area-item": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.update_time", "profile.data_sources.*.update_time"),
		},
		"/api/pjsk/education/bonds": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.update_time", "profile.data_sources.*.update_time"),
		},
		"/api/pjsk/education/leader-count": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.update_time", "profile.data_sources.*.update_time"),
		},
		"/api/pjsk/mysekai/resource": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.data_sources.*.update_time"),
		},
		"/api/pjsk/mysekai/fixture-list": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.data_sources.*.update_time"),
		},
		"/api/pjsk/mysekai/door-upgrade": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.data_sources.*.update_time"),
		},
		"/api/pjsk/mysekai/music-record": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.data_sources.*.update_time"),
		},
		"/api/pjsk/mysekai/talk-list": {
			Enabled:     true,
			IgnorePaths: renderCacheStringSet("profile.data_sources.*.update_time"),
		},
		"/api/pjsk/sk/line": {
			Enabled:          true,
			TTL:              skRenderCacheBucket,
			BucketFieldNames: renderCacheBucketSet(skRenderCacheBucket, "dt", "aggregate_at", "time", "record_start_at", "forecast_time", "update_time"),
		},
		"/api/pjsk/sk/query": {
			Enabled:          true,
			TTL:              skRenderCacheBucket,
			BucketFieldNames: renderCacheBucketSet(skRenderCacheBucket, "dt", "aggregate_at", "time", "record_start_at"),
		},
		"/api/pjsk/sk/check-room": {
			Enabled:          true,
			TTL:              skRenderCacheBucket,
			BucketFieldNames: renderCacheBucketSet(skRenderCacheBucket, "dt", "aggregate_at", "update_at", "time", "record_start_at"),
		},
		"/api/pjsk/sk/speed": {
			Enabled:          true,
			TTL:              skRenderCacheBucket,
			BucketFieldNames: renderCacheBucketSet(skRenderCacheBucket, "dt", "event_aggregate_at", "record_time"),
		},
		"/api/pjsk/sk/player-trace": {
			Enabled:          true,
			TTL:              skRenderCacheBucket,
			BucketFieldNames: renderCacheBucketSet(skRenderCacheBucket, "dt", "time", "record_start_at"),
		},
		"/api/pjsk/sk/rank-trace": {
			Enabled:          true,
			TTL:              skRenderCacheBucket,
			BucketFieldNames: renderCacheBucketSet(skRenderCacheBucket, "dt", "time", "record_start_at"),
		},
		"/api/pjsk/sk/winrate": {
			Enabled:          true,
			TTL:              skRenderCacheBucket,
			BucketFieldNames: renderCacheBucketSet(skRenderCacheBucket, "dt", "updated_at", "event_aggregate_at"),
		},
	}
)

func resolveRenderCacheRule(endpointPath string) renderCacheRule {
	rule := cloneRenderCacheRule(defaultRenderCacheRule)
	if specific, ok := renderCacheRules[strings.TrimSpace(endpointPath)]; ok {
		rule = mergeRenderCacheRule(rule, specific)
	}
	if !rule.Enabled {
		rule.Enabled = true
	}
	return rule
}

func cloneRenderCacheRule(rule renderCacheRule) renderCacheRule {
	return renderCacheRule{
		Enabled:          rule.Enabled,
		TTL:              rule.TTL,
		IgnoreFieldNames: cloneRenderCacheStringSet(rule.IgnoreFieldNames),
		IgnorePaths:      cloneRenderCacheStringSet(rule.IgnorePaths),
		BucketFieldNames: cloneRenderCacheBucketMap(rule.BucketFieldNames),
		BucketPaths:      cloneRenderCacheBucketMap(rule.BucketPaths),
	}
}

func mergeRenderCacheRule(base renderCacheRule, override renderCacheRule) renderCacheRule {
	merged := cloneRenderCacheRule(base)
	if override.Enabled {
		merged.Enabled = true
	}
	if override.TTL > 0 {
		merged.TTL = override.TTL
	}
	for key := range override.IgnoreFieldNames {
		merged.IgnoreFieldNames[key] = struct{}{}
	}
	for key := range override.IgnorePaths {
		merged.IgnorePaths[key] = struct{}{}
	}
	for key, bucket := range override.BucketFieldNames {
		merged.BucketFieldNames[key] = bucket
		delete(merged.IgnoreFieldNames, key)
	}
	for key, bucket := range override.BucketPaths {
		merged.BucketPaths[key] = bucket
		delete(merged.IgnorePaths, key)
	}
	return merged
}

func cloneRenderCacheStringSet(input map[string]struct{}) map[string]struct{} {
	if len(input) == 0 {
		return map[string]struct{}{}
	}
	cloned := make(map[string]struct{}, len(input))
	for key := range input {
		cloned[key] = struct{}{}
	}
	return cloned
}

func cloneRenderCacheBucketMap(input map[string]time.Duration) map[string]time.Duration {
	if len(input) == 0 {
		return map[string]time.Duration{}
	}
	cloned := make(map[string]time.Duration, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func renderCacheStringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return set
}

func renderCacheBucketSet(bucket time.Duration, values ...string) map[string]time.Duration {
	set := make(map[string]time.Duration, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = bucket
	}
	return set
}

func sanitizeRenderCachePayload(endpointPath string, payload any) renderCacheRule {
	rule := resolveRenderCacheRule(endpointPath)
	if !rule.Enabled {
		return rule
	}
	normalizeRenderCacheNode(payload, nil, rule)
	return rule
}

func normalizeRenderCacheNode(node any, path []string, rule renderCacheRule) any {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			childPath := appendRenderCachePath(path, key)
			if shouldIgnoreRenderCachePath(rule, childPath) {
				delete(value, key)
				continue
			}
			value[key] = normalizeRenderCacheNode(child, childPath, rule)
		}
		return value
	case []any:
		for idx, child := range value {
			value[idx] = normalizeRenderCacheNode(child, appendRenderCachePath(path, "*"), rule)
		}
		return value
	default:
		if bucket := renderCacheBucketFor(rule, path); bucket > 0 {
			if bucketed, ok := bucketRenderCacheValue(value, bucket); ok {
				return bucketed
			}
		}
		return value
	}
}

func appendRenderCachePath(path []string, segment string) []string {
	next := make([]string, 0, len(path)+1)
	next = append(next, path...)
	next = append(next, segment)
	return next
}

func shouldIgnoreRenderCachePath(rule renderCacheRule, path []string) bool {
	pathKey := renderCachePathKey(path)
	if _, ok := rule.BucketPaths[pathKey]; ok {
		return false
	}
	if _, ok := rule.IgnorePaths[pathKey]; ok {
		return true
	}
	fieldName := renderCacheFieldName(path)
	if _, ok := rule.BucketFieldNames[fieldName]; ok {
		return false
	}
	_, ok := rule.IgnoreFieldNames[fieldName]
	return ok
}

func renderCacheBucketFor(rule renderCacheRule, path []string) time.Duration {
	pathKey := renderCachePathKey(path)
	if bucket, ok := rule.BucketPaths[pathKey]; ok {
		return bucket
	}
	return rule.BucketFieldNames[renderCacheFieldName(path)]
}

func renderCachePathKey(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return strings.Join(path, ".")
}

func renderCacheFieldName(path []string) string {
	for idx := len(path) - 1; idx >= 0; idx-- {
		if path[idx] != "*" {
			return path[idx]
		}
	}
	return ""
}

func bucketRenderCacheValue(value any, bucket time.Duration) (any, bool) {
	if bucket <= 0 {
		return nil, false
	}

	normalized := normalizeDrawingUpdateTime(value, 0)
	if normalized <= 0 {
		return nil, false
	}

	bucketMs := bucket.Milliseconds()
	if bucketMs <= 0 {
		return nil, false
	}

	bucketed := (normalized / bucketMs) * bucketMs
	switch value.(type) {
	case string:
		return strconv.FormatInt(bucketed, 10), true
	default:
		return bucketed, true
	}
}
