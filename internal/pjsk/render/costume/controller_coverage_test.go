package costume

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type controllerCoverageContextKey struct{}

type controllerCoverageSource struct {
	region      renderregion.Value
	ctx         context.Context
	costumes    map[int]*masterdata.Costume3d
	filtered    []*masterdata.Costume3d
	variants    []*masterdata.Costume3d
	characters  map[int]*masterdata.Character
	sourceCards map[int][]int
	getErr      error
	filterErr   error
	variantsErr error
	cardsErr    error
	lastFilter  Filter
}

func (s *controllerCoverageSource) DefaultRegion() renderregion.Value {
	if s == nil || s.region.IsZero() {
		return renderregion.JP
	}
	return s.region
}

func (s *controllerCoverageSource) WithContext(ctx context.Context) DataSource {
	clone := *s
	clone.ctx = ctx
	return &clone
}

func (s *controllerCoverageSource) GetCostumeByID(id int) (*masterdata.Costume3d, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if item := s.costumes[id]; item != nil {
		return item, nil
	}
	return nil, fmt.Errorf("costume not found: %d", id)
}

func (s *controllerCoverageSource) FilterCostumes(filter Filter) ([]*masterdata.Costume3d, error) {
	s.lastFilter = filter
	if s.filterErr != nil {
		return nil, s.filterErr
	}
	if s.filtered != nil {
		return slices.Clone(s.filtered), nil
	}
	items := make([]*masterdata.Costume3d, 0, len(s.costumes))
	for _, item := range s.costumes {
		if item == nil || filter.PartType != "" && item.PartType != filter.PartType || filter.ColorID > 0 && item.ColorID != filter.ColorID || filter.CharacterID > 0 && item.CharacterID != filter.CharacterID {
			continue
		}
		if len(filter.CharacterIDs) > 0 && !slices.Contains(filter.CharacterIDs, item.CharacterID) {
			continue
		}
		if filter.Keyword != "" && !strings.Contains(strings.ToLower(item.Name), strings.ToLower(filter.Keyword)) {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *controllerCoverageSource) GetCostumeVariants(groupID int, partType string, characterID int) ([]*masterdata.Costume3d, error) {
	if s.variantsErr != nil {
		return nil, s.variantsErr
	}
	if s.variants != nil {
		return slices.Clone(s.variants), nil
	}
	items := make([]*masterdata.Costume3d, 0)
	for _, item := range s.costumes {
		if item != nil && item.GroupID == groupID && item.PartType == partType && item.CharacterID == characterID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *controllerCoverageSource) GetCostumeSourceCardIDs(_ []int) (map[int][]int, error) {
	if s.cardsErr != nil {
		return nil, s.cardsErr
	}
	return s.sourceCards, nil
}

func (s *controllerCoverageSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	if character := s.characters[id]; character != nil {
		return character, nil
	}
	return nil, fmt.Errorf("character not found: %d", id)
}

func controllerCoverageCostume(id int, partType string) *masterdata.Costume3d {
	return &masterdata.Costume3d{
		ID:                 id,
		Seq:                id,
		GroupID:            33_000,
		Name:               fmt.Sprintf("Costume %d", id),
		CharacterID:        20,
		PartType:           partType,
		ColorID:            1,
		ArchivePublishedAt: int64(id),
	}
}

func TestControllerCoverageLifecycleAndContext(t *testing.T) {
	var nilController *Controller
	nilController.RegisterSource(&controllerCoverageSource{})
	nilController.Set3DPreviewConfig(Preview3DConfig{})
	if nilController.WithContext(context.Background()) != nil {
		t.Fatal("nil controller must stay nil when contextualized")
	}
	if got := nilController.default3DPreviewStaticOutputDir("x"); got != "" {
		t.Fatalf("nil controller returned an output directory: %q", got)
	}

	ctx := context.WithValue(context.Background(), controllerCoverageContextKey{}, "request")
	source := &controllerCoverageSource{region: renderregion.JP}
	assetRoot := t.TempDir()
	controller := NewController(source, drawing.NewHarukiDrawingClient("http://drawing.invalid"), renderassets.NewAssetHelper(assetRoot, nil))
	controller.Set3DPreviewConfig(Preview3DConfig{Enabled: true, EngineBaseURL: "http://preview.invalid", StaticRelativeDir: "custom/previews"})
	if got, want := controller.preview3D.cfg.StaticOutputDir, filepath.Join(assetRoot, "custom", "previews"); got != want {
		t.Fatalf("default static output dir = %q, want %q", got, want)
	}

	clone := controller.WithContext(ctx)
	if clone == controller || clone.drawing == controller.drawing || clone.assets == controller.assets {
		t.Fatal("WithContext must clone the controller and context-aware clients")
	}
	if clone.preview3D != controller.preview3D || clone.ctx != ctx {
		t.Fatal("WithContext did not preserve the preview service or request context")
	}
	contextualSource, ok := clone.sources.SourceForRegion(renderregion.JP)
	if !ok || contextualSource.(*controllerCoverageSource).ctx != ctx {
		t.Fatal("WithContext did not contextualize registered data sources")
	}

	plainSource := denseListTestSource{}
	controller.RegisterSource(plainSource)
	plainClone := controller.WithContext(ctx)
	if _, ok := plainClone.sources.SourceForRegion(renderregion.JP); !ok {
		t.Fatal("WithContext dropped a non-contextual source")
	}

	if got := (&Controller{}).default3DPreviewStaticOutputDir("x"); got != "" {
		t.Fatalf("controller without assets returned %q", got)
	}
	for _, root := range []string{"", ".", "https://assets.example", "HTTP://assets.example"} {
		candidate := NewController(source, nil, renderassets.NewAssetHelper(root, nil))
		if got := candidate.default3DPreviewStaticOutputDir(""); got != "" {
			t.Fatalf("non-local asset root %q returned output dir %q", root, got)
		}
	}
	defaultDirController := NewController(source, nil, renderassets.NewAssetHelper(assetRoot, nil))
	if got, want := defaultDirController.default3DPreviewStaticOutputDir(""), filepath.Join(assetRoot, filepath.FromSlash(defaultPreview3DStaticRelativeDir)); got != want {
		t.Fatalf("default output dir = %q, want %q", got, want)
	}
}

func TestControllerCoverageRenderCostumeList(t *testing.T) {
	item := controllerCoverageCostume(33_001, "body")
	source := &controllerCoverageSource{
		costumes:    map[int]*masterdata.Costume3d{item.ID: item},
		characters:  map[int]*masterdata.Character{20: {ID: 20, GivenName: "Kanade", Unit: "school_refusal", Gender: "female"}},
		sourceCards: map[int][]int{item.ID: {10}},
	}

	var receivedPath string
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		_, _ = w.Write([]byte("list-png"))
	}))
	defer drawingServer.Close()
	controller := NewController(source, drawing.NewHarukiDrawingClient(drawingServer.URL), nil)

	data, err := controller.RenderCostumeList(ListQuery{PartType: "body"})
	if err != nil || string(data) != "list-png" || receivedPath != "/api/pjsk/costume/list" {
		t.Fatalf("RenderCostumeList data=%q path=%q err=%v", data, receivedPath, err)
	}
	data, payload, err := controller.RenderCostumeListWithRequest(ListQuery{PartType: "body"})
	if err != nil || string(data) != "list-png" || payload == nil || payload.Total != 1 {
		t.Fatalf("RenderCostumeListWithRequest data=%q payload=%+v err=%v", data, payload, err)
	}

	var nilController *Controller
	if _, _, err := nilController.RenderCostumeListWithRequest(ListQuery{}); err == nil {
		t.Fatal("nil controller must reject list rendering")
	}
	if _, err := NewController(source, nil, nil).RenderCostumeList(ListQuery{}); err == nil {
		t.Fatal("controller without drawing client must reject list rendering")
	}
	if _, _, err := NewController(nil, drawing.NewHarukiDrawingClient(drawingServer.URL), nil).RenderCostumeListWithRequest(ListQuery{}); err == nil {
		t.Fatal("controller without a data source must reject list rendering")
	}
	if _, _, err := controller.RenderCostumeListWithRequest(ListQuery{Query: "角色"}); err == nil {
		t.Fatal("invalid list query must be returned before drawing")
	}

	failingSource := *source
	failingSource.filterErr = errors.New("filter failed")
	if _, _, err := NewController(&failingSource, drawing.NewHarukiDrawingClient(drawingServer.URL), nil).RenderCostumeListWithRequest(ListQuery{}); !errors.Is(err, failingSource.filterErr) {
		t.Fatalf("filter error = %v, want %v", err, failingSource.filterErr)
	}
	failingCards := *source
	failingCards.cardsErr = errors.New("cards failed")
	if _, _, err := NewController(&failingCards, drawing.NewHarukiDrawingClient(drawingServer.URL), nil).RenderCostumeListWithRequest(ListQuery{}); !errors.Is(err, failingCards.cardsErr) {
		t.Fatalf("source-card error = %v, want %v", err, failingCards.cardsErr)
	}

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "drawing failed", http.StatusInternalServerError)
	}))
	defer errorServer.Close()
	data, payload, err = NewController(source, drawing.NewHarukiDrawingClient(errorServer.URL), nil).RenderCostumeListWithRequest(ListQuery{})
	if err == nil || data != nil || payload == nil {
		t.Fatalf("drawing error should retain payload: data=%q payload=%+v err=%v", data, payload, err)
	}
}

