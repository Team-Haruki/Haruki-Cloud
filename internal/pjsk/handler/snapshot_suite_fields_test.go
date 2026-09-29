package handler

import (
	"bytes"
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	renderprofile "haruki-cloud/internal/pjsk/render/profile"
	"haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestMysekaiSuiteScopePreservesShopRequest(t *testing.T) {
	const suite = `{
  "upload_time":1710000000,"now":1710000000000,
  "userGamedata":{"userId":339871638031728641,"name":"fixture","deck":1},
  "userProfile":{"profileImageType":"default"},
  "userDecks":[{"deckId":1,"leader":1}],"userCards":[{"cardId":1}],
  "userPlayerFrames":[{"playerFrameId":2}],
  "userMysekaiShops":[{"mysekaiShopId":1,"count":2}],
  "userMysekaiColorfulPass":{"expiredAt":4102444800000},
  "userMysekaiGamedata":{"mysekaiRank":10,"mysekaiMaterialPossessionLevel":1},
  "userMysekaiMaterialPossession":{"quantity":7},
  "userMusicResults":[{"musicId":99,"musicDifficulty":"expert","playResult":"full_combo"}]
 }`
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(suite), &document); err != nil {
		t.Fatal(err)
	}
	projected := map[string]json.RawMessage{}
	for _, field := range mysekaiRenderContextOptionsForMode(mySekaiShopCommand).SuiteFields {
		projected[field] = document[field]
	}
	slim, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(slim, []byte("userMusicResults")) || len(slim) >= len(suite) {
		t.Fatal("shop scope retained unrelated music payload")
	}
	for _, mysekai := range []string{
		`{"upload_time":1710000100,"updatedResources":{}}`,
		`{"upload_time":1710000100,"updatedResources":{"userMysekaiShops":[{"mysekaiShopId":1,"count":3}],"userMysekaiMaterialPossession":{"quantity":99}}}`,
	} {
		app := newMysekaiJP700App(t, "")
		var requests []*drawing.MysekaiShopRequest
		for _, data := range [][]byte{[]byte(suite), slim} {
			snap, err := snapshot.NewDefaultSnapshotFactory(nil, nil).Build(t.Context(), snapshot.BuildInput{Region: renderregion.JP, SuiteJSON: data, MySekaiJSON: []byte(mysekai)})
			if err != nil {
				t.Fatal(err)
			}
			request, err := app.MySekai.WithSnapshot(snap).BuildShopRequest(rendermysekai.ShopQuery{
				Region: "jp", NowMillis: 1800000000000, ShowAll: true,
				ResourceBox: func(int) []rendermysekai.ShopResource {
					return []rendermysekai.ShopResource{{ResourceType: "mysekai_material", ResourceID: 1, Quantity: 1}}
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			requests = append(requests, request)
		}
		if !reflect.DeepEqual(requests[0], requests[1]) {
			t.Fatalf("Suite projection changed shop/profile request: full=%+v slim=%+v", requests[0], requests[1])
		}
		if len(requests[1].Shops) != 1 || len(requests[1].Shops[0].Items) != 1 || !*requests[1].PassActive {
			t.Fatal("fixture did not exercise shop states")
		}
	}
}

type suiteScopeProvider struct {
	*runtimeSnapshotProviderStub
	fields []string
}

func (p *suiteScopeProvider) Resolve(ctx context.Context, selector snapshot.Selector, opts snapshot.ResolveOptions) (snapshot.Snapshot, error) {
	p.fields = slices.Clone(opts.SuiteFields)
	return p.runtimeSnapshotProviderStub.Resolve(ctx, selector, opts)
}

func TestMysekaiSuiteScopesReachSnapshotProvider(t *testing.T) {
	ctx := t.Context()
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode, required string
		mysekai        bool
	}{
		{mySekaiShopCommand, "userMysekaiColorfulPass", true},
		{mySekaiTalkListCommand, "userMysekaiCharacterTalks", true},
		{mySekaiDoorUpgradeCommand, "userMysekaiGates", false},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			app := newMysekaiJP700App(t, "")
			app.Bindings = service
			app.Config.UserSnapshot.AllowFallback = true
			provider := &suiteScopeProvider{runtimeSnapshotProviderStub: &runtimeSnapshotProviderStub{snapshot: &runtimeSnapshotStub{}}}
			app.Snapshots = provider
			_, err := resolveMySekaiRenderContextWithOptions(ctx, app, userQueryParams{Mode: "self", Platform: "qq", PlatformUserID: "42"}, "jp", false, mysekaiRenderContextOptionsForMode(tc.mode))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(provider.fields, tc.required) || !slices.Contains(provider.fields, "userPlayerFrames") || len(provider.resolveNeedFlags) != 1 || provider.resolveNeedFlags[0] != tc.mysekai {
				t.Fatalf("snapshot scope = %v, MySekai=%v", provider.fields, provider.resolveNeedFlags)
			}
		})
	}
	if opts := mysekaiRenderContextOptionsForMode("future-command"); len(opts.SuiteFields) != 0 {
		t.Fatal("unaudited command must retain full Suite")
	}
}

