package honor

import (
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func TestControllerBuildHonorRequestUsesRequestedRegionSource(t *testing.T) {
	cn := newTestHonorSource(renderregion.CN)
	cn.honors[1] = &masterdata.Honor{ID: 1, GroupID: 10, HonorRarity: "low", AssetBundleName: "cn_honor"}
	cn.groups[10] = &masterdata.HonorGroup{ID: 10, HonorType: "normal"}

	jp := newTestHonorSource(renderregion.JP)
	jp.honors[1] = &masterdata.Honor{ID: 1, GroupID: 20, HonorRarity: "high", AssetBundleName: "jp_honor"}
	jp.groups[20] = &masterdata.HonorGroup{ID: 20, HonorType: "normal"}

	controller := NewController(cn, nil, assets.NewAssetHelper("", nil))
	controller.RegisterSource(jp)

	req, err := controller.BuildHonorRequest(Query{
		Region:     renderregion.JP,
		HonorID:    1,
		HonorLevel: 1,
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	if req.HonorImgPath == nil || *req.HonorImgPath != "jp-assets/startapp/honor/jp_honor/degree_sub.png" {
		t.Fatalf("expected JP honor path, got %#v", req.HonorImgPath)
	}
}