func TestControllerCoverageListValidationAndPagination(t *testing.T) {
	item := controllerCoverageCostume(33_001, "body")
	source := &controllerCoverageSource{costumes: map[int]*masterdata.Costume3d{item.ID: item}}
	controller := NewController(source, nil, nil)

	queries := []ListQuery{
		{PartType: "hair", Character3DID: 1},
		{PartType: "head"},
		{PartType: "body", AccessoryIDs: []int{1}},
		{Character3DID: 32},
		{Character: "1", Gender: "male"},
	}
	for _, query := range queries {
		if _, err := controller.BuildCostumeListRequest(query); err == nil {
			t.Fatalf("BuildCostumeListRequest(%+v) unexpectedly succeeded", query)
		}
	}

	if got, pages := paginateCostumeListItems(nil, ListQuery{}, 2, 9); len(got) != 0 || pages != 1 {
		t.Fatalf("empty costume pagination = %v, %d", got, pages)
	}
	items := []*masterdata.Costume3d{
		controllerCoverageCostume(1, "body"),
		controllerCoverageCostume(2, "head"),
		controllerCoverageCostume(3, "hair"),
	}
	if got, pages := paginateCostumeListItems(items, ListQuery{}, 2, 0); len(got) != 2 || pages != 2 {
		t.Fatalf("first page = %v, %d", got, pages)
	}
	if got, _ := paginateCostumeListItems(items, ListQuery{}, 2, 9); len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("clamped last page = %+v", got)
	}
	balanced, _ := paginateCostumeListItems(items, ListQuery{Character: "miku"}, 2, 1)
	if len(balanced) != 2 || balanced[0].PartType != "body" || balanced[1].PartType != "head" {
		t.Fatalf("balanced page = %+v", balanced)
	}

	logical := []costumeAccessoryListItem{{costume: items[0]}, {costume: items[1], accessoryID: 2}, {costume: items[2]}}
	if got, pages := paginateCostumeAccessoryListItems(nil, 2, 0); len(got) != 0 || pages != 1 {
		t.Fatalf("empty accessory pagination = %v, %d", got, pages)
	}
	if got, pages := paginateCostumeAccessoryListItems(logical, 2, 9); len(got) != 1 || pages != 2 {
		t.Fatalf("clamped accessory page = %+v, %d", got, pages)
	}
	if got, pages := paginateCostumeLogicalListItems(logical, ListQuery{}, 2, 0); len(got) != 2 || pages != 2 {
		t.Fatalf("logical first page = %+v, %d", got, pages)
	}
	if got, pages := paginateCostumeLogicalListItems(logical, ListQuery{}, 2, 9); len(got) != 1 || pages != 2 {
		t.Fatalf("logical last page = %+v, %d", got, pages)
	}
	if got, _ := paginateCostumeLogicalListItems(logical, ListQuery{Character3DID: 1}, 2, 1); len(got) != 2 {
		t.Fatalf("balanced logical page = %+v", got)
	}
}

