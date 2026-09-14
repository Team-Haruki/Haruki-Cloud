package mysekai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/core/urlhost"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

const fixtureReactionKey = "jp-assets/ondemand/mysekai/system/fixture_reaction_data/fixture_reaction_data.json"

func TestLoadLocalAssetObjectFromStore(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{fixtureReactionKey: []byte(`{"id": 7}`)})
	controller := NewController(nil, nil, renderregion.JP, assets.NewAssetHelper("", nil), MasterdataOptions{
		AssetReader: assets.NewAssetReader(nil, memory),
	})
	var target map[string]any
	if !controller.loadLocalAssetObject(renderregion.JP, "mysekai/system/fixture_reaction_data/fixture_reaction_data.json", &target) {
		t.Fatal("store object was not loaded")
	}
	if target["id"] == nil {
		t.Fatalf("decoded object = %v", target)
	}
	if controller.loadLocalAssetObject(renderregion.JP, "mysekai/system/fixture_reaction_data_rip/fixture_reaction_data.asset", &target) {
		t.Fatal("missing rip object must not load")
	}

	memory.Seed(map[string][]byte{"jp-assets/ondemand/mysekai/bad.json": []byte("{")})
	if controller.loadLocalAssetObject(renderregion.JP, "mysekai/bad.json", &target) {
		t.Fatal("undecodable object must not load")
	}
	var nilController *Controller
	if nilController.loadLocalAssetObject(renderregion.JP, "x.json", &target) {
		t.Fatal("nil controller must not load")
	}
}

func TestLoadLocalAssetObjectFromLocalTree(t *testing.T) {
	root := t.TempDir()
	full := filepath.Join(root, filepath.FromSlash(fixtureReactionKey))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(`{"id": 8}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, reader := range map[string]*assets.AssetReader{
		"default reader": nil,
		"disabled store": assets.NewAssetReader(assets.NewAssetHelper(root, nil), storage.Disabled()),
		"local fs store": mustLocalReader(t, root),
	} {
		controller := NewController(nil, nil, renderregion.JP, assets.NewAssetHelper(root, nil), MasterdataOptions{AssetReader: reader})
		var target map[string]any
		if !controller.loadLocalAssetObject(renderregion.JP, "mysekai/system/fixture_reaction_data/fixture_reaction_data.json", &target) || target["id"] == nil {
			t.Fatalf("%s: object not loaded (%v)", name, target)
		}
	}
}

func mustLocalReader(t *testing.T, root string) *assets.AssetReader {
	t.Helper()
	local, err := storage.NewLocal(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	return assets.NewAssetReader(nil, local)
}

func TestHousingBannerSourceFromStore(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/ondemand/mysekai/banner.png": []byte("stored")})
	cache := newHousingCompetitionBannerCache("", assets.NewAssetReader(nil, memory), urlhost.Single("https://assets.example"))
	cache.httpClient = &http.Client{Transport: housingRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("must not download a stored banner")
	})}
	raw, err := cache.BytesContext(context.Background(), "asset/jp-assets/ondemand/mysekai/banner.png")
	if err != nil || string(raw) != "stored" {
		t.Fatalf("store banner = %q, %v", raw, err)
	}

	failing := storagetest.NewMemory()
	failing.FailGet = func(storage.Key) error { return errors.New("backend down") }
	broken := newHousingCompetitionBannerCache("", assets.NewAssetReader(nil, failing), urlhost.Single("https://assets.example"))
	if _, err := broken.Bytes("asset/jp-assets/ondemand/mysekai/banner.png"); err == nil || !strings.Contains(err.Error(), "backend down") {
		t.Fatalf("store failure = %v", err)
	}
}

func TestHousingBannerHTTPFallbackRotatesHosts(t *testing.T) {
	hosts, err := urlhost.FromList([]string{"https://a.example", "https://b.example"}, urlhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	cache := newHousingCompetitionBannerCache("", assets.NewAssetReader(nil, storagetest.NewMemory()), hosts)
	var requested []string
	cache.httpClient = &http.Client{Transport: housingRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requested = append(requested, req.URL.String())
		switch req.URL.Host {
		case "a.example":
			return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("down")), Header: make(http.Header)}, nil
		default:
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("remote")), Header: make(http.Header)}, nil
		}
	})}
	raw, err := cache.Bytes("asset/jp-assets/ondemand/mysekai/banner.png")
	if err != nil || string(raw) != "remote" {
		t.Fatalf("fallback banner = %q, %v (requests %v)", raw, err, requested)
	}
	if len(requested) != 2 || requested[0] != "https://a.example/jp-assets/ondemand/mysekai/banner.png" || requested[1] != "https://b.example/jp-assets/ondemand/mysekai/banner.png" {
		t.Fatalf("requests = %v", requested)
	}
	var cooled bool
	for _, status := range hosts.Snapshot() {
		if status.Name == "a.example" && !status.Healthy {
			cooled = true
		}
	}
	if !cooled {
		t.Fatalf("failed host was not cooled down: %+v", hosts.Snapshot())
	}

	allDown := newHousingCompetitionBannerCache("", nil, hosts)
	allDown.httpClient = &http.Client{Transport: housingRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport")
	})}
	if _, err := allDown.Bytes("asset/jp-assets/ondemand/mysekai/other.png"); err == nil || !strings.Contains(err.Error(), "transport") {
		t.Fatalf("all hosts down = %v", err)
	}

	invalid := newHousingCompetitionBannerCache("", nil, hosts)
	if _, _, err := invalid.fetch(context.Background(), "://bad"); err == nil {
		t.Fatal("invalid URL must fail")
	}
}
