package main

import (
	"haruki-cloud/config"
	"haruki-cloud/internal/storage"
	"testing"
)

func TestReconcileRejectsTargetsThatCannotRepresentGarageIndexPaths(t *testing.T) {
	remote := storage.ProviderConfig{Scheme: "s3", Bucket: "images", Endpoints: []string{"http://127.0.0.1:1"}}
	for _, cfg := range []config.PJSKRenderConfig{
		{ImageCache: config.ImageCacheConfig{Dir: t.TempDir()}, Storage: storage.SetConfig{ImageCache: remote}},
		{Storage: storage.SetConfig{ImageCache: storage.ProviderConfig{Scheme: "fs", Root: t.TempDir()}}},
		{},
		{Storage: storage.SetConfig{ImageCache: storage.ProviderConfig{Scheme: "s3", Bucket: "images", Endpoints: remote.Endpoints, Root: "wrong-prefix"}}},
	} {
		if _, err := openReconcileObjects(cfg); err == nil {
			t.Fatal("unsafe target accepted")
		}
	}
	if _, err := openReconcileObjects(config.PJSKRenderConfig{Storage: storage.SetConfig{ImageCache: remote}}); err != nil {
		t.Fatal(err)
	}
}