func TestControllerCoverageRenderCostumeDetailErrors(t *testing.T) {
	item := controllerCoverageCostume(33_001, "body")
	source := &controllerCoverageSource{
		costumes:   map[int]*masterdata.Costume3d{item.ID: item},
		characters: map[int]*masterdata.Character{20: {ID: 20, GivenName: "Kanade", Unit: "school_refusal"}},
	}
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("detail-png"))
	}))
	defer drawingServer.Close()

	var nilController *Controller
	if _, err := nilController.RenderCostumeDetail(Query{}); err == nil {
		t.Fatal("nil controller must reject detail rendering")
	}
	if _, err := NewController(source, nil, nil).RenderCostumeDetail(Query{ID: item.ID}); err == nil {
		t.Fatal("controller without drawing client must reject detail rendering")
	}
	if _, err := NewController(nil, drawing.NewHarukiDrawingClient(drawingServer.URL), nil).RenderCostumeDetail(Query{ID: item.ID}); err == nil {
		t.Fatal("controller without source must reject detail rendering")
	}

	getFailure := *source
	getFailure.getErr = errors.New("get failed")
	if _, err := NewController(&getFailure, drawing.NewHarukiDrawingClient(drawingServer.URL), nil).RenderCostumeDetail(Query{ID: item.ID}); !errors.Is(err, getFailure.getErr) {
		t.Fatalf("get error = %v, want %v", err, getFailure.getErr)
	}
	controller := NewController(source, drawing.NewHarukiDrawingClient(drawingServer.URL), nil)
	if _, err := controller.RenderCostumeDetail(Query{ID: item.ID, ExpectedPartType: "hair"}); err == nil {
		t.Fatal("wrong expected part must be rejected")
	}

	variantFailure := *source
	variantFailure.variantsErr = errors.New("variant lookup failed")
	request, err := NewController(&variantFailure, nil, nil).BuildCostumeDetailRequest(Query{ID: item.ID})
	if err != nil || len(request.Costume.Variants) != 1 {
		t.Fatalf("variant failure should fall back to current costume: request=%+v err=%v", request, err)
	}
	cardsFailure := *source
	cardsFailure.cardsErr = errors.New("cards failed")
	if _, err := NewController(&cardsFailure, nil, nil).BuildCostumeDetailRequest(Query{ID: item.ID}); !errors.Is(err, cardsFailure.cardsErr) {
		t.Fatalf("cards error = %v, want %v", err, cardsFailure.cardsErr)
	}

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "drawing failed", http.StatusBadGateway)
	}))
	defer errorServer.Close()
	if _, err := NewController(source, drawing.NewHarukiDrawingClient(errorServer.URL), nil).RenderCostumeDetail(Query{ID: item.ID}); err == nil {
		t.Fatal("drawing detail failure must be returned")
	}
}

