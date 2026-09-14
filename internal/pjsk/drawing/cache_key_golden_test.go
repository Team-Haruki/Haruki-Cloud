package drawing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mitchellh/hashstructure/v2"

	"haruki-cloud/internal/pjsk/displaytime"
)

// The render-cache key pipeline (cache_helpers.go, cache_hash.go,
// cache_clone_sanitized.go, sonar_constants.go, cache_types.go:79-94,
// request_dt.go:14 and the rule table in cache_rules.go) decides the keys
// already persisted in the image cache index. These golden values were
// computed from the tree before the storage abstraction work started and
// must never change: a diff here means every stored render-cache key would
// be invalidated.

func renderCacheKeyGoldenFixtures() []struct {
	name, endpoint string
	request        any
} {
	fixtures := cacheKeyBenchmarkRequests()
	startAt := time.Unix(1710000000, 0).Add(-48 * time.Hour).UnixMilli()
	endAt := time.Unix(1710000000, 0).Add(72 * time.Hour).UnixMilli()
	eventList := &EventListRequest{EventInfo: []EventBrief{{
		ID:              120,
		EventName:       "golden event",
		EventType:       "marathon",
		EventTypeName:   "Marathon",
		StartAt:         startAt,
		EndAt:           endAt,
		EventBannerPath: "asset/jp-assets/startapp/home/banner/event_120/event_120.png",
	}}}
	title := "golden music list"
	musicList := &MusicListRequest{
		UserResults:          map[int]any{1: map[string]any{"master": "clear"}},
		MusicList:            []map[string]any{{"id": 1, "difficulty": "master"}, {"id": 2, "difficulty": "expert"}},
		JacketsPathList:      map[int]string{1: "jacket/1.png", 2: "jacket/2.png"},
		RequiredDifficulties: "master",
		Profile:              &DetailedProfileCardRequest{ID: "7354311836516539153", Region: "jp", Nickname: "golden", Source: "suite", UpdateTime: 1710000000},
		Title:                &title,
	}
	fixtures = append(fixtures,
		struct {
			name, endpoint string
			request        any
		}{"EventList", "/api/pjsk/event/list", eventList},
		struct {
			name, endpoint string
			request        any
		}{"MusicList", "/api/pjsk/music/list?show_id=true&show_leak=false", musicList},
	)
	return fixtures
}

var renderCacheKeyGoldenValues = map[string]string{
	"Profile/Asia/Tokyo":        "5b61b66eabf6907d88212e3d1d213dcf489941dec0688723e37568787ba71293",
	"Profile/Asia/Shanghai":     "73be3495725fb4b802b82c63bf9a90f970f5345db8bfe9525da690a73a38f133",
	"CardBox100/Asia/Tokyo":     "04b6f3e83caf0c9695915976f1e1f69f23dc48a0c4eea4fda81529983a1f9e61",
	"CardBox100/Asia/Shanghai":  "6c71a6d2affb027dfc1f86177b26c0990e2d51b2fbf18fcd8fd2eb80830aa480",
	"CardBox1000/Asia/Tokyo":    "d263d50499532e2a0187c1f8ed583018e5bfdac11e8e2aa6da2ffc778f8753bc",
	"CardBox1000/Asia/Shanghai": "f5303a021d53d41fc04eb39710a10c09db24851022365d81a169fd8f5593c721",
	"EventList/Asia/Tokyo":      "683ea9922ab0b86fcc6ba00b696ae861fba0e3fa816a3b73e7a80de40e6200fa",
	"EventList/Asia/Shanghai":   "59d87ab661bc720d7f9003994e941676288f39bc375b0d3b28972c68e5f7c4b3",
	"MusicList/Asia/Tokyo":      "ab274a8e8e411bf8696639a258f3e1b8831c00317dd544ae33644190209a2c9b",
	"MusicList/Asia/Shanghai":   "45d71c2ea5231d7d46d68bf3abb25984ac7fd72b8f01811693edb44943dd4a45",
}

func computeRenderCacheGoldenKey(t *testing.T, endpoint string, request any, zone string) string {
	t.Helper()
	ctx := displaytime.WithRequestTimeZone(context.Background(), zone)
	prepared := prepareDrawingRequestBody(endpoint, request, time.Unix(1710000000, 0), ctx)
	policy, err := buildRenderCachePolicy(endpoint, preparedRenderCachePayload{payload: prepared})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy(%s): %v", endpoint, err)
	}
	key, err := buildRenderCacheKey(policy)
	if err != nil {
		t.Fatalf("buildRenderCacheKey(%s): %v", endpoint, err)
	}
	return key
}

func TestRenderCacheKeyGolden(t *testing.T) {
	seen := map[string]string{}
	for _, fixture := range renderCacheKeyGoldenFixtures() {
		for _, zone := range []string{"Asia/Tokyo", "Asia/Shanghai"} {
			name := fixture.name + "/" + zone
			got := computeRenderCacheGoldenKey(t, fixture.endpoint, fixture.request, zone)
			if other, dup := seen[got]; dup {
				t.Errorf("%s produced the same key as %s: %s", name, other, got)
			}
			seen[got] = name
			want, ok := renderCacheKeyGoldenValues[name]
			if !ok {
				t.Errorf("missing golden key for %s (computed %q)", name, got)
				continue
			}
			if got != want {
				t.Errorf("render cache key for %s changed: got %s, want %s", name, got, want)
			}
		}
	}
	if len(seen) != len(renderCacheKeyGoldenValues) {
		t.Errorf("golden table has %d entries, fixtures produced %d keys", len(renderCacheKeyGoldenValues), len(seen))
	}
}