type suiteScopeProfileSource struct{ *handlerMySekaiProfileSource }

func (suiteScopeProfileSource) GetPlayerFrameByID(id int) (*masterdata.PlayerFrame, error) {
	return &masterdata.PlayerFrame{ID: id, PlayerFrameGroupID: 1}, nil
}
func (suiteScopeProfileSource) GetPlayerFrameGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	return &masterdata.PlayerFrameGroup{ID: id, AssetBundleName: "scope_frame"}, nil
}

func TestMysekaiSuiteScopesPreservePublicProfileFallbacks(t *testing.T) {
	const suite = `{
 "upload_time":1710000000,"now":1710000000000,"userGamedata":{"userId":123,"name":"fixture","deck":1},
 "userProfile":{"profileImageType":"default"},"userDecks":[{"deckId":1,"leader":1}],
 "userCards":[{"cardId":1,"defaultImage":"special_training","specialTrainingStatus":"done"}],
 "userPlayerFrames":[{"playerFrameId":2,"playerFrameAttachStatus":"equipped"}],
 "userMusics":[{"musicId":99}]
 }`
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(suite), &document); err != nil {
		t.Fatal(err)
	}
	source := suiteScopeProfileSource{&handlerMySekaiProfileSource{region: renderregion.JP, cards: map[int]*masterdata.Card{1: {ID: 1, AssetBundleName: "scope_card"}}}}
	controller := renderprofile.NewController(source, nil, assets.NewAssetHelper("", nil), nil)
	response := &sekaiapi.GetAnotherProfileResponse{User: sekaiapi.AnotherUser{UserID: 123, Name: "API name"}}
	build := func(data []byte) *drawing.ProfileCardRequest {
		t.Helper()
		snap, err := snapshot.NewDefaultSnapshotFactory(nil, nil).Build(t.Context(), snapshot.BuildInput{Region: renderregion.JP, SuiteJSON: data})
		if err != nil {
			t.Fatal(err)
		}
		profile, err := controller.BuildProfileCardFromAPIWithSnapshot(renderprofile.Query{Region: "jp"}, response, snap)
		if err != nil {
			t.Fatal(err)
		}
		return profile
	}
	full := build([]byte(suite))
	if !full.Profile.HasFrame || full.Profile.FramePath == nil || !strings.Contains(full.Profile.LeaderImagePath, "scope_card_after_training") {
		t.Fatalf("fixture must exercise equipped frame and missing API card/deck fallback: %+v", full.Profile)
	}
	for _, mode := range []string{mySekaiShopCommand, mySekaiTalkListCommand, mySekaiDoorUpgradeCommand} {
		projected := map[string]json.RawMessage{}
		for _, field := range mysekaiSuiteFields(mode) {
			projected[field] = document[field]
		}
		data, err := json.Marshal(projected)
		if err != nil {
			t.Fatal(err)
		}
		if got := build(data); !reflect.DeepEqual(got, full) {
			t.Fatalf("%s projection changed API profile fallback", mode)
		}
	}
}