func TestControllerCoverageResolveAndCacheHelpers(t *testing.T) {
	item := controllerCoverageCostume(33_001, "body")
	source := &controllerCoverageSource{costumes: map[int]*masterdata.Costume3d{item.ID: item}, filtered: []*masterdata.Costume3d{item}}
	controller := NewController(source, nil, nil)

	var nilController *Controller
	if _, _, err := nilController.resolveSource("jp"); err == nil {
		t.Fatal("nil controller must reject source resolution")
	}
	if _, _, err := NewController(nil, nil, nil).resolveSource("en"); err == nil {
		t.Fatal("missing regional source must be rejected")
	}
	if region, resolved, err := controller.resolveSource(""); err != nil || region != renderregion.JP || resolved != source {
		t.Fatalf("default source resolution = %s, %T, %v", region, resolved, err)
	}

	for _, query := range []Query{
		{OutfitID: 33, Character3DID: 0},
		{HairID: 1, Character3DID: 1},
		{AccessoryID: 1, Character3DID: 1},
	} {
		if _, err := controller.resolveNormalizedCostume(renderregion.JP, source, query); err == nil {
			t.Fatalf("resolveNormalizedCostume(%+v) unexpectedly succeeded", query)
		}
	}
	resolved, err := controller.resolveNormalizedCostume(renderregion.JP, source, Query{OutfitID: 33, Character3DID: 20})
	if err != nil || resolved != item || source.lastFilter.ColorID != 1 {
		t.Fatalf("normalized outfit fallback = %+v filter=%+v err=%v", resolved, source.lastFilter, err)
	}
	source.filtered = nil
	if _, err := controller.resolveNormalizedCostume(renderregion.JP, source, Query{OutfitID: 99, Character3DID: 20}); err == nil {
		t.Fatal("missing normalized outfit must be rejected")
	}
	filterFailure := *source
	filterFailure.filterErr = errors.New("filter failed")
	if _, err := controller.resolveNormalizedCostume(renderregion.JP, &filterFailure, Query{OutfitID: 33, Character3DID: 20}); !errors.Is(err, filterFailure.filterErr) {
		t.Fatalf("normalized filter error = %v, want %v", err, filterFailure.filterErr)
	}

	if path, err := nilController.resolve3DPreviewPath(context.Background(), renderregion.JP, item, Query{}); err != nil || path != "" {
		t.Fatalf("nil preview resolver = %q, %v", path, err)
	}
	if path, err := controller.resolve3DPreviewPath(context.Background(), renderregion.JP, nil, Query{}); err != nil || path != "" {
		t.Fatalf("nil costume preview resolver = %q, %v", path, err)
	}
	if err := nilController.ensure3DPreviewCapture(context.Background(), renderregion.JP, item, Query{}); err != nil {
		t.Fatalf("nil preview capture = %v", err)
	}
	if err := controller.ensure3DPreviewCapture(context.Background(), renderregion.JP, nil, Query{}); err != nil {
		t.Fatalf("nil costume capture = %v", err)
	}

	setCostumeDetailPreviewPath(nil, "preview")
	setCostumeDetailPreviewPath(map[string]any{"costume": "invalid"}, "preview")
	payload := map[string]any{"costume": map[string]any{}}
	setCostumeDetailPreviewPath(payload, "preview")
	if payload["costume"].(map[string]any)["preview_image_path"] != "preview" {
		t.Fatal("preview path was not set on a prepared drawing payload")
	}
	if got := controller.costumeDetailCacheRequest(nil); got == nil || !reflect.ValueOf(got).IsNil() {
		t.Fatalf("nil cache request = %#v", got)
	}
	cacheRequest := controller.costumeDetailCacheRequest(&drawing.CostumeDetailRequest{Region: "jp"}).(costumeDetailCacheRequest)
	if cacheRequest.Region != "jp" || cacheRequest.Preview3DCacheSignature != "" {
		t.Fatalf("cache request = %+v", cacheRequest)
	}
}

