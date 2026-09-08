package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/imagecache"
)

func TestRenderedImageMessageSkipsImageReadHashAndStore(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "cached.png")
	if err := os.WriteFile(file, make([]byte, 2<<20), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("cache hit tried to register image")
		}
		raw, _ := json.Marshal(map[string]string{"file_path": file})
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
	}))
	defer server.Close()
	cache := drawing.NewRenderCacheClient(drawing.RenderCacheConfig{BaseURL: server.URL, StorageDir: root, TTL: time.Hour})
	ctx, trace := commandtrace.WithTrace(t.Context())
	result, err := cache.RenderImageSharedContext(ctx, "/api/pjsk/card/list", &drawing.CardListRequest{Region: "jp"}, func(context.Context) ([]byte, error) { t.Error("cache hit rendered again"); return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	rc := &RequestContext{Ctx: ctx, App: &renderapp.App{ImageCache: imagecache.New("https://cdn.test", root)}}
	message, err := rc.RenderedImageMessage(result)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(message)
	if !strings.Contains(string(raw), "https://cdn.test/cached.png") {
		t.Fatalf("message=%s", raw)
	}
	for _, op := range trace.Snapshot().Operations {
		switch op.Name {
		case "drawing.cache_read", "image.hash", "image.lookup", "image.store":
			t.Errorf("artifact path performed %s", op.Name)
		}
	}
}
