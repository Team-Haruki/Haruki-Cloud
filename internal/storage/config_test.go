package storage_test

import (
	"reflect"
	"strings"
	"testing"

	"haruki-cloud/internal/storage"

	"gopkg.in/yaml.v3"
)

func boolPtr(v bool) *bool { return &v }

func mustResolve(t *testing.T, c storage.ProviderConfig) storage.Resolved {
	t.Helper()
	resolved, err := storage.Resolve(c)
	if err != nil {
		t.Fatalf("Resolve(%+v) error = %v", c, err)
	}
	return resolved
}

func TestProviderConfigYAMLAliases(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want storage.ProviderConfig
	}{
		{"name alias", "name: garage\n", storage.ProviderConfig{Provider: "garage"}},
		{"provider wins over name", "provider: a\nname: b\n", storage.ProviderConfig{Provider: "a"}},
		{"kind alias", "kind: s3\n", storage.ProviderConfig{Scheme: "s3"}},
		{"scheme wins over kind", "scheme: fs\nkind: s3\n", storage.ProviderConfig{Scheme: "fs"}},
		{"public_base_url alias", "public_base_url: https://cdn\n", storage.ProviderConfig{BaseURL: "https://cdn"}},
		{"base_url wins", "base_url: https://a\npublic_base_url: https://b\n", storage.ProviderConfig{BaseURL: "https://a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got storage.ProviderConfig
			if err := yaml.Unmarshal([]byte(tc.yaml), &got); err != nil {
				t.Fatalf("Unmarshal error = %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestProviderConfigYAMLFullBlock(t *testing.T) {
	doc := `
user_upload:
  provider: garage
  kind: s3
  endpoints: ["http://100.64.0.11:3900", "http://100.64.0.12:3900"]
  tls: false
  bucket: user-upload
  root: ""
  region: garage
  access_key_id: "GKexample"
  secret_access_key: "secret"
  public_read: true
  path_style: true
  options: { request_timeout: "10s", max_attempts: 3, proxy: none }
  mirror: { name: disk, kind: local, root: "/asset" }
  mirror_mode: write
`
	var cfg storage.SetConfig
	if err := yaml.Unmarshal([]byte(doc), &cfg); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	got := cfg.UserUpload
	if got.Scheme != "s3" || got.Provider != "garage" || len(got.Endpoints) != 2 || got.TLS == nil || *got.TLS ||
		got.Bucket != "user-upload" || got.AccessKeyID != "GKexample" || got.SecretAccessKey != "secret" ||
		!got.PublicRead || got.PathStyle == nil || !*got.PathStyle || got.MirrorMode != "write" {
		t.Fatalf("unexpected block %+v", got)
	}
	if got.Options["max_attempts"] != "3" || got.Options["request_timeout"] != "10s" {
		t.Fatalf("options = %#v", got.Options)
	}
	if got.Mirror == nil || got.Mirror.Scheme != "local" || got.Mirror.Provider != "disk" || got.Mirror.Root != "/asset" {
		t.Fatalf("mirror = %+v", got.Mirror)
	}
	if !cfg.Assets.IsZero() || cfg.UserUpload.IsZero() {
		t.Fatal("IsZero mismatch")
	}
	if err := storage.Validate("user_upload", got); err != nil {
		t.Fatalf("Validate error = %v", err)
	}
}

func TestProviderConfigYAMLDecodeError(t *testing.T) {
	var got storage.ProviderConfig
	if err := yaml.Unmarshal([]byte("endpoints: {a: b}\n"), &got); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestResolveSchemes(t *testing.T) {
	cases := map[string]string{"": "fs", "fs": "fs", "local": "fs", " LOCAL ": "fs", "S3": "s3"}
	for raw, want := range cases {
		resolved := mustResolve(t, storage.ProviderConfig{Scheme: raw, Root: "/data"})
		if resolved.Scheme != want {
			t.Errorf("scheme %q resolved to %q, want %q", raw, resolved.Scheme, want)
		}
		if resolved.Provider != want {
			t.Errorf("provider label for %q = %q, want scheme", raw, resolved.Provider)
		}
	}
	if _, err := storage.Resolve(storage.ProviderConfig{Scheme: "gcs"}); err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("unsupported scheme error = %v", err)
	}
}

func TestResolveFS(t *testing.T) {
	resolved := mustResolve(t, storage.ProviderConfig{Provider: "disk", Root: "/data/cache/", Endpoint: "ignored:1", BaseURL: "https://x"})
	if resolved.Root != "/data/cache" || resolved.Provider != "disk" || resolved.BaseURL != "" || resolved.Endpoints != nil {
		t.Fatalf("resolved = %+v", resolved)
	}
	if resolved.Region != "" || !resolved.PathStyle || resolved.PublicRead {
		t.Fatalf("fs defaults = %+v", resolved)
	}
	prefixed := mustResolve(t, storage.ProviderConfig{Prefix: "/asset"})
	if prefixed.Root != "/asset" {
		t.Fatalf("prefix fallback root = %q", prefixed.Root)
	}
}

func TestResolveS3(t *testing.T) {
	resolved := mustResolve(t, storage.ProviderConfig{
		Scheme:          "s3",
		Endpoint:        "garage.local:3900/",
		Bucket:          " image-cache ",
		Prefix:          "/pjsk/",
		AccessKeyID:     "AK",
		SecretAccessKey: "SK",
		PublicRead:      true,
	})
	if !reflect.DeepEqual(resolved.Endpoints, []string{"https://garage.local:3900"}) {
		t.Fatalf("endpoints = %#v", resolved.Endpoints)
	}
	if resolved.Bucket != "image-cache" || resolved.Root != "pjsk" || resolved.Region != "garage" {
		t.Fatalf("resolved = %+v", resolved)
	}
	if !resolved.PathStyle || !resolved.PublicRead || !resolved.HasCredentials() {
		t.Fatalf("flags = %+v", resolved)
	}
	if resolved.BaseURL != "https://garage.local:3900" || resolved.Options["endpoint"] != "https://garage.local:3900" {
		t.Fatalf("base url = %q options=%v", resolved.BaseURL, resolved.Options["endpoint"])
	}
	if resolved.Options["default_acl"] != "public-read" {
		t.Fatalf("default_acl = %q", resolved.Options["default_acl"])
	}
	if len(resolved.Warnings) != 0 {
		t.Fatalf("warnings = %v", resolved.Warnings)
	}
}

func TestResolveEndpointConstruction(t *testing.T) {
	cases := []struct {
		name string
		cfg  storage.ProviderConfig
		want []string
	}{
		{"bare host tls default", storage.ProviderConfig{Endpoint: "h:1"}, []string{"https://h:1"}},
		{"bare host tls true", storage.ProviderConfig{Endpoint: "h:1", TLS: boolPtr(true)}, []string{"https://h:1"}},
		{"bare host tls false", storage.ProviderConfig{Endpoint: "h:1", TLS: boolPtr(false)}, []string{"http://h:1"}},
		{"full url untouched", storage.ProviderConfig{Endpoint: "HTTP://h:1/base/", TLS: boolPtr(true)}, []string{"HTTP://h:1/base"}},
		{"list deduped and empties dropped", storage.ProviderConfig{Endpoints: []string{"http://a", "", "http://a/", "b"}, TLS: boolPtr(false)}, []string{"http://a", "http://b"}},
		{"empty list falls back to endpoint", storage.ProviderConfig{Endpoints: []string{" "}, Endpoint: "http://c"}, []string{"http://c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.Scheme = "s3"
			tc.cfg.Bucket = "b"
			resolved := mustResolve(t, tc.cfg)
			if !reflect.DeepEqual(resolved.Endpoints, tc.want) {
				t.Fatalf("endpoints = %#v, want %#v", resolved.Endpoints, tc.want)
			}
		})
	}
}

func TestResolveEndpointsWinOverEndpoint(t *testing.T) {
	resolved := mustResolve(t, storage.ProviderConfig{
		Scheme: "s3", Bucket: "b", Endpoint: "http://single", Endpoints: []string{"http://one", "http://two"},
	})
	if !reflect.DeepEqual(resolved.Endpoints, []string{"http://one", "http://two"}) {
		t.Fatalf("endpoints = %#v", resolved.Endpoints)
	}
	if resolved.Options["endpoint"] != "http://one" {
		t.Fatalf("options endpoint = %q", resolved.Options["endpoint"])
	}
	if len(resolved.Warnings) != 1 || !strings.Contains(resolved.Warnings[0], "http://single") {
		t.Fatalf("warnings = %v", resolved.Warnings)
	}
}

func TestResolveExplicitOptionsWin(t *testing.T) {
	resolved := mustResolve(t, storage.ProviderConfig{
		Scheme:      "s3",
		Bucket:      "derived",
		Endpoint:    "http://e",
		Region:      "derived-region",
		PublicRead:  true,
		PathStyle:   boolPtr(true),
		AccessKeyID: "AK", SecretAccessKey: "SK",
		BaseURL: "https://cdn.example/",
		Options: map[string]string{
			"bucket":                    "explicit",
			"region":                    "explicit-region",
			"default_acl":               "private",
			"enable_virtual_host_style": "TRUE",
			"root":                      "/explicit/root/",
			"custom_knob":               "1",
			"another":                   "2",
		},
	})
	if resolved.Bucket != "explicit" || resolved.Region != "explicit-region" || resolved.Root != "explicit/root" {
		t.Fatalf("explicit options lost: %+v", resolved)
	}
	if resolved.PublicRead || resolved.PathStyle {
		t.Fatalf("flags = public_read %v path_style %v", resolved.PublicRead, resolved.PathStyle)
	}
	if resolved.BaseURL != "https://cdn.example" {
		t.Fatalf("base url = %q", resolved.BaseURL)
	}
	if resolved.Options["custom_knob"] != "1" {
		t.Fatal("unknown option not kept")
	}
	if len(resolved.Warnings) != 1 || !strings.Contains(resolved.Warnings[0], "another,custom_knob") {
		t.Fatalf("warnings = %v", resolved.Warnings)
	}
}

func TestResolvePathStyleFalse(t *testing.T) {
	resolved := mustResolve(t, storage.ProviderConfig{Scheme: "s3", Bucket: "b", Endpoint: "http://e", PathStyle: boolPtr(false)})
	if resolved.PathStyle || resolved.Options["enable_virtual_host_style"] != "true" {
		t.Fatalf("path style = %v options=%v", resolved.PathStyle, resolved.Options)
	}
	anonymous := mustResolve(t, storage.ProviderConfig{Scheme: "s3", Bucket: "b", Endpoint: "http://e"})
	if anonymous.HasCredentials() || anonymous.PublicRead {
		t.Fatalf("anonymous = %+v", anonymous)
	}
}

func TestResolveDoesNotAliasInputOptions(t *testing.T) {
	input := map[string]string{"proxy": "none"}
	resolved := mustResolve(t, storage.ProviderConfig{Scheme: "s3", Bucket: "b", Endpoint: "http://e", Options: input})
	if len(input) != 1 || resolved.Options["bucket"] != "b" {
		t.Fatalf("input options mutated: %v", input)
	}
}

func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name string
		slot string
		cfg  storage.ProviderConfig
		want string
	}{
		{"bad scheme", "cache", storage.ProviderConfig{Scheme: "ftp"}, "storage.cache.scheme"},
		{"fs root missing", "cache", storage.ProviderConfig{Scheme: "fs"}, "storage.cache.root: required"},
		{"fs root relative", "assets", storage.ProviderConfig{Root: "asset"}, "storage.assets.root: \"asset\" must be an absolute"},
		{"s3 bucket missing", "image_cache", storage.ProviderConfig{Scheme: "s3", Endpoint: "http://e"}, "storage.image_cache.bucket"},
		{"s3 endpoint missing", "image_cache", storage.ProviderConfig{Scheme: "s3", Bucket: "b"}, "storage.image_cache.endpoints"},
		{"s3 endpoint without host", "image_cache", storage.ProviderConfig{Scheme: "s3", Bucket: "b", Endpoint: "http:///bucket-path"}, "storage.image_cache.endpoints"},
		{"s3 endpoint unparsable", "image_cache", storage.ProviderConfig{Scheme: "s3", Bucket: "b", Endpoint: "http://[::1"}, "storage.image_cache.endpoints"},
		{"half credentials", "static", storage.ProviderConfig{Scheme: "s3", Bucket: "b", Endpoint: "http://e", AccessKeyID: "AK"}, "storage.static.access_key_id"},
		{"half credentials secret", "static", storage.ProviderConfig{Root: "/a", SecretAccessKey: "SK"}, "storage.static.access_key_id"},
		{"mirror on wrong slot", "cache", storage.ProviderConfig{Root: "/a", Mirror: &storage.ProviderConfig{Root: "/b"}}, "storage.cache.mirror: only"},
		{"mirror mode without mirror", "user_upload", storage.ProviderConfig{Root: "/a", MirrorMode: "write"}, "storage.user_upload.mirror_mode: set without mirror"},
		{"bad mirror mode", "user_upload", storage.ProviderConfig{Root: "/a", Mirror: &storage.ProviderConfig{Root: "/b"}, MirrorMode: "sync"}, "storage.user_upload.mirror_mode: unsupported"},
		{"nested mirror", "user_upload", storage.ProviderConfig{Root: "/a", Mirror: &storage.ProviderConfig{Root: "/b", Mirror: &storage.ProviderConfig{Root: "/c"}}}, "storage.user_upload.mirror.mirror"},
		{"nested mirror mode", "user_upload", storage.ProviderConfig{Root: "/a", Mirror: &storage.ProviderConfig{Root: "/b", MirrorMode: "write"}}, "storage.user_upload.mirror.mirror"},
		{"invalid mirror", "user_upload", storage.ProviderConfig{Root: "/a", Mirror: &storage.ProviderConfig{Scheme: "s3"}}, "storage.user_upload.mirror.bucket"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := storage.Validate(tc.slot, tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestValidateAccepts(t *testing.T) {
	valid := []storage.ProviderConfig{
		{Root: "/asset"},
		{Scheme: "local", Root: "/asset"},
		{Scheme: "s3", Bucket: "b", Endpoints: []string{"http://a:3900", "b:3900"}, AccessKeyID: "AK", SecretAccessKey: "SK"},
		{Root: "/a", Mirror: &storage.ProviderConfig{Scheme: "s3", Bucket: "u", Endpoint: "http://e"}, MirrorMode: "write_only"},
	}
	for i, cfg := range valid {
		if err := storage.Validate("user_upload", cfg); err != nil {
			t.Errorf("case %d: Validate error = %v", i, err)
		}
	}
}