func TestControllerCoveragePureHelpers(t *testing.T) {
	controller := NewController(&controllerCoverageSource{}, nil, renderassets.NewAssetHelper("https://assets.example/root", nil))
	if got := controller.buildThumbnailPath(renderregion.JP, nil); got != "" {
		t.Fatalf("nil costume thumbnail = %q", got)
	}
	for _, id := range []int{27, 28, 29, 30, 31, 1, 999} {
		if got := controller.buildCharacterIconPath(id, ""); got == "" {
			t.Fatalf("empty icon path for character %d", id)
		}
	}
	if got := controller.buildUnitLogoPath(""); got != "" {
		t.Fatalf("empty unit logo = %q", got)
	}
	if got := controller.buildUnitLogoPath("idol"); !strings.Contains(got, "logo_idol.png") {
		t.Fatalf("unit logo path = %q", got)
	}

	if got := compactQuery(" ", " one ", "two", ""); got != "one two" {
		t.Fatalf("compactQuery = %q", got)
	}
	if containsString([]string{"a", "b"}, "c") {
		t.Fatal("containsString reported a missing value")
	}
	if got := uniqueCostumeSourceCardIDs([]*masterdata.Costume3d{nil, {ID: 1}, {ID: 2}}, map[int][]int{1: {3, 2}, 2: {2, 1}}); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("unique source cards = %v", got)
	}

	if normalizedOutfitID(nil) != 0 || normalizedOutfitID(&masterdata.Costume3d{PartType: "head", GroupID: 33_000}) != 0 || normalizedOutfitID(&masterdata.Costume3d{PartType: "body", GroupID: 999}) != 0 {
		t.Fatal("normalizedOutfitID accepted an invalid costume")
	}
	if _, ok := characterIDFor3DRole(99); ok {
		t.Fatal("invalid 3D role was accepted")
	}
	for characterID, want := range map[int][]int{1: {1}, 21: {21, 22, 23, 24, 25, 26}, 22: {27}, 99: nil} {
		if got := character3DIDsForCharacter(characterID); !reflect.DeepEqual(got, want) {
			t.Fatalf("character3DIDsForCharacter(%d) = %v, want %v", characterID, got, want)
		}
	}

	if costumeSortTime(nil) != 0 || costumeSortTime(&masterdata.Costume3d{PublishedAt: 3, ArchivePublishedAt: 2}) != 3 || costumeSortTime(&masterdata.Costume3d{ArchivePublishedAt: 2}) != 2 {
		t.Fatal("costumeSortTime precedence is incorrect")
	}
	for _, test := range []struct {
		costume *masterdata.Costume3d
		want    string
	}{
		{nil, ""},
		{&masterdata.Costume3d{AssetBundleName: "ready_body"}, "ready_body"},
		{&masterdata.Costume3d{AssetBundleName: "raw"}, "raw"},
		{&masterdata.Costume3d{ID: 33_001, PartType: "body", ColorID: 2}, "cos0033_body_01"},
	} {
		if got := buildCostumeAssetBundleName(test.costume); got != test.want {
			t.Fatalf("buildCostumeAssetBundleName(%+v) = %q, want %q", test.costume, got, test.want)
		}
	}
	for _, test := range []struct {
		costume *masterdata.Costume3d
		want    string
	}{
		{nil, ""},
		{&masterdata.Costume3d{Name: " Name ", Description: "description"}, "Name"},
		{&masterdata.Costume3d{Description: " description "}, "description"},
		{&masterdata.Costume3d{AssetBundleName: "asset"}, "asset"},
	} {
		if got := costumeDisplayName(test.costume); got != test.want {
			t.Fatalf("costumeDisplayName(%+v) = %q, want %q", test.costume, got, test.want)
		}
	}
	if partTypeName("unknown") != "unknown" {
		t.Fatal("unknown part type should be preserved")
	}
	for _, test := range []struct {
		character *masterdata.Character
		want      string
	}{
		{nil, "角色7"},
		{&masterdata.Character{FirstName: " ", GivenName: " "}, "角色7"},
		{&masterdata.Character{FirstName: "Hatsune", GivenName: "Miku"}, "HatsuneMiku"},
	} {
		if got := characterName(test.character, 7); got != test.want {
			t.Fatalf("characterName(%+v) = %q, want %q", test.character, got, test.want)
		}
	}
	if characterUnit(nil) != "" || characterGender(nil) != "" || characterUnit(&masterdata.Character{Unit: " idol "}) != "idol" || characterGender(&masterdata.Character{Gender: " female "}) != "female" {
		t.Fatal("character metadata helpers returned unexpected values")
	}

	items := []*masterdata.Costume3d{
		{ID: 1, Seq: 1, PublishedAt: 1},
		{ID: 3, Seq: 1, PublishedAt: 1},
		{ID: 2, Seq: 2, PublishedAt: 1},
		{ID: 4, ArchivePublishedAt: 4},
	}
	sortCostumesForDisplay(items)
	if got := []int{items[0].ID, items[1].ID, items[2].ID, items[3].ID}; !reflect.DeepEqual(got, []int{4, 2, 3, 1}) {
		t.Fatalf("sorted costume IDs = %v", got)
	}
}