// TestRenderCacheKeyGoldenEventListUsesVersionFive proves the event/list
// golden key is derived from key version 5 while every other endpoint stays
// on version 3.
func TestRenderCacheKeyGoldenEventListUsesVersionFive(t *testing.T) {
	if renderCacheKeyVersion != 3 || renderCacheEventListKeyVersion != 5 {
		t.Fatalf("key versions changed: default=%d event/list=%d", renderCacheKeyVersion, renderCacheEventListKeyVersion)
	}
	for _, fixture := range renderCacheKeyGoldenFixtures() {
		if fixture.name != "EventList" {
			continue
		}
		ctx := displaytime.WithRequestTimeZone(context.Background(), "Asia/Tokyo")
		prepared := prepareDrawingRequestBody(fixture.endpoint, fixture.request, time.Unix(1710000000, 0), ctx)
		policy, err := buildRenderCachePolicy(fixture.endpoint, preparedRenderCachePayload{payload: prepared})
		if err != nil {
			t.Fatalf("buildRenderCachePolicy: %v", err)
		}
		want := renderCacheKeyGoldenValues["EventList/Asia/Tokyo"]
		if got := keyWithVersion(t, policy, 5); got != want {
			t.Fatalf("version 5 key = %s, want golden %s", got, want)
		}
		if got := keyWithVersion(t, policy, 3); got == want {
			t.Fatal("event/list golden key must not match the version 3 derivation")
		}
		return
	}
	t.Fatal("EventList fixture not found")
}

func keyWithVersion(t *testing.T, policy renderCachePolicy, version int) string {
	t.Helper()
	hashValue, err := hashstructure.Hash(renderCacheKeyMaterial{
		Version:  version,
		Endpoint: policy.Endpoint,
		APIPath:  policy.APIPath,
		UserID:   policy.UserID,
		Params:   renderCacheHashPayload{value: policy.Params},
	}, hashstructure.FormatV2, nil)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	digest := sha256.Sum256([]byte(strconv.FormatUint(hashValue, 10)))
	return hex.EncodeToString(digest[:])
}

func TestRenderCacheRuleTableShape(t *testing.T) {
	endpoints := make([]string, 0, len(renderCacheRules)+len(renderCacheDisabledEndpoints))
	for endpoint := range renderCacheRules {
		if !strings.HasPrefix(endpoint, "/api/pjsk/") {
			t.Errorf("rule %q is not under /api/pjsk/", endpoint)
		}
		endpoints = append(endpoints, endpoint)
	}
	for endpoint := range renderCacheDisabledEndpoints {
		if _, dup := renderCacheRules[endpoint]; dup {
			t.Errorf("disabled endpoint %q also has a rule", endpoint)
			continue
		}
		endpoints = append(endpoints, endpoint)
	}
	sort.Strings(endpoints)
	if len(endpoints) != 39 {
		t.Fatalf("rule table has %d /api/pjsk/ entries (incl. disabled), want 39: %v", len(endpoints), endpoints)
	}
	if len(renderCacheRules) != 38 {
		t.Fatalf("renderCacheRules has %d entries, want 38", len(renderCacheRules))
	}

	if len(renderCacheDisabledEndpoints) != 1 {
		t.Fatalf("renderCacheDisabledEndpoints = %v, want exactly {/api/pjsk/event/detail}", renderCacheDisabledEndpoints)
	}
	if _, ok := renderCacheDisabledEndpoints["/api/pjsk/event/detail"]; !ok {
		t.Fatalf("renderCacheDisabledEndpoints = %v, want exactly {/api/pjsk/event/detail}", renderCacheDisabledEndpoints)
	}
	if rule := resolveRenderCacheRule("/api/pjsk/event/detail"); rule.Enabled {
		t.Fatal("event/detail must resolve to a disabled rule")
	}

	if !defaultRenderCacheRule.Enabled || defaultRenderCacheRule.Infinite || defaultRenderCacheRule.TTL != 24*time.Hour {
		t.Fatalf("default rule changed: %+v", defaultRenderCacheRule)
	}
	if _, ok := defaultRenderCacheRule.IgnoreFieldNames["dt"]; !ok || len(defaultRenderCacheRule.IgnoreFieldNames) != 1 {
		t.Fatalf("default rule ignore fields changed: %v", defaultRenderCacheRule.IgnoreFieldNames)
	}
	if rule := resolveRenderCacheRule("/api/pjsk/unknown/golden"); !rule.Enabled || rule.TTL != 24*time.Hour || rule.Infinite {
		t.Fatalf("unlisted endpoint must use the 24h default rule, got %+v", rule)
	}

	// chara-birthday has no table entry and derives its TTL from the next
	// day boundary in the request timezone (resolveRenderCacheWindowTTL).
	if _, listed := renderCacheRules["/api/pjsk/misc/chara-birthday"]; listed {
		t.Fatal("chara-birthday unexpectedly gained a static rule")
	}
	shanghai := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, time.June, 12, 22, 30, 0, 0, shanghai)
	payload := map[string]any{"cid": 6, "dt": now.UnixMilli(), "timezone": "Asia/Shanghai"}
	ttl, ok := resolveRenderCacheWindowTTL("/api/pjsk/misc/chara-birthday", payload)
	if !ok || ttl != 90*time.Minute {
		t.Fatalf("chara-birthday window TTL = %v (ok=%v), want 1h30m", ttl, ok)
	}
	rule := adjustRenderCacheRuleForPayload("/api/pjsk/misc/chara-birthday", payload, resolveRenderCacheRule("/api/pjsk/misc/chara-birthday"))
	if rule.TTL != 90*time.Minute || rule.Infinite {
		t.Fatalf("chara-birthday adjusted rule = %+v, want TTL 1h30m", rule)
	}
}
