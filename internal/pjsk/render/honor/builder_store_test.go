package honor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

// C1 (T15): each honor fork sends its candidates in today's preference order.
// The optional overlays that are not forks (scroll, birthday level icon, the
// event rank overlay next to a rank background) keep an existence check
// (pending contract item C1-overlays), so with no assets at all they are omitted.
func TestBuildHonorRequestEventHonorImageCandidates(t *testing.T) {
	source := newTestHonorSource(renderregion.JP)
	source.honors[900] = &masterdata.Honor{ID: 900, GroupID: 901, HonorRarity: "high", AssetBundleName: "honor_top_000100_event_demo"}
	source.groups[901] = &masterdata.HonorGroup{ID: 901, HonorType: "event", BackgroundAssetBundleName: new("honor_bg_event_demo")}

	req, err := NewBuilder(source, assets.NewAssetHelper("", nil)).BuildHonorRequest(Query{Region: renderregion.JP, HonorID: 900, HonorLevel: 1})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	want := drawing.AssetKey{
		"asset/jp-assets/startapp/honor/honor_bg_event_demo/degree_sub.png",
		"asset/jp-assets/startapp/honor/honor_bg_event_demo/rank_sub.png",
	}
	if !slices.Equal(req.HonorImgPath, want) {
		t.Fatalf("event honor candidates = %v, want %v", req.HonorImgPath, want)
	}
	wantRank := "asset/jp-assets/startapp/honor/honor_top_000100_event_demo/rank_sub.png"
	if req.RankImgPath == nil || *req.RankImgPath != wantRank {
		t.Fatalf("event rank = %v", req.RankImgPath)
	}
	if req.ScrollImgPath != nil {
		t.Fatalf("missing scroll must be omitted, got %v", *req.ScrollImgPath)
	}
	wantFrame := drawing.AssetKey{"static_images/honor/frame_degree_s_3.png"}
	if !slices.Equal(req.FrameImgPath, wantFrame) {
		t.Fatalf("event frame without name = %v", req.FrameImgPath)
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if list, ok := wire["honor_img_path"].([]any); !ok || len(list) != 2 || list[0] != want[0] || list[1] != want[1] {
		t.Fatalf("honor_img_path wire = %#v", wire["honor_img_path"])
	}
	if wire["frame_img_path"] != wantFrame[0] {
		t.Fatalf("frame_img_path wire = %#v", wire["frame_img_path"])
	}
}

func TestBuildHonorRequestEventDerivedBackgroundCandidate(t *testing.T) {
	source := newTestHonorSource(renderregion.JP)
	source.honors[910] = &masterdata.Honor{ID: 910, GroupID: 911, HonorRarity: "low", AssetBundleName: "honor_top_000100_event_demo"}
	source.groups[911] = &masterdata.HonorGroup{ID: 911, HonorType: "event"}

	req, err := NewBuilder(source, nil).BuildHonorRequest(Query{Region: renderregion.JP, HonorID: 910, HonorLevel: 1, IsMain: true})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	derived := deriveHonorBackgroundAssetName("honor_top_000100_event_demo")
	if derived == "" {
		t.Fatal("expected a derived background name for the fixture")
	}
	want := drawing.AssetCandidates(
		"asset/jp-assets/startapp/honor/honor_top_000100_event_demo/degree_main.png",
		"asset/jp-assets/startapp/honor/"+derived+"/degree_main.png",
		"asset/jp-assets/startapp/honor/honor_top_000100_event_demo/rank_main.png",
	)
	if !slices.Equal(req.HonorImgPath, want) || len(want) != 3 {
		t.Fatalf("derived candidates = %v, want %v", req.HonorImgPath, want)
	}
	// Nothing exists: the background resolves to no rank image, so the rank
	// overlay is sent (pre-C1 behaviour).
	if req.RankImgPath == nil || *req.RankImgPath != want.Last() {
		t.Fatalf("event rank = %v", req.RankImgPath)
	}
}

// When only the rank fallback exists Drawing resolves the background to the
// rank image, so the same image must not also be sent as the overlay. Any
// earlier candidate existing restores the overlay. Local and store-backed
// readers agree.
func TestBuildHonorRequestEventRankOverlaySuppressedOnRankBackground(t *testing.T) {
	source := newTestHonorSource(renderregion.JP)
	source.honors[910] = &masterdata.Honor{ID: 910, GroupID: 911, HonorRarity: "low", AssetBundleName: "honor_top_000100_event_demo"}
	source.groups[911] = &masterdata.HonorGroup{ID: 911, HonorType: "event"}
	query := Query{Region: renderregion.JP, HonorID: 910, HonorLevel: 1, IsMain: true}
	rank := "jp-assets/startapp/honor/honor_top_000100_event_demo/rank_main.png"
	derived := "jp-assets/startapp/honor/" + deriveHonorBackgroundAssetName("honor_top_000100_event_demo") + "/degree_main.png"
	ctx := context.Background()

	build := func(t *testing.T, files ...string) map[string]*drawing.HonorRequest {
		t.Helper()
		root := t.TempDir()
		memory := storagetest.NewMemory()
		for _, rel := range files {
			mustWriteHonorAsset(t, root, filepath.Join("asset", filepath.FromSlash(rel)))
			memory.Seed(map[string][]byte{rel: []byte("png")})
		}
		builders := map[string]*Builder{
			"local": NewBuilder(source, assets.NewAssetHelper(root, nil)),
			"store": NewBuilder(source, assets.NewAssetHelper("", nil)).WithAssetReader(ctx, assets.NewAssetReader(nil, memory)),
		}
		out := map[string]*drawing.HonorRequest{}
		for name, builder := range builders {
			req, err := builder.BuildHonorRequest(query)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			out[name] = req
		}
		return out
	}

	for name, req := range build(t, rank) {
		if req.RankImgPath != nil {
			t.Fatalf("%s: rank overlay duplicated on the rank background: %v", name, *req.RankImgPath)
		}
		if req.HonorImgPath.Last() != "asset/"+rank {
			t.Fatalf("%s: rank fallback must stay the last candidate: %v", name, req.HonorImgPath)
		}
	}
	for name, req := range build(t, rank, derived) {
		if req.RankImgPath == nil || *req.RankImgPath != "asset/"+rank {
			t.Fatalf("%s: overlay must be sent on a degree background: %v", name, req.RankImgPath)
		}
	}
}

// scroll.png is sent only when it exists, through either reader branch.
func TestBuildHonorRequestEventScrollSentOnlyWhenPresent(t *testing.T) {
	source := newTestHonorSource(renderregion.JP)
	source.honors[900] = &masterdata.Honor{ID: 900, GroupID: 901, HonorRarity: "high", AssetBundleName: "honor_top_000100_event_demo"}
	source.groups[901] = &masterdata.HonorGroup{ID: 901, HonorType: "event", BackgroundAssetBundleName: new("honor_bg_event_demo")}
	scroll := "jp-assets/startapp/honor/honor_top_000100_event_demo/scroll.png"
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{scroll: []byte("png")})
	builder := NewBuilder(source, assets.NewAssetHelper("", nil)).WithAssetReader(context.Background(), assets.NewAssetReader(nil, memory))
	req, err := builder.BuildHonorRequest(Query{Region: renderregion.JP, HonorID: 900, HonorLevel: 1})
	if err != nil {
		t.Fatal(err)
	}
	if req.ScrollImgPath == nil || *req.ScrollImgPath != "asset/"+scroll {
		t.Fatalf("store-backed scroll = %v", req.ScrollImgPath)
	}
}

