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
// already persisted in the image cache index. A diff here means every stored
// render-cache key is invalidated, so these values change only together with
// an intentional renderCacheKeyVersion bump (version 4: image labels moved to
// the i18n catalog; version 6: raw keys and region/account labels sent next
// to the labels, catalog supply labels).

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
	styleName := "white"
	chart := &GenerateMusicChartRequest{
		MusicID:    1,
		Title:      "golden chart",
		Artist:     "golden artist",
		Difficulty: "master",
		PlayLevel:  30,
		Skill:      true,
		JacketPath: "asset/jp-assets/startapp/music/jacket/jacket_s_001/jacket_s_001.png",
		SusPath:    "asset/jp-assets/ondemand/music/music_score/0001_01/master",
		StylePath:  &styleName,
		NoteHost:   "https://notes.example",
		MusicMeta:  map[string]any{"music_id": 1, "difficulty": "master", "skill_score_solo": []any{0.1, 0.2}},
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
		struct {
			name, endpoint string
			request        any
		}{"Chart", "/api/pjsk/chart", chart},
	)
	return fixtures
}

var renderCacheKeyGoldenValues = map[string]string{
	"Profile/Asia/Tokyo":        "023fd37b62d798a2bff205a76a35e80da872a4841cab69c50a462ae34bda4217",
	"Profile/Asia/Shanghai":     "2be742ab4f54d7bb2c8fc3c6c58c98e07ebece2df84d6b3f88c7f3871c39608c",
	"CardBox100/Asia/Tokyo":     "21a2d6139710611caade2cbabe6ae7c015e62478af51d5751ea1e82ccbb4cf34",
	"CardBox100/Asia/Shanghai":  "ee2ab7c982ce72737f85971a03a2d3de957155acbbca1504e280a981342b3993",
	"CardBox1000/Asia/Tokyo":    "763b46e0ee411c2c0928b7eec4721992476a8efd061678595657a10ebb5f26e8",
	"CardBox1000/Asia/Shanghai": "33fec5bb0ed3141b2ea2ae9164e119965aacdf67d303172dbfef225bd0037d7b",
	"EventList/Asia/Tokyo":      "683ea9922ab0b86fcc6ba00b696ae861fba0e3fa816a3b73e7a80de40e6200fa",
	"EventList/Asia/Shanghai":   "59d87ab661bc720d7f9003994e941676288f39bc375b0d3b28972c68e5f7c4b3",
	"MusicList/Asia/Tokyo":      "5d8b5cec9579afb6cf105e49cafb92d4cf3305005922a8c3f2f96cd9f281dcd6",
	"MusicList/Asia/Shanghai":   "f4d47ab8759e8a861d989bfb1e0dc614fe8ccd1247fcac8b6f6b7331d10db0e6",
	// The /api/pjsk/chart rule changes only the TTL, never the key.
	"Chart/Asia/Tokyo":    "ce007f0bdfb9b402b000db320153c0af3eb44ea4f57f2c37dd98f864232146a5",
	"Chart/Asia/Shanghai": "7ebf17e77bc2a3ba45871180dedfa5f2cc1fd744ac9639891f6b4e33c62e24c9",
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
// golden key is derived from key version 5 while every other endpoint is on
// version 6 (the event list draws no label that version 6 changed).
func TestRenderCacheKeyGoldenEventListUsesVersionFive(t *testing.T) {
	if renderCacheKeyVersion != 6 || renderCacheEventListKeyVersion != 5 {
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
		if got := keyWithVersion(t, policy, renderCacheKeyVersion); got == want {
			t.Fatal("event/list golden key must not match the default version derivation")
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
	// 38 rules from before the storage work, plus /api/pjsk/chart added by
	// T16 (the chart static cache moved onto the render path) and the
	// standalone info panel.
	if len(endpoints) != 41 {
		t.Fatalf("rule table has %d /api/pjsk/ entries (incl. disabled), want 41: %v", len(endpoints), endpoints)
	}
	if len(renderCacheRules) != 40 {
		t.Fatalf("renderCacheRules has %d entries, want 40", len(renderCacheRules))
	}
	if rule := resolveRenderCacheRule("/api/pjsk/chart"); !rule.Enabled || rule.Infinite || rule.TTL != 7*24*time.Hour {
		t.Fatalf("chart rule = %+v, want a 7-day TTL", rule)
	}
	if _, ok := resolveRenderCacheRule("/api/pjsk/chart").IgnoreFieldNames["dt"]; !ok {
		t.Fatal("chart rule must keep the default dt ignore field")
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
