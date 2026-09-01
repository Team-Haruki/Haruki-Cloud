package assets

import (
	"context"
	"haruki-cloud/internal/testutil"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAssetHelperConstructorAndPathBranches(t *testing.T) {
	helper := NewAssetHelper(" root/../root ", []string{"root", "", ".", "https://example.test/assets/"})
	{
		got := helper.Roots()
		testutil.Require(t, reflect.DeepEqual(got, []string{"root", "https://example.test/assets"}), "unexpected roots: %v", got)
	}

	roots := helper.Roots()
	roots[0] = "mutated"
	testutil.RequireArgs(t, !(helper.Primary() != "root"), "Roots returned shared storage")
	testutil.RequireArgs(t, !(helper.WithContext(context.Background()) == helper), "WithContext should clone")
	testutil.RequireArgs(t, !((*AssetHelper)(nil).WithContext(context.Background()) != nil), "nil helper context clone should stay nil")

	empty := &AssetHelper{}
	{
		testutil.RequireArgs(t, !(empty.Primary() != ""), "empty helper should not have a primary path")
		testutil.RequireArgs(t, !(empty.Join("x") != ""), "empty helper should not have a primary path")
	}
	{

		got := helper.Join("icons", "a.png")
		testutil.Require(t, !(got != "root/icons/a.png"), "unexpected joined path: %q", got)
	}
	{

		got := helper.FirstExisting("", "  ")
		testutil.Require(t, !(got != ""), "blank candidates resolved to %q", got)
	}

	(*AssetHelper)(nil).ClearResolutionCache()
	empty.ClearResolutionCache()
	{

		got := helper.localCandidatePaths("/absolute/a.png")
		testutil.Require(t, reflect.DeepEqual(got, []string{"/absolute/a.png"}), "unexpected absolute candidates: %v", got)
	}
	{

		got := helper.localCandidatePaths("icons/a.png")
		testutil.Require(t, reflect.DeepEqual(got, []string{filepath.Join("root", "icons/a.png")}), "unexpected local candidates: %v", got)
	}
	{

		got := assetPathCandidates(" ")
		testutil.Require(t, !(got != nil), "blank asset candidate returned %v", got)
	}
	{

		got := assetPathCandidates("asset/jp-assets/a.png")
		{
			testutil.Require(t, !(len(got) != 2), "unexpected prefixed candidates: %v", got)
			testutil.Require(t, !(got[1] != "jp-assets/a.png"), "unexpected prefixed candidates: %v", got)
		}
	}
	{

		key := assetResolutionKey([]string{" a ", "", "b"})
		testutil.Require(t, !(key != "a\x00b"), "unexpected resolution key: %q", key)
	}

}

func TestRegionAndRelativeAssetPathBranches(t *testing.T) {
	{
		got := RegionAssetDir("")
		testutil.Require(t, !(got != "asset/jp-assets/startapp"), "unexpected default region dir: %q", got)
	}
	{

		got := RegionAssetDirByMode(" EN ", " ONDEMAND ")
		testutil.Require(t, !(got != "asset/en-assets/ondemand"), "unexpected region dir: %q", got)
	}
	{

		got := CloudRegionAssetDirByMode("", "")
		testutil.Require(t, !(got != "jp-assets/startapp"), "unexpected cloud region dir: %q", got)
	}
	{

		got := RegionAssetDirs("tw")
		testutil.Require(t, reflect.DeepEqual(got, []string{"asset/tw-assets/startapp", "asset/tw-assets/ondemand"}), "unexpected region dirs: %v", got)
	}
	{

		got := preferredRegionAssetModes("event/banner.png")
		testutil.Require(t, !(got[0] != RegionAssetOnDemand), "event should prefer ondemand: %v", got)
	}
	{

		got := preferredRegionAssetModes("music/jacket.png")
		testutil.Require(t, !(got[0] != RegionAssetStartApp), "music should prefer startapp: %v", got)
	}
	{

		got := ResolveRegionAssetPath(nil, "jp")
		testutil.Require(t, !(got != ""), "empty region candidates resolved to %q", got)
	}
	{

		got := ResolveRegionAssetPath(nil, "jp", " ")
		testutil.Require(t, !(got != ""), "blank region candidate resolved to %q", got)
	}
	{

		got := ResolveEventBannerPath(nil, "jp", " ")
		testutil.Require(t, !(got != ""), "blank event banner resolved to %q", got)
	}
	{

		got := ResolveProfilePlaceholderPath(nil)
		testutil.Require(t, !(got != "static_images/unknown.jpg"), "unexpected placeholder: %q", got)
	}
	{

		got := ResolveAssetPath(nil, "")
		testutil.Require(t, !(got != ""), "empty asset paths resolved to %q", got)
	}
	{

		got := ResolveAssetPath(nil, "", "a.png")
		testutil.Require(t, !(got != "a.png"), "unexpected relative fallback: %q", got)
	}
	{

		got := MakeRelative("", "target")
		testutil.Require(t, !(got != "target"), "unexpected empty-base relative result: %q", got)
	}
	{

		got := MakeRelative("/base", "/other/file")
		testutil.Require(t, !(got != "/other/file"), "unexpected outside-base result: %q", got)
	}
	{

		got := joinAssetPath("", "a", "b")
		testutil.Require(t, !(got != "a"), "empty base should return first part: %q", got)
	}
	{

		got := joinAssetPath("")
		testutil.Require(t, !(got != ""), "empty join returned %q", got)
	}
	{

		got := joinAssetPath("https://example.test/base", "", "a b.png")
		testutil.Require(t, strings.Contains(got, "a%20b.png"), "unexpected URL join: %q", got)
	}
	{

		got := joinAssetPath("https://example.test/%zz", "a")
		testutil.Require(t, !(got != "https://example.test/%zz"), "malformed URL fallback changed: %q", got)
	}
	{
		testutil.RequireArgs(t, !(normalizeAssetRoot(" ") != ""), "asset root/url normalization branches failed")
		testutil.RequireArgs(t, isAssetURL(" HTTP://example.test/a "), "asset root/url normalization branches failed")
		testutil.RequireArgs(t, !(isAssetURL("ftp://example.test")), "asset root/url normalization branches failed")
	}

}
