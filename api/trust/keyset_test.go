package trust

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"haruki-cloud/internal/core/trustsign"

	"github.com/gofiber/fiber/v3"
)

func signedKeyset(t *testing.T, version int64) []byte {
	t.Helper()
	root, err := trustsign.NewSigner("root", make([]byte, trustsign.SeedSize))
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	doc := trustsign.KeysetDocument{
		Version:   version,
		IssuedAt:  1_700_000_000,
		ExpiresAt: 1_700_600_000,
		NoiseKeys: []trustsign.KeysetKey{{KeyID: "noise", PublicKey: root.PublicKeyHex()}},
	}
	payload, _ := json.Marshal(doc)
	env, err := root.Sign(trustsign.DomainKeyset, trustsign.EncodingJSON, payload)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	raw, _ := json.Marshal(env)
	return raw
}

func get(t *testing.T, app *fiber.App) (int, []byte, http.Header) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, KeysetRoute, nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, resp.Header
}

func TestRegisterTrustRoutesIsNoopWithoutPath(t *testing.T) {
	app := fiber.New()
	RegisterTrustRoutes(app, "  ")
	if status, _, _ := get(t, app); status != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestKeysetIsServedVerbatimAndReloaded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keyset.json")
	first := signedKeyset(t, 1)
	if err := os.WriteFile(path, first, 0o600); err != nil {
		t.Fatalf("write keyset: %v", err)
	}

	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	source := &keysetSource{path: path, now: func() time.Time { return now }}
	app := fiber.New()
	app.Get(KeysetRoute, source.handler)

	status, body, header := get(t, app)
	if status != fiber.StatusOK || string(body) != string(first) {
		t.Fatalf("first fetch = %d %s", status, body)
	}
	if ct := header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q", ct)
	}
	if cc := header.Get("Cache-Control"); cc != "public, max-age=60" {
		t.Fatalf("cache control = %q", cc)
	}

	// A rewrite inside the reload interval is not picked up yet.
	second := signedKeyset(t, 2)
	if err := os.WriteFile(path, second, 0o600); err != nil {
		t.Fatalf("rewrite keyset: %v", err)
	}
	future := now.Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if _, body, _ := get(t, app); string(body) != string(first) {
		t.Fatal("keyset reloaded before the interval elapsed")
	}

	// Once the interval passes the new mtime triggers a re-read.
	now = now.Add(keysetReloadInterval + time.Second)
	if _, body, _ := get(t, app); string(body) != string(second) {
		t.Fatalf("keyset not reloaded after interval: %s", body)
	}
}

func TestKeysetRejectsBrokenFiles(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string][]byte{
		"not json":        []byte("nope"),
		"unsigned":        []byte(`{"alg":"ed25519","domain":"haruki-cloud/keyset/v1","payload":"e30=","signature":""}`),
		"wrong domain":    mustRelabel(t, signedKeyset(t, 1), trustsign.DomainManifest),
		"invalid payload": mustRelabel(t, signedKeyset(t, 0), trustsign.DomainKeyset),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".json")
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			app := fiber.New()
			RegisterTrustRoutes(app, path)
			if status, _, _ := get(t, app); status != fiber.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", status)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		app := fiber.New()
		RegisterTrustRoutes(app, filepath.Join(dir, "missing.json"))
		if status, _, _ := get(t, app); status != fiber.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", status)
		}
	})
}

func mustRelabel(t *testing.T, raw []byte, domain string) []byte {
	t.Helper()
	var env trustsign.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	env.Domain = domain
	out, _ := json.Marshal(env)
	return out
}
