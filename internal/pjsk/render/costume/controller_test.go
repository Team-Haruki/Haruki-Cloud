package costume

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type denseListTestSource struct {
	costumes []*masterdata.Costume3d
}

func (s denseListTestSource) DefaultRegion() renderregion.Value {
	return renderregion.JP
}

func (s denseListTestSource) GetCostumeByID(id int) (*masterdata.Costume3d, error) {
	for _, item := range s.costumes {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, fmt.Errorf("costume not found: %d", id)
}

func (s denseListTestSource) FilterCostumes(filter Filter) ([]*masterdata.Costume3d, error) {
	items := make([]*masterdata.Costume3d, 0, len(s.costumes))
	for _, item := range s.costumes {
		if filter.ColorID > 0 && item.ColorID != filter.ColorID {
			continue
		}
		if filter.PartType != "" && item.PartType != filter.PartType {
			continue
		}
		if filter.CharacterID > 0 && item.CharacterID != filter.CharacterID {
			continue
		}
		if len(filter.CharacterIDs) > 0 && !containsInt(filter.CharacterIDs, item.CharacterID) {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (s denseListTestSource) GetCostumeVariants(groupID int, partType string, characterID int) ([]*masterdata.Costume3d, error) {
	var variants []*masterdata.Costume3d
	for _, item := range s.costumes {
		if item.GroupID == groupID && item.PartType == partType && item.CharacterID == characterID {
			variants = append(variants, item)
		}
	}
	return variants, nil
}

func (s denseListTestSource) GetCostumeSourceCardIDs(costumeIDs []int) (map[int][]int, error) {
	return map[int][]int{}, nil
}

func (s denseListTestSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	return &masterdata.Character{ID: id, FirstName: "测试", GivenName: "角色"}, nil
}

func TestBuildCostumeListRequestUsesDenseDefaultPageSize(t *testing.T) {
	controller := NewController(denseListTestSource{costumes: makeDenseListTestCostumes(500)}, nil, nil)

	request, err := controller.BuildCostumeListRequest(ListQuery{Query: "服装"})
	if err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	if request.PageSize != DefaultPageSize {
		t.Fatalf("expected default page size %d, got %d", DefaultPageSize, request.PageSize)
	}
	if got := len(request.Costumes); got != DefaultPageSize {
		t.Fatalf("expected %d costumes on first page, got %d", DefaultPageSize, got)
	}
	if request.TotalPages != 3 {
		t.Fatalf("expected 3 total pages, got %d", request.TotalPages)
	}
}

func TestBuildCostumeListRequestSupportsMaxPageSizeToken(t *testing.T) {
	controller := NewController(denseListTestSource{costumes: makeDenseListTestCostumes(500)}, nil, nil)

	request, err := controller.BuildCostumeListRequest(ListQuery{Query: "服装 每页999 p2"})
	if err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	if request.PageSize != MaxPageSize {
		t.Fatalf("expected page size capped at %d, got %d", MaxPageSize, request.PageSize)
	}
	if request.Page != 2 {
		t.Fatalf("expected page 2, got %d", request.Page)
	}
	if got := len(request.Costumes); got != 20 {
		t.Fatalf("expected 20 costumes on second page, got %d", got)
	}

	request, err = controller.BuildCostumeListRequest(ListQuery{Query: "服装 全部"})
	if err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	if request.PageSize != MaxPageSize {
		t.Fatalf("expected all token to use page size %d, got %d", MaxPageSize, request.PageSize)
	}
}

func TestBuildCostumeListRequestTreatsGenderOnlyAsBodyCostume(t *testing.T) {
	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostume(33001, "body", 1),
		makeDenseListTestCostume(33002, "hair", 1),
		makeDenseListTestCostume(33003, "head", 1),
		makeDenseListTestCostume(33004, "body", 11),
	}}, nil, nil)

	request, err := controller.BuildCostumeListRequest(ListQuery{Query: "女装"})
	if err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	if len(request.Costumes) != 1 {
		t.Fatalf("expected only one female body costume, got %d", len(request.Costumes))
	}
	if got := request.Costumes[0]; got.PartType != "body" || got.CharacterID != 1 {
		t.Fatalf("expected female body costume, got part=%s character=%d", got.PartType, got.CharacterID)
	}
}

func TestBuildCostumeListRequestSupportsDirectGenderPartTokens(t *testing.T) {
	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostume(33001, "body", 1),
		makeDenseListTestCostume(33002, "hair", 1),
		makeDenseListTestCostume(33003, "head", 1),
		makeDenseListTestCostume(33004, "body", 11),
		makeDenseListTestCostume(33005, "hair", 11),
		makeDenseListTestCostume(33006, "head", 11),
	}}, nil, nil)

	request, err := controller.BuildCostumeListRequest(ListQuery{Query: "女饰品"})
	if err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	if len(request.Costumes) != 1 {
		t.Fatalf("expected one female accessory, got %d", len(request.Costumes))
	}
	if got := request.Costumes[0]; got.PartType != "head" || got.CharacterID != 1 {
		t.Fatalf("expected female accessory, got part=%s character=%d", got.PartType, got.CharacterID)
	}

	request, err = controller.BuildCostumeListRequest(ListQuery{Query: "男发型"})
	if err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	if len(request.Costumes) != 1 {
		t.Fatalf("expected one male hair, got %d", len(request.Costumes))
	}
	if got := request.Costumes[0]; got.PartType != "hair" || got.CharacterID != 11 {
		t.Fatalf("expected male hair, got part=%s character=%d", got.PartType, got.CharacterID)
	}
}

