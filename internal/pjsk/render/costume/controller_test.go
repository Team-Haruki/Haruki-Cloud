package costume

import (
	"fmt"
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
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

func TestBuildListPromptIncludesPagingAndDetailHint(t *testing.T) {
	controller := NewController(denseListTestSource{costumes: makeDenseListTestCostumes(500)}, nil, nil)

	request, err := controller.BuildCostumeListRequest(ListQuery{Query: "女装"})
	if err != nil {
		t.Fatalf("BuildCostumeListRequest failed: %v", err)
	}
	prompt := BuildListPrompt(request)
	for _, want := range []string{"第 1/3 页", "本页 240 项", "共 500 项", "/查服装 ID", "p2"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got %q", want, prompt)
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
