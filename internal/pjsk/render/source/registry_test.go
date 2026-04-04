package source

import (
	"testing"

	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type fakeSource struct {
	region renderregion.Value
	name   string
}

func (s *fakeSource) DefaultRegion() renderregion.Value {
	return s.region
}

func TestRegistryUsesDefaultAndRegionSpecificSource(t *testing.T) {
	registry := NewRegistry[*fakeSource](renderregion.Unknown)
	jp := &fakeSource{region: renderregion.Value("JP"), name: "jp"}
	cn := &fakeSource{region: renderregion.CN, name: "cn"}

	registry.RegisterSource(jp)
	registry.RegisterSource(cn)

	if got := registry.ResolveRegion(renderregion.Unknown); got != renderregion.JP {
		t.Fatalf("expected default region jp, got %q", got)
	}

	src, ok := registry.SourceForRegion(renderregion.Value("CN"))
	if !ok {
		t.Fatal("expected source for cn")
	}
	if src.name != "cn" {
		t.Fatalf("expected cn source, got %q", src.name)
	}
}

func TestRegistryFallsBackToFirstRegisteredSource(t *testing.T) {
	registry := NewRegistry[*fakeSource](renderregion.JP)
	jp := &fakeSource{region: renderregion.JP, name: "jp"}

	registry.RegisterSource(jp)

	src, ok := registry.SourceForRegion(renderregion.Unknown)
	if !ok {
		t.Fatal("expected fallback source")
	}
	if src.name != "jp" {
		t.Fatalf("expected jp fallback, got %q", src.name)
	}
}

func TestRegistryDoesNotFallbackForExplicitRegion(t *testing.T) {
	registry := NewRegistry[*fakeSource](renderregion.JP)
	registry.RegisterSource(&fakeSource{region: renderregion.JP, name: "jp"})

	if _, ok := registry.SourceForRegion(renderregion.TW); ok {
		t.Fatal("expected explicit tw lookup without a tw source to fail")
	}
}