func TestBuildCostumeListRequestSupportsExplicitPartTypeFromShortcut(t *testing.T) {
	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostume(33001, "body", 20),
		makeDenseListTestCostume(33002, "head", 20),
		makeDenseListTestCostume(33003, "hair", 20),
	}}, nil, nil)

	request, err := controller.BuildCostumeListRequest(ListQuery{Query: "mzk p1", PartType: "head"})
	if err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	if len(request.Costumes) != 1 {
		t.Fatalf("expected one accessory, got %d", len(request.Costumes))
	}
	if got := request.Costumes[0]; got.PartType != "head" {
		t.Fatalf("expected accessory part, got %s", got.PartType)
	}
}

func TestBuildCostumeListRequestSupportsCharacterSourceQuery(t *testing.T) {
	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostume(33001, "body", 20),
		makeDenseListTestCostume(33002, "head", 20),
		makeDenseListTestCostume(33003, "hair", 20),
		makeDenseListTestCostume(33004, "body", 1),
	}}, nil, nil)

	request, err := controller.BuildCostumeListRequest(ListQuery{Query: "mzk"})
	if err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	if len(request.Costumes) != 3 {
		t.Fatalf("expected three Mizuki costume entries, got %d", len(request.Costumes))
	}
	for _, item := range request.Costumes {
		if item.CharacterID != 20 {
			t.Fatalf("expected Mizuki character id 20, got %d", item.CharacterID)
		}
	}
}

func TestBuildCostumeListRequestBalancesCharacterSourceCategories(t *testing.T) {
	costumes := make([]*masterdata.Costume3d, 0, 305)
	for i := range 300 {
		costumes = append(costumes, makeDenseListTestCostume(33001+i, "body", 20))
	}
	costumes = append(costumes,
		makeDenseListTestCostume(34001, "head", 20),
		makeDenseListTestCostume(34002, "head", 20),
		makeDenseListTestCostume(34003, "hair", 20),
	)
	controller := NewController(denseListTestSource{costumes: costumes}, nil, nil)

	request, err := controller.BuildCostumeListRequest(ListQuery{Query: "mzk"})
	if err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	counts := map[string]int{}
	for _, item := range request.Costumes {
		counts[item.PartType]++
	}
	for _, partType := range []string{"body", "head", "hair"} {
		if counts[partType] == 0 {
			t.Fatalf("expected first page to include %s entries, got counts %+v", partType, counts)
		}
	}
}

func TestBuildCostumeListRequestKeepsOnlyInitialColor(t *testing.T) {
	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostumeWithColor(33001, "body", 20, 1),
		makeDenseListTestCostumeWithColor(33002, "body", 20, 2),
		makeDenseListTestCostumeWithColor(33003, "body", 20, 3),
	}}, nil, nil)

	request, err := controller.BuildCostumeListRequest(ListQuery{Query: "mzk"})
	if err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	if len(request.Costumes) != 1 {
		t.Fatalf("expected one initial color entry, got %d", len(request.Costumes))
	}
	if got := request.Costumes[0]; got.CostumeID != 33001 || got.ColorID != 1 {
		t.Fatalf("expected color 1 costume 33001, got id=%d color=%d", got.CostumeID, got.ColorID)
	}
}

func TestBuildCostumeDetailRequestIncludesAllColorVariants(t *testing.T) {
	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostumeWithColor(33001, "body", 20, 1),
		makeDenseListTestCostumeWithColor(33002, "body", 20, 2),
		makeDenseListTestCostumeWithColor(33003, "body", 20, 3),
	}}, nil, nil)

	request, err := controller.BuildCostumeDetailRequest(Query{ID: 33002})
	if err != nil {
		t.Fatalf("BuildCostumeDetailRequest failed: %v", err)
	}
	if request.Costume.CostumeID != 33002 {
		t.Fatalf("expected selected costume 33002, got %d", request.Costume.CostumeID)
	}
	if len(request.Costume.Variants) != 3 {
		t.Fatalf("expected three color variants, got %d", len(request.Costume.Variants))
	}
	for i, variant := range request.Costume.Variants {
		if variant.ColorID != i+1 {
			t.Fatalf("expected variant color %d, got %d", i+1, variant.ColorID)
		}
	}
}