// assetExists answers identically through a store-backed reader and through
// the local helper probe over the same tree.
func TestBuilderAssetExistsParity(t *testing.T) {
	root := t.TempDir()
	full := filepath.Join(root, "jp-assets", "startapp", "honor", "frame.png")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/honor/frame.png": []byte("png")})
	source := newTestHonorSource(renderregion.JP)
	ctx := context.Background()

	legacy := NewBuilder(source, assets.NewAssetHelper(root, nil))
	store := NewBuilder(source, assets.NewAssetHelper("", nil)).WithAssetReader(ctx, assets.NewAssetReader(nil, memory))
	disabled := NewBuilder(source, assets.NewAssetHelper(root, nil)).WithAssetReader(ctx, assets.NewAssetReader(nil, storage.Disabled()))
	for _, builder := range []*Builder{legacy, store, disabled} {
		if !builder.assetExists("asset/jp-assets/startapp/honor/frame.png") {
			t.Fatal("hit branch mismatch")
		}
		if builder.assetExists("asset/jp-assets/startapp/honor/missing.png") {
			t.Fatal("miss branch mismatch")
		}
	}

	storeOnly := NewBuilder(source, nil).WithAssetReader(ctx, assets.NewAssetReader(nil, memory))
	if !storeOnly.assetExists("jp-assets/startapp/honor/frame.png") {
		t.Fatal("store reader without helper must still answer")
	}
	var nilBuilder *Builder
	if nilBuilder.WithAssetReader(ctx, nil) != nil {
		t.Fatal("nil builder must stay nil")
	}

	controller := NewController(source, nil, nil)
	controller.SetAssetReader(assets.NewAssetReader(nil, memory))
	if controller.assetReader == nil {
		t.Fatal("controller reader not set")
	}
	var nilController *Controller
	nilController.SetAssetReader(nil)
}

