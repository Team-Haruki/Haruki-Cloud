//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package drawing

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	json "haruki-cloud/internal/jsonutil"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestRenderCacheEndpointPayloadAndScalarBranches(t *testing.T) {
	for _, raw := range []string{"", "   ", "https://example.test"} {
		if _, err := parseRenderCacheEndpoint(raw); err == nil {
			t.Fatalf("parseRenderCacheEndpoint(%q) unexpectedly succeeded", raw)
		}
	}
	parsed, err := parseRenderCacheEndpoint("https://example.test/api/pjsk/card/list?b=2&a=1")
	if err != nil || parsed.Path != "/api/pjsk/card/list" || parsed.Normalized != "/api/pjsk/card/list?a=1&b=2" {
		t.Fatalf("unexpected parsed endpoint: %+v err=%v", parsed, err)
	}
	if got, err := normalizeRenderCachePayload(nil); err != nil || got != nil {
		t.Fatalf("nil payload = %#v, %v", got, err)
	}
	if _, err := normalizeRenderCachePayload(make(chan int)); err == nil {
		t.Fatal("unsupported payload should fail marshaling")
	}
	if got := buildRenderCacheAPIPath(renderCacheEndpoint{Path: "///"}, "ignored", nil); got != "" {
		t.Fatalf("blank API path normalized to %q", got)
	}
	if got := normalizeRenderCacheAPIPath("api/pjsk/../secret"); got != "" {
		t.Fatalf("unsafe API path normalized to %q", got)
	}
	if got := normalizeRenderCacheAPIPath(`api/pjsk/bad name`); got != "" {
		t.Fatalf("spaced API path normalized to %q", got)
	}

	values := []struct {
		value any
		want  string
	}{
		{nil, ""}, {"text", "text"}, {true, "true"}, {false, "false"},
		{float64(4), "4"}, {float64(4.5), "4.5"}, {float32(5), "5"}, {float32(5.25), "5.25"},
		{int(6), "6"}, {int64(7), "7"}, {json.Number("8"), "8"}, {[]int{1}, "[1]"},
	}
	for _, tc := range values {
		if got := scalarString(tc.value); got != tc.want {
			t.Fatalf("scalarString(%T(%v)) = %q, want %q", tc.value, tc.value, got, tc.want)
		}
	}
}

func TestRenderCacheProfileAndPathHelperBranches(t *testing.T) {
	testRenderCacheProfileRecognition(t)
	testRenderCacheUserIDNormalization(t)
	testRenderCacheNestedValueHelpers(t)
}

func testRenderCacheProfileRecognition(t *testing.T) {
	t.Helper()
	profiles := []map[string]any{
		{"profile": map[string]any{"id": "nested", "nickname": "n"}},
		{"data_sources": []any{map[string]any{"source": "real"}}, "id": 22},
		{"nickname": "Nick", "id": 23},
		{"leader_image_path": "leader.png", "id": 24},
		{"source": "remote", "id": 25},
	}
	wants := []string{"nested", "22", "23", "24", "25"}
	for i, profile := range profiles {
		if !isProfileLikeMap(profile) {
			t.Fatalf("profile %d not recognized: %#v", i, profile)
		}
		if got := extractRenderCacheUserID(profile); got != wants[i] {
			t.Fatalf("profile %d user id = %q, want %q", i, got, wants[i])
		}
	}
	if isProfileLikeMap(nil) || isProfileLikeMap(map[string]any{}) {
		t.Fatal("empty maps should not be profile-like")
	}
	for _, placeholder := range []map[string]any{
		{"id": " service ", "nickname": "n"},
		{"source": "LUNABOT-SERVICE"},
		{"source": "local_fallback"},
		{"data_sources": []any{map[string]any{"source": "local_fallback"}}},
	} {
		if !isPlaceholderProfile(placeholder) || extractUserIDFromMap(placeholder) != "" {
			t.Fatalf("placeholder not rejected: %#v", placeholder)
		}
	}
	if isPlaceholderProfile(nil) || extractUserIDFromMap(nil) != "" {
		t.Fatal("nil profile helper result is invalid")
	}
}

