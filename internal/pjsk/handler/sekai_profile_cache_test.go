package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	harukiConfig "haruki-cloud/config"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	json "github.com/bytedance/sonic"
	"golang.org/x/sync/singleflight"
)

func TestFetchCachedSekaiUserProfileUsesCloudCache(t *testing.T) {
	oldTTL := harukiConfig.Cfg.Backend.APICacheTTL
	harukiConfig.Cfg.Backend.APICacheTTL = time.Minute
	t.Cleanup(func() {
		harukiConfig.Cfg.Backend.APICacheTTL = oldTTL
		sekaiProfileCacheMu.Lock()
		sekaiProfileCache = make(map[string]sekaiProfileCacheEntry)
		sekaiProfileCacheMu.Unlock()
		sekaiProfileCacheGroup = singleflight.Group{}
		sekaiProfileCacheNextCleanup.Store(0)
	})

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		resp := &sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 12345678901234,
				Name:   "cached-user",
				Rank:   88,
			},
		}
		raw, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	app := &renderapp.App{
		SekaiAPI: sekaiapi.NewSekaiAPIClient(&harukiConfig.SekaiAPIConfig{BaseURL: server.URL}),
	}

	first, err := fetchCachedSekaiUserProfile(context.Background(), app, "jp", "12345678901234")
	if err != nil {
		t.Fatalf("first fetch error: %v", err)
	}
	first.User.Name = "mutated"

	second, err := fetchCachedSekaiUserProfile(context.Background(), app, "jp", "12345678901234")
	if err != nil {
		t.Fatalf("second fetch error: %v", err)
	}

	if hits.Load() != 1 {
		t.Fatalf("expected one upstream fetch, got %d", hits.Load())
	}
	if second.User.Name != "cached-user" {
		t.Fatalf("expected cached profile clone, got %q", second.User.Name)
	}
}
