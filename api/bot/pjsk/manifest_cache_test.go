package pjsk

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"haruki-cloud/internal/core/trustsign"

	"github.com/gofiber/fiber/v3"
)

func TestManifestCacheReusesSignedEnvelope(t *testing.T) {
	cache := &manifestCache{}
	payload := []byte(`{"entries":[]}`)
	builds, encodes := 0, 0
	build := func() ([]byte, string) { builds++; return payload, "" }
	encode := func(p []byte) ([]byte, string) { encodes++; return append([]byte("body:"), p...), "" }

	start := time.Unix(1_700_000_000, 0)
	first, failure := cache.get(start, build, encode)
	if failure != "" || first.etag == "" {
		t.Fatalf("first get = %+v %q", first, failure)
	}
	if again, _ := cache.get(start.Add(manifestRefreshInterval/2), build, encode); again.etag != first.etag || builds != 1 {
		t.Fatalf("within the interval the cached entry is served without a rebuild (builds=%d)", builds)
	}
	// After the interval the rows are re-read, but an unchanged payload is
	// not re-signed or re-encoded.
	if again, _ := cache.get(start.Add(manifestRefreshInterval), build, encode); again.etag != first.etag || builds != 2 || encodes != 1 {
		t.Fatalf("unchanged payload: builds=%d encodes=%d etag %s vs %s", builds, encodes, again.etag, first.etag)
	}
	payload = []byte(`{"entries":[{"command_path":"new"}]}`)
	changed, _ := cache.get(start.Add(3*manifestRefreshInterval), build, encode)
	if changed.etag == first.etag || encodes != 2 || !bytes.HasSuffix(changed.body, payload) {
		t.Fatalf("changed payload must be re-encoded with a new ETag: %+v encodes=%d", changed, encodes)
	}
	// A failed refresh keeps serving the last good manifest.
	failing := func() ([]byte, string) { return nil, "加载指令清单失败" }
	if stale, failure := cache.get(start.Add(5*manifestRefreshInterval), failing, encode); failure != "" || stale.etag != changed.etag {
		t.Fatalf("stale fallback = %+v %q", stale, failure)
	}
	if _, failure := (&manifestCache{}).get(start, failing, encode); failure == "" {
		t.Fatal("an empty cache must report the failure")
	}
	if _, failure := (&manifestCache{}).get(start, build, func([]byte) ([]byte, string) { return nil, "签名指令清单失败" }); failure == "" {
		t.Fatal("an encode failure must be reported")
	}
}

func TestManifestETagMatches(t *testing.T) {
	const etag = `"abc"`
	cases := map[string]bool{
		``:              false,
		`*`:             true,
		`"abc"`:         true,
		`W/"abc"`:       true,
		`"x", "abc"`:    true,
		`"abcd"`:        false,
		` "x" , W/"y" `: false,
	}
	for header, want := range cases {
		if got := manifestETagMatches(header, etag); got != want {
			t.Fatalf("manifestETagMatches(%q) = %v, want %v", header, got, want)
		}
	}
	if manifestETagMatches("*", "") {
		t.Fatal("no ETag never matches")
	}
}

func TestBotManifestEndpointETagAndNotModified(t *testing.T) {
	botClient := newBotCommandTestClient(t, "manifest_etag")
	t.Cleanup(func() { _ = botClient.Close() })
	signer, err := trustsign.NewSigner("manifest-test", bytes.Repeat([]byte{7}, trustsign.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	dispatcher := RegisterPJSKBotRoutesWithOptions(context.Background(), app, testRenderApp(t, nil), nil, botClient, BotRouteOptions{ManifestSigner: signer})
	if dispatcher != nil {
		t.Cleanup(dispatcher.Close)
	}
	get := func(ifNoneMatch string) (*http.Response, []byte) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v2/bot/"+testBotID+"/command/manifests", nil)
		req.Host = "localhost"
		if ifNoneMatch != "" {
			req.Header.Set(fiber.HeaderIfNoneMatch, ifNoneMatch)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, body
	}

	first, firstBody := get("")
	etag := first.Header.Get(fiber.HeaderETag)
	if first.StatusCode != http.StatusOK || etag == "" {
		t.Fatalf("first response = %d etag %q", first.StatusCode, etag)
	}
	if ct := first.Header.Get(fiber.HeaderContentType); ct != fiber.MIMEApplicationJSONCharsetUTF8 {
		t.Fatalf("content type = %q", ct)
	}
	second, secondBody := get("")
	if second.Header.Get(fiber.HeaderETag) != etag || !bytes.Equal(firstBody, secondBody) {
		t.Fatal("the cached manifest must be byte-identical with the same ETag")
	}
	notModified, body := get(etag)
	if notModified.StatusCode != http.StatusNotModified || len(body) != 0 {
		t.Fatalf("If-None-Match = %d body %q, want an empty 304", notModified.StatusCode, body)
	}
	if notModified.Header.Get(fiber.HeaderETag) != etag {
		t.Fatal("a 304 repeats the ETag")
	}
	if stale, _ := get(`"stale"`); stale.StatusCode != http.StatusOK {
		t.Fatalf("a stale ETag gets the full manifest, got %d", stale.StatusCode)
	}
}
