package drawing

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveRenderCacheRuleUsesOneDayTTLByDefault(t *testing.T) {
	rule := resolveRenderCacheRule("/api/pjsk/profile")
	if rule.TTL != renderCacheTTLOneDay {
		t.Fatalf("expected default ttl %s, got %s", renderCacheTTLOneDay, rule.TTL)
	}
}

func TestResolveRenderCacheRuleUsesHalfDayTTLForSelectedEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"/api/pjsk/deck/recommend",
		"/api/pjsk/music/rewards/basic",
		"/api/pjsk/music/rewards/detail",
		"/api/pjsk/mysekai/map",
		"/api/pjsk/mysekai/resource",
		"/api/pjsk/mysekai/talk-list",
	} {
		rule := resolveRenderCacheRule(endpoint)
		if rule.TTL != renderCacheTTLHalfDay {
			t.Fatalf("%s ttl = %s, want %s", endpoint, rule.TTL, renderCacheTTLHalfDay)
		}
	}
}

func TestResolveRenderCacheRuleDisablesEventDetail(t *testing.T) {
	rule := resolveRenderCacheRule("/api/pjsk/event/detail")
	if rule.Enabled {
		t.Fatal("event detail render cache should be disabled")
	}
}

func TestResolveRenderCacheRuleEnablesCharacterBirthday(t *testing.T) {
	rule := resolveRenderCacheRule("/api/pjsk/misc/chara-birthday")
	if !rule.Enabled {
		t.Fatal("character birthday render cache should be enabled")
	}
	if rule.TTL != renderCacheTTLOneDay {
		t.Fatalf("character birthday ttl = %s, want %s", rule.TTL, renderCacheTTLOneDay)
	}
}

func TestResolveRenderCacheRuleUsesSevenDayTTLForCustomProfileCard(t *testing.T) {
	rule := resolveRenderCacheRule("/api/pjsk/profile/custom-profile-card")
	if rule.TTL != renderCacheTTLSevenDay {
		t.Fatalf("custom profile card ttl = %s, want %s", rule.TTL, renderCacheTTLSevenDay)
	}
}

func TestRenderCacheClientAllowsInternalSelfSignedHTTPS(t *testing.T) {
	storageDir := t.TempDir()
	cacheKey := strings.Repeat("a", 64)
	cachePath := filepath.Join(storageDir, "cached.png")
	if err := os.WriteFile(cachePath, []byte("cached-image"), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cache" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != cacheKey {
			t.Fatalf("unexpected key: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"key":%q,"file_path":%q}`, cacheKey, cachePath)
	}))
	defer server.Close()

	client := NewRenderCacheClient(RenderCacheConfig{
		BaseURL:    server.URL,
		StorageDir: storageDir,
		TTL:        time.Minute,
	})
	if client == nil {
		t.Fatal("expected render cache client")
	}

	data, ok := client.lookup(cacheKey, "api/pjsk/profile")
	if !ok {
		t.Fatal("expected self-signed HTTPS cache lookup to hit")
	}
	if string(data) != "cached-image" {
		t.Fatalf("unexpected cache data: %q", string(data))
	}
}

func TestRenderCacheClientRemoteMissUsesSingleflight(t *testing.T) {
	storageDir := t.TempDir()
	var renderCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/cache":
			http.Error(w, `{"error":"miss"}`, http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/cache":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewRenderCacheClient(RenderCacheConfig{
		BaseURL:    server.URL,
		StorageDir: storageDir,
		TTL:        time.Minute,
	})
	if client == nil {
		t.Fatal("expected render cache client")
	}

	endpoint := "/api/pjsk/sk/query"
	request := map[string]any{
		"region": "jp",
		"ranks":  []any{1, 2, 3},
	}
	render := func() ([]byte, error) {
		atomic.AddInt32(&renderCalls, 1)
		time.Sleep(50 * time.Millisecond)
		return []byte("rendered-image"), nil
	}

	const callers = 8
	var wg sync.WaitGroup
	results := make([][]byte, callers)
	errs := make([]error, callers)
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = client.Render(endpoint, request, render)
		}(i)
	}
	wg.Wait()

	if got := atomic.LoadInt32(&renderCalls); got != 1 {
		t.Fatalf("render called %d times, want 1", got)
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d returned error: %v", i, err)
		}
		if string(results[i]) != "rendered-image" {
			t.Fatalf("call %d returned %q, want %q", i, string(results[i]), "rendered-image")
		}
	}
}