func TestBuildHonorRequestNamedFrameCandidates(t *testing.T) {
	source := newTestHonorSource(renderregion.JP)
	source.honors[920] = &masterdata.Honor{ID: 920, GroupID: 921, HonorRarity: "highest", AssetBundleName: "honor_920"}
	source.groups[921] = &masterdata.HonorGroup{ID: 921, HonorType: "birthday", BackgroundAssetBundleName: new("honor_bg_birthday_01_13"), FrameName: new("honor_frame_birthday_01_13")}
	source.honors[930] = &masterdata.Honor{ID: 930, GroupID: 931, HonorRarity: "middle", AssetBundleName: "honor_930"}
	source.groups[931] = &masterdata.HonorGroup{ID: 931, HonorType: "event", BackgroundAssetBundleName: new("honor_bg_930"), FrameName: new("event_frame")}
	source.honors[940] = &masterdata.Honor{ID: 940, GroupID: 941, HonorRarity: "high", AssetBundleName: "honor_940"}
	source.groups[941] = &masterdata.HonorGroup{ID: 941, HonorType: "sekai_echo", BackgroundAssetBundleName: new("honor_bg_940")}
	builder := NewBuilder(source, assets.NewAssetHelper("", nil))

	birthday, err := builder.BuildHonorRequest(Query{Region: renderregion.JP, HonorID: 920, HonorLevel: 1})
	if err != nil {
		t.Fatalf("birthday: %v", err)
	}
	wantFrame := drawing.AssetKey{
		"asset/jp-assets/startapp/honor_frame/honor_frame_birthday_01_13/frame_degree_s_4.png",
		"static_images/honor/frame_degree_s_4.png",
	}
	if !slices.Equal(birthday.FrameImgPath, wantFrame) {
		t.Fatalf("birthday frame = %v, want %v", birthday.FrameImgPath, wantFrame)
	}
	if birthday.FrameDegreeLevelImgPath != nil {
		t.Fatalf("birthday level icon must be omitted without assets, got %v", *birthday.FrameDegreeLevelImgPath)
	}

	// An event frame below its start rarity keeps the single static frame.
	event, err := builder.BuildHonorRequest(Query{Region: renderregion.JP, HonorID: 930, HonorLevel: 1})
	if err != nil {
		t.Fatalf("event: %v", err)
	}
	if !slices.Equal(event.FrameImgPath, drawing.AssetKey{"static_images/honor/frame_degree_s_2.png"}) || event.FrameDegreeLevelImgPath != nil {
		t.Fatalf("event frame = %v level = %v", event.FrameImgPath, event.FrameDegreeLevelImgPath)
	}

	echo, err := builder.BuildHonorRequest(Query{Region: renderregion.JP, HonorID: 940, HonorLevel: 1})
	if err != nil {
		t.Fatalf("sekai_echo: %v", err)
	}
	wantRank := "asset/jp-assets/startapp/honor/honor_940/rank_sub.png"
	if echo.RankImgPath == nil || *echo.RankImgPath != wantRank {
		t.Fatalf("sekai_echo rank = %v", echo.RankImgPath)
	}
}
