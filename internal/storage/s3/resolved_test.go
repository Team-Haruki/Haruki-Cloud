package s3

import (
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/storage"
)

func resolvedFor(t *testing.T, cfg storage.ProviderConfig) storage.Resolved {
	t.Helper()
	cfg.Scheme = storage.SchemeS3
	resolved, err := storage.Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve error = %v", err)
	}
	return resolved
}

func TestConfigFromResolvedTypedOptions(t *testing.T) {
	falseValue := false
	resolved := resolvedFor(t, storage.ProviderConfig{
		Endpoints:       []string{"http://a:3900", "http://b:3900"},
		Bucket:          "image-cache",
		Root:            "/pjsk/",
		AccessKeyID:     "AK",
		SecretAccessKey: "SK",
		PublicRead:      true,
		PathStyle:       &falseValue,
		Options: map[string]string{
			"request_timeout":   "11s",
			"dial_timeout":      "2s",
			"stat_timeout":      "4s",
			"failover_cooldown": "1m",
			"max_attempts":      "5",
			"max_object_bytes":  "1024",
			"proxy":             " environment ",
		},
	})
	cfg, err := ConfigFromResolved(resolved)
	if err != nil {
		t.Fatalf("ConfigFromResolved error = %v", err)
	}
	if len(cfg.Endpoints) != 2 || cfg.Bucket != "image-cache" || cfg.Root != "pjsk" || cfg.Region != "garage" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.AccessKey != "AK" || cfg.SecretKey != "SK" || cfg.PathStyle || cfg.DefaultACL != "public-read" {
		t.Fatalf("cfg flags = %+v", cfg)
	}
	if cfg.RequestTimeout != 11*time.Second || cfg.DialTimeout != 2*time.Second || cfg.StatTimeout != 4*time.Second ||
		cfg.FailoverCooldown != time.Minute || cfg.MaxAttempts != 5 || cfg.MaxObjectBytes != 1024 || cfg.Proxy != ProxyEnvironment {
		t.Fatalf("typed options = %+v", cfg)
	}
}

func TestConfigFromResolvedDefaults(t *testing.T) {
	resolved := resolvedFor(t, storage.ProviderConfig{Endpoint: "http://a", Bucket: "b"})
	cfg, err := ConfigFromResolved(resolved)
	if err != nil {
		t.Fatalf("ConfigFromResolved error = %v", err)
	}
	if cfg.RequestTimeout != 0 || cfg.MaxAttempts != 0 || cfg.DefaultACL != "" || !cfg.PathStyle || cfg.AccessKey != "" {
		t.Fatalf("cfg = %+v", cfg)
	}
	store, err := Open(resolved)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	c := store.(*client)
	if c.cfg.RequestTimeout != DefaultRequestTimeout || c.cfg.MaxAttempts != 1 || c.transport.Proxy != nil {
		t.Fatalf("client defaults = %+v", c.cfg)
	}
}

func TestConfigFromResolvedRejectsBadOptions(t *testing.T) {
	cases := map[string]string{
		"request_timeout":   "soon",
		"dial_timeout":      "-1s",
		"stat_timeout":      "0s",
		"failover_cooldown": "x",
		"max_attempts":      "0",
		"max_object_bytes":  "big",
	}
	for key, value := range cases {
		resolved := resolvedFor(t, storage.ProviderConfig{Endpoint: "http://a", Bucket: "b", Options: map[string]string{key: value}})
		if _, err := ConfigFromResolved(resolved); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=%q error = %v", key, value, err)
		}
		if _, err := Open(resolved); err == nil {
			t.Errorf("Open accepted %s=%q", key, value)
		}
	}
	resolved := resolvedFor(t, storage.ProviderConfig{Endpoint: "http://a", Bucket: "b", Options: map[string]string{"proxy": "socks"}})
	if _, err := Open(resolved); err == nil || !strings.Contains(err.Error(), "proxy") {
		t.Fatalf("invalid proxy error = %v", err)
	}
}