func testRenderCacheUserIDNormalization(t *testing.T) {
	t.Helper()
	payload := map[string]any{"user_info": map[string]any{"nickname": "user", "id": "u1"}}
	if got := extractRenderCacheUserID(payload); got != "u1" {
		t.Fatalf("user_info id = %q", got)
	}
	if got := extractRenderCacheUserID(map[string]any{}); got != "" {
		t.Fatalf("empty payload id = %q", got)
	}
	if got := normalizeRenderCacheUserID(" "); got != renderCachePublic {
		t.Fatalf("blank user id = %q", got)
	}
	if got := normalizeRenderCacheUserID("safe.User-1"); got != "safe.User-1" {
		t.Fatalf("safe user id changed: %q", got)
	}
	if got := normalizeRenderCacheUserID("unsafe/user"); !strings.HasPrefix(got, "user-") {
		t.Fatalf("unsafe user id was not hashed: %q", got)
	}
	for _, unsafe := range []string{"", ".", "..", "a/b", "a b", "中文"} {
		if safeRenderCachePathSegment(unsafe) {
			t.Fatalf("unsafe path segment accepted: %q", unsafe)
		}
	}
	if !safeRenderCachePathSegment("A_z-1.2") {
		t.Fatal("safe path segment rejected")
	}
}

func testRenderCacheNestedValueHelpers(t *testing.T) {
	t.Helper()
	root := map[string]any{"a": map[string]any{"b": []any{1, 2}}}
	if got := valueAt(root, "a", "b"); len(got.([]any)) != 2 {
		t.Fatalf("unexpected nested value: %#v", got)
	}
	if valueAt(root, "missing", "b") != nil || mapAt(root, "a", "b") != nil || asMap(1) != nil {
		t.Fatal("invalid nested lookup unexpectedly succeeded")
	}
	if got := sliceAt(root, "a", "b"); len(got) != 2 {
		t.Fatalf("unexpected nested slice: %#v", got)
	}
	if sliceAt(root, "a") != nil {
		t.Fatal("map converted to slice")
	}
}

func TestRenderCacheRuleAndTimeBranches(t *testing.T) {
	testRenderCacheRuleMerging(t)
	testRenderCacheMillisConversions(t)
	testRenderCacheTTLClamps(t)
	testRenderCacheWindowTTL(t)
}

func testRenderCacheRuleMerging(t *testing.T) {
	t.Helper()
	if got := cloneRenderCacheBucketMap(nil); len(got) != 0 {
		t.Fatalf("nil bucket map clone = %v", got)
	}
	original := map[string]time.Duration{"dt": time.Second}
	cloned := cloneRenderCacheBucketMap(original)
	cloned["dt"] = time.Minute
	if original["dt"] != time.Second {
		t.Fatal("bucket map clone shared storage")
	}
	if got := renderCacheStringSet("", " a ", "a"); len(got) != 1 {
		t.Fatalf("unexpected string set: %v", got)
	}
	if got := renderCacheBucketSet(time.Second, "", "dt"); len(got) != 1 || got["dt"] != time.Second {
		t.Fatalf("unexpected bucket set: %v", got)
	}
	merged := mergeRenderCacheRule(renderCacheRule{
		IgnoreFieldNames: renderCacheStringSet("dt"),
		IgnorePaths:      renderCacheStringSet("profile.update_time"),
	}, renderCacheRule{
		Enabled:          true,
		Infinite:         true,
		BucketFieldNames: renderCacheBucketSet(time.Minute, "dt"),
		BucketPaths:      renderCacheBucketSet(time.Second, "profile.update_time"),
	})
	if !merged.Enabled || !merged.Infinite || merged.TTL != 0 || len(merged.IgnoreFieldNames) != 0 || len(merged.IgnorePaths) != 0 {
		t.Fatalf("unexpected merged rule: %+v", merged)
	}
}

