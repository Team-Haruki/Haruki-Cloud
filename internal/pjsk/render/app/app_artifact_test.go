package app

import (
	"testing"
	"time"

	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func TestAppArtifactConfigFillsImageCacheDependencies(t *testing.T) {
	objects := storagetest.NewMemory()
	hosts := urlhost.Single("https://ic.example")
	cfg := Config{
		DrawingArtifact: drawing.ArtifactConfig{Endpoints: []string{"api/pjsk/card/box"}, FetchTimeout: time.Second},
		Stores:          storage.Set{ImageCache: objects},
		ImageHosts:      hosts,
	}
	got := appArtifactConfig(cfg)
	if got.Objects != objects || got.Hosts != hosts || got.FetchTimeout != time.Second || len(got.Endpoints) != 1 {
		t.Fatalf("artifact config = %+v", got)
	}
	explicitObjects := storagetest.NewMemory()
	explicitHosts := urlhost.Single("https://other.example")
	cfg.DrawingArtifact.Objects, cfg.DrawingArtifact.Hosts = explicitObjects, explicitHosts
	if got := appArtifactConfig(cfg); got.Objects != explicitObjects || got.Hosts != explicitHosts {
		t.Fatalf("explicit dependencies overridden: %+v", got)
	}
}

func TestAppDrawingOptionsAddsArtifactOptionOnlyWhenAllowListed(t *testing.T) {
	if got := len(appDrawingOptions(Config{})); got != 0 {
		t.Fatalf("zero config options = %d", got)
	}
	cfg := Config{DrawingArtifact: drawing.ArtifactConfig{Endpoints: []string{"*"}}}
	options := appDrawingOptions(cfg)
	if len(options) != 1 {
		t.Fatalf("artifact options = %d", len(options))
	}
	if client := drawing.NewHarukiDrawingClient("http://drawing.invalid", options...); client == nil {
		t.Fatal("client with artifact option is nil")
	}
}