func TestControllerCoverageParsingErrorsAndAliases(t *testing.T) {
	for _, test := range []struct {
		raw      string
		partType string
		matched  bool
		wantErr  bool
	}{
		{"", "invalid", false, true},
		{"", "body", false, false},
		{"服装1 角色", "body", true, true},
		{"服装1 角色 unknown", "body", true, true},
		{"服装1 角色1 角色2", "body", true, true},
		{"服装1 颜色1 颜色2 角色1", "body", true, true},
		{"服装1 角色1 2 3", "body", true, true},
		{"角色1", "body", false, false},
		{"服装1 角色99", "body", true, true},
		{"服装1 角色1 颜色5", "body", true, true},
		{"服装1 角色1 ???", "body", true, true},
		{"text", "body", false, false},
	} {
		_, matched, err := ParseLookupQuery(test.raw, test.partType)
		if matched != test.matched || (err != nil) != test.wantErr {
			t.Fatalf("ParseLookupQuery(%q, %q) matched=%v err=%v", test.raw, test.partType, matched, err)
		}
	}
	for _, partType := range []string{"body", "head", "hair"} {
		query, matched, err := ParseLookupQuery("1 角色2 颜色3", partType)
		if err != nil || !matched || query.Character3DID != 2 || query.ColorID != 3 {
			t.Fatalf("ParseLookupQuery short %s = %+v, %v, %v", partType, query, matched, err)
		}
	}

	for _, test := range []struct {
		raw      string
		partType string
		matched  bool
		wantErr  bool
	}{
		{"name", "invalid", false, true},
		{"", "body", false, false},
		{"name 角色", "body", true, true},
		{"name 角色 unknown", "body", true, true},
		{"name 角色1 角色2", "body", true, true},
		{"角色1", "body", true, true},
		{"name 角色99", "body", true, true},
	} {
		_, matched, err := ParseNamedLookupQuery(test.raw, test.partType)
		if matched != test.matched || (err != nil) != test.wantErr {
			t.Fatalf("ParseNamedLookupQuery(%q, %q) matched=%v err=%v", test.raw, test.partType, matched, err)
		}
	}

	for token, want := range map[string]string{"unit:n25": "school_refusal", "team:ln": "light_sound", "missing": ""} {
		got, ok := parseCostumeUnitAlias(token)
		if got != want || ok != (want != "") {
			t.Fatalf("parseCostumeUnitAlias(%q) = %q, %v", token, got, ok)
		}
	}
	if value, ok := resolveCharacterID("2"); !ok || value != 2 {
		t.Fatalf("resolveCharacterID numeric = %d, %v", value, ok)
	}
	if value, ok := resolveCharacterID(""); ok || value != 0 {
		t.Fatalf("resolveCharacterID empty = %d, %v", value, ok)
	}

	for token, want := range map[string]string{"male": "male", "girl": "female", "secret": "secret", "invalid": ""} {
		got, ok := normalizeGender(token)
		if got != want || ok != (want != "") {
			t.Fatalf("normalizeGender(%q) = %q, %v", token, got, ok)
		}
	}
	for _, token := range []string{"男装", "女饰品", "男发型", "女发型", "invalid"} {
		_, _, _ = normalizeGenderPart(token)
	}
	for _, gender := range []string{"male", "female", "secret", "invalid"} {
		_ = characterIDsForGender(gender)
	}
	for _, token := range []string{"all", "pagesize999", "3条/页", "invalid"} {
		_, _ = parsePageSizeToken(token)
	}
	for _, test := range []struct {
		token string
		max   int
	}{
		{"invalid", 10}, {"0", 10}, {"99", 10}, {"2", 10},
	} {
		_, _ = parsePositiveBoundedInt(test.token, test.max)
	}
}

