package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	json "github.com/bytedance/sonic"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	renderhonor "haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/provider"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestBuildCustomProfileResourcesResolvesPathsInCloud(t *testing.T) {
	root := t.TempDir()
	masterRoot := filepath.Join(root, "masterdata")
	master := filepath.Join(masterRoot, "haruki-sekai-sc-master", "master")
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileTextColors.json"), []map[string]any{
		{"id": 10, "colorCode": "#ffffff"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileTextFonts.json"), []map[string]any{
		{"id": 11, "fontName": "FOT-RodinNTLGPro-DB"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileShapeResources.json"), []map[string]any{
		{"id": 12, "resourceLoadVal": "custom_profile/shape", "fileName": "circle"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "stamps.json"), []map[string]any{
		{"id": 146, "assetbundleName": "stamp0230", "characterId": 1},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "cards.json"), []map[string]any{
		{"id": 915, "characterId": 10, "cardRarityType": "rarity_4", "attr": "cute", "prefix": "Card", "assetbundleName": "res010_no034"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "honors.json"), []map[string]any{
		{"id": 7001, "groupId": 701, "honorRarity": "low", "assetbundleName": "honor_top_event_demo"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "honorGroups.json"), []map[string]any{
		{"id": 701, "honorType": "event", "backgroundAssetBundleName": "honor_bg_event_demo"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "bondsHonors.json"), []map[string]any{
		{"id": 1020501, "gameCharacterUnitID1": 11, "gameCharacterUnitID2": 22, "honorRarity": "highest"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "bondsHonorWords.json"), []map[string]any{
		{"id": 10205002, "assetBundleName": "honorname_0205_default_0502"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "gameCharacterUnits.json"), []map[string]any{
		{"id": 11, "gameCharacterID": 2},
		{"id": 22, "gameCharacterID": 5},
	})

	src := provider.NewLocalProvider(master, renderregion.CN)
	app := &renderapp.App{
		Assets:   assets.NewAssetHelper(root, nil),
		Provider: src,
		Providers: map[renderregion.Value]provider.MasterDataProvider{
			renderregion.CN: src,
		},
		Honors: renderhonor.NewController(renderhonor.NewProviderAdapter(src), nil, assets.NewAssetHelper(root, nil)),
		Config: renderapp.Config{
			LocalMasterdata: renderapp.LocalMasterdataConfig{Dir: masterRoot},
		},
	}

	card := sekaiapi.UserCustomProfileCard{
		CustomProfileID:     1,
		CustomProfileCardID: 1,
		CustomProfileCard: sekaiapi.ProfileCardData{
			Texts: []sekaiapi.TextData{{
				FontID:         11,
				ColorID:        10,
				OutlineColorID: 10,
			}},
			Shapes:      []sekaiapi.ShapeData{{ID: 12, ColorID: 10}},
			Stamps:      []sekaiapi.ImageData{{ID: 146}},
			CardMembers: []sekaiapi.CardData{{ID: 915}},
			Honors:      []sekaiapi.HonorData{{ID: 7001, FullSize: true}},
			BondsHonors: []sekaiapi.BondsHonorData{{ID: 1020501, FullSize: true, WordID: 10205002, Inverse: true}},
		},
	}
	resp := &sekaiapi.GetAnotherProfileResponse{
		UserCards:       []sekaiapi.AnotherUserCard{{CardID: 915, SpecialTrainingStatus: "done", DefaultImage: "special_training"}},
		UserHonors:      []sekaiapi.UserHonor{{HonorID: 7001, Level: 1}},
		UserBondsHonors: []sekaiapi.UserBondsHonor{{BondsHonorID: 1020501, Level: 3}},
	}

	resources, err := buildCustomProfileResources(context.Background(), app, "cn", card, resp)
	if err != nil {
		t.Fatalf("buildCustomProfileResources() error = %v", err)
	}

	shape := resources["customProfileShapeResources"].(map[int]map[string]any)[12]
	if got := shape["imagePath"].(string); got != "asset/cn-assets/startapp/custom_profile/shape/circle.png" {
		t.Fatalf("unexpected shape image path: %s", got)
	}
	stamp := resources["stampAssets"].(map[int]map[string]any)[146]
	if got := stamp["imagePath"].(string); got != "asset/cn-assets/startapp/stamp/stamp0230/stamp0230.png" {
		t.Fatalf("unexpected stamp image path: %s", got)
	}
	cardAsset := resources["cardAssets"].(map[int]map[string]any)[915]
	if got := cardAsset["afterTrainingPath"].(string); got != "asset/cn-assets/startapp/character/member/res010_no034/card_after_training.png" {
		t.Fatalf("unexpected card after-training path: %s", got)
	}
	if got := cardAsset["deckAfterTrainingPath"].(string); got != "asset/cn-assets/startapp/character/member_cutout_trm/res010_no034/after_training.png" {
		t.Fatalf("unexpected deck after-training path: %s", got)
	}
	if got := cardAsset["smallAfterTrainingPath"].(string); got != "asset/cn-assets/startapp/character/member_small/res010_no034/card_after_training.png" {
		t.Fatalf("unexpected small after-training path: %s", got)
	}
	honorRequests := resources["honorRequests"].(map[string]any)
	honorReq := honorRequests["7001:1:main"].(*drawing.HonorRequest)
	if honorReq.HonorImgPath == nil || !strings.HasSuffix(*honorReq.HonorImgPath, "honor/honor_bg_event_demo/degree_main.png") {
		t.Fatalf("unexpected honor image path: %+v", honorReq.HonorImgPath)
	}
	bondsHonorRequests := resources["bondsHonorRequests"].(map[string]any)
	bondsReq := bondsHonorRequests["1020501:3:main:10205002:reverse"].(*drawing.HonorRequest)
	if bondsReq.WordImgPath == nil || *bondsReq.WordImgPath != "asset/cn-assets/startapp/bonds_honor/word/honorname_0205_default_0502_01.png" {
		t.Fatalf("unexpected bonds honor word path: %+v", bondsReq.WordImgPath)
	}
	if bondsReq.CharaID == nil || *bondsReq.CharaID != "22" || bondsReq.CharaID2 == nil || *bondsReq.CharaID2 != "11" {
		t.Fatalf("unexpected reverse bonds honor character order: %+v %+v", bondsReq.CharaID, bondsReq.CharaID2)
	}
}

func writeCustomProfileJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
