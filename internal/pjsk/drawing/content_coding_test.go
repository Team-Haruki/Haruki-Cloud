package drawing

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/httpcoding"
)

type fakeDrawingNode struct {
	advertise bool
	refuse    bool

	mu       sync.Mutex
	encoded  int
	identity int
	refused  int
	lastBody string
}

func (f *fakeDrawingNode) serve(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.advertise {
			w.Header().Set("Accept-Encoding", "zstd")
		}
		if r.URL.Path == "/cache/identity" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":1,"renderer_epoch":"` + strings.Repeat("a", 64) + `"}`))
			return
		}
		if r.Header.Get("Content-Encoding") == "zstd" {
			if f.refuse {
				f.refused++
				w.WriteHeader(http.StatusUnsupportedMediaType)
				return
			}
			decoded, err := httpcoding.DecodeBody("zstd", raw, 64<<20)
			if err != nil {
				t.Errorf("node could not decode: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			raw = decoded
			f.encoded++
		} else {
			f.identity++
		}
		f.lastBody = string(raw)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	}))
	t.Cleanup(server.Close)
	return server
}

func bigRenderRequest() map[string]any {
	cards := make([]map[string]any, 0, 300)
	for i := range 300 {
		cards = append(cards, map[string]any{"card_id": i, "thumbnail_path": "asset/jp-assets/thumbnail/chara/res" + strings.Repeat("0", 3) + "_normal.png", "level": 60})
	}
	return map[string]any{"cards": cards}
}

func TestDrawingRequestsUseZstdOnlyAfterAdvertisement(t *testing.T) {
	node := &fakeDrawingNode{advertise: true}
	server := node.serve(t)
	client := NewHarukiDrawingClient(server.URL).WithContext(context.Background())

	if _, err := client.postPrepared("/api/pjsk/card/box", bigRenderRequest()); err != nil {
		t.Fatalf("first render: %v", err)
	}
	if node.identity != 1 || node.encoded != 0 {
		t.Fatalf("first request must be identity: identity=%d encoded=%d", node.identity, node.encoded)
	}
	if _, err := client.postPrepared("/api/pjsk/card/box", bigRenderRequest()); err != nil {
		t.Fatalf("second render: %v", err)
	}
	if node.encoded != 1 {
		t.Fatalf("second request must be zstd, encoded=%d", node.encoded)
	}
	if !strings.Contains(node.lastBody, `"card_id":299`) {
		t.Fatal("node must receive the full decoded body")
	}
	// Small bodies stay identity even once supported.
	if _, err := client.postPrepared("/api/pjsk/profile", map[string]any{"id": 1}); err != nil {
		t.Fatalf("small render: %v", err)
	}
	if node.identity != 2 {
		t.Fatalf("small body must be identity, identity=%d", node.identity)
	}
}

func TestDrawingRefusedZstdFallsBackToIdentity(t *testing.T) {
	node := &fakeDrawingNode{advertise: true, refuse: true}
	server := node.serve(t)
	client := NewHarukiDrawingClient(server.URL).WithContext(context.Background())
	client.coding.Observe(server.URL, http.Header{"Accept-Encoding": {"zstd"}})

	data, err := client.postPrepared("/api/pjsk/card/box", bigRenderRequest())
	if err != nil || string(data) != "png-bytes" {
		t.Fatalf("render after refusal: data=%q err=%v", data, err)
	}
	if node.refused != 1 || node.identity != 1 {
		t.Fatalf("refused=%d identity=%d", node.refused, node.identity)
	}
	if client.coding.Supports(server.URL) {
		t.Fatal("a refusal must withdraw zstd for the node")
	}
}

func TestDrawingIdentityPollLearnsAdvertisement(t *testing.T) {
	node := &fakeDrawingNode{advertise: true}
	server := node.serve(t)
	client := NewHarukiDrawingClient(server.URL)
	v := &cacheVersions{http: server.Client(), coding: client.coding}
	if _, err := v.identity(context.Background(), server.URL); err != nil {
		t.Fatalf("identity: %v", err)
	}
	if !client.coding.Supports(server.URL) {
		t.Fatal("identity poll must record the advertisement")
	}
}

func TestDrawingPoolTargetsNegotiateIndependently(t *testing.T) {
	newer := &fakeDrawingNode{advertise: true}
	older := &fakeDrawingNode{}
	a, b := newer.serve(t), older.serve(t)
	client := NewHarukiDrawingClientWithTargets("", []upstream.TargetConfig{{Name: "a", BaseURL: a.URL}, {Name: "b", BaseURL: b.URL}})
	client.coding.Observe(a.URL, http.Header{"Accept-Encoding": {"zstd"}})
	if !client.coding.Supports(a.URL) || client.coding.Supports(b.URL) {
		t.Fatal("support must be tracked per node")
	}
	for range 4 {
		if _, err := client.WithContext(context.Background()).postPrepared("/api/pjsk/card/box", bigRenderRequest()); err != nil {
			t.Fatalf("render: %v", err)
		}
	}
	if older.encoded != 0 {
		t.Fatalf("a node that never advertised must never get zstd, got %d", older.encoded)
	}
}
