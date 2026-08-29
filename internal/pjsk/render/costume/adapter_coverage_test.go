package costume

import (
	"context"
	"errors"
	"reflect"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

type costumeAdapterCoverageProvider struct {
	provider.MasterDataProvider
	costumes   provider.CostumeProvider
	characters provider.CharacterProvider
}

func (p *costumeAdapterCoverageProvider) Region() renderregion.Value {
	return renderregion.EN
}

func (p *costumeAdapterCoverageProvider) Costumes() provider.CostumeProvider {
	return p.costumes
}

func (p *costumeAdapterCoverageProvider) Characters() provider.CharacterProvider {
	return p.characters
}

type costumeAdapterCoverageCostumes struct {
	ctx           context.Context
	id            int
	filter        *provider.CostumeFilter
	groupID       int
	partType      string
	characterID   int
	costumeIDs    []int
	costume       *masterdata.Costume3d
	filtered      []*masterdata.Costume3d
	variants      []*masterdata.Costume3d
	sourceCardIDs map[int][]int
	err           error
}

func (p *costumeAdapterCoverageCostumes) GetByID(ctx context.Context, id int) (*masterdata.Costume3d, error) {
	p.ctx = ctx
	p.id = id
	return p.costume, p.err
}

func (p *costumeAdapterCoverageCostumes) Filter(ctx context.Context, filter *provider.CostumeFilter) ([]*masterdata.Costume3d, error) {
	p.ctx = ctx
	p.filter = filter
	return p.filtered, p.err
}

func (p *costumeAdapterCoverageCostumes) GetVariants(ctx context.Context, groupID int, partType string, characterID int) ([]*masterdata.Costume3d, error) {
	p.ctx = ctx
	p.groupID = groupID
	p.partType = partType
	p.characterID = characterID
	return p.variants, p.err
}

func (p *costumeAdapterCoverageCostumes) GetSourceCardIDs(ctx context.Context, costumeIDs []int) (map[int][]int, error) {
	p.ctx = ctx
	p.costumeIDs = append([]int(nil), costumeIDs...)
	return p.sourceCardIDs, p.err
}

type costumeAdapterCoverageCharacters struct {
	ctx       context.Context
	id        int
	character *masterdata.Character
	err       error
}

func (p *costumeAdapterCoverageCharacters) GetByID(ctx context.Context, id int) (*masterdata.Character, error) {
	p.ctx = ctx
	p.id = id
	return p.character, p.err
}

func (*costumeAdapterCoverageCharacters) GetColorCode(context.Context, int) (string, bool) {
	return "", false
}

func (*costumeAdapterCoverageCharacters) GetGameCharacterUnit(context.Context, int) (*masterdata.GameCharacterUnit, error) {
	return nil, nil
}

func TestProviderAdapterDelegatesCostumeQueriesWithContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), controllerCoverageContextKey{}, "adapter")
	costume := controllerCoverageCostume(101, "body")
	character := &masterdata.Character{ID: 20, GivenName: "Kanade"}
	costumes := &costumeAdapterCoverageCostumes{
		costume:       costume,
		filtered:      []*masterdata.Costume3d{costume},
		variants:      []*masterdata.Costume3d{costume},
		sourceCardIDs: map[int][]int{101: {1001}},
	}
	characters := &costumeAdapterCoverageCharacters{character: character}
	master := &costumeAdapterCoverageProvider{costumes: costumes, characters: characters}
	adapter := NewProviderAdapter(master)
	if adapter.DefaultRegion() != renderregion.EN {
		t.Fatalf("default region = %s", adapter.DefaultRegion())
	}

	var nilAdapter *ProviderAdapter
	if nilAdapter.WithContext(ctx) != nil {
		t.Fatal("nil adapter must stay nil when contextualized")
	}
	contextual, ok := adapter.WithContext(ctx).(*ProviderAdapter)
	if !ok || contextual == adapter || contextual.P != master || contextual.Context() != ctx {
		t.Fatalf("contextual adapter = %#v", contextual)
	}

	gotCostume, err := contextual.GetCostumeByID(101)
	if err != nil || gotCostume != costume || costumes.id != 101 || costumes.ctx != ctx {
		t.Fatalf("GetCostumeByID result=%+v id=%d ctx=%v err=%v", gotCostume, costumes.id, costumes.ctx, err)
	}

	characterIDs := []int{20, 21}
	filter := Filter{
		PartType:     "body",
		CostumeType:  "normal",
		CharacterID:  20,
		CharacterIDs: characterIDs,
		ColorID:      2,
		Keyword:      "test",
		Limit:        5,
		Offset:       10,
	}
	gotFiltered, err := contextual.FilterCostumes(filter)
	characterIDs[0] = 999
	wantFilter := &provider.CostumeFilter{
		PartType:     "body",
		CostumeType:  "normal",
		CharacterID:  20,
		CharacterIDs: []int{20, 21},
		ColorID:      2,
		Keyword:      "test",
		Limit:        5,
		Offset:       10,
	}
	if err != nil || !reflect.DeepEqual(gotFiltered, costumes.filtered) || !reflect.DeepEqual(costumes.filter, wantFilter) || costumes.ctx != ctx {
		t.Fatalf("FilterCostumes result=%+v filter=%+v ctx=%v err=%v", gotFiltered, costumes.filter, costumes.ctx, err)
	}

	gotVariants, err := contextual.GetCostumeVariants(33, "body", 20)
	if err != nil || !reflect.DeepEqual(gotVariants, costumes.variants) || costumes.groupID != 33 || costumes.partType != "body" || costumes.characterID != 20 || costumes.ctx != ctx {
		t.Fatalf("GetCostumeVariants result=%+v args=%d/%q/%d ctx=%v err=%v", gotVariants, costumes.groupID, costumes.partType, costumes.characterID, costumes.ctx, err)
	}

	gotCards, err := contextual.GetCostumeSourceCardIDs([]int{101, 102})
	if err != nil || !reflect.DeepEqual(gotCards, costumes.sourceCardIDs) || !reflect.DeepEqual(costumes.costumeIDs, []int{101, 102}) || costumes.ctx != ctx {
		t.Fatalf("GetCostumeSourceCardIDs result=%+v ids=%v ctx=%v err=%v", gotCards, costumes.costumeIDs, costumes.ctx, err)
	}

	gotCharacter, err := contextual.GetCharacterByID(20)
	if err != nil || gotCharacter != character || characters.id != 20 || characters.ctx != ctx {
		t.Fatalf("GetCharacterByID result=%+v id=%d ctx=%v err=%v", gotCharacter, characters.id, characters.ctx, err)
	}
}

func TestProviderAdapterReturnsProviderErrors(t *testing.T) {
	wantErr := errors.New("provider failed")
	costumes := &costumeAdapterCoverageCostumes{err: wantErr}
	characters := &costumeAdapterCoverageCharacters{err: wantErr}
	adapter := NewProviderAdapter(&costumeAdapterCoverageProvider{costumes: costumes, characters: characters})

	if _, err := adapter.GetCostumeByID(1); !errors.Is(err, wantErr) {
		t.Fatalf("GetCostumeByID error = %v", err)
	}
	if _, err := adapter.FilterCostumes(Filter{}); !errors.Is(err, wantErr) {
		t.Fatalf("FilterCostumes error = %v", err)
	}
	if _, err := adapter.GetCostumeVariants(1, "body", 1); !errors.Is(err, wantErr) {
		t.Fatalf("GetCostumeVariants error = %v", err)
	}
	if _, err := adapter.GetCostumeSourceCardIDs([]int{1}); !errors.Is(err, wantErr) {
		t.Fatalf("GetCostumeSourceCardIDs error = %v", err)
	}
	if _, err := adapter.GetCharacterByID(1); !errors.Is(err, wantErr) {
		t.Fatalf("GetCharacterByID error = %v", err)
	}
}
