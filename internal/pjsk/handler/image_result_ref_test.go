package handler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/core/urlhost"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/imagecache"
)

func refMessageJSON(t *testing.T, rc *RequestContext, image drawing.ImageResult) string {
	t.Helper()
	message, err := rc.RenderedImageMessage(image)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(message)
	return string(raw)
}

func TestRenderedImageMessageRefPrefersRenderingNode(t *testing.T) {
	hosts, err := urlhost.New(map[string]string{
		"cn01": "https://ic-cn01.example",
		"cn09": "https://ic-cn09.example",
	}, urlhost.Options{Order: []string{"cn01", "cn09"}})
	if err != nil {
		t.Fatal(err)
	}
	rc := &RequestContext{Ctx: t.Context(), App: &renderapp.App{ImageHosts: hosts}}
	ref := &drawing.ArtifactRef{CDNPath: "pjsk/api/pjsk/card/box/a b.png", NodeName: "cn09"}
	for range 3 {
		if raw := refMessageJSON(t, rc, drawing.ImageArtifact(ref)); !strings.Contains(raw, "https://ic-cn09.example/pjsk/api/pjsk/card/box/a%20b.png") {
			t.Fatalf("message=%s", raw)
		}
	}
	hosts.MarkFailure("cn09")
	if raw := refMessageJSON(t, rc, drawing.ImageArtifact(ref)); !strings.Contains(raw, "https://ic-cn01.example/") {
		t.Fatalf("cooling node still preferred: %s", raw)
	}
}

type handlerFakeIndex struct{ entry imagecache.RenderIndexEntry }

func (f handlerFakeIndex) LookupRender(context.Context, string) (imagecache.RenderIndexEntry, bool, error) {
	return f.entry, true, nil
}
func (handlerFakeIndex) TouchRender(context.Context, []string) (int64, error)  { return 0, nil }
func (handlerFakeIndex) DeleteRender(context.Context, []string) (int64, error) { return 0, nil }

func TestRenderedImageMessageIndexHitEmitsHostURL(t *testing.T) {
	index := handlerFakeIndex{entry: imagecache.RenderIndexEntry{
		ContentHash: strings.Repeat("c", 64), TTLSeconds: 0,
		Entry: imagecache.ImageEntry{CDNPath: "pjsk/api/pjsk/card/list/c.png", StorageBackend: imagecache.BackendGarage},
	}}
	cache := drawing.NewRenderCacheClient(drawing.RenderCacheConfig{TTL: time.Hour, Index: index})
	defer func() { _ = cache.Close() }()
	result, err := cache.RenderImageSharedContext(t.Context(), "/api/pjsk/card/list", &drawing.CardListRequest{Region: "jp"},
		func(context.Context) ([]byte, error) { t.Error("index hit rendered"); return nil, nil })
	if err != nil || result.Ref() == nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	rc := &RequestContext{Ctx: t.Context(), App: &renderapp.App{ImageHosts: urlhost.Single("https://ic.example")}}
	if raw := refMessageJSON(t, rc, result); !strings.Contains(raw, "https://ic.example/pjsk/api/pjsk/card/list/c.png") {
		t.Fatalf("message=%s", raw)
	}
}

func TestRenderedImageMessageRefWithoutHostsFallsBackToBytes(t *testing.T) {
	ref := &drawing.ArtifactRef{CDNPath: "pjsk/x.png"}
	for name, app := range map[string]*renderapp.App{"no hosts": {}, "no app": nil} {
		t.Run(name, func(t *testing.T) {
			rc := &RequestContext{Ctx: t.Context(), App: app}
			if _, err := rc.RenderedImageMessage(drawing.ImageArtifact(ref)); !errors.Is(err, drawing.ErrArtifactBytesUnavailable) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