func TestBuildCostumeDetailRequestRejectsWrongExpectedPartType(t *testing.T) {
	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostumeWithColor(33001, "body", 20, 1),
	}}, nil, nil)

	_, err := controller.BuildCostumeDetailRequest(Query{ID: 33001, ExpectedPartType: "hair"})
	if err == nil || !strings.Contains(err.Error(), "not") {
		t.Fatalf("expected part type mismatch, got %v", err)
	}
}

func TestBuildCostumeDetailRequestLeaves3DPreviewForRenderMiss(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("build should not call 3d preview engine: %s %s", r.Method, r.URL.Path)
	}))
	defer engine.Close()

	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostumeWithColor(33001, "body", 20, 1),
		makeDenseListTestCostumeWithColor(33002, "body", 20, 2),
		makeDenseListTestCostumeWithColor(33011, "head", 20, 1),
		makeDenseListTestCostumeWithColor(33021, "hair", 20, 1),
	}}, nil, nil)
	controller.Set3DPreviewConfig(Preview3DConfig{Enabled: true, EngineBaseURL: engine.URL})

	request, err := controller.BuildCostumeDetailRequest(Query{ID: 33002})
	if err != nil {
		t.Fatalf("BuildCostumeDetailRequest failed: %v", err)
	}
	if request.Costume.PreviewImagePath != nil {
		t.Fatalf("build should leave preview path for render miss, got %q", *request.Costume.PreviewImagePath)
	}
}

