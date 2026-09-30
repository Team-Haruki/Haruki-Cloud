package pjsk

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	harukiConfig "haruki-cloud/config"
	noiseCrypto "haruki-cloud/internal/core/crypto"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/assets"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
	"haruki-cloud/utils/imagecache"

	"github.com/gofiber/fiber/v3"
	noiseMP "github.com/shamaton/msgpack/v3"
)

// countingRenderIndex counts render_cache_index lookups; it never hits.
type countingRenderIndex struct{ lookups atomic.Int32 }

func (i *countingRenderIndex) LookupRender(context.Context, string) (imagecache.RenderIndexEntry, bool, error) {
	i.lookups.Add(1)
	return imagecache.RenderIndexEntry{}, false, nil
}
func (*countingRenderIndex) TouchRender(context.Context, []string) (int64, error) { return 0, nil }
func (*countingRenderIndex) DeleteExpiredRender(context.Context, []string, time.Time) (int64, error) {
	return 0, nil
}

// TestBotNoiseNoStoreRenderDeliversImageURL runs a drawing_artifact
// no_store_paths render (/sk) with a realistic image size through the real
// bot v2 route and Noise middleware. v3.7.3 sent these bytes inline as
// base64://, which cannot fit one Noise message (65535 bytes) and made every
// such command answer HTTP 500 "noise response encryption failed".
func TestBotNoiseNoStoreRenderDeliversImageURL(t *testing.T) {
	image := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 450<<10)...)
	random := rand.NewChaCha8([32]byte{1})
	_, _ = random.Read(image[8:])
	if base64.StdEncoding.EncodedLen(len(image)) <= 65535 {
		t.Fatal("test image must not fit one Noise message as base64")
	}

	var storeHeader atomic.Value
	var artifactRequests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Errorf("unexpected drawing path: %s", r.URL.Path)
		}
		storeHeader.Store(r.Header.Get("X-Haruki-Cache-Store"))
		if r.Header.Get("X-Haruki-Cache-Store") != "0" {
			artifactRequests.Add(1)
		}
		w.Header().Set("X-Haruki-Cache-Store", "0")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(image)
	}))
	defer srv.Close()

	index := &countingRenderIndex{}
	client := drawing.NewHarukiDrawingClient(srv.URL, drawing.WithArtifactConfig(drawing.ArtifactConfig{
		Endpoints:    []string{"*"},
		NoStorePaths: harukiConfig.DefaultDrawingArtifactNoStorePaths(),
	}))
	cache := drawing.NewRenderCacheClient(drawing.RenderCacheConfig{TTL: time.Hour, Index: index})
	t.Cleanup(func() { _ = cache.Close() })
	client.SetRenderCache(cache)

	serverKP, err := noiseCrypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	ring, err := noiseCrypto.SingleKeyRing(serverKP)
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := t.TempDir()
	app := fiber.New()
	runtime := testRenderApp(t, client)
	runtime.ImageCache = imagecache.New("https://image-cache.test", cacheDir)
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, ring)

	plaintext, err := noiseMP.Marshal(BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101 100 1"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	initiator, err := noiseCrypto.NewInitiator(serverKP.Public)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := initiator.EncryptPacket(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, botPJSKPath("sk/query"), bytes.NewReader(ciphertext))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	wire, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d (wire %d bytes)", resp.StatusCode, len(wire))
	}
	decrypted, err := initiator.DecryptPacket(wire)
	if err != nil {
		t.Fatalf("decrypt response: %v", err)
	}
	if bytes.Contains(decrypted, []byte("base64://")) {
		t.Fatal("no-store render was sent inline")
	}

	var envelope map[string]any
	if err := noiseMP.Unmarshal(decrypted, &envelope); err != nil {
		t.Fatal(err)
	}
	rawData, _ := json.Marshal(envelope["data"])
	var message onebot11.Message
	if err := json.Unmarshal(rawData, &message); err != nil || len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("message = %s (err %v, envelope message %v)", rawData, err, envelope["message"])
	}
	segment, _ := message[0].Data.(map[string]any)
	url, _ := segment["file"].(string)
	if !strings.HasPrefix(url, "https://image-cache.test/pjsk/") {
		t.Fatalf("image url = %q", url)
	}
	stored, err := os.ReadFile(filepath.Join(cacheDir, filepath.FromSlash(strings.TrimPrefix(url, "https://image-cache.test/"))))
	if err != nil || !bytes.Equal(stored, image) {
		t.Fatalf("stored image: %d bytes, err %v", len(stored), err)
	}

	if got, _ := storeHeader.Load().(string); got != "0" || artifactRequests.Load() != 0 {
		t.Fatalf("drawing X-Haruki-Cache-Store = %q, storing requests = %d", got, artifactRequests.Load())
	}
	if n := index.lookups.Load(); n != 0 {
		t.Fatalf("no-store path consulted render_cache_index %d times", n)
	}
}
