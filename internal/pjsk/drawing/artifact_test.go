package drawing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"haruki-cloud/config"
)

// noContext exercises the nil-context guards without a literal nil argument.
var noContext context.Context

func testArtifactRefJSON(overrides map[string]string) string {
	fields := map[string]string{
		"kind":            `"artifact_ref"`,
		"hash":            `"` + strings.Repeat("a", 64) + `"`,
		"cdn_path":        `"pjsk/api/pjsk/card/box/` + strings.Repeat("a", 64) + `.png"`,
		"storage_backend": `"garage"`,
		"bucket":          `"image-cache"`,
		"object_key":      `"pjsk/api/pjsk/card/box/` + strings.Repeat("a", 64) + `.png"`,
		"size_bytes":      `12`,
		"media_type":      `"image/png"`,
		"width":           `100`,
		"height":          `200`,
		"cache_key":       `"` + strings.Repeat("b", 64) + `"`,
		"ttl_seconds":     `3600`,
		"expires_at":      `"2026-09-14T10:00:00Z"`,
		"reused":          `false`,
		"index_written":   `true`,
		"upload_elapsed":  `0.25`,
		"node_name":       `"cn09"`,
	}
	for key, value := range overrides {
		if value == "" {
			delete(fields, key)
			continue
		}
		fields[key] = value
	}
	parts := make([]string, 0, len(fields))
	for key, value := range fields {
		parts = append(parts, `"`+key+`":`+value)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func TestParseArtifactRefTable(t *testing.T) {
	cases := []struct {
		name      string
		overrides map[string]string
		body      string
		wantErr   string
		check     func(t *testing.T, ref *ArtifactRef)
	}{
		{name: "valid", check: func(t *testing.T, ref *ArtifactRef) {
			if ref.NodeName != "cn09" || ref.Width != 100 || ref.Height != 200 || !ref.IndexWritten || ref.SizeBytes != 12 {
				t.Fatalf("ref = %+v", ref)
			}
			expires, err := ref.ExpiresAtTime()
			if err != nil || !expires.Equal(time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)) {
				t.Fatalf("expires = %v, %v", expires, err)
			}
		}},
		{name: "wrong kind", overrides: map[string]string{"kind": `"image"`}, wantErr: "kind"},
		{name: "bad hash", overrides: map[string]string{"hash": `"abc"`}, wantErr: "hash"},
		{name: "non hex hash", overrides: map[string]string{"hash": `"` + strings.Repeat("z", 64) + `"`}, wantErr: "hash"},
		{name: "traversal cdn_path", overrides: map[string]string{"cdn_path": `"pjsk/../secret.png"`}, wantErr: "cdn_path"},
		{name: "absolute cdn_path", overrides: map[string]string{"cdn_path": `"/pjsk/a.png"`}, wantErr: "cdn_path"},
		{name: "backslash cdn_path", overrides: map[string]string{"cdn_path": `"pjsk\\a.png"`}, wantErr: "cdn_path"},
		{name: "non canonical cdn_path", overrides: map[string]string{"cdn_path": `"pjsk/./a.png"`}, wantErr: "cdn_path"},
		{name: "empty cdn_path", overrides: map[string]string{"cdn_path": `""`}, wantErr: "cdn_path"},
		{name: "non pjsk prefix accepted", overrides: map[string]string{"cdn_path": `"misc/` + strings.Repeat("a", 64) + `.png"`}, check: func(t *testing.T, ref *ArtifactRef) {
			if !strings.HasPrefix(ref.CDNPath, "misc/") {
				t.Fatalf("cdn_path = %q", ref.CDNPath)
			}
		}},
		{name: "node_name absent", overrides: map[string]string{"node_name": ""}, check: func(t *testing.T, ref *ArtifactRef) {
			if ref.NodeName != "" {
				t.Fatalf("node = %q", ref.NodeName)
			}
		}},
		{name: "missing media_type", overrides: map[string]string{"media_type": ""}},
		{name: "non image media_type", overrides: map[string]string{"media_type": `"text/html"`}, wantErr: "media_type"},
		{name: "empty storage backend defaults to garage", overrides: map[string]string{"storage_backend": ""}, check: func(t *testing.T, ref *ArtifactRef) {
			if ref.StorageBackend != artifactStorageGarage {
				t.Fatalf("backend = %q", ref.StorageBackend)
			}
		}},
		{name: "legacy disk backend", overrides: map[string]string{"storage_backend": `"legacy_disk"`}},
		{name: "unknown backend", overrides: map[string]string{"storage_backend": `"lightcos"`}, wantErr: "storage_backend"},
		{name: "negative size", overrides: map[string]string{"size_bytes": `-1`}, wantErr: "size_bytes"},
		{name: "expires_at null is infinite", overrides: map[string]string{"expires_at": `null`}, check: requireInfiniteRef},
		{name: "expires_at empty is infinite", overrides: map[string]string{"expires_at": `""`}, check: requireInfiniteRef},
		{name: "malformed expires_at", overrides: map[string]string{"expires_at": `"tomorrow"`}, wantErr: "expires_at"},
		{name: "null width and height", overrides: map[string]string{"width": `null`, "height": `null`}, check: func(t *testing.T, ref *ArtifactRef) {
			if ref.Width != 0 || ref.Height != 0 {
				t.Fatalf("size = %dx%d", ref.Width, ref.Height)
			}
		}},
		{name: "bad cache key", overrides: map[string]string{"cache_key": `"short"`}, wantErr: "cache_key"},
		{name: "absent cache key", overrides: map[string]string{"cache_key": ""}},
		{name: "not json", body: "not-json", wantErr: "decode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.body
			if body == "" {
				body = testArtifactRefJSON(tc.overrides)
			}
			ref, err := parseArtifactRef([]byte(body))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if tc.check != nil {
				tc.check(t, ref)
			}
		})
	}
}