func TestRenderCostumeDetailEnsures3DPreviewOnCacheMiss(t *testing.T) {
	var capturePayload map[string]any
	var engineRequests atomic.Int32
	const capturePNG = "preview-png"
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		engineRequests.Add(1)
		if r.Method == http.MethodHead && strings.HasPrefix(r.URL.Path, "/captures/") {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/captures/") {
			w.Header().Set("content-type", "image/png")
			fmt.Fprint(w, capturePNG)
			return
		}
		switch r.URL.Path {
		case "/runtime/character3d-index.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"entries":[{"character3dId":5,"characterId":20,"unit":"school_refusal","bodyCostume3dId":33001,"headCostume3dId":33011,"hairCostume3dId":33021,"status":"ok"}]}`)
		case "/runtime/parts/part-registry.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"entries":[
				{"costume3dId":33001,"partType":"body","characterId":20,"unit":"school_refusal","colorId":1,"costume3dGroupId":330},
				{"costume3dId":33002,"partType":"body","characterId":20,"unit":"school_refusal","colorId":2,"costume3dGroupId":330},
				{"costume3dId":33011,"partType":"head","characterId":20,"unit":"school_refusal","colorId":1,"costume3dGroupId":330},
				{"costume3dId":33021,"partType":"hair","characterId":20,"unit":"school_refusal","colorId":1,"costume3dGroupId":330}
			]}`)
		case "/runtime/parts/head-hair-compatibility.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"rules":[{"unit":"school_refusal","headCostume3dId":33011,"hairCostume3dId":33021,"state":"available"}]}`)
		case "/capture":
			if err := json.NewDecoder(r.Body).Decode(&capturePayload); err != nil {
				t.Fatalf("decode capture payload: %v", err)
			}
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"ok":true}`)
		default:
			t.Fatalf("unexpected engine request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer engine.Close()

	var drawingRequests atomic.Int32
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		drawingRequests.Add(1)
		if r.URL.Path != "/api/pjsk/costume/detail" {
			t.Fatalf("unexpected drawing request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode drawing payload: %v", err)
		}
		costumeBody, _ := body["costume"].(map[string]any)
		if _, ok := costumeBody["preview_image_path"].(string); !ok {
			t.Fatalf("drawing payload should include preview_image_path: %+v", costumeBody)
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "detail-png")
	}))
	defer drawingServer.Close()

	assetRoot := t.TempDir()
	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostumeWithColor(33001, "body", 20, 1),
		makeDenseListTestCostumeWithColor(33002, "body", 20, 2),
		makeDenseListTestCostumeWithColor(33011, "head", 20, 1),
		makeDenseListTestCostumeWithColor(33021, "hair", 20, 1),
	}}, drawing.NewHarukiDrawingClient(drawingServer.URL), renderassets.NewAssetHelper(assetRoot, nil))
	controller.Set3DPreviewConfig(Preview3DConfig{
		Enabled:             true,
		EngineBaseURL:       engine.URL,
		StaticRelativeDir:   "static_images/pjsk_3d_preview",
		Width:               700,
		Height:              500,
		Scale:               2,
		CaptureCacheVersion: "test",
		CameraPreset:        "capture",
	})

	data, err := controller.RenderCostumeDetail(Query{ID: 33002})
	if err != nil {
		t.Fatalf("RenderCostumeDetail failed: %v", err)
	}
	if string(data) != "detail-png" {
		t.Fatalf("unexpected detail data: %q", string(data))
	}
	wantImageID := "pjsk3d_jp_c20_school_refusal_i33002_b33002_h33011_r33021_o0"
	if capturePayload["imageId"] != wantImageID {
		t.Fatalf("unexpected capture image id: %v", capturePayload["imageId"])
	}
	if capturePayload["cacheMode"] != "persistent" {
		t.Fatalf("unexpected capture cache mode: %v", capturePayload["cacheMode"])
	}
	if capturePayload["cameraPreset"] != "capture" {
		t.Fatalf("unexpected capture camera preset: %v", capturePayload["cameraPreset"])
	}
	staticPreviewPath := filepath.Join(assetRoot, "static_images", "pjsk_3d_preview", wantImageID+".png")
	if data, err := os.ReadFile(staticPreviewPath); err != nil || string(data) != capturePNG {
		t.Fatalf("expected synced static preview at %s, data=%q err=%v", staticPreviewPath, string(data), err)
	}
	engineRequestsAfterFirst := engineRequests.Load()
	drawingRequestsAfterFirst := drawingRequests.Load()
	data, err = controller.RenderCostumeDetail(Query{ID: 33002})
	if err != nil {
		t.Fatalf("second RenderCostumeDetail failed: %v", err)
	}
	if string(data) != "detail-png" {
		t.Fatalf("unexpected second detail data: %q", string(data))
	}
	if engineRequests.Load() != engineRequestsAfterFirst {
		t.Fatalf("drawing cache hit should not call 3d engine again")
	}
	if drawingRequests.Load() != drawingRequestsAfterFirst {
		t.Fatalf("drawing cache hit should not call drawing api again")
	}
}

func TestBuildCostumeDetailRequestSkipsMissing3DPreviewParts(t *testing.T) {
	captureCalled := false
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.HasPrefix(r.URL.Path, "/captures/") {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case "/runtime/character3d-index.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"entries":[{"character3dId":5,"characterId":20,"unit":"school_refusal","bodyCostume3dId":33001,"headCostume3dId":33011,"hairCostume3dId":33021,"status":"available"}]}`)
		case "/runtime/parts/part-registry.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"entries":[
				{"costume3dId":33001,"partType":"body","characterId":20,"unit":"school_refusal","colorId":1,"costume3dGroupId":330,"status":"available"},
				{"costume3dId":33002,"partType":"body","characterId":20,"unit":"school_refusal","colorId":2,"costume3dGroupId":330,"status":"missing"},
				{"costume3dId":33011,"partType":"head","characterId":20,"unit":"school_refusal","colorId":1,"costume3dGroupId":330,"status":"available"},
				{"costume3dId":33021,"partType":"hair","characterId":20,"unit":"school_refusal","colorId":1,"costume3dGroupId":330,"status":"available"}
			]}`)
		case "/runtime/parts/head-hair-compatibility.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"rules":[{"unit":"school_refusal","headCostume3dId":33011,"hairCostume3dId":33021,"state":"available"}]}`)
		case "/capture":
			captureCalled = true
			http.Error(w, "missing runtime part should not be captured", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected engine request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer engine.Close()

	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostumeWithColor(33001, "body", 20, 1),
		makeDenseListTestCostumeWithColor(33002, "body", 20, 2),
		makeDenseListTestCostumeWithColor(33011, "head", 20, 1),
		makeDenseListTestCostumeWithColor(33021, "hair", 20, 1),
	}}, nil, nil)
	controller.Set3DPreviewConfig(Preview3DConfig{Enabled: true, EngineBaseURL: engine.URL})

	request, err := controller.BuildCostumeDetailRequest(Query{ID: 33002})
	if err != nil {
		t.Fatalf("BuildCostumeDetailRequest failed: %v", err)
	}
	if request.Costume.PreviewImagePath != nil {
		t.Fatalf("missing runtime part should not produce preview path, got %q", *request.Costume.PreviewImagePath)
	}
	if captureCalled {
		t.Fatalf("missing runtime part should not call capture")
	}
}

func TestBuildCostumeDetailRequestDoesNotCall3DPreview(t *testing.T) {
	var called atomic.Bool
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		http.Error(w, "build should not call 3d preview engine", http.StatusInternalServerError)
	}))
	defer engine.Close()

	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostumeWithColor(33002, "body", 20, 2),
	}}, nil, nil)
	controller.Set3DPreviewConfig(Preview3DConfig{Enabled: true, EngineBaseURL: engine.URL})

	request, err := controller.BuildCostumeDetailRequest(Query{ID: 33002})
	if err != nil {
		t.Fatalf("BuildCostumeDetailRequest failed: %v", err)
	}
	if request.Costume.PreviewImagePath != nil {
		t.Fatalf("build should not produce preview path, got %q", *request.Costume.PreviewImagePath)
	}
	if called.Load() {
		t.Fatalf("build should not call 3d preview engine")
	}
}

