package honor

import (
	"encoding/json"
	"slices"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// C1 (T15): the honor builder no longer probes existence. Each former fork
// sends its candidates in today's preference order, and the request is the
// same whether or not any asset exists locally (no helper roots here).
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
	wantScroll := "asset/jp-assets/startapp/honor/honor_top_000100_event_demo/scroll.png"
	if req.ScrollImgPath == nil || *req.ScrollImgPath != wantScroll {
		t.Fatalf("event scroll = %v", req.ScrollImgPath)
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
	// The rank overlay is still sent next to the rank fallback candidate.
	if req.RankImgPath == nil || *req.RankImgPath != want.Last() {
		t.Fatalf("event rank = %v", req.RankImgPath)
	}
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
	wantLevel := "asset/jp-assets/startapp/honor_frame/honor_frame_birthday_01_13/frame_degree_level_4.png"
	if birthday.FrameDegreeLevelImgPath == nil || *birthday.FrameDegreeLevelImgPath != wantLevel {
		t.Fatalf("birthday level = %v", birthday.FrameDegreeLevelImgPath)
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
