package drawing

import (
	"context"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func deepCloneRenderPayload(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = deepCloneRenderPayload(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = deepCloneRenderPayload(item)
		}
		return out
	default:
		return value
	}
}

func TestBuildDirectivePolicyMatchesProductionKeyOnEnabledEndpoints(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		endpoint string
		body     any
	}{
		{"/api/pjsk/card/box", map[string]any{"region": "jp", "cards": []any{map[string]any{"id": 1}}, "dt": 123}},
		{"/api/pjsk/profile", map[string]any{"profile": map[string]any{"user_id": "12345", "name": "a"}, "region": "jp"}},
		{"/api/pjsk/music/list?show_id=true&show_leak=false", map[string]any{"region": "cn", "musics": []any{1, 2, 3}}},
		{"/api/pjsk/sk/query", map[string]any{"region": "jp", "ranks": []any{1, 2}, "dt": 1}},
		{"/api/pjsk/mysekai/fixture-detail", []any{map[string]any{"fixture_id": 7, "region": "tw"}}},
		{"/api/pjsk/misc/chara-birthday", map[string]any{"cid": 3, "region": "jp"}},
	}
	for _, tc := range cases {
		t.Run(tc.endpoint, func(t *testing.T) {
			prepared := prepareDrawingRequestBody(tc.endpoint, tc.body, now, context.Background())
			before := deepCloneRenderPayload(prepared)
			directivePolicy, err := buildDirectivePolicy(tc.endpoint, prepared)
			if err != nil {
				t.Fatalf("directive policy: %v", err)
			}
			if !reflect.DeepEqual(prepared, before) {
				t.Fatal("buildDirectivePolicy mutated the prepared body")
			}
			production, err := buildRenderCachePolicy(tc.endpoint, preparedRenderCachePayload{payload: prepared})
			if err != nil {
				t.Fatalf("production policy: %v", err)
			}
			directiveKey, err := buildRenderCacheKey(directivePolicy)
			if err != nil {
				t.Fatal(err)
			}
			productionKey, err := buildRenderCacheKey(production)
			if err != nil {
				t.Fatal(err)
			}
			if directiveKey != productionKey || !isHex64(directiveKey) {
				t.Fatalf("directive key %s != production key %s", directiveKey, productionKey)
			}
			if directivePolicy.TTL != production.TTL || directivePolicy.Infinite != production.Infinite || directivePolicy.APIPath != production.APIPath || directivePolicy.UserID != production.UserID {
				t.Fatalf("policy drift: %+v vs %+v", directivePolicy, production)
			}
		})
	}
}

func TestBuildDirectivePolicyKeysUncachedEndpointsStably(t *testing.T) {
	first := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	later := first.Add(time.Hour)
	keys := map[string]string{}
	ttl := map[string]time.Duration{}
	for _, tc := range []struct {
		endpoint string
		body     any
	}{
		{"/api/pjsk/event/detail", &EventDetailRequest{}},
		{"/api/pjsk/misc/alias-list", &AliasListRequest{}},
	} {
		var got []string
		for _, now := range []time.Time{first, later} {
			prepared := prepareDrawingRequestBody(tc.endpoint, tc.body, now, context.Background())
			policy, err := buildDirectivePolicy(tc.endpoint, prepared)
			if err != nil {
				t.Fatalf("%s policy: %v", tc.endpoint, err)
			}
			key, err := buildRenderCacheKey(policy)
			if err != nil || !isHex64(key) {
				t.Fatalf("%s key = %q, %v", tc.endpoint, key, err)
			}
			got = append(got, key)
			ttl[tc.endpoint] = time.Duration(directiveTTLSeconds(policy.TTL, policy.Infinite)) * time.Second
		}
		if got[0] != got[1] {
			t.Fatalf("%s key moved with dt: %v", tc.endpoint, got)
		}
		keys[tc.endpoint] = got[0]
	}
	if keys["/api/pjsk/event/detail"] == keys["/api/pjsk/misc/alias-list"] {
		t.Fatal("uncached endpoints share a key")
	}
	if ttl["/api/pjsk/event/detail"] != 24*time.Hour || ttl["/api/pjsk/misc/alias-list"] != 0 {
		t.Fatalf("ttl = %v", ttl)
	}
	if _, err := buildRenderCachePolicy("/api/pjsk/event/detail", preparedRenderCachePayload{payload: map[string]any{}}); err == nil {
		t.Fatal("protected policy builder no longer refuses the disabled endpoint")
	}
}