func TestRenderCostumeDetailWorksWhen3DPreviewDisabled(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode drawing payload: %v", err)
		}
		costumeBody, _ := body["costume"].(map[string]any)
		if _, ok := costumeBody["preview_image_path"]; ok {
			t.Fatalf("disabled 3D preview must not publish preview_image_path: %+v", costumeBody)
		}
		_, _ = fmt.Fprint(w, "detail-without-3d")
	}))
	defer drawingServer.Close()

	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostumeWithColor(33002, "body", 20, 2),
	}}, drawing.NewHarukiDrawingClient(drawingServer.URL), nil)
	controller.Set3DPreviewConfig(Preview3DConfig{Enabled: false})

	data, err := controller.RenderCostumeDetail(Query{ID: 33002})
	if err != nil {
		t.Fatalf("RenderCostumeDetail failed with 3D disabled: %v", err)
	}
	if string(data) != "detail-without-3d" {
		t.Fatalf("unexpected detail response: %q", data)
	}
}

func TestBuildCostumeListRequestDoesNotCall3DPreview(t *testing.T) {
	called := false
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer engine.Close()
	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostumeWithColor(33001, "body", 20, 1),
	}}, nil, nil)
	controller.Set3DPreviewConfig(Preview3DConfig{Enabled: true, EngineBaseURL: engine.URL})

	if _, err := controller.BuildCostumeListRequest(ListQuery{Query: "mzk"}); err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	if called {
		t.Fatalf("list request should not call 3D preview engine")
	}
}

func TestBuildHairListUsesRoleLocalIDs(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/runtime/character3d-index.json":
			fmt.Fprint(w, `{"entries":[{"character3dId":23,"characterId":21,"unit":"light_sound","hairCostume3dId":392161,"status":"available"}]}`)
		case "/runtime/parts/part-registry.json":
			fmt.Fprint(w, `{"entries":[
				{"costume3dId":392161,"partType":"hair","characterId":21,"unit":"light_sound","status":"available"},
				{"costume3dId":221,"partType":"hair","characterId":21,"unit":"light_sound","status":"available"},
				{"costume3dId":999,"partType":"hair","characterId":21,"unit":"idol","status":"available"},
				{"costume3dId":888,"partType":"hair","characterId":21,"unit":"light_sound","status":"missing"}
			]}`)
		case "/runtime/parts/head-hair-compatibility.json":
			fmt.Fprint(w, `{"rules":[]}`)
		default:
			t.Fatalf("unexpected engine request: %s", r.URL.Path)
		}
	}))
	defer engine.Close()

	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		makeDenseListTestCostumeWithColor(392161, "hair", 21, 1),
		makeDenseListTestCostumeWithColor(221, "hair", 21, 1),
		makeDenseListTestCostumeWithColor(999, "hair", 21, 1),
	}}, nil, nil)
	controller.Set3DPreviewConfig(Preview3DConfig{Enabled: true, EngineBaseURL: engine.URL})

	request, err := controller.BuildCostumeListRequest(ListQuery{Query: "miku ln", PartType: "hair"})
	if err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	if request.Total != 2 || len(request.Costumes) != 2 {
		t.Fatalf("expected two role-usable hairs, got total=%d items=%d", request.Total, len(request.Costumes))
	}
	for index, item := range request.Costumes {
		if item.HairID != index+1 || item.Character3DID != 23 {
			t.Fatalf("unexpected normalized hair at %d: %+v", index, item)
		}
	}
	if request.Costumes[0].CostumeID != 392161 || request.Costumes[1].CostumeID != 221 {
		t.Fatalf("expected default hair first and remaining raw IDs sorted, got %d and %d", request.Costumes[0].CostumeID, request.Costumes[1].CostumeID)
	}
}