func TestRenderCacheClientStoresRenderedImageUnderRequestKeyDir(t *testing.T) {
	storageDir := t.TempDir()
	var registeredPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/cache":
			http.Error(w, `{"error":"miss"}`, http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/cache":
			registeredPath = r.FormValue("file_path")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewRenderCacheClient(RenderCacheConfig{
		BaseURL:    server.URL,
		StorageDir: storageDir,
		TTL:        time.Minute,
	})
	if client == nil {
		t.Fatal("expected render cache client")
	}

	image := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}
	data, err := client.Render("/api/pjsk/profile", map[string]any{"id": "123"}, func() ([]byte, error) {
		return image, nil
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(data) != string(image) {
		t.Fatalf("unexpected image bytes")
	}
	if !strings.HasPrefix(registeredPath, filepath.Join(storageDir, "api", "pjsk", "profile", "public")+string(os.PathSeparator)) {
		t.Fatalf("registered path %q should keep request-scoped directory", registeredPath)
	}
	if filepath.Ext(registeredPath) != ".jpg" {
		t.Fatalf("registered path ext = %q, want .jpg", filepath.Ext(registeredPath))
	}
	if _, err := os.Stat(registeredPath); err != nil {
		t.Fatalf("expected shared image file to exist: %v", err)
	}
}

func TestRenderCacheClientKeepsExistingContentFileWhenRegisterFails(t *testing.T) {
	storageDir := t.TempDir()
	image := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}

	client := NewRenderCacheClient(RenderCacheConfig{
		BaseURL:    "http://127.0.0.1:1",
		StorageDir: storageDir,
		TTL:        time.Minute,
	})
	if client == nil {
		t.Fatal("expected render cache client")
	}
	_, targetPath := client.contentFilePath("api/pjsk/profile", "public", strings.Repeat("c", 64), image)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(targetPath, image, 0o644); err != nil {
		t.Fatalf("write existing content file: %v", err)
	}

	err := client.store(strings.Repeat("c", 64), "api/pjsk/profile", "public", image, time.Minute, false)
	if err == nil {
		t.Fatal("expected register failure")
	}
	if _, statErr := os.Stat(targetPath); statErr != nil {
		t.Fatalf("existing content file should remain after register failure: %v", statErr)
	}
}

func TestBuildRenderCachePolicyAliasListIsInfiniteAndIgnoresDT(t *testing.T) {
	reqA := map[string]any{
		"title":        "角色别名",
		"entity_label": "角色ID",
		"entity_id":    5,
		"entity_name":  "花里みのり",
		"aliases":      []any{"花里", "花里みのり", "minori"},
		"dt":           int64(1713852000000), // 2024-04-23 12:00:00 UTC
	}
	reqB := map[string]any{
		"title":        "角色别名",
		"entity_label": "角色ID",
		"entity_id":    5,
		"entity_name":  "花里みのり",
		"aliases":      []any{"花里", "花里みのり", "minori"},
		"dt":           int64(1713852660000), // +11m
	}

	policyA, err := buildRenderCachePolicy("/api/pjsk/misc/alias-list", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/misc/alias-list", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("alias-list key should ignore dt when aliases are unchanged: %s != %s", keyA, keyB)
	}
	if !policyA.Infinite {
		t.Fatalf("expected alias-list cache policy to be infinite")
	}
	if policyA.TTL != 0 {
		t.Fatalf("expected infinite alias-list cache ttl to be 0, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyCharacterBirthdayIgnoresDT(t *testing.T) {
	reqA := map[string]any{
		"cid":                 6,
		"month":               6,
		"day":                 12,
		"region_name":         "JP",
		"days_until_birthday": 0,
		"color_code":          "#33AAEE",
		"sd_image_path":       "sd/6.png",
		"title_image_path":    "title/6.png",
		"card_image_path":     "card/6.png",
		"cards": []any{
			map[string]any{"id": 1001, "thumbnail_path": "thumb/1001.png"},
		},
		"all_characters": []any{
			map[string]any{"cid": 6, "month": 6, "day": 12, "icon_path": "icon/6.png"},
		},
		"dt": int64(1781251200000),
	}
	reqB := map[string]any{
		"cid":                 6,
		"month":               6,
		"day":                 12,
		"region_name":         "JP",
		"days_until_birthday": 0,
		"color_code":          "#33AAEE",
		"sd_image_path":       "sd/6.png",
		"title_image_path":    "title/6.png",
		"card_image_path":     "card/6.png",
		"cards": []any{
			map[string]any{"id": 1001, "thumbnail_path": "thumb/1001.png"},
		},
		"all_characters": []any{
			map[string]any{"cid": 6, "month": 6, "day": 12, "icon_path": "icon/6.png"},
		},
		"dt": int64(1781254800000),
	}

	policyA, err := buildRenderCachePolicy("/api/pjsk/misc/chara-birthday", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/misc/chara-birthday", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("birthday key should ignore dt when request content is unchanged: %s != %s", keyA, keyB)
	}
}

func TestBuildRenderCachePolicyCharacterBirthdayExpiresAtNextDayBoundary(t *testing.T) {
	now := time.Date(2026, time.June, 12, 22, 30, 0, 0, time.FixedZone("CST", 8*3600))
	policy, err := buildRenderCachePolicy("/api/pjsk/misc/chara-birthday", map[string]any{
		"cid":                 6,
		"month":               6,
		"day":                 12,
		"region_name":         "JP",
		"days_until_birthday": 0,
		"color_code":          "#33AAEE",
		"sd_image_path":       "sd/6.png",
		"title_image_path":    "title/6.png",
		"card_image_path":     "card/6.png",
		"cards": []any{
			map[string]any{"id": 1001, "thumbnail_path": "thumb/1001.png"},
		},
		"all_characters": []any{
			map[string]any{"cid": 6, "month": 6, "day": 12, "icon_path": "icon/6.png"},
		},
		"timezone": "Asia/Shanghai",
		"dt":       now.UnixMilli(),
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}

	if policy.TTL != 90*time.Minute {
		t.Fatalf("birthday ttl = %s, want %s", policy.TTL, 90*time.Minute)
	}
}

func TestBuildRenderCachePolicyCharacterBirthdayVariesByCharacterAndDay(t *testing.T) {
	base := map[string]any{
		"cid":                 6,
		"month":               6,
		"day":                 12,
		"region_name":         "JP",
		"days_until_birthday": 0,
		"color_code":          "#33AAEE",
		"sd_image_path":       "sd/6.png",
		"title_image_path":    "title/6.png",
		"card_image_path":     "card/6.png",
		"cards": []any{
			map[string]any{"id": 1001, "thumbnail_path": "thumb/1001.png"},
		},
		"all_characters": []any{
			map[string]any{"cid": 6, "month": 6, "day": 12, "icon_path": "icon/6.png"},
		},
	}
	dayChanged := map[string]any{
		"cid":                 6,
		"month":               6,
		"day":                 12,
		"region_name":         "JP",
		"days_until_birthday": 1,
		"color_code":          "#33AAEE",
		"sd_image_path":       "sd/6.png",
		"title_image_path":    "title/6.png",
		"card_image_path":     "card/6.png",
		"cards": []any{
			map[string]any{"id": 1001, "thumbnail_path": "thumb/1001.png"},
		},
		"all_characters": []any{
			map[string]any{"cid": 6, "month": 6, "day": 12, "icon_path": "icon/6.png"},
		},
	}
	characterChanged := map[string]any{
		"cid":                 7,
		"month":               6,
		"day":                 24,
		"region_name":         "JP",
		"days_until_birthday": 0,
		"color_code":          "#EE8833",
		"sd_image_path":       "sd/7.png",
		"title_image_path":    "title/7.png",
		"card_image_path":     "card/7.png",
		"cards": []any{
			map[string]any{"id": 1002, "thumbnail_path": "thumb/1002.png"},
		},
		"all_characters": []any{
			map[string]any{"cid": 7, "month": 6, "day": 24, "icon_path": "icon/7.png"},
		},
	}

	baseKey := mustBuildRenderCacheKey(t, "/api/pjsk/misc/chara-birthday", base)
	dayChangedKey := mustBuildRenderCacheKey(t, "/api/pjsk/misc/chara-birthday", dayChanged)
	characterChangedKey := mustBuildRenderCacheKey(t, "/api/pjsk/misc/chara-birthday", characterChanged)

	if baseKey == dayChangedKey {
		t.Fatal("birthday key should change when days_until_birthday changes")
	}
	if baseKey == characterChangedKey {
		t.Fatal("birthday key should change when character changes")
	}
}

func TestResolveRenderCacheRuleUsesInfiniteTTLForStaticEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"/api/pjsk/card/detail",
		"/api/pjsk/card/list",
		"/api/pjsk/help/render",
		"/api/pjsk/mysekai/fixture-list",
		"/api/pjsk/mysekai/fixture-detail",
	} {
		rule := resolveRenderCacheRule(endpoint)
		if !rule.Infinite {
			t.Fatalf("%s should use infinite ttl", endpoint)
		}
		if rule.TTL != 0 {
			t.Fatalf("%s ttl = %s, want 0 for infinite ttl", endpoint, rule.TTL)
		}
	}
}

func TestBuildRenderCachePolicyEventListUsesNextPhaseBoundaryTTL(t *testing.T) {
	now := int64(1774118400000)
	endAt := now + int64((2*time.Hour)/time.Millisecond)
	policy, err := buildRenderCachePolicy("/api/pjsk/event/list", map[string]any{
		"dt": now,
		"event_info": []any{
			map[string]any{
				"id":       101,
				"start_at": now - int64((time.Hour)/time.Millisecond),
				"end_at":   endAt,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	want := 2 * time.Hour
	if policy.TTL != want {
		t.Fatalf("event list ttl = %v, want %v", policy.TTL, want)
	}
	if policy.Infinite {
		t.Fatal("event list should no longer use infinite ttl")
	}
}

func TestBuildRenderCachePolicyEventListExpiresAtNextEventStart(t *testing.T) {
	now := int64(1774118400000)
	nextStartAt := now + int64((45*time.Minute)/time.Millisecond)
	nextEndAt := nextStartAt + int64((7*24*time.Hour)/time.Millisecond)
	policy, err := buildRenderCachePolicy("/api/pjsk/event/list", map[string]any{
		"dt": now,
		"event_info": []any{
			map[string]any{
				"id":       101,
				"start_at": now - int64((7*24*time.Hour)/time.Millisecond),
				"end_at":   now - int64((time.Hour)/time.Millisecond),
			},
			map[string]any{
				"id":       102,
				"start_at": nextStartAt,
				"end_at":   nextEndAt,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	want := 45 * time.Minute
	if policy.TTL != want {
		t.Fatalf("event list ttl = %v, want %v", policy.TTL, want)
	}
}

func TestBuildRenderCachePolicyVLiveUsesDynamicWindowTTL(t *testing.T) {
	now := int64(1774118400000)
	endAt := now + int64((90*time.Minute)/time.Millisecond)
	policy, err := buildRenderCachePolicy("/api/pjsk/vlive/list", map[string]any{
		"dt": now,
		"lives": []any{
			map[string]any{
				"id":     1,
				"end_at": endAt,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	want := 90*time.Minute + renderCacheWindowTTLBuffer
	if policy.TTL != want {
		t.Fatalf("vlive ttl = %v, want %v", policy.TTL, want)
	}
}

func TestBuildRenderCachePolicyEventDetailIsDisabled(t *testing.T) {
	now := int64(1774118400000)
	endAt := now - int64((time.Hour)/time.Millisecond)
	_, err := buildRenderCachePolicy("/api/pjsk/event/detail", map[string]any{
		"dt": now,
		"event_info": map[string]any{
			"id":     101,
			"end_at": endAt,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "render cache disabled") {
		t.Fatalf("expected disabled render cache error, got %v", err)
	}
}

func TestBuildRenderCachePolicyMarksCardListAsInfinite(t *testing.T) {
	policy, err := buildRenderCachePolicy("/api/pjsk/card/list", CardListRequest{
		Region: "JP",
		Cards: []CardBasic{
			{CardID: 1001},
		},
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	if !policy.Infinite {
		t.Fatalf("expected card list cache policy to be infinite")
	}
	if policy.TTL != 0 {
		t.Fatalf("expected infinite cache policy ttl to be 0, got %s", policy.TTL)
	}
}

func TestBuildRenderCachePolicyIgnoresEventRecordUserUpdateTime(t *testing.T) {
	reqA := EventRecordRequest{
		EventInfo: []EventHistory{{ID: 1, EventName: "Event"}},
		UserInfo: DetailedProfileCardRequest{
			ID:              "123",
			Region:          "JP",
			Nickname:        "Tester",
			Source:          "suite",
			UpdateTime:      1,
			IsHideUID:       true,
			LeaderImagePath: "leader.png",
		},
	}
	reqB := reqA
	reqB.UserInfo.UpdateTime = 2

	policyA, err := buildRenderCachePolicy("/api/pjsk/event/record", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/event/record", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("event record key should ignore user update time: %s != %s", keyA, keyB)
	}
	if policyA.APIPath != policyB.APIPath {
		t.Fatalf("event record api_path should stay stable: %s != %s", policyA.APIPath, policyB.APIPath)
	}
}

func TestBuildRenderCachePolicyIgnoresUnusedProfileUpdateTime(t *testing.T) {
	reqA := ProfileRequest{
		Profile: BasicProfile{
			ID:              "123",
			Region:          "JP",
			Nickname:        "Tester",
			IsHideUID:       true,
			LeaderImagePath: "leader.png",
		},
		UpdateTime: new(int64(1)),
	}
	reqB := reqA
	reqB.UpdateTime = new(int64(2))

	policyA, err := buildRenderCachePolicy("/api/pjsk/profile", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/profile", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("profile key should ignore unused update_time: %s != %s", keyA, keyB)
	}
	if policyA.APIPath != policyB.APIPath {
		t.Fatalf("profile api_path should stay stable: %s != %s", policyA.APIPath, policyB.APIPath)
	}
}

func TestBuildRenderCachePolicyBucketsSKWinRateUpdatedAtBy10Seconds(t *testing.T) {
	reqA := WinRateRequest{
		UpdatedAt:        1774118400000,
		EventStartAt:     10,
		EventAggregateAt: 1774118404000,
		TeamInfo: []TeamInfo{
			{TeamID: 1, TeamName: "A", WinRate: 0.5},
			{TeamID: 2, TeamName: "B", WinRate: 0.5},
		},
	}
	reqB := reqA
	reqB.UpdatedAt = 1774118409000
	reqB.EventAggregateAt = 1774118409000
	reqC := reqA
	reqC.UpdatedAt = 1774118411000
	reqC.EventAggregateAt = 1774118411000

	policyA, err := buildRenderCachePolicy("/api/pjsk/sk/winrate", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/sk/winrate", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}
	policyC, err := buildRenderCachePolicy("/api/pjsk/sk/winrate", reqC)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqC: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}
	keyC, err := buildRenderCacheKey(policyC)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqC: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("winrate key should bucket updated_at/event_aggregate_at within 10s: %s != %s", keyA, keyB)
	}
	if keyA == keyC {
		t.Fatalf("winrate key should change after 10s bucket boundary")
	}
	if policyA.TTL != 10*time.Second {
		t.Fatalf("expected 10s ttl for winrate cache, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyMusicListUsesRenderFlagsAndPublicFallback(t *testing.T) {
	req := MusicListRequest{
		UserResults: map[int]any{1: "ap"},
		MusicList: []map[string]any{
			{"id": 1, "difficulty": 32},
		},
		RequiredDifficulties: "master",
		Profile: &DetailedProfileCardRequest{
			ID:              "service",
			Region:          "JP",
			Nickname:        "Lunabot",
			Source:          "lunabot-service",
			UpdateTime:      1,
			IsHideUID:       true,
			LeaderImagePath: "leader.png",
		},
	}

	showPolicy, err := buildRenderCachePolicy("/api/pjsk/music/list?show_id=true&show_leak=false", req)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy show: %v", err)
	}
	hidePolicy, err := buildRenderCachePolicy("/api/pjsk/music/list?show_id=false&show_leak=false", req)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy hide: %v", err)
	}

	if showPolicy.UserID != renderCachePublic {
		t.Fatalf("expected public fallback user_id, got %s", showPolicy.UserID)
	}
	if showPolicy.APIPath != "api/pjsk/music/list" {
		t.Fatalf("unexpected api_path: %s", showPolicy.APIPath)
	}
	if showPolicy.APIPath != hidePolicy.APIPath {
		t.Fatalf("expected stable api_path across render flags")
	}

	keyShow, err := buildRenderCacheKey(showPolicy)
	if err != nil {
		t.Fatalf("buildRenderCacheKey show: %v", err)
	}
	keyHide, err := buildRenderCacheKey(hidePolicy)
	if err != nil {
		t.Fatalf("buildRenderCacheKey hide: %v", err)
	}
	if keyShow == keyHide {
		t.Fatalf("expected different keys for different render flags")
	}
}

func TestBuildRenderCachePolicySKQueryIgnoresTopLevelEventIDForUserID(t *testing.T) {
	req := SKRequest{
		ID:     1,
		Region: "JP",
		Name:   "Event",
		Ranks: []RankInfo{
			{Rank: 100, Name: "Tester"},
		},
	}

	policy, err := buildRenderCachePolicy("/api/pjsk/sk/query", req)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}

	if policy.UserID != renderCachePublic {
		t.Fatalf("expected public user_id, got %s", policy.UserID)
	}
	if policy.APIPath != "api/pjsk/sk/query" {
		t.Fatalf("unexpected api_path: %s", policy.APIPath)
	}
}

func TestBuildRenderCachePolicySKQueryOnlyChangesWhenPulledInfoChanges(t *testing.T) {
	reqA := SKRequest{
		ID:          1,
		Region:      "JP",
		Name:        "Event",
		AggregateAt: 1774118404000,
		Ranks: []RankInfo{
			{Rank: 100, Name: "Tester", Time: 1774118405000},
		},
	}
	reqB := reqA
	reqC := reqA
	reqC.AggregateAt = 1774118420000
	reqC.Ranks[0].Time = 1774118420000

	policyA, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}
	policyC, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqC)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqC: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}
	keyC, err := buildRenderCacheKey(policyC)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqC: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("sk query key should stay stable when pulled info is unchanged: %s != %s", keyA, keyB)
	}
	if keyA == keyC {
		t.Fatalf("sk query key should change after pulled info changes")
	}
	if policyA.TTL != renderCacheTTLHalfDay {
		t.Fatalf("expected half-day ttl for sk query cache, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyTWSKQueryDoesNotBucketByTime(t *testing.T) {
	reqA := SKRequest{
		ID:          1,
		Region:      "TW",
		Name:        "Event",
		AggregateAt: 1774118404000,
		Ranks: []RankInfo{
			{Rank: 100, Name: "Tester", Time: 1774118405000},
		},
	}
	reqB := reqA
	reqB.AggregateAt = 1774118429000
	reqB.Ranks[0].Time = 1774118429000
	reqC := reqA
	reqC.AggregateAt = 1774118404000
	reqC.Ranks[0].Time = 1774118405000

	policyA, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}
	policyC, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqC)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqC: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}
	keyC, err := buildRenderCacheKey(policyC)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqC: %v", err)
	}

	if keyA == keyB {
		t.Fatalf("tw sk query key should change when tracker timestamps differ")
	}
	if keyA != keyC {
		t.Fatalf("tw sk query key should remain identical for identical content: %s != %s", keyA, keyC)
	}
	if policyA.TTL != renderCacheTTLHalfDay {
		t.Fatalf("expected half-day ttl for tw sk query cache, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyENSKQueryDoesNotBucketByTime(t *testing.T) {
	reqA := SKRequest{
		ID:          1,
		Region:      "EN",
		Name:        "Event",
		AggregateAt: 1774118404000,
		Ranks: []RankInfo{
			{Rank: 100, Name: "Tester", Time: 1774118405000},
		},
	}
	reqB := reqA
	reqB.AggregateAt = 1774118459000
	reqB.Ranks[0].Time = 1774118459000
	reqC := reqA
	reqC.AggregateAt = 1774118404000
	reqC.Ranks[0].Time = 1774118405000

	policyA, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}
	policyC, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqC)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqC: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}
	keyC, err := buildRenderCacheKey(policyC)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqC: %v", err)
	}

	if keyA == keyB {
		t.Fatalf("en sk query key should change when tracker timestamps differ")
	}
	if keyA != keyC {
		t.Fatalf("en sk query key should remain identical for identical content: %s != %s", keyA, keyC)
	}
	if policyA.TTL != renderCacheTTLHalfDay {
		t.Fatalf("expected half-day ttl for en sk query cache, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyIgnoresRootDT(t *testing.T) {
	policyA, err := buildRenderCachePolicy("/api/pjsk/event/list", map[string]any{
		"region": "JP",
		"dt":     1774118400000,
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/event/list", map[string]any{
		"region": "JP",
		"dt":     1774118700000,
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("expected dt to be ignored by cache key: %s != %s", keyA, keyB)
	}
}

func mustBuildRenderCacheKey(t *testing.T, endpoint string, request any) string {
	t.Helper()
	policy, err := buildRenderCachePolicy(endpoint, request)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	key, err := buildRenderCacheKey(policy)
	if err != nil {
		t.Fatalf("buildRenderCacheKey: %v", err)
	}
	return key
}