func testRenderCacheMillisConversions(t *testing.T) {
	t.Helper()
	millisCases := []struct {
		value any
		want  int64
		ok    bool
	}{
		{nil, 0, false}, {int64(1), 1, true}, {int64(0), 0, false},
		{int(2), 2, true}, {int(0), 0, false}, {float64(3.9), 3, true}, {float64(-1), 0, false},
		{float32(4.9), 4, true}, {float32(0), 0, false}, {json.Number("5"), 5, true}, {json.Number("x"), 0, false},
		{" 6 ", 6, true}, {"", 0, false}, {"x", 0, false}, {true, 0, false},
	}
	for _, tc := range millisCases {
		got, ok := renderCacheMillis(tc.value)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("renderCacheMillis(%T(%v)) = %d,%v want %d,%v", tc.value, tc.value, got, ok, tc.want, tc.ok)
		}
	}
}

func testRenderCacheTTLClamps(t *testing.T) {
	t.Helper()
	if got := clampRenderCacheWindowTTL(-int64(time.Hour / time.Millisecond)); got != renderCacheWindowTTLMin {
		t.Fatalf("window min clamp = %v", got)
	}
	if got := clampRenderCacheWindowTTL(int64((30 * 24 * time.Hour) / time.Millisecond)); got != renderCacheWindowTTLMax {
		t.Fatalf("window max clamp = %v", got)
	}
	if got := clampEventListPhaseCacheTTL(1); got != renderCacheEventListPhaseTTLMin {
		t.Fatalf("event min clamp = %v", got)
	}
	if got := clampEventListPhaseCacheTTL(int64(time.Hour / time.Millisecond)); got != time.Hour {
		t.Fatalf("event middle clamp = %v", got)
	}
	if got := clampEventListPhaseCacheTTL(int64((24 * time.Hour) / time.Millisecond)); got != renderCacheEventListPhaseTTLMax {
		t.Fatalf("event max clamp = %v", got)
	}
	if got := clampBirthdayDayBoundaryCacheTTL(time.Minute); got != renderCacheBirthdayTTLMin {
		t.Fatalf("birthday min clamp = %v", got)
	}
	if got := clampBirthdayDayBoundaryCacheTTL(48 * time.Hour); got != renderCacheTTLOneDay {
		t.Fatalf("birthday max clamp = %v", got)
	}
	if got := clampBirthdayDayBoundaryCacheTTL(time.Hour); got != time.Hour {
		t.Fatalf("birthday middle clamp = %v", got)
	}
}

func testRenderCacheWindowTTL(t *testing.T) {
	t.Helper()
	if _, ok := resolveRenderCacheWindowTTL("/api/pjsk/event/list", nil); ok {
		t.Fatal("nil event payload produced window ttl")
	}
	if _, ok := resolveRenderCacheWindowTTL("/api/pjsk/vlive/list", map[string]any{"dt": 0}); ok {
		t.Fatal("invalid vlive time produced window ttl")
	}
	now := time.Now().UnixMilli()
	if ttl, ok := resolveRenderCacheWindowTTL("/api/pjsk/vlive/list", map[string]any{
		"dt": now,
		"lives": []any{
			map[string]any{"end_at": now + 1000},
			map[string]any{"end_at": now + 2000},
			map[string]any{"end_at": "bad"},
		},
	}); !ok || ttl != renderCacheWindowTTLBuffer+2*time.Second {
		t.Fatalf("unexpected vlive ttl: %v %v", ttl, ok)
	}
	if _, ok := resolveRenderCacheWindowTTL("unknown", map[string]any{}); ok {
		t.Fatal("unknown endpoint produced window ttl")
	}
}

func TestRenderCacheNodeAndLocalCacheBranches(t *testing.T) {
	testRenderCacheNodeNormalization(t)
	testLocalRenderCacheBranches(t)
}