func TestParseComboQuerySupportsComponentLocalColors(t *testing.T) {
	labeled, err := parseComboQuery(ComboQuery{Query: "角色23 服装330 颜色2 发型2 饰品301 颜色3", Region: "jp"})
	if err != nil {
		t.Fatalf("parse labeled combo failed: %v", err)
	}
	if labeled.Character3DID != 23 || labeled.OutfitID != 330 || labeled.OutfitColorID != 2 || labeled.HairID != 2 || labeled.AccessoryID != 301 || labeled.AccessoryColorID != 3 {
		t.Fatalf("unexpected labeled combo: %+v", labeled)
	}
	defaults, err := parseComboQuery(ComboQuery{Query: "角色23 服装330 饰品301"})
	if err != nil {
		t.Fatalf("parse default colors failed: %v", err)
	}
	if defaults.OutfitColorID != 1 || defaults.AccessoryColorID != 1 {
		t.Fatalf("expected omitted colors to default to 1, got %+v", defaults)
	}
	numericColors, err := parseComboQuery(ComboQuery{Query: "角色23 服装330 2 饰品301 3"})
	if err != nil {
		t.Fatalf("parse adjacent numeric colors failed: %v", err)
	}
	if numericColors.OutfitColorID != 2 || numericColors.AccessoryColorID != 3 {
		t.Fatalf("expected adjacent numeric colors, got %+v", numericColors)
	}
	roleDefaults, err := parseComboQuery(ComboQuery{Query: "角色23"})
	if err != nil || roleDefaults.OutfitColorID != 1 || roleDefaults.AccessoryColorID != 1 {
		t.Fatalf("expected role-only default selection with original colors, got %+v err=%v", roleDefaults, err)
	}

	if _, err := parseComboQuery(ComboQuery{Query: "角色23 饰品301 饰品531"}); err == nil {
		t.Fatal("expected duplicate accessories to be rejected")
	}
	if _, err := parseComboQuery(ComboQuery{Query: "角色23 颜色2"}); err == nil || !strings.Contains(err.Error(), "紧跟") {
		t.Fatalf("expected detached color to be rejected, got %v", err)
	}
}

func TestParseComboQueryAcceptsLegacyUnitSuffix(t *testing.T) {
	for _, raw := range []string{"角色23 发型1 ln", "角色23 饰品1 ln"} {
		query, err := parseComboQuery(ComboQuery{Query: raw, Region: "jp"})
		if err != nil {
			t.Fatalf("parse combo %q with legacy unit suffix failed: %v", raw, err)
		}
		if query.Character3DID != 23 {
			t.Fatalf("unexpected combo query for %q: %+v", raw, query)
		}
	}
}

func TestParseComboQuerySupportsExactCharacterAliases(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		role int
	}{
		{name: "short alias", raw: "mnr 发型1", role: 5},
		{name: "full name", raw: "角色花里实乃里 发型1", role: 5},
		{name: "separated role label", raw: "角色 mnr 发型1", role: 5},
		{name: "non Miku virtual singer", raw: "rin 发型1", role: 27},
		{name: "Miku virtual singer", raw: "miku vs 发型1", role: 21},
		{name: "Miku more more jump", raw: "初音未来 mmj 发型1", role: 22},
		{name: "Miku leo need", raw: "miku ln 发型1", role: 23},
		{name: "Miku vivid bad squad", raw: "miku vbs 发型1", role: 24},
		{name: "Miku wonderlands showtime", raw: "miku wxs 发型1", role: 25},
		{name: "Miku nightcord", raw: "miku n25 发型1", role: 26},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := parseComboQuery(ComboQuery{Query: tt.raw, Region: "jp"})
			if err != nil {
				t.Fatalf("parseComboQuery(%q) failed: %v", tt.raw, err)
			}
			if query.Character3DID != tt.role || query.HairID != 1 {
				t.Fatalf("unexpected combo query: %+v", query)
			}
		})
	}
	for raw, role := range map[string]int{"服装1 mnr": 5, "miku 服装1 ln": 23} {
		query, err := parseComboQuery(ComboQuery{Query: raw, Region: "jp"})
		if err != nil || query.Character3DID != role || query.OutfitID != 1 {
			t.Fatalf("unexpected combo query for %q: %+v err=%v", raw, query, err)
		}
	}

	if _, err := parseComboQuery(ComboQuery{Query: "miku 发型1"}); err == nil || !strings.Contains(err.Error(), "团队") {
		t.Fatalf("expected Miku without a team to be rejected, got %v", err)
	}
	if _, err := parseComboQuery(ComboQuery{Query: "miku ln mmj 发型1"}); err == nil || !strings.Contains(err.Error(), "一个团队") {
		t.Fatalf("expected conflicting Miku teams to be rejected, got %v", err)
	}
	if _, err := parseComboQuery(ComboQuery{Query: "miku mm 发型1"}); err == nil {
		t.Fatal("expected incomplete MMJ alias mm to be rejected")
	}
	if _, err := parseComboQuery(ComboQuery{Query: "mnrx 发型1"}); err == nil {
		t.Fatal("expected a partial nickname match to be rejected")
	}
}