func TestControllerCoverageComboValidation(t *testing.T) {
	var nilController *Controller
	if _, err := nilController.RenderCostumeCombo(ComboQuery{}); err == nil {
		t.Fatal("nil combo controller must be rejected")
	}
	if _, err := NewController(&controllerCoverageSource{}, nil, nil).RenderCostumeCombo(ComboQuery{}); err == nil {
		t.Fatal("combo rendering without preview service must be rejected")
	}
	controller := NewController(&controllerCoverageSource{}, nil, nil)
	controller.Set3DPreviewConfig(Preview3DConfig{Enabled: true})
	if _, err := controller.RenderCostumeCombo(ComboQuery{Query: "角色"}); err == nil {
		t.Fatal("invalid combo query must be returned before capture")
	}
	controller.ctx = nil
	if _, err := controller.RenderCostumeCombo(ComboQuery{Character3DID: 1}); err == nil {
		t.Fatal("missing preview endpoint must be returned")
	}

	invalidQueries := []string{
		"角色1 角色2",
		"角色1 服装1 服装2",
		"角色1 发型1 发型2",
		"角色1 饰品1 饰品2",
		"角色1 服装1 颜色5",
		"角色1 颜色1",
		"角色1 服装",
		"角色1 unknown",
		"99",
	}
	for _, raw := range invalidQueries {
		if _, err := parseComboQuery(ComboQuery{Query: raw}); err == nil {
			t.Fatalf("parseComboQuery(%q) unexpectedly succeeded", raw)
		}
	}

	query := ComboQuery{}
	last := ""
	if err := assignComboValue(&query, "unknown", 1, &last); err == nil {
		t.Fatal("unknown combo label was accepted")
	}
	if err := assignComboValue(&query, "outfit", 0, &last); err == nil {
		t.Fatal("non-positive combo ID was accepted")
	}
	if err := assignComboColor(&query, "unknown", 1); err == nil {
		t.Fatal("unknown color target was accepted")
	}
	query.OutfitColorID = 1
	if err := assignComboColor(&query, "outfit", 2); err == nil {
		t.Fatal("duplicate outfit color was accepted")
	}
	query.AccessoryColorID = 1
	if err := assignComboColor(&query, "accessory", 2); err == nil {
		t.Fatal("duplicate accessory color was accepted")
	}
	for _, label := range []string{"role", "outfit_color", "accessory_color", "color", "outfit", "hair", "accessory", "head_optional", "unknown"} {
		_, _ = normalizeComboLabel(label)
	}
}