func TestBuildDirectivePolicyErrors(t *testing.T) {
	if _, err := buildDirectivePolicy("", map[string]any{}); err == nil {
		t.Fatal("empty endpoint keyed")
	}
	if _, err := buildDirectivePolicy("/api/pjsk/card/box", make(chan int)); err == nil {
		t.Fatal("unencodable payload keyed")
	}
	if _, err := buildDirectivePolicy("/", map[string]any{}); err == nil {
		t.Fatal("empty api path keyed")
	}
	policy, err := buildDirectivePolicy("/api/pjsk/card/box", struct {
		Region string `json:"region"`
	}{Region: "jp"})
	if err != nil || policy.APIPath != "api/pjsk/card/box" {
		t.Fatalf("struct payload policy = %+v, %v", policy, err)
	}
}

func TestNewUncachedDirectiveDropsUnkeyableAndNonAllowListed(t *testing.T) {
	client := NewHarukiDrawingClient("http://drawing.invalid", WithArtifactConfig(ArtifactConfig{Endpoints: []string{"api/pjsk/misc/alias-list"}}))
	if d, ok := client.newUncachedDirective("/api/pjsk/misc/alias-list", make(chan int)); ok || d != nil {
		t.Fatalf("unkeyable directive = %+v", d)
	}
	if d, ok := client.newUncachedDirective("/api/pjsk/event/detail", map[string]any{}); ok || d != nil {
		t.Fatalf("non allow-listed directive = %+v", d)
	}
	var nilClient *HarukiDrawingClient
	if _, ok := nilClient.newUncachedDirective("/api/pjsk/misc/alias-list", map[string]any{}); ok {
		t.Fatal("nil client built a directive")
	}
	d, ok := client.newUncachedDirective("/api/pjsk/misc/alias-list", map[string]any{"region": "jp"})
	if !ok || d.Store || !d.Artifact || d.TTLSeconds != 0 || d.Group != "pjsk" || d.APIPath != "api/pjsk/misc/alias-list" || d.outcome == nil {
		t.Fatalf("directive = %+v", d)
	}
}

// Attach point C is deliberately not taken: every rule-disabled endpoint must
// reach Drawing through postUncached, never through cachedPost.
func TestRenderCacheDisabledEndpointsUsePostUncached(t *testing.T) {
	source, err := os.ReadFile("client.go")
	if err != nil {
		t.Fatal(err)
	}
	callSites := map[string]bool{}
	for _, match := range regexp.MustCompile(`c\.postUncached\("([^"]+)"`).FindAllStringSubmatch(string(source), -1) {
		callSites[match[1]] = true
	}
	if len(callSites) != 2 {
		t.Fatalf("postUncached call sites = %v", callSites)
	}
	for endpoint := range renderCacheDisabledEndpoints {
		if !callSites[endpoint] {
			t.Fatalf("disabled endpoint %s is not a postUncached call site", endpoint)
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	legacyPost := regexp.MustCompile(`func \(c \*HarukiDrawingClient\) post\(|\bc\.post\(`)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if legacyPost.Match(data) {
			t.Fatalf("%s still uses the legacy post helper", name)
		}
	}
}

func TestRenderCacheKeyVersionForAndTTLEncoding(t *testing.T) {
	if renderCacheKeyVersionFor("api/pjsk/event/list") != 5 || renderCacheKeyVersionFor("api/pjsk/card/box") != 3 {
		t.Fatal("key version drift")
	}
	if directiveTTLSeconds(0, true) != 0 || directiveTTLSeconds(1500*time.Millisecond, false) != 2 || directiveTTLSeconds(0, false) != 1 {
		t.Fatal("ttl encoding drift")
	}
	d := &renderDirective{TTLSeconds: 60 * 86400}
	if d.wireTTLSeconds() != drawingMaxCacheTTLSeconds || d.TTLSeconds != 60*86400 {
		t.Fatalf("clamp = %d / %d", d.wireTTLSeconds(), d.TTLSeconds)
	}
	if got, ok := directiveFrom(noContext); ok || got != nil {
		t.Fatal("nil context carries a directive")
	}
	if got, ok := directiveFrom(withDirective(noContext, nil)); ok || got != nil {
		t.Fatal("nil directive reported present")
	}
}
