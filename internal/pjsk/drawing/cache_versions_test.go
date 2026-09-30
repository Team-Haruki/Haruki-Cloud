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
	json "haruki-cloud/internal/jsonutil"
)

func versionFixture(targets []upstream.TargetConfig, epochs map[string]string) *cacheVersions {
	_, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	close(done)
	v := &cacheVersions{cfg: CacheVersionConfig{MaxStale: time.Minute}, targets: targets, cancel: cancel, done: done}
	state := &rendererIdentities{epochs: epochs, checked: map[string]time.Time{}}
	for base := range epochs {
		state.checked[base] = time.Now()
	}
	v.current.Store(state)
	return v
}

func TestRendererVersionSetSupportsStableHeterogeneousNodesAndChangesKeys(t *testing.T) {
	targets := []upstream.TargetConfig{{BaseURL: "http://amd64"}, {BaseURL: "http://arm64"}}
	epochs := map[string]string{"http://amd64": strings.Repeat("a", 64), "http://arm64": strings.Repeat("b", 64)}
	v := versionFixture(targets, epochs)
	original := v.snapshot(map[string]any{})
	if !original.ready {
		t.Fatal("stable heterogeneous renderer set must be cacheable")
	}
	first := versionedCacheKey(attachVersions(t.Context(), original), "payload")
	v.targets = []upstream.TargetConfig{targets[1], targets[0]}
	if reordered := versionedCacheKey(attachVersions(t.Context(), v.snapshot(nil)), "payload"); reordered != first {
		t.Fatal("target order changed key")
	}
	v.current.Load().epochs["http://amd64"] = strings.Repeat("c", 64)
	next := versionedCacheKey(attachVersions(t.Context(), v.snapshot(nil)), "payload")
	if next == first {
		t.Fatal("renderer rollout reused old key")
	}
	delete(v.current.Load().epochs, "http://arm64")
	if v.snapshot(nil).ready {
		t.Fatal("unknown target identity must bypass caches")
	}
}

type atomicResourceVersions struct{}

func (atomicResourceVersions) Revision() string              { panic("non-atomic global read") }
func (atomicResourceVersions) RevisionForPayload(any) string { panic("non-atomic scoped read") }
func (atomicResourceVersions) SnapshotForPayload(any) (string, string) {
	return strings.Repeat("a", 64), strings.Repeat("b", 64)
}
func TestCacheVersionUsesAtomicResourceSnapshot(t *testing.T) {
	v := versionFixture(nil, nil)
	v.resources = atomicResourceVersions{}
	got := v.snapshot(map[string]any{})
	if got.asset != strings.Repeat("a", 64) || got.scoped != strings.Repeat("b", 64) {
		t.Fatal(got)
	}
}

func TestDrawingNoStoreResponseBypassesPendingAndLocalCaches(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(fmt.Sprintf("remote=%v", remote), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count := calls.Add(1)
				w.Header().Set("Content-Type", "image/png")
				if count <= 2 {
					w.Header().Set(headerCacheStore, "0")
					_, _ = w.Write([]byte("placeholder"))
					return
				}
				_, _ = w.Write([]byte("complete"))
			}))
			defer server.Close()
			client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(ArtifactConfig{Endpoints: []string{"*"}}))
			if remote {
				client.SetRenderCache(newIndexClient(t, &fakeRenderIndex{}))
			}
			for i, want := range []string{"placeholder", "placeholder", "complete", "complete"} {
				data, err := client.GenerateCardList(&CardListRequest{})
				if err != nil || string(data) != want {
					t.Fatalf("request %d = %s %v", i, data, err)
				}
			}
			if calls.Load() != 3 {
				t.Fatalf("render calls = %d; no-store must retry, complete response must cache", calls.Load())
			}
		})
	}
}

func TestUnknownRendererIdentityBypassesExistingCacheAndSendsStoreZero(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get(headerCacheStore) != "0" {
			t.Error("unknown identity did not disable Drawing cache")
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("fresh"))
	}))
	defer server.Close()
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(ArtifactConfig{Endpoints: []string{"*"}}))
	client.SetRenderCache(newIndexClient(t, &fakeRenderIndex{}))
	client.versions = versionFixture([]upstream.TargetConfig{{BaseURL: server.URL}}, nil)
	for range 2 {
		if _, err := client.GenerateCardList(&CardListRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("unknown identity cached %d calls", calls.Load())
	}
}

func TestActualRendererEpochHeaderAndNoStoreAfterIdentityChanged(t *testing.T) {
	expected := strings.Repeat("a", 64)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get(headerCacheKeyVersion) != "6" {
			t.Errorf("versioned key header = %s", r.Header.Get(headerCacheKeyVersion))
		}
		if r.Header.Get("X-Haruki-Renderer-Epoch") != expected {
			t.Errorf("selected epoch = %s", r.Header.Get("X-Haruki-Renderer-Epoch"))
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set(headerCacheStore, "0") // renderer changed after the last identity poll
		_, _ = w.Write([]byte("new-renderer"))
	}))
	defer server.Close()
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(ArtifactConfig{Endpoints: []string{"*"}}))
	client.SetRenderCache(newIndexClient(t, &fakeRenderIndex{}))
	client.versions = versionFixture([]upstream.TargetConfig{{BaseURL: server.URL}}, map[string]string{server.URL: expected})
	for range 2 {
		if _, err := client.GenerateCardList(&CardListRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("changed renderer response was cached under old epoch")
	}
}

// Only an asset carried by the actual Drawing body changes in this source.
type renderOnlyResourceVersion struct{ revision string }

func (v *renderOnlyResourceVersion) Revision() string { return hashVersion(v.revision) }
func (v *renderOnlyResourceVersion) RevisionForPayload(payload any) string {
	data, _ := json.Marshal(payload)
	if strings.Contains(string(data), "jp-assets/startapp/costume/preview.png") {
		return hashVersion(v.revision)
	}
	return ""
}

func TestSeparateRenderBodyContributesResourceVersionWithoutEagerHook(t *testing.T) {
	client := NewHarukiDrawingClient("http://drawing.invalid")
	client.versions = versionFixture([]upstream.TargetConfig{{BaseURL: client.baseURL}}, map[string]string{client.baseURL: strings.Repeat("a", 64)})
	resources := &renderOnlyResourceVersion{revision: "before"}
	client.versions.resources = resources
	cacheRequest := map[string]any{"costume_id": 1}
	renderRequest := struct {
		ImagePath string `json:"image_path"`
	}{ImagePath: "jp-assets/startapp/costume/preview.png"}
	var prepares, renders int
	for i, revision := range []string{"before", "before", "after", "after"} {
		resources.revision = revision
		data, err := client.RenderWithCacheRequestAndPrepare("/api/pjsk/card/list", cacheRequest, renderRequest, func(any) error {
			prepares++
			return nil
		}, func(any) ([]byte, error) {
			renders++
			return []byte(revision), nil
		})
		if err != nil || string(data) != revision {
			t.Fatalf("request %d = %q, %v", i, data, err)
		}
	}
	if prepares != 2 || renders != 2 {
		t.Fatalf("prepare/render = %d/%d, want one of each per asset version", prepares, renders)
	}
}