func TestControllerCoverageListNormalizationAndPrompt(t *testing.T) {
	for _, test := range []struct {
		name    string
		query   ListQuery
		check   func(ListQuery) bool
		wantErr bool
	}{
		{
			name:  "separated character alias and Miku unit",
			query: ListQuery{Query: "角色 miku n25"},
			check: func(query ListQuery) bool { return query.Character3DID == 26 },
		},
		{
			name:  "separated numeric role",
			query: ListQuery{Query: "角色 2"},
			check: func(query ListQuery) bool { return query.Character3DID == 2 },
		},
		{
			name:    "duplicate separated numeric role",
			query:   ListQuery{Query: "角色 2", Character3DID: 1},
			wantErr: true,
		},
		{
			name:    "invalid separated role",
			query:   ListQuery{Query: "角色 unknown"},
			wantErr: true,
		},
		{
			name:  "compound gender locks part",
			query: ListQuery{Query: "男装 发型"},
			check: func(query ListQuery) bool { return query.Gender == "male" && query.PartType == "body" },
		},
		{
			name:  "standalone gender",
			query: ListQuery{Query: "female"},
			check: func(query ListQuery) bool { return query.Gender == "female" },
		},
		{
			name:    "duplicate labeled role",
			query:   ListQuery{Query: "角色2", Character3DID: 1},
			wantErr: true,
		},
		{
			name:  "plain number is a page shortcut",
			query: ListQuery{Query: "2"},
			check: func(query ListQuery) bool { return query.Page == 2 },
		},
		{
			name:  "explicit compound gender",
			query: ListQuery{Gender: "女饰品"},
			check: func(query ListQuery) bool { return query.Gender == "female" && query.PartType == "head" },
		},
		{
			name:  "explicit simple gender",
			query: ListQuery{Gender: "secret"},
			check: func(query ListQuery) bool { return query.Gender == "secret" },
		},
		{
			name:  "explicit keyword joins parsed keywords",
			query: ListQuery{Query: "alpha", Keyword: "beta"},
			check: func(query ListQuery) bool { return query.Keyword == "alpha beta" },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, err := normalizeListQuery(test.query)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizeListQuery(%+v) = %+v, %v", test.query, query, err)
			}
			if err == nil && test.check != nil && !test.check(query) {
				t.Fatalf("normalizeListQuery(%+v) = %+v", test.query, query)
			}
		})
	}

	if prompt := BuildListPrompt(nil); prompt != "" {
		t.Fatalf("nil list prompt = %q", prompt)
	}
	prompt := BuildListPrompt(&drawing.CostumeListRequest{
		Costumes: []drawing.CostumeBasic{{HairID: 1}},
	})
	if !strings.Contains(prompt, "第 1/1 页") || !strings.Contains(prompt, "试穿") {
		t.Fatalf("default hair list prompt = %q", prompt)
	}
	title := " Custom title "
	prompt = BuildListPrompt(&drawing.CostumeListRequest{
		Title:      &title,
		Page:       3,
		TotalPages: 2,
	})
	if !strings.Contains(prompt, "Custom title") || !strings.Contains(prompt, "p2") {
		t.Fatalf("last-page prompt = %q", prompt)
	}

	for _, query := range []ListQuery{
		{Gender: "male"},
		{Gender: "female"},
		{Gender: "secret"},
		{PartType: "hair", Character: "miku", Character3DID: 21, AccessoryIDs: []int{2, 1}, Keyword: "keyword"},
	} {
		if label := buildFilterLabel(query); label == "" {
			t.Fatalf("empty filter label for %+v", query)
		}
	}
}