func testRenderCacheNodeNormalization(t *testing.T) {
	t.Helper()
	rule := renderCacheRule{
		Enabled:          true,
		IgnoreFieldNames: renderCacheStringSet("remove"),
		IgnorePaths:      renderCacheStringSet("items.*.nested"),
		BucketFieldNames: renderCacheBucketSet(time.Second, "dt"),
	}
	payload := map[string]any{
		"remove": "x",
		"dt":     int64(2500),
		"items":  []any{map[string]any{"nested": "x", "keep": true}},
	}
	normalizeRenderCacheNode(payload, nil, rule)
	if _, ok := payload["remove"]; ok || payload["dt"] != int64(2_500_000) {
		t.Fatalf("payload was not sanitized: %#v", payload)
	}
	item := payload["items"].([]any)[0].(map[string]any)
	if _, ok := item["nested"]; ok || item["keep"] != true {
		t.Fatalf("nested payload was not sanitized: %#v", item)
	}
	if renderCachePathKey(nil) != "" || renderCacheFieldName([]string{"*", "*"}) != "" {
		t.Fatal("empty cache paths were not normalized")
	}
	if value, ok := bucketRenderCacheValue("2500", time.Second); !ok || value != "2500000" {
		t.Fatalf("string bucket = %#v,%v", value, ok)
	}
	if _, ok := bucketRenderCacheValue(10, 0); ok {
		t.Fatal("zero bucket unexpectedly succeeded")
	}
	if _, ok := bucketRenderCacheValue("bad", time.Second); ok {
		t.Fatal("invalid bucket value unexpectedly succeeded")
	}
	if _, ok := bucketRenderCacheValue(10, time.Nanosecond); ok {
		t.Fatal("sub-millisecond bucket unexpectedly succeeded")
	}
}

func testLocalRenderCacheBranches(t *testing.T) {
	t.Helper()
	var nilCache *localRenderCache
	if data, ok := nilCache.get("x"); ok || data != nil {
		t.Fatalf("nil cache hit: %v %q", ok, data)
	}
	nilCache.set("x", []byte("x"), time.Second, false)
	if cloneRenderBytes(nil) != nil {
		t.Fatal("nil byte clone should stay nil")
	}
	cache := &localRenderCache{ttl: time.Second, maxEntries: 2, maxBytes: 8}
	cache.set("a", []byte("a"), 0, false)
	if got, ok := cache.get("a"); !ok || string(got) != "a" {
		t.Fatalf("initialized cache miss: %q %v", got, ok)
	}
	cache.mu.Lock()
	cache.entries["nil"] = nil
	cache.sweepExpiredLocked(time.Now())
	cache.removeEntryLocked("a", &localRenderEntry{})
	cache.totalBytes = -1
	cache.removeEntryLocked("missing", nil)
	cache.mu.Unlock()

	calls := 0
	data, err := cache.Render("/api/pjsk/event/detail", nil, func() ([]byte, error) {
		calls++
		return []byte("rendered"), nil
	})
	if err != nil || string(data) != "rendered" || calls != 1 {
		t.Fatalf("Render fallback = %q,%v calls=%d", data, err, calls)
	}
	data, err = cache.RenderContext(nil, "/api/pjsk/event/detail", nil, func() ([]byte, error) {
		return nil, errors.New("render failed")
	})
	if err == nil || data != nil {
		t.Fatalf("RenderContext error = %q,%v", data, err)
	}
}

func TestRemoteRenderCacheSmallHelpers(t *testing.T) {
	testRemoteRenderCacheConstruction(t)
	testRemoteRenderCachePaths(t)
	testRemoteRenderCacheFileTypes(t)
	testRemoteRenderCacheFileIO(t)
}

func testRemoteRenderCacheConstruction(t *testing.T) {
	t.Helper()
	for _, cfg := range []RenderCacheConfig{
		{},
		{BaseURL: "http://example.test", StorageDir: "/tmp", TTL: 0},
		{BaseURL: "", StorageDir: "/tmp", TTL: time.Second},
		{BaseURL: "http://example.test", StorageDir: "", TTL: time.Second},
	} {
		if NewRenderCacheClient(cfg) != nil {
			t.Fatalf("invalid config produced client: %+v", cfg)
		}
	}
	var nilClient *RenderCacheClient
	data, err := nilClient.Render("ignored", nil, func() ([]byte, error) { return []byte("ok"), nil })
	if err != nil || string(data) != "ok" {
		t.Fatalf("nil client Render = %q,%v", data, err)
	}
	data, err = nilClient.RenderContext(nil, "ignored", nil, func() ([]byte, error) { return []byte("ctx"), nil })
	if err != nil || string(data) != "ctx" {
		t.Fatalf("nil client RenderContext = %q,%v", data, err)
	}
	if shortRenderCacheKey(" short ") != "short" || shortRenderCacheKey("123456789012345") != "123456789012" {
		t.Fatal("short cache key branches failed")
	}
}