func requireInfiniteRef(t *testing.T, ref *ArtifactRef) {
	t.Helper()
	expires, err := ref.ExpiresAtTime()
	if err != nil || !expires.IsZero() {
		t.Fatalf("expires = %v, %v", expires, err)
	}
}

func TestArtifactRefExpiresAtNilReceiver(t *testing.T) {
	var ref *ArtifactRef
	if expires, err := ref.ExpiresAtTime(); err != nil || !expires.IsZero() {
		t.Fatalf("nil ref expires = %v, %v", expires, err)
	}
}

func TestErrDrawingBadArtifactWraps(t *testing.T) {
	cause := errors.New("cause")
	err := errDrawingBadArtifact(cause)
	if !errors.Is(err, errDrawingBadArtifactRef) || !errors.Is(err, cause) {
		t.Fatalf("err = %v", err)
	}
}

func TestArtifactAllowList(t *testing.T) {
	list := newArtifactAllowList([]string{" api/pjsk/card/box ", "/api/pjsk/event/list/", "/api/pjsk/music/list?show_id=true", "../bad", ""})
	for _, path := range []string{"api/pjsk/card/box", "api/pjsk/event/list", "api/pjsk/music/list"} {
		if !list.has(path) {
			t.Fatalf("allow-list misses %q", path)
		}
	}
	if list.has("api/pjsk/card/list") || list.has("") || list.empty() {
		t.Fatalf("allow-list = %+v", list)
	}
	all := newArtifactAllowList([]string{"*"})
	if !all.has("api/pjsk/anything") || all.has("") || all.empty() {
		t.Fatalf("wildcard = %+v", all)
	}
	if !newArtifactAllowList(nil).empty() {
		t.Fatal("empty allow-list is not empty")
	}
}

