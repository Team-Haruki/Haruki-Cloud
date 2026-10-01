package drawing

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/core/upstream"
)

func TestForceRenderContextHelpers(t *testing.T) {
	var absent context.Context
	if ForceRenderFrom(absent) || ForceRenderFrom(context.Background()) {
		t.Fatal("unforced context reported force")
	}
	if !ForceRenderFrom(WithForceRender(absent)) {
		t.Fatal("WithForceRender(nil) lost the flag")
	}
	forced := WithForceRender(t.Context())
	if got := forceRenderFlightKey(forced, "k"); got != "k"+forceRenderFlightSuffix {
		t.Fatalf("forced flight key = %q", got)
	}
	if got := forceRenderFlightKey(t.Context(), "k"); got != "k" {
		t.Fatalf("plain flight key = %q", got)
	}
}

func TestRunSharedRenderFlightKeepsForce(t *testing.T) {
	result := runSharedRenderFlight(WithForceRender(t.Context()), func(ctx context.Context) ([]byte, error) {
		if !ForceRenderFrom(ctx) {
			t.Error("shared flight dropped the force flag")
		}
		return []byte("ok"), nil
	})
	if result.err != nil {
		t.Fatal(result.err)
	}
}

// Forced renders skip the cache, tell Drawing to bypass its own caches, and
// leave the fresh image for the next unforced request.
func TestForcedRenderBypassesAndRefreshesRenderCache(t *testing.T) {
	var calls atomic.Int32
	var forcedHeaders atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Header.Get(headerRenderForce) == "1" {
			forcedHeaders.Add(1)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = fmt.Fprintf(w, "render-%d", n)
	}))
	defer server.Close()
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(ArtifactConfig{Endpoints: []string{"*"}}))
	client.SetRenderCache(newIndexClient(t, &fakeRenderIndex{}))
	client.versions = versionFixture([]upstream.TargetConfig{{BaseURL: server.URL}}, map[string]string{server.URL: strings.Repeat("a", 64)})

	render := func(ctx context.Context) string {
		t.Helper()
		data, err := client.WithContext(ctx).GenerateCardList(&CardListRequest{})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if got := render(t.Context()); got != "render-1" {
		t.Fatalf("first render = %q", got)
	}
	if got := render(t.Context()); got != "render-1" {
		t.Fatalf("cached render = %q", got)
	}
	if got := render(WithForceRender(t.Context())); got != "render-2" {
		t.Fatalf("forced render = %q", got)
	}
	if got := render(t.Context()); got != "render-2" {
		t.Fatalf("render after force = %q, want the refreshed image", got)
	}
	if calls.Load() != 2 || forcedHeaders.Load() != 1 {
		t.Fatalf("drawing calls = %d, forced headers = %d", calls.Load(), forcedHeaders.Load())
	}
}

func TestLocalRenderCacheForceBypassesAndStores(t *testing.T) {
	cache := newLocalRenderCache(time.Hour)
	var renders atomic.Int32
	render := func(context.Context) ([]byte, error) {
		return fmt.Appendf(nil, "local-%d", renders.Add(1)), nil
	}
	request := map[string]any{"region": "jp"}
	endpoint := "/api/pjsk/card/list"
	for _, step := range []struct {
		ctx  context.Context
		want string
	}{
		{t.Context(), "local-1"},
		{t.Context(), "local-1"},
		{WithForceRender(t.Context()), "local-2"},
		{t.Context(), "local-2"},
	} {
		data, err := cache.RenderSharedContext(step.ctx, endpoint, request, render)
		if err != nil || string(data) != step.want {
			t.Fatalf("render = %q, %v; want %q", data, err, step.want)
		}
	}
}