func testRemoteRenderCachePaths(t *testing.T) {
	t.Helper()
	var nilClient *RenderCacheClient
	root := t.TempDir()
	client := &RenderCacheClient{storageDir: root, imageCacheDir: filepath.Join(root, "images")}
	path := client.defaultFilePath("api/pjsk/card/list", "user", "key")
	if !strings.HasSuffix(path, filepath.Join("api", "pjsk", "card", "list", "user", "key.png")) {
		t.Fatalf("unexpected default path: %q", path)
	}
	if rel, ok := client.imageCacheRelativePath(filepath.Join(root, "images", "a", "b.png")); !ok || rel != "a/b.png" {
		t.Fatalf("relative image path = %q,%v", rel, ok)
	}
	for _, target := range []string{"", root, filepath.Join(root, "outside.png")} {
		if target == "" {
			client.imageCacheDir = ""
		} else {
			client.imageCacheDir = filepath.Join(root, "images")
		}
		if _, ok := client.imageCacheRelativePath(target); ok {
			t.Fatalf("unsafe image cache path accepted: %q", target)
		}
	}
	if _, ok := nilClient.imageCacheRelativePath("x"); ok {
		t.Fatal("nil client returned relative image path")
	}
}

func testRemoteRenderCacheFileTypes(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	if renderCacheFileExtFromData([]byte("plain")) != ".png" {
		t.Fatal("plain data should use png fallback")
	}
	if renderCacheFileExtFromData([]byte{0xff, 0xd8, 0xff, 0xe0}) != ".jpg" {
		t.Fatal("jpeg data was not detected")
	}
	if renderCacheFileExtFromData([]byte("GIF89a")) != ".gif" {
		t.Fatal("gif data was not detected")
	}
	if !renderCachePathWithin(root, filepath.Join(root, "child")) || renderCachePathWithin(root, filepath.Dir(root)) {
		t.Fatal("cache containment branches failed")
	}
	if _, _, err := absoluteContainedCachePath(root, filepath.Dir(root)); err == nil {
		t.Fatal("outside absolute cache path was accepted")
	}
	if err := ensureRenderCacheDirectory(root, root); err != nil {
		t.Fatalf("root cache directory rejected: %v", err)
	}
	if _, err := url.Parse("https://example.test"); err != nil {
		t.Fatal(err)
	}
}

func testRemoteRenderCacheFileIO(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	client := &RenderCacheClient{storageDir: root}
	target := filepath.Join(root, "api", "pjsk", "card", "public", "image.gif")
	prepared, err := client.prepareCacheTarget(target)
	if err != nil || prepared != target {
		t.Fatalf("prepare cache target = %q, %v", prepared, err)
	}
	data := []byte("GIF89a-render-cache")
	if err := writeRenderCacheFileAtomic(prepared, data); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	got, err := client.readCacheFile(prepared)
	if err != nil || string(got) != string(data) {
		t.Fatalf("read cache file = %q, %v", got, err)
	}
	if preparedAgain, err := client.prepareCacheTarget(target); err != nil || preparedAgain != target {
		t.Fatalf("prepare existing cache target = %q, %v", preparedAgain, err)
	}
	hash, contentPath := client.contentFilePath("api/pjsk/card", "public", "key", data)
	if len(hash) != sha256.Size*2 || filepath.Ext(contentPath) != ".gif" {
		t.Fatalf("content path = %q, hash=%q", contentPath, hash)
	}
}

func TestDrawingPointerProfileAndTimestampHelpers(t *testing.T) {
	testDrawingPointerAndProfileHelpers(t)
	testDrawingTimestampHelpers(t)
	testDetailedProfileDetection(t)
}

