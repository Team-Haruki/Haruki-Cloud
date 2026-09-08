package drawing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mitchellh/hashstructure/v2"
	json "haruki-cloud/internal/jsonutil"
)

func randomCacheHashValue(r *rand.Rand, depth int) any {
	if depth > 4 {
		return r.Float64()
	}
	switch r.Intn(8) {
	case 0:
		return nil
	case 1:
		return r.Intn(2) == 0
	case 2:
		return r.Float64() * 1e15
	case 3:
		return int64(r.Uint64())
	case 4:
		return []string{"", "中文", "a.b", "*", "\u0000", "😀"}[r.Intn(6)]
	case 5:
		return json.Number(strconv.FormatUint(r.Uint64(), 10))
	case 6:
		out := make([]any, r.Intn(5))
		for i := range out {
			out[i] = randomCacheHashValue(r, depth+1)
		}
		return out
	default:
		out := make(map[string]any)
		for i := 0; i < r.Intn(8); i++ {
			out[strconv.Itoa(i)] = randomCacheHashValue(r, depth+1)
		}
		return out
	}
}

func TestRenderCacheJSONHashMatchesFormatV2(t *testing.T) {
	values := []any{
		nil, true, false, int(0), int(-1), int64(math.MinInt64), uint64(math.MaxUint64),
		float64(0), math.Copysign(0, -1), math.Inf(1), math.NaN(),
		"", "中文😀", json.Number("9007199254740993"),
		map[string]any(nil), map[string]any{}, []any(nil), []any{},
		map[string]any{"x": []any{nil, 1, int64(3), float64(3), json.Number("3")}},
		int8(-5), uint32(15), float32(1.5), []int{1, 2}, map[int]string{2: "two"},
		time.Unix(1710000000, 0), new(int), make(chan int),
	}
	r := rand.New(rand.NewSource(93571))
	for range 2000 {
		values = append(values, randomCacheHashValue(r, 0))
	}
	for i, value := range values {
		want, werr := hashstructure.Hash(value, hashstructure.FormatV2, nil)
		got, gerr := hashRenderCacheJSON(value)
		if (werr == nil) != (gerr == nil) || got != want {
			t.Fatalf("case %d (%T) hash=%x err=%v, want %x err=%v", i, value, got, gerr, want, werr)
		}
		if werr != nil && werr.Error() != gerr.Error() {
			t.Fatalf("fallback error changed: %v vs %v", werr, gerr)
		}
	}
}

func TestFusedRenderCacheSanitizationMatchesReference(t *testing.T) {
	r := rand.New(rand.NewSource(18564))
	paths := []string{"/unknown"}
	for path := range renderCacheRules {
		paths = append(paths, path)
	}
	for _, endpoint := range paths {
		for range 20 {
			input := map[string]any{
				"dt": int64(1710000000000), "region": "TW", "timezone": "Asia/Tokyo",
				"update_time": float64(1710000001234),
				"user_info":   map[string]any{"id": "123", "update_time": "1710000001", "data": randomCacheHashValue(r, 0)},
				"profile":     map[string]any{"id": "123", "update_time": int64(1710000001)},
				"cards":       []any{map[string]any{"dt": 123, "x": randomCacheHashValue(r, 0)}},
				"random":      randomCacheHashValue(r, 0),
			}
			rule := adjustRenderCacheRuleForPayload(endpoint, input, resolveRenderCacheRule(endpoint))
			want := referenceCloneCachePayload(input)
			normalizeRenderCacheNode(want, nil, rule)
			got := cloneSanitizedRenderCacheNode(input, nil, rule)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s fused sanitizer changed output", endpoint)
			}
			if _, ok := input["dt"]; !ok {
				t.Fatal("sanitizer mutated original")
			}
			got.(map[string]any)["cards"].([]any)[0].(map[string]any)["new"] = "changed"
			if _, ok := input["cards"].([]any)[0].(map[string]any)["new"]; ok {
				t.Fatal("retained subtree aliases original")
			}
		}
	}
}

func referenceRenderCacheKey(policy renderCachePolicy) (string, error) {
	version := renderCacheKeyVersion
	if strings.TrimSpace(policy.APIPath) == "api/pjsk/event/list" {
		version = renderCacheEventListKeyVersion
	}
	hash, err := hashstructure.Hash(renderCacheKeyMaterial{Version: version, Endpoint: policy.Endpoint, APIPath: policy.APIPath, UserID: policy.UserID, Params: policy.Params}, hashstructure.FormatV2, nil)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(strconv.FormatUint(hash, 10)))
	return hex.EncodeToString(digest[:]), nil
}

func TestRenderCachePipelineKeepsPersistedKeys(t *testing.T) {
	for _, tc := range cacheKeyBenchmarkRequests() {
		for _, tz := range []string{"Asia/Tokyo", "Asia/Shanghai"} {
			prepared := prepareDrawingRequestBody(tc.endpoint, tc.request, time.Unix(1710000000, 0), context.Background())
			prepared.(map[string]any)["timezone"] = tz
			policy, err := buildRenderCachePolicy(tc.endpoint, preparedRenderCachePayload{payload: prepared})
			if err != nil {
				t.Fatal(err)
			}
			oldPayload := referenceCloneCachePayload(prepared)
			oldRule := sanitizeRenderCachePayload(tc.endpoint, oldPayload)
			if !reflect.DeepEqual(policy.Params, oldPayload) || policy.TTL != oldRule.TTL || policy.Infinite != oldRule.Infinite {
				t.Fatal("policy changed")
			}
			want, err := referenceRenderCacheKey(policy)
			if err != nil {
				t.Fatal(err)
			}
			got, err := buildRenderCacheKey(policy)
			if err != nil || got != want {
				t.Fatalf("%s key=%s err=%v want=%s", tc.name, got, err, want)
			}
		}
	}
}
func referenceCloneCachePayload(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, child := range typed {
			cloned[key] = referenceCloneCachePayload(child)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, child := range typed {
			cloned[index] = referenceCloneCachePayload(child)
		}
		return cloned
	default:
		return value
	}
}
