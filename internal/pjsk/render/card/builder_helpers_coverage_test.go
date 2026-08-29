package card

import (
	"errors"
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type builderCoverageSource struct {
	*lookupTestSource
	skills       map[int]*masterdata.Skill
	descriptions map[int]string
	skillErr     error
	costumes     []*masterdata.Costume3d
	costumeErr   error
}

func (s *builderCoverageSource) GetSkillByID(id int) (*masterdata.Skill, error) {
	if s.skillErr != nil {
		return nil, s.skillErr
	}
	if skill := s.skills[id]; skill != nil {
		return new(*skill), nil
	}
	return nil, errors.New("skill not found")
}

func (s *builderCoverageSource) FormatSkillDescription(skill *masterdata.Skill, _ int) string {
	if skill == nil {
		return ""
	}
	return s.descriptions[skill.ID]
}

func (s *builderCoverageSource) GetCostume3dsByCardID(int) ([]*masterdata.Costume3d, error) {
	return s.costumes, s.costumeErr
}

func TestBuilderCardBasicSkillAndAlreadyTrainedBranches(t *testing.T) {
	primary := &builderCoverageSource{
		lookupTestSource: &lookupTestSource{
			characters:   map[int]*masterdata.Character{5: {ID: 5, FirstName: "花里", GivenName: "实乃理"}},
			unitByCard:   map[int]string{1001: "idol"},
			supplyByCard: map[int]string{1001: "term_limited"},
		},
		skills: map[int]*masterdata.Skill{
			10: {ID: 10, DescriptionSpriteName: "score_up"},
			20: {ID: 20, DescriptionSpriteName: "life_recovery"},
		},
		descriptions: map[int]string{10: "primary normal", 20: "primary special"},
	}
	translation := &builderCoverageSource{
		lookupTestSource: &lookupTestSource{},
		skills: map[int]*masterdata.Skill{
			10: {ID: 10},
			20: {ID: 20},
		},
		descriptions: map[int]string{10: "translated normal", 20: "translated special"},
	}
	builder := NewBuilder(primary, translation, nil, nil)
	card := &masterdata.Card{
		ID:                           1001,
		CharacterID:                  5,
		CardRarityType:               "rarity_4",
		Attr:                         "cute",
		AssetBundleName:              "card_1",
		SkillID:                      10,
		CardSkillName:                "normal",
		SpecialTrainingSkillID:       20,
		SpecialTrainingSkillName:     "special",
		InitialSpecialTrainingStatus: "done",
	}
	info := builder.BuildCardBasic(card, renderregion.JP)
	if info.Skill == nil || info.Skill.SkillDetail != "primary normal\ntranslated normal" {
		t.Fatalf("normal skill = %+v", info.Skill)
	}
	if info.SpecialSkillInfo == nil || info.SpecialSkillInfo.SkillDetail != "primary special\ntranslated special" {
		t.Fatalf("special skill = %+v", info.SpecialSkillInfo)
	}
	if len(info.ThumbnailInfo) != 1 || info.CharacterName == nil || *info.CharacterName != "花里实乃理" {
		t.Fatalf("basic info = %+v", info)
	}
	if info.Unit == nil || *info.Unit != "idol" || info.SupplyType == nil {
		t.Fatalf("unit/supply info = %+v", info)
	}
	if info.Skill.SkillTypeIconPath == nil || !strings.Contains(*info.Skill.SkillTypeIconPath, "skill_score_up.png") {
		t.Fatalf("skill icon = %+v", info.Skill.SkillTypeIconPath)
	}
	if builder.calculatePower(nil) != nil {
		t.Fatal("nil card produced power")
	}
}

func TestBuilderDualSkillAndLineCombinationBranches(t *testing.T) {
	skill := &masterdata.Skill{ID: 10}
	card := &masterdata.Card{CharacterID: 5}
	primary := &builderCoverageSource{
		lookupTestSource: &lookupTestSource{},
		skills:           map[int]*masterdata.Skill{10: skill},
		descriptions:     map[int]string{10: " same "},
	}
	translation := &builderCoverageSource{
		lookupTestSource: &lookupTestSource{},
		skills:           map[int]*masterdata.Skill{10: skill},
		descriptions:     map[int]string{10: "same"},
	}
	builder := NewBuilder(primary, translation, nil, nil)
	if got := builder.buildDualSkillDetail(nil, skill, renderregion.JP); got != "" {
		t.Fatalf("nil card skill detail = %q", got)
	}
	if got := builder.buildDualSkillDetail(card, nil, renderregion.JP); got != "" {
		t.Fatalf("nil skill detail = %q", got)
	}
	if got := builder.buildDualSkillDetail(card, skill, renderregion.JP); got != "same" {
		t.Fatalf("deduplicated skill detail = %q", got)
	}
	translation.skillErr = errors.New("translation unavailable")
	if got := builder.buildDualSkillDetail(card, skill, renderregion.JP); got != "same" {
		t.Fatalf("failed translation detail = %q", got)
	}
	translation.skillErr = nil
	if got := builder.buildDualSkillDetail(card, skill, renderregion.CN); got != "same" {
		t.Fatalf("non-JP skill detail = %q", got)
	}
	if got := combineSkillLines("", " first ", "first", " second ", " "); got != "first\nsecond" {
		t.Fatalf("combineSkillLines() = %q", got)
	}
}

func TestBuilderPathHelperEdgeBranches(t *testing.T) {
	source := &builderCoverageSource{lookupTestSource: &lookupTestSource{}}
	builder := NewBuilder(source, nil, nil, nil)
	if got := builder.buildCardImagePaths(nil, renderregion.JP); got != nil {
		t.Fatalf("nil card image paths = %+v", got)
	}
	if got := builder.buildCostumeImagePaths(nil, renderregion.JP); len(got) != 0 {
		t.Fatalf("nil costume paths = %+v", got)
	}
	source.costumeErr = errors.New("costumes unavailable")
	if got := builder.buildCostumeImagePaths(&masterdata.Card{ID: 1}, renderregion.JP); len(got) != 0 {
		t.Fatalf("failed costume paths = %+v", got)
	}
	source.costumeErr = nil
	source.costumes = []*masterdata.Costume3d{nil, {ID: 1}, {ID: 2000, PartType: "body", ColorID: 1}}
	paths := builder.buildCostumeImagePaths(&masterdata.Card{ID: 1}, renderregion.JP)
	if len(paths) != 1 || !strings.Contains(paths[0], "cos0002_body.png") {
		t.Fatalf("filtered costume paths = %+v", paths)
	}
	if buildCostumeAssetBundleName(nil) != "" {
		t.Fatal("nil costume produced an asset bundle")
	}

	characterCases := map[int]string{
		27: "miku_light_sound.png",
		28: "miku_idol.png",
		29: "miku_street.png",
		30: "miku_theme_park.png",
		31: "miku_school_refusal.png",
		5:  "mnr.png",
		99: "chr_icon_99.png",
	}
	for characterID, suffix := range characterCases {
		if got := builder.BuildCharacterIconPath(characterID, "", renderregion.JP); !strings.HasSuffix(got, suffix) {
			t.Errorf("BuildCharacterIconPath(%d) = %q, want suffix %q", characterID, got, suffix)
		}
	}
	if got := builder.buildUnitLogoPath("", renderregion.JP); got != "" {
		t.Fatalf("empty unit logo = %q", got)
	}
	if got := builder.buildSkillTypeIconPath(" ", renderregion.JP); got != nil {
		t.Fatalf("empty skill icon = %q", *got)
	}
	if got := builder.buildSkillTypeIconPath("score_up", renderregion.JP); got == nil || !strings.HasSuffix(*got, "skill_score_up.png") {
		t.Fatalf("skill icon = %+v", got)
	}
	if got := builder.buildGachaBannerPath(0, renderregion.JP); got != "" {
		t.Fatalf("zero gacha banner = %q", got)
	}
	if got := builder.buildGachaBannerPath(123, renderregion.JP); !strings.Contains(got, "banner_gacha123") {
		t.Fatalf("gacha banner = %q", got)
	}
	if stringValue(nil) != "" {
		t.Fatal("nil string pointer was not empty")
	}
	value := "value"
	if stringValue(&value) != value {
		t.Fatal("string pointer value was not preserved")
	}
}