func testDrawingPointerAndProfileHelpers(t *testing.T) {
	t.Helper()
	if *StringPtr("value") != "value" || *IntPtr(7) != 7 || *Int64Ptr(8) != 8 {
		t.Fatal("pointer helpers returned unexpected values")
	}
	contextValue := NewCustomProfileContext(nil)
	if contextValue.User.UserID != 0 {
		t.Fatalf("nil custom profile context was nonzero: %+v", contextValue)
	}
	resp := &sekaiapi.GetAnotherProfileResponse{}
	resp.User.UserID = 123
	contextValue = NewCustomProfileContext(resp)
	if contextValue.User.UserID != 123 {
		t.Fatalf("custom profile context did not copy response: %+v", contextValue)
	}
	request := NewCustomProfileCardRenderRequest("jp", sekaiapi.UserCustomProfileCard{}, resp, nil)
	if request.SchemaVersion != 1 || request.RenderVersion != CustomProfileCardRenderVersion || request.Kind != CustomProfileCardRequestKind || request.Resources == nil || request.ProfileContext.User.UserID != 123 {
		t.Fatalf("unexpected custom profile request: %+v", request)
	}
	resources := CustomProfileResources{"asset": "path"}
	if got := NewCustomProfileCardRenderRequest("en", sekaiapi.UserCustomProfileCard{}, nil, resources); got.Resources["asset"] != "path" {
		t.Fatalf("custom resources were not preserved: %+v", got.Resources)
	}
}

func testDrawingTimestampHelpers(t *testing.T) {
	t.Helper()
	valid := []struct {
		value any
		want  int64
	}{
		{int(1), 1}, {int8(2), 2}, {int16(3), 3}, {int32(4), 4}, {int64(5), 5},
		{uint(6), 6}, {uint8(7), 7}, {uint16(8), 8}, {uint32(9), 9}, {uint64(10), 10},
		{float32(11.9), 11}, {float64(12.9), 12}, {" 13 ", 13},
	}
	for _, tc := range valid {
		got, ok := parseDrawingUpdateTime(tc.value)
		if !ok || got != tc.want {
			t.Fatalf("parseDrawingUpdateTime(%T(%v)) = %d,%v want %d", tc.value, tc.value, got, ok, tc.want)
		}
	}
	for _, invalid := range []any{^uint64(0), "", "bad", true, nil} {
		if got, ok := parseDrawingUpdateTime(invalid); ok || got != 0 {
			t.Fatalf("invalid timestamp %T(%v) = %d,%v", invalid, invalid, got, ok)
		}
	}
	if got := normalizeDrawingUpdateTime("bad", 99); got != 99 {
		t.Fatalf("invalid timestamp fallback = %d", got)
	}
	if got := normalizeDrawingUpdateTime(10, 99); got != 10_000 {
		t.Fatalf("seconds timestamp = %d", got)
	}
	if got := normalizeDrawingUpdateTime(drawingTimestampMsThreshold, 99); got != drawingTimestampMsThreshold {
		t.Fatalf("millisecond timestamp = %d", got)
	}
}

func testDetailedProfileDetection(t *testing.T) {
	t.Helper()
	if looksLikeDetailedProfileCard(nil) || looksLikeDetailedProfileCard(map[string]any{}) {
		t.Fatal("empty profile classified as detailed")
	}
	if !looksLikeDetailedProfileCard(map[string]any{"source": "remote"}) ||
		!looksLikeDetailedProfileCard(map[string]any{"user_cards": []any{}}) ||
		!looksLikeDetailedProfileCard(map[string]any{"update_time": 1, "leader_image_path": "x"}) {
		t.Fatal("detailed profile markers were not recognized")
	}
}

func TestDrawingClientSeparateCacheRequestWrapper(t *testing.T) {
	var client *HarukiDrawingClient
	cacheRequest := map[string]any{"cache": true}
	renderRequest := map[string]any{"render": true}
	prepared := false
	data, err := client.RenderWithCacheRequestAndPrepare(
		"/api/pjsk/event/detail",
		cacheRequest,
		renderRequest,
		func(value any) error {
			prepared = true
			if value.(map[string]any)["render"] != true {
				t.Fatalf("prepare received wrong request: %#v", value)
			}
			return nil
		},
		func(value any) ([]byte, error) {
			if value.(map[string]any)["render"] != true {
				t.Fatalf("render received wrong request: %#v", value)
			}
			return []byte("separate"), nil
		},
	)
	if err != nil || string(data) != "separate" || !prepared {
		t.Fatalf("separate wrapper = %q,%v prepared=%v", data, err, prepared)
	}
	wantErr := errors.New("prepare")
	if _, err := client.RenderWithCacheRequestAndPrepare("/api/pjsk/event/detail", nil, nil, func(any) error { return wantErr }, func(any) ([]byte, error) { return nil, nil }); !errors.Is(err, wantErr) {
		t.Fatalf("prepare error = %v", err)
	}
	data, err = client.RenderWithCacheRequestAndPrepare("/api/pjsk/event/detail", nil, nil, nil, func(any) ([]byte, error) { return []byte("nil prepare"), nil })
	if err != nil || string(data) != "nil prepare" {
		t.Fatalf("nil prepare wrapper = %q,%v", data, err)
	}
}

