package stamp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

type errorStampSource struct{ err error }

func (errorStampSource) DefaultRegion() renderregion.Value { return renderregion.JP }
func (s errorStampSource) GetStamps() ([]masterdata.Stamp, error) {
	return nil, s.err
}

func TestStampControllerErrorsRenderingAndAssetHelpers(t *testing.T) {
	if (*Controller)(nil).WithContext(context.Background()) != nil {
		t.Fatal("nil controller context clone should stay nil")
	}
	controller := NewController(newTestStampSource(renderregion.JP), nil, nil)
	if _, err := controller.RenderStampList(ListQuery{}); err == nil {
		t.Fatal("render without drawing client should fail")
	}
	if _, err := controller.RenderStampListPages(ListQuery{}); err == nil {
		t.Fatal("page render without drawing client should fail")
	}
	if _, err := NewController(errorStampSource{err: errors.New("boom")}, nil, nil).BuildStampListRequest(ListQuery{}); err == nil || err.Error() != "boom" {
		t.Fatalf("expected source error, got %v", err)
	}
	if _, err := controller.BuildStampListRequest(ListQuery{}); err == nil || !strings.Contains(err.Error(), "no stamp data") {
		t.Fatalf("expected empty-source error, got %v", err)
	}

	source := newTestStampSource(renderregion.JP)
	for id := 1; id <= 26; id++ {
		source.stamps = append(source.stamps, masterdata.Stamp{ID: id, AssetBundleName: "stamp", CharacterID2: 22})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/stamp/list" {
			t.Errorf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("image"))
	}))
	defer server.Close()
	controller = NewController(source, drawing.NewHarukiDrawingClient(server.URL), assets.NewAssetHelper("", nil))
	if _, err := controller.BuildStampListRequest(ListQuery{Page: 3}); err == nil {
		t.Fatal("out-of-range page should fail")
	}
	data, err := controller.RenderStampList(ListQuery{CharacterIDs: []int{-1, 22}, IDs: []int{-1, 1}})
	if err != nil || string(data) != "image" {
		t.Fatalf("single page render = %q,%v", data, err)
	}
	pages, err := controller.RenderStampListPages(ListQuery{All: true})
	if err != nil || len(pages) != 2 || string(pages[0]) != "image" || string(pages[1]) != "image" {
		t.Fatalf("all page render = %q,%v", pages, err)
	}

	if got := (&Controller{}).makeRelativeAsset("jp-assets/startapp/stamp/a.png"); got != "asset/jp-assets/startapp/stamp/a.png" {
		t.Fatalf("nil-assets relative path = %q", got)
	}
	if got := normalizeStampRelativeAsset("."); got != "" {
		t.Fatalf("dot asset path = %q", got)
	}
	if got := normalizeStampRelativeAsset("cn-assets/ondemand/stamp/a.png"); got != "asset/cn-assets/ondemand/stamp/a.png" {
		t.Fatalf("region asset path = %q", got)
	}
}

func TestStampProviderAdapterUsesLocalProvider(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stamps.json"), []byte(`[{"ID":7,"AssetBundleName":"mapped","CharacterID":21}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewProviderAdapter(provider.NewLocalProvider(dir, renderregion.JP))
	if (*ProviderAdapter)(nil).WithContext(context.Background()) != nil {
		t.Fatal("nil adapter context clone should stay nil")
	}
	contextual, ok := adapter.WithContext(context.Background()).(*ProviderAdapter)
	if !ok {
		t.Fatal("adapter context clone has wrong type")
	}
	items, err := contextual.GetStamps()
	if err != nil || len(items) != 1 || items[0].ID != 7 {
		t.Fatalf("mapped stamps = %+v,%v", items, err)
	}
}