func TestRenderCostumeComboUsesTemporaryCapture(t *testing.T) {
	const png = "fake-png"
	var capturePayload map[string]any
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.HasPrefix(r.URL.Path, "/captures/tmp_pjsk3d_") {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/captures/tmp_pjsk3d_") {
			w.Header().Set("content-type", "image/png")
			fmt.Fprint(w, png)
			return
		}
		switch r.URL.Path {
		case "/runtime/character3d-index.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"entries":[{"character3dId":5,"characterId":20,"unit":"school_refusal","bodyCostume3dId":33001,"headCostume3dId":33011,"hairCostume3dId":33021,"status":"available"}]}`)
		case "/runtime/parts/part-registry.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"entries":[
				{"costume3dId":33001,"partType":"body","characterId":20,"unit":"school_refusal","colorId":1,"costume3dGroupId":330,"outfitId":33,"status":"available"},
				{"costume3dId":33011,"partType":"head","characterId":20,"unit":"school_refusal","colorId":1,"costume3dGroupId":330,"status":"available"},
				{"costume3dId":33021,"partType":"hair","characterId":20,"unit":"school_refusal","colorId":1,"costume3dGroupId":330,"status":"available"},
				{"costume3dId":53129,"partType":"head_optional","characterId":20,"unit":"school_refusal","colorId":1,"costume3dGroupId":531000,"accessoryId":531,"status":"available"}
			]}`)
		case "/runtime/parts/head-hair-compatibility.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"rules":[{"unit":"school_refusal","headCostume3dId":33011,"hairCostume3dId":33021,"state":"available"}]}`)
		case "/capture":
			if err := json.NewDecoder(r.Body).Decode(&capturePayload); err != nil {
				t.Fatalf("decode capture payload: %v", err)
			}
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"ok":true}`)
		default:
			t.Fatalf("unexpected engine request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer engine.Close()

	controller := NewController(denseListTestSource{}, nil, nil)
	controller.Set3DPreviewConfig(Preview3DConfig{Enabled: true, EngineBaseURL: engine.URL, CaptureCacheVersion: "test"})

	data, err := controller.RenderCostumeCombo(ComboQuery{Query: "角色5 服装33 颜色1 发型1 饰品531 颜色1", Region: "jp"})
	if err != nil {
		t.Fatalf("RenderCostumeCombo failed: %v", err)
	}
	if string(data) != png {
		t.Fatalf("unexpected png data: %q", string(data))
	}
	if capturePayload["cacheMode"] != "temporary" {
		t.Fatalf("expected temporary cache mode, got %v", capturePayload["cacheMode"])
	}
	if capturePayload["headCostume3dId"] != float64(53129) || capturePayload["headOptionalCostume3dId"] != nil {
		t.Fatalf("expected business head 53129 without a second accessory slot, got head=%v optional=%v", capturePayload["headCostume3dId"], capturePayload["headOptionalCostume3dId"])
	}
	if imageID, _ := capturePayload["imageId"].(string); !strings.HasPrefix(imageID, "tmp_pjsk3d_") {
		t.Fatalf("expected tmp image id, got %v", capturePayload["imageId"])
	}
	if capturePayload["ttlSeconds"] == nil {
		t.Fatalf("expected temporary capture ttlSeconds")
	}
}

func TestBuildListPromptIncludesPagingAndDetailHint(t *testing.T) {
	controller := NewController(denseListTestSource{costumes: makeDenseListTestCostumes(500)}, nil, nil)

	request, err := controller.BuildCostumeListRequest(ListQuery{Query: "女装"})
	if err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	prompt := BuildListPrompt(request)
	for _, want := range []string{"第 1/3 页", "本页 240 项", "共 500 项", "/查服装 服装ID 角色ID", "/查饰品 饰品ID 角色ID", "p2"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got %q", want, prompt)
		}
	}
}

func TestParseLookupQueryUsesShortIDRoleAndOptionalColor(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		partType  string
		outfit    int
		accessory int
		role      int
		color     int
	}{
		{name: "outfit labels", raw: "1 角色23 颜色2", partType: "body", outfit: 1, role: 23, color: 2},
		{name: "outfit mixed", raw: "1 角色23 2", partType: "body", outfit: 1, role: 23, color: 2},
		{name: "outfit positional", raw: "1 23", partType: "body", outfit: 1, role: 23, color: 1},
		{name: "outfit short alias", raw: "1 mnr 2", partType: "body", outfit: 1, role: 5, color: 2},
		{name: "outfit full name", raw: "1 角色花里实乃里 颜色2", partType: "body", outfit: 1, role: 5, color: 2},
		{name: "outfit separated role alias", raw: "1 角色 mnr 颜色2", partType: "body", outfit: 1, role: 5, color: 2},
		{name: "accessory labels", raw: "20 角色27 颜色3", partType: "head", accessory: 20, role: 27, color: 3},
		{name: "accessory positional", raw: "20 27 4", partType: "head", accessory: 20, role: 27, color: 4},
		{name: "accessory Miku leo need", raw: "20 miku ln 3", partType: "head", accessory: 20, role: 23, color: 3},
		{name: "accessory Miku more more jump", raw: "20 初音未来 mmj 4", partType: "head", accessory: 20, role: 22, color: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, ok, err := ParseLookupQuery(tt.raw, tt.partType)
			if err != nil || !ok {
				t.Fatalf("ParseLookupQuery(%q) = ok=%v err=%v", tt.raw, ok, err)
			}
			if query.OutfitID != tt.outfit || query.AccessoryID != tt.accessory || query.Character3DID != tt.role || query.ColorID != tt.color {
				t.Fatalf("unexpected query: %+v", query)
			}
		})
	}
}

func TestParseLookupQueryRequiresRoleAndValidColor(t *testing.T) {
	for _, raw := range []string{"1", "1 角色32", "1 角色23 颜色5"} {
		if _, ok, err := ParseLookupQuery(raw, "body"); !ok || err == nil {
			t.Fatalf("ParseLookupQuery(%q) should return a recognized error, ok=%v err=%v", raw, ok, err)
		}
	}
	if _, ok, err := ParseLookupQuery("瑞希", "body"); ok || err != nil {
		t.Fatalf("name query should remain a list search, ok=%v err=%v", ok, err)
	}
	if _, ok, err := ParseLookupQuery("1 miku", "body"); !ok || err == nil || !strings.Contains(err.Error(), "团队") {
		t.Fatalf("Miku detail query without a team should fail clearly, ok=%v err=%v", ok, err)
	}
}

func TestNormalizeListQuerySupportsCharacterAliases(t *testing.T) {
	ordinary, err := normalizeListQuery(ListQuery{Query: "发型 花里实乃里"})
	if err != nil || ordinary.Character3DID != 5 || ordinary.PartType != "hair" {
		t.Fatalf("unexpected ordinary character list query: %+v err=%v", ordinary, err)
	}
	miku, err := normalizeListQuery(ListQuery{Query: "发型 miku mmj"})
	if err != nil || miku.Character3DID != 22 || miku.PartType != "hair" {
		t.Fatalf("unexpected Miku list query: %+v err=%v", miku, err)
	}
	if _, err := normalizeListQuery(ListQuery{Query: "发型 miku"}); err == nil || !strings.Contains(err.Error(), "团队") {
		t.Fatalf("expected Miku list query without a team to fail, got %v", err)
	}
	keyword, err := normalizeListQuery(ListQuery{Query: "foo idol bar"})
	if err != nil || keyword.Keyword != "foo idol bar" {
		t.Fatalf("unit-like keyword order changed: %+v err=%v", keyword, err)
	}
}

func TestBuildCostumeDetailRequestResolvesShortIDBy3DRole(t *testing.T) {
	controller := NewController(denseListTestSource{costumes: []*masterdata.Costume3d{
		{ID: 21002, GroupID: 1021, PartType: "body", CharacterID: 21, ColorID: 2, Name: "测试服装"},
		{ID: 22002, GroupID: 1022, PartType: "body", CharacterID: 22, ColorID: 2, Name: "另一个角色"},
	}}, nil, nil)

	request, err := controller.BuildCostumeDetailRequest(Query{OutfitID: 1, Character3DID: 23, ColorID: 2, ExpectedPartType: "body"})
	if err != nil {
		t.Fatalf("BuildCostumeDetailRequest failed: %v", err)
	}
	if request.Costume.CostumeID != 21002 || request.Costume.OutfitID != 1 {
		t.Fatalf("unexpected resolved costume: %+v", request.Costume)
	}
	if request.Costume.Character3DID != 23 || len(request.Costume.Character3DIDs) != 1 || request.Costume.Character3DIDs[0] != 23 {
		t.Fatalf("expected selected role 23, got id=%d ids=%v", request.Costume.Character3DID, request.Costume.Character3DIDs)
	}
}

func TestCharacterIDFor3DRoleMapsVirtualSingers(t *testing.T) {
	for role, want := range map[int]int{21: 21, 22: 21, 26: 21, 27: 22, 31: 26} {
		got, ok := characterIDFor3DRole(role)
		if !ok || got != want {
			t.Fatalf("characterIDFor3DRole(%d) = %d, %v; want %d", role, got, ok, want)
		}
	}
}

func makeDenseListTestCostumes(count int) []*masterdata.Costume3d {
	costumes := make([]*masterdata.Costume3d, 0, count)
	for i := range count {
		id := 33001 + i
		costumes = append(costumes, makeDenseListTestCostume(id, "body", 1))
	}
	return costumes
}

func makeDenseListTestCostume(id int, partType string, characterID int) *masterdata.Costume3d {
	return makeDenseListTestCostumeWithColor(id, partType, characterID, 1)
}

func makeDenseListTestCostumeWithColor(id int, partType string, characterID int, colorID int) *masterdata.Costume3d {
	return &masterdata.Costume3d{
		ID:              id,
		Seq:             id,
		GroupID:         id / 100,
		Name:            fmt.Sprintf("服装%d", id),
		PartType:        partType,
		Costume3DType:   "normal",
		CharacterID:     characterID,
		ColorID:         colorID,
		AssetBundleName: fmt.Sprintf("%04d", id),
	}
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