func TestDrawingLimiterRemainingBranches(t *testing.T) {
	testDrawingLimiterConfiguration(t)
	testDrawingSlotAcquisition(t)
	testDrawingLimiterAcquisition(t)
}

func testDrawingLimiterConfiguration(t *testing.T) {
	t.Helper()
	option := WithLimiter(LimiterConfig{})
	option(nil, nil)
	client := &HarukiDrawingClient{}
	option(nil, client)
	if client.limiter == nil || cap(client.limiter.sk) != defaultDrawingSKMaxConcurrency || client.limiter.acquireTimeout != defaultDrawingAcquireTimeout {
		t.Fatalf("default limiter = %+v", client.limiter)
	}
	override := newDrawingLimiter(LimiterConfig{MaxConcurrency: 2})
	if cap(override.global) != 2 || cap(override.sk) != 2 {
		t.Fatalf("global limiter override = %+v", override)
	}
	if permit, err := (*HarukiDrawingClient)(nil).acquireRenderPermit("x"); err != nil || permit != (drawingPermit{}) {
		t.Fatalf("nil client permit = %+v,%v", permit, err)
	}
	if permit, err := (&HarukiDrawingClient{}).acquireRenderPermit("x"); err != nil || permit != (drawingPermit{}) {
		t.Fatalf("unlimited client permit = %+v,%v", permit, err)
	}
	if permit, err := (*drawingLimiter)(nil).acquire(nil, "x"); err != nil || permit != (drawingPermit{}) {
		t.Fatalf("nil limiter permit = %+v,%v", permit, err)
	}
}

func testDrawingSlotAcquisition(t *testing.T) {
	t.Helper()
	if err := acquireDrawingSlot(nil, nil, 0); err != nil {
		t.Fatalf("nil drawing slot = %v", err)
	}
	channel := make(chan struct{}, 1)
	if err := acquireDrawingSlot(nil, channel, 0); err != nil {
		t.Fatalf("blocking drawing slot = %v", err)
	}
	<-channel
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	channel <- struct{}{}
	if err := acquireDrawingSlot(canceled, channel, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled drawing slot = %v", err)
	}
	<-channel
	channel <- struct{}{}
	if err := acquireDrawingSlot(context.Background(), channel, time.Millisecond); err == nil {
		t.Fatal("full drawing slot did not time out")
	}
	<-channel
}

func testDrawingLimiterAcquisition(t *testing.T) {
	t.Helper()
	limiter := &drawingLimiter{
		global:         make(chan struct{}, 1),
		sk:             make(chan struct{}, 1),
		acquireTimeout: time.Millisecond,
	}
	permit, err := limiter.acquire(context.Background(), "/api/pjsk/profile")
	if err != nil || permit.global == nil || permit.sk != nil {
		t.Fatalf("global-only permit = %+v,%v", permit, err)
	}
	permit.release()
	permit, err = limiter.acquire(context.Background(), "/api/pjsk/sk/query")
	if err != nil || permit.global == nil || permit.sk == nil {
		t.Fatalf("SK permit = %+v,%v", permit, err)
	}
	permit.release()

	limiter.sk <- struct{}{}
	if permit, err := limiter.acquire(context.Background(), "/api/pjsk/sk/query"); err == nil || permit != (drawingPermit{}) || len(limiter.global) != 0 {
		t.Fatalf("failed SK acquire did not release global: %+v,%v global=%d", permit, err, len(limiter.global))
	}
	<-limiter.sk
}
