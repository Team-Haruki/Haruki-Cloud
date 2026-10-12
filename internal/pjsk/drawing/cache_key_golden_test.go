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
// to the labels, catalog supply labels; version 7: Drawing's own labels sent
// as labels, data source labels, spec typography in Drawing's text; version 8:
// MySekai fixture lists carry first-craft ribbons and a per-view legend).

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
	"Profile/Asia/Tokyo":        "de89dfc1c9d86a9abcdd3ee1c18d38724f0339e28ed3629c63e40d8c624ba2ff",
	"Profile/Asia/Shanghai":     "8044c3ef9f9e01f5cdfd3f8bcf9d63c9ef248a44ec5c50152cf6c5baff324144",
	"CardBox100/Asia/Tokyo":     "32e0d69c12671cc42e776e6481558071a9f05d2169eb9d435992948f4bfba31f",
	"CardBox100/Asia/Shanghai":  "e7117bebd1abcc3974c9c0e24965f27fe979ebd5e8404cc461c1f03d317d07d4",
	"CardBox1000/Asia/Tokyo":    "cd1b4179dd4f679ec7f7039eb2400ccd5c3ffece526e1bee15d91b30a9225767",
	"CardBox1000/Asia/Shanghai": "046d1fa72d1ac38c01a9723f67ef96fa772515515b8a5e8f4ca5aa28c191517c",
	"EventList/Asia/Tokyo":      "683ea9922ab0b86fcc6ba00b696ae861fba0e3fa816a3b73e7a80de40e6200fa",
	"EventList/Asia/Shanghai":   "59d87ab661bc720d7f9003994e941676288f39bc375b0d3b28972c68e5f7c4b3",
	"MusicList/Asia/Tokyo":      "dd8099e71376ee546db73e2ac6c1fe02ad6a6c1b94bab38d117288e94fcd41f3",
	"MusicList/Asia/Shanghai":   "76cc07f5984227f7a678adc8d172c3df549d1610fd5de9b527e3933b6ea55468",
	// The /api/pjsk/chart rule changes only the TTL, never the key.
	"Chart/Asia/Tokyo":    "d1e28b5f5ccedcb89bf493c00e1246a5c7b9d63115a761e01ea464399ac5ba0c",
	"Chart/Asia/Shanghai": "bd5068744016453d7d753c55930e7c762410f51b29199abaf284eaf01f950a00",
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
// version 8 (the event list draws nothing that versions 6 to 8 changed).
func TestRenderCacheKeyGoldenEventListUsesVersionFive(t *testing.T) {
	if renderCacheKeyVersion != 8 || renderCacheEventListKeyVersion != 5 {
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
