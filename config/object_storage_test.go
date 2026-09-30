package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestObjectStorageRuntimeEnvOverrides(t *testing.T) {
	for name, value := range map[string]string{
		"HARUKI_PJSK_RENDER_STORAGE_IO_MAX_CONCURRENT":            "12",
		"HARUKI_PJSK_RENDER_STORAGE_IO_MAX_PER_ORIGIN":            "6",
		"HARUKI_PJSK_RENDER_STORAGE_IO_MAX_BACKGROUND":            "2",
		"HARUKI_PJSK_RENDER_ASSET_INDEX_ENABLED":                  "true",
		"HARUKI_PJSK_RENDER_ASSET_INDEX_POLL_INTERVAL":            "45s",
		"HARUKI_PJSK_RENDER_ASSET_INDEX_TIMEOUT":                  "20s",
		"HARUKI_PJSK_RENDER_ASSET_INDEX_MAX_STALE":                "6h",
		"HARUKI_PJSK_RENDER_ASSET_INDEX_MAX_OBJECTS":              "300000",
		"HARUKI_PJSK_RENDER_DRAWING_CACHE_VERSIONS_ENABLED":       "true",
		"HARUKI_PJSK_RENDER_DRAWING_CACHE_VERSIONS_POLL_INTERVAL": "15s",
		"HARUKI_PJSK_RENDER_DRAWING_CACHE_VERSIONS_MAX_STALE":     "1m",
		"HARUKI_PJSK_RENDER_CACHE_PERSISTENCE_NAMESPACE":          "cloud-cn08",
		"HARUKI_PJSK_RENDER_IMAGE_CACHE_GC_OBJECT_DELETE_ENABLED": "true",
	} {
		t.Setenv(name, value)
	}
	cfg := Config{}
	if err := ApplyEnvOverrides(&cfg); err != nil {
		t.Fatal(err)
	}
	c := cfg.PJSKRender
	if c.Storage.IO.MaxConcurrent != 12 || c.Storage.IO.MaxPerOrigin != 6 || c.Storage.IO.MaxBackground != 2 {
		t.Fatalf("io=%+v", c.Storage.IO)
	}
	if !c.AssetIndex.Enabled || c.AssetIndex.PollInterval != 45*time.Second || c.AssetIndex.Timeout != 20*time.Second || c.AssetIndex.MaxStale != 6*time.Hour || c.AssetIndex.MaxObjects != 300000 {
		t.Fatalf("index=%+v", c.AssetIndex)
	}
	if !c.DrawingCacheVersions.Enabled || c.DrawingCacheVersions.PollInterval != 15*time.Second || c.DrawingCacheVersions.MaxStale != time.Minute || c.CachePersistenceNamespace != "cloud-cn08" || !c.ImageCache.GC.ObjectDeleteEnabled {
		t.Fatal("version/lifecycle overrides missing")
	}
}

func TestCacheVersionsRequireFullArtifactProtocolAfterEnvironmentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("pjsk_render:\n  enabled: true\n  drawing_cache_versions:\n    enabled: true\n  drawing_artifact:\n    endpoints: ['*']\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARUKI_PJSK_RENDER_ENABLED", "true")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_CACHE_VERSIONS_ENABLED", "true")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_ARTIFACT_ENDPOINTS", "api/pjsk/card/box")
	if _, err := ReadConfig(path); err == nil {
		t.Fatal("partial artifact protocol accepted")
	}
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_ARTIFACT_ENDPOINTS", "[]")
	if _, err := ReadConfig(path); err == nil {
		t.Fatal("disabled artifact protocol accepted")
	}
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_ARTIFACT_ENDPOINTS", "*")
	if _, err := ReadConfig(path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_CACHE_VERSIONS_ENABLED", "false")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_ARTIFACT_ENDPOINTS", "[]")
	if _, err := ReadConfig(path); err != nil {
		t.Fatal(err)
	}
}
