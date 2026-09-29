package gacha

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestBuildGachaListRequestOnlyResolvesVisibleAssets(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	source := newTestGachaSource(renderregion.JP)
	for id := 25; id > 0; id-- {
		item := &masterdata.Gacha{
			ID: id, Name: fmt.Sprintf("Featured %02d", id), GachaType: "ceil",
			AssetBundleName: fmt.Sprintf("gacha_%d", id),
			StartAt:         now.Add(-time.Hour).UnixMilli() + int64(id/2)*1000,
			EndAt:           now.Add(time.Hour).UnixMilli(),
			GachaPickups:    []masterdata.GachaPickup{{CardID: 42}},
		}
		source.gachas = append(source.gachas, item)
		source.gachaByID[id] = item
		for _, relative := range []string{gachaPaginationLogo(id), gachaPaginationBanner(id)} {
			fullPath := filepath.Join(root, relative)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fullPath, []byte("asset"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i := range 4 {
		rejected := new(*source.gachaByID[1])
		rejected.ID = 101 + i
		switch i {
		case 0:
			rejected.Name = "Other"
		case 1:
			rejected.GachaPickups = []masterdata.GachaPickup{{CardID: 43}}
		case 2:
			rejected.StartAt = now.Add(time.Hour).UnixMilli()
		case 3:
			rejected.EndAt = now.Add(-time.Minute).UnixMilli()
		}
		source.gachas = append(source.gachas, rejected)
	}

	for _, tc := range []struct {
		name        string
		page        int
		wantPage    int
		first, last int
	}{
		{name: "default latest", wantPage: 3, first: 21, last: 25},
		{name: "negative latest", page: -1, wantPage: 3, first: 21, last: 25},
		{name: "first", page: 1, wantPage: 1, first: 1, last: 10},
		{name: "middle", page: 2, wantPage: 2, first: 11, last: 20},
		{name: "past last", page: 99, wantPage: 3, first: 21, last: 25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, trace := commandtrace.WithTrace(t.Context())
			builder := NewBuilder(source, assets.NewAssetHelper(root, nil).WithContext(ctx))
			req, err := builder.BuildGachaListRequest(ListQuery{
				Page: tc.page, PageSize: 10, OnlyCurrent: true, CardID: 42, Keyword: "FEATURED",
			})
			if err != nil {
				t.Fatal(err)
			}
			want := &drawing.GachaListRequest{
				PageSize: 10, Region: "jp", CurrentPage: tc.wantPage, TotalPage: 3,
				PrePaginated: true, Filter: drawing.GachaFilter{Page: tc.wantPage},
				GachaLogos: make(map[int]string), GachaBanners: make(map[int]string),
			}
			for id := tc.first; id <= tc.last; id++ {
				item := source.gachaByID[id]
				want.Gachas = append(want.Gachas, drawing.GachaBrief{
					ID: id, Name: item.Name, GachaType: item.GachaType, AssetName: item.AssetBundleName,
					StartAt: item.StartAt, EndAt: item.EndAt,
				})
				want.GachaLogos[id] = gachaPaginationLogo(id)
				want.GachaBanners[id] = gachaPaginationBanner(id)
			}
			if !reflect.DeepEqual(req, want) {
				t.Fatalf("request = %+v, want %+v", req, want)
			}
			counts := make(map[string]int)
			for _, op := range trace.Snapshot().Operations {
				counts[op.Name] = op.Count
			}
			t.Logf("matched=25 displayed=%d asset resolutions=%d stats=%d", len(req.Gachas), counts["asset.resolve_cache_miss"], counts["asset.stat"])
			for _, operation := range []string{"asset.resolve_cache_miss", "asset.stat"} {
				if got, want := counts[operation], 2*len(req.Gachas); got != want {
					t.Errorf("%s count = %d, want %d (only each displayed logo and banner)", operation, got, want)
				}
			}
		})
	}
}

func gachaPaginationLogo(id int) string {
	return fmt.Sprintf("asset/jp-assets/ondemand/gacha/gacha_%d/logo/logo.png", id)
}

func gachaPaginationBanner(id int) string {
	return fmt.Sprintf("asset/jp-assets/startapp/home/banner/banner_gacha%d/banner_gacha%d.png", id, id)
}