func TestNewArtifactSettingsDefaultsAndOff(t *testing.T) {
	if newArtifactSettings(ArtifactConfig{}) != nil {
		t.Fatal("empty endpoints enabled artifact mode")
	}
	settings := newArtifactSettings(ArtifactConfig{Endpoints: []string{"*"}})
	if settings == nil || settings.artifactTimeout != defaultArtifactRenderTimeout || settings.fetcher.timeout != defaultArtifactFetchTimeout {
		t.Fatalf("settings = %+v", settings)
	}
	custom := newArtifactSettings(ArtifactConfig{Endpoints: []string{"*"}, ArtifactTimeout: time.Second, FetchTimeout: 2 * time.Second})
	if custom.artifactTimeout != time.Second || custom.fetcher.timeout != 2*time.Second {
		t.Fatalf("custom = %+v", custom)
	}
	var nilSettings *artifactSettings
	if nilSettings.allowsEndpoint("/api/pjsk/card/box") {
		t.Fatal("nil settings allow an endpoint")
	}
	if settings.allowsEndpoint("") {
		t.Fatal("empty endpoint allowed")
	}
}

func TestWithArtifactConfigOption(t *testing.T) {
	client := NewHarukiDrawingClient("http://drawing.invalid", WithArtifactConfig(ArtifactConfig{}))
	if client.artifact != nil {
		t.Fatal("empty artifact config enabled artifact mode")
	}
	client = NewHarukiDrawingClient("http://drawing.invalid", WithArtifactConfig(ArtifactConfig{Endpoints: []string{"api/pjsk/card/box"}}))
	if client.artifact == nil || !client.artifact.allow.has("api/pjsk/card/box") {
		t.Fatalf("artifact = %+v", client.artifact)
	}
	WithArtifactConfig(ArtifactConfig{Endpoints: []string{"*"}})(nil, nil)
}

func TestWithArtifactModeMarksOnlyCachedAllowListedRequests(t *testing.T) {
	client := NewHarukiDrawingClient("http://drawing.invalid", WithArtifactConfig(ArtifactConfig{Endpoints: []string{"api/pjsk/card/box"}}))
	if got := client.withArtifactMode(noContext, "/api/pjsk/card/box"); got != nil {
		t.Fatal("client without render cache marked the context")
	}
	client.SetRenderCache(&RenderCacheClient{})
	if artifactModeFrom(client.withArtifactMode(noContext, "/api/pjsk/card/box")) == nil {
		t.Fatal("allow-listed cached request is not marked")
	}
	if artifactModeFrom(client.withArtifactMode(context.Background(), "/api/pjsk/card/list")) != nil {
		t.Fatal("non allow-listed request is marked")
	}
	var nilClient *HarukiDrawingClient
	if nilClient.withArtifactMode(noContext, "/api/pjsk/card/box") != nil {
		t.Fatal("nil client marked the context")
	}
	if artifactModeFrom(noContext) != nil {
		t.Fatal("nil context has a mode")
	}
}

func TestRunSharedRenderFlightExtendsBudgetInArtifactMode(t *testing.T) {
	previous := config.Cfg.PJSKRender.DrawingTimeout
	config.Cfg.PJSKRender.DrawingTimeout = 2 * time.Second
	t.Cleanup(func() { config.Cfg.PJSKRender.DrawingTimeout = previous })
	settings := &artifactSettings{artifactTimeout: 5 * time.Second}
	parent := context.WithValue(context.Background(), artifactModeCtxKey{}, settings)
	var remaining time.Duration
	var propagated *artifactSettings
	runSharedRenderFlight(parent, func(ctx context.Context) ([]byte, error) {
		deadline, _ := ctx.Deadline()
		remaining = time.Until(deadline)
		propagated = artifactModeFrom(ctx)
		return nil, nil
	})
	if remaining <= 5*time.Second || remaining > 7*time.Second || propagated != settings {
		t.Fatalf("artifact budget = %v, mode = %v", remaining, propagated)
	}
	runSharedRenderFlight(context.Background(), func(ctx context.Context) ([]byte, error) {
		deadline, _ := ctx.Deadline()
		remaining = time.Until(deadline)
		propagated = artifactModeFrom(ctx)
		return nil, nil
	})
	if remaining > 2*time.Second || propagated != nil {
		t.Fatalf("byte budget = %v, mode = %v", remaining, propagated)
	}
}
