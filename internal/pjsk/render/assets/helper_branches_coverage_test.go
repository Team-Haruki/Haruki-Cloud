package assets

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAssetHelperConstructorAndPathBranches(t *testing.T) {
	helper := NewAssetHelper(" root/../root ", []string{"root", "", ".", "https://example.test/assets/"})
	if got := helper.Roots(); !reflect.DeepEqual(got, []string{"root", "https://example.test/assets"}) {
		t.Fatalf("unexpected roots: %v", got)
	}
	roots := helper.Roots()
	roots[0] = "mutated"
	if helper.Primary() != "root" {
		t.Fatal("Roots returned shared storage")
	}
	if helper.WithContext(context.Background()) == helper {
		t.Fatal("WithContext should clone")
	}
	if (*AssetHelper)(nil).WithContext(context.Background()) != nil {
		t.Fatal("nil helper context clone should stay nil")
	}

	empty := &AssetHelper{}
	if empty.Primary() != "" || empty.Join("x") != "" {
		t.Fatal("empty helper should not have a primary path")
	}
	if got := helper.Join("icons", "a.png"); got != "root/icons/a.png" {
		t.Fatalf("unexpected joined path: %q", got)
	}
	if got := helper.FirstExisting("", "  "); got != "" {
		t.Fatalf("blank candidates resolved to %q", got)
	}
	(*AssetHelper)(nil).ClearResolutionCache()
	empty.ClearResolutionCache()

	if got := helper.localCandidatePaths("/absolute/a.png"); !reflect.DeepEqual(got, []string{"/absolute/a.png"}) {
		t.Fatalf("unexpected absolute candidates: %v", got)
	}
	if got := helper.localCandidatePaths("icons/a.png"); !reflect.DeepEqual(got, []string{filepath.Join("root", "icons/a.png")}) {
		t.Fatalf("unexpected local candidates: %v", got)
	}
	if got := assetPathCandidates(" "); got != nil {
		t.Fatalf("blank asset candidate returned %v", got)
	}
	if got := assetPathCandidates("asset/jp-assets/a.png"); len(got) != 2 || got[1] != "jp-assets/a.png" {
		t.Fatalf("unexpected prefixed candidates: %v", got)
	}
	if key := assetResolutionKey([]string{" a ", "", "b"}); key != "a\x00b" {
		t.Fatalf("unexpected resolution key: %q", key)
	}
}

func TestRegionAndRelativeAssetPathBranches(t *testing.T) {
	if got := RegionAssetDir(""); got != "asset/jp-assets/startapp" {
		t.Fatalf("unexpected default region dir: %q", got)
	}
	if got := RegionAssetDirByMode(" EN ", " ONDEMAND "); got != "asset/en-assets/ondemand" {
		t.Fatalf("unexpected region dir: %q", got)
	}
	if got := CloudRegionAssetDirByMode("", ""); got != "jp-assets/startapp" {
		t.Fatalf("unexpected cloud region dir: %q", got)
	}
	if got := RegionAssetDirs("tw"); !reflect.DeepEqual(got, []string{"asset/tw-assets/startapp", "asset/tw-assets/ondemand"}) {
		t.Fatalf("unexpected region dirs: %v", got)
	}
	if got := preferredRegionAssetModes("event/banner.png"); got[0] != RegionAssetOnDemand {
		t.Fatalf("event should prefer ondemand: %v", got)
	}
	if got := preferredRegionAssetModes("music/jacket.png"); got[0] != RegionAssetStartApp {
		t.Fatalf("music should prefer startapp: %v", got)
	}
	if got := ResolveRegionAssetPath(nil, "jp"); got != "" {
		t.Fatalf("empty region candidates resolved to %q", got)
	}
	if got := ResolveRegionAssetPath(nil, "jp", " "); got != "" {
		t.Fatalf("blank region candidate resolved to %q", got)
	}
	if got := ResolveEventBannerPath(nil, "jp", " "); got != "" {
		t.Fatalf("blank event banner resolved to %q", got)
	}
	if got := ResolveProfilePlaceholderPath(nil); got != "static_images/unknown.jpg" {
		t.Fatalf("unexpected placeholder: %q", got)
	}

	if got := ResolveAssetPath(nil, ""); got != "" {
		t.Fatalf("empty asset paths resolved to %q", got)
	}
	if got := ResolveAssetPath(nil, "", "a.png"); got != "a.png" {
		t.Fatalf("unexpected relative fallback: %q", got)
	}
	if got := MakeRelative("", "target"); got != "target" {
		t.Fatalf("unexpected empty-base relative result: %q", got)
	}
	if got := MakeRelative("/base", "/other/file"); got != "/other/file" {
		t.Fatalf("unexpected outside-base result: %q", got)
	}
	if got := joinAssetPath("", "a", "b"); got != "a" {
		t.Fatalf("empty base should return first part: %q", got)
	}
	if got := joinAssetPath(""); got != "" {
		t.Fatalf("empty join returned %q", got)
	}
	if got := joinAssetPath("https://example.test/base", "", "a b.png"); !strings.Contains(got, "a%20b.png") {
		t.Fatalf("unexpected URL join: %q", got)
	}
	if got := joinAssetPath("https://example.test/%zz", "a"); got != "https://example.test/%zz" {
		t.Fatalf("malformed URL fallback changed: %q", got)
	}
	if normalizeAssetRoot(" ") != "" || !isAssetURL(" HTTP://example.test/a ") || isAssetURL("ftp://example.test") {
		t.Fatal("asset root/url normalization branches failed")
	}
}
