package card

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type lookupTestSource struct {
	region           renderregion.Value
	card             *masterdata.Card
	cards            []*masterdata.Card
	characters       map[int]*masterdata.Character
	costumesByCard   map[int][]*masterdata.Costume3d
	unitByCard       map[int]string
	supplyByCard     map[int]string
	filterFunc       func(*PjskCardQueryInfo) ([]*masterdata.Card, error)
	allowEmptyFilter bool
}

func (s *lookupTestSource) DefaultRegion() renderregion.Value {
	if s.region.IsZero() {
		return renderregion.JP
	}
	return s.region
}

func (s *lookupTestSource) GetCardByID(id int) (*masterdata.Card, error) {
	if s.card != nil && s.card.ID == id {
		cp := *s.card
		if s.card.CardParameters != nil {
			cp.CardParameters = append([]masterdata.CardParameter(nil), s.card.CardParameters...)
		}
		return &cp, nil
	}
	for _, item := range s.cards {
		if item != nil && item.ID == id {
			cp := *item
			if item.CardParameters != nil {
				cp.CardParameters = append([]masterdata.CardParameter(nil), item.CardParameters...)
			}
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("card %d not found", id)
}

func (s *lookupTestSource) GetCardByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error) {
	items := make([]*masterdata.Card, 0, len(s.cards))
	for _, item := range s.cards {
		if item != nil && item.CharacterID == characterID {
			items = append(items, new(*item))
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("card not found: %d/%d", characterID, seq)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ReleaseAt == items[j].ReleaseAt {
			return items[i].ID < items[j].ID
		}
		return items[i].ReleaseAt < items[j].ReleaseAt
	})
	if seq >= 0 {
		return nil, fmt.Errorf("card sequence must be negative: %d", seq)
	}
	index := len(items) + seq
	if index < 0 || index >= len(items) {
		return nil, fmt.Errorf("card not found: %d/%d", characterID, seq)
	}
	return items[index], nil
}

func (s *lookupTestSource) FilterCards(info *PjskCardQueryInfo) ([]*masterdata.Card, error) {
	if s.filterFunc != nil {
		return s.filterFunc(info)
	}
	if info == nil {
		return nil, fmt.Errorf("filter not supported: %+v", info)
	}
	if s.allowEmptyFilter && info.CharacterID == 0 && info.Rarity == "" && info.Attr == "" &&
		info.SkillType == "" && info.Unit == "" && info.MainUnit == "" && info.SupportUnit == "" &&
		info.SupplyType == "" && info.Year == 0 && info.EventID == 0 && info.BanCharID == 0 && info.BanSeq == 0 {
		out := make([]*masterdata.Card, 0, len(s.cards))
		for _, item := range s.cards {
			if item == nil {
				continue
			}
			out = append(out, new(*item))
		}
		return out, nil
	}
	if info.CharacterID != 5 || info.Rarity != "rarity_4" {
		return nil, fmt.Errorf("filter not supported: %+v", info)
	}
	out := make([]*masterdata.Card, 0, len(s.cards))
	for _, item := range s.cards {
		if item == nil {
			continue
		}
		out = append(out, new(*item))
	}
	return out, nil
}

func (s *lookupTestSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	if s.characters != nil {
		if item := s.characters[id]; item != nil {
			return new(*item), nil
		}
	}
	return nil, fmt.Errorf("character %d not found", id)
}

func (s *lookupTestSource) GetCharacterColorCode(id int) (string, bool) {
	return "", false
}

func (s *lookupTestSource) GetUnitByCardID(cardID int) (string, error) {
	if s.unitByCard != nil {
		if unit, ok := s.unitByCard[cardID]; ok {
			return unit, nil
		}
	}
	return "", nil
}

func (s *lookupTestSource) GetCardSupplyType(card *masterdata.Card) string {
	if s.supplyByCard != nil && card != nil {
		if supply, ok := s.supplyByCard[card.ID]; ok {
			return supply
		}
	}
	return ""
}

func (s *lookupTestSource) GetSkillByID(id int) (*masterdata.Skill, error) {
	return nil, fmt.Errorf("skill %d not found", id)
}

func (s *lookupTestSource) FormatSkillDescription(skill *masterdata.Skill, cardCharacterID int) string {
	return ""
}

func (s *lookupTestSource) GetGachaByCardID(cardID int) (*masterdata.Gacha, error) {
	return nil, fmt.Errorf("gacha not found: %d", cardID)
}

func (s *lookupTestSource) GetCostume3dsByCardID(cardID int) ([]*masterdata.Costume3d, error) {
	if s.costumesByCard != nil {
		items := s.costumesByCard[cardID]
		out := make([]*masterdata.Costume3d, 0, len(items))
		for _, item := range items {
			if item == nil {
				continue
			}
			out = append(out, new(*item))
		}
		return out, nil
	}
	return nil, nil
}

func TestResolveCardImagesSupportsStandardAndRipPaths(t *testing.T) {
	root := t.TempDir()
	normal := filepath.Join(root, "asset", "jp-assets", "startapp", "character", "member", "card_test", "card_normal.png")
	after := filepath.Join(root, "asset", "jp-assets", "startapp", "character", "member", "card_test", "card_after_training.png")
	for _, path := range []string{normal, after} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte("png"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	source := &lookupTestSource{
		card: &masterdata.Card{
			ID:              1001,
			CharacterID:     5,
			CardRarityType:  "rarity_4",
			Attr:            "cute",
			Prefix:          "Test Card",
			AssetBundleName: "card_test",
		},
	}
	controller := NewController(source, nil, nil, assets.NewAssetHelper(root, nil))

	result, err := controller.ResolveCardImages(Query{Query: "1001", Region: "jp"})
	if err != nil {
		t.Fatalf("ResolveCardImages() error = %v", err)
	}
	if len(result.Paths) != 2 {
		t.Fatalf("expected 2 images, got %d (%v)", len(result.Paths), result.Paths)
	}
	if filepath.Clean(result.Paths[0]) != filepath.Clean(normal) {
		t.Fatalf("unexpected normal path: %q", result.Paths[0])
	}
	if filepath.Clean(result.Paths[1]) != filepath.Clean(after) {
		t.Fatalf("unexpected after-training path: %q", result.Paths[1])
	}
}

func TestSearchServiceSupportsGlobalLatestVisibleCard(t *testing.T) {
	now := time.Now().UnixMilli()
	source := &lookupTestSource{
		cards: []*masterdata.Card{
			{ID: 101, CharacterID: 5, CardRarityType: "rarity_4", Prefix: "Old", AssetBundleName: "card_old", ReleaseAt: now - 3000},
			{ID: 102, CharacterID: 6, CardRarityType: "rarity_4", Prefix: "Latest Visible", AssetBundleName: "card_latest", ReleaseAt: now - 1000},
			{ID: 103, CharacterID: 7, CardRarityType: "rarity_4", Prefix: "Future", AssetBundleName: "card_future", ReleaseAt: now + 1000},
		},
		allowEmptyFilter: true,
	}

	searcher := NewSearchService(source, NewParser(defaultNicknames))
	cardInfo, err := searcher.Search("-1")
	if err != nil {
		t.Fatalf("Search(-1) error = %v", err)
	}
	if cardInfo.ID != 102 {
		t.Fatalf("expected latest visible card 102, got %+v", cardInfo)
	}

	list, err := searcher.SearchList("-2")
	if err != nil {
		t.Fatalf("SearchList(-2) error = %v", err)
	}
	if len(list) != 1 || list[0].ID != 101 {
		t.Fatalf("expected second latest visible card 101, got %+v", list)
	}
}

func TestSearchServiceSupportsCharacterLatestVisibleCard(t *testing.T) {
	now := time.Now().UnixMilli()
	source := &lookupTestSource{
		cards: []*masterdata.Card{
			{ID: 501, CharacterID: 5, CardRarityType: "rarity_4", Prefix: "Old Visible", AssetBundleName: "card_old_visible", ReleaseAt: now - 3000},
			{ID: 502, CharacterID: 5, CardRarityType: "rarity_4", Prefix: "Latest Visible", AssetBundleName: "card_latest_visible", ReleaseAt: now - 1000},
			{ID: 503, CharacterID: 5, CardRarityType: "rarity_4", Prefix: "Future", AssetBundleName: "card_future", ReleaseAt: now + 1000},
		},
	}
	source.filterFunc = func(info *PjskCardQueryInfo) ([]*masterdata.Card, error) {
		if info == nil || info.CharacterID != 5 {
			return nil, fmt.Errorf("filter not supported: %+v", info)
		}
		out := make([]*masterdata.Card, 0, 3)
		for _, item := range source.cards {
			if item == nil || item.CharacterID != info.CharacterID {
				continue
			}
			out = append(out, new(*item))
		}
		return out, nil
	}

	searcher := NewSearchService(source, NewParser(defaultNicknames))
	cardInfo, err := searcher.Search("mnr-1")
	if err != nil {
		t.Fatalf("Search(mnr-1) error = %v", err)
	}
	if cardInfo.ID != 502 {
		t.Fatalf("expected latest visible character card 502, got %+v", cardInfo)
	}

	list, err := searcher.SearchList("mnr-2")
	if err != nil {
		t.Fatalf("SearchList(mnr-2) error = %v", err)
	}
	if len(list) != 1 || list[0].ID != 501 {
		t.Fatalf("expected second latest visible character card 501, got %+v", list)
	}
}

func TestBuildCardDetailRequestSkipsEmptyCostumeAssetBundlePaths(t *testing.T) {
	source := &lookupTestSource{
		region: renderregion.CN,
		card: &masterdata.Card{
			ID:              1001,
			CharacterID:     5,
			CardRarityType:  "rarity_4",
			Attr:            "cute",
			Prefix:          "Test Card",
			AssetBundleName: "card_test",
		},
		characters: map[int]*masterdata.Character{
			5: {ID: 5, FirstName: "花里", GivenName: "实乃理", Unit: "idol"},
		},
		costumesByCard: map[int][]*masterdata.Costume3d{
			1001: {
				{ID: 1, CharacterID: 5, AssetBundleName: ""},
				{ID: 2, CharacterID: 5, AssetBundleName: "head_default_01"},
			},
		},
	}

	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardDetailRequest(Query{Query: "1001", Region: "cn"})
	if err != nil {
		t.Fatalf("BuildCardDetailRequest() error = %v", err)
	}

	if len(req.CostumeImagesPath) != 1 {
		t.Fatalf("expected exactly one valid costume path, got %+v", req.CostumeImagesPath)
	}
	if got := req.CostumeImagesPath[0]; got != "asset/cn-assets/startapp/thumbnail/costume/head_default_01.png" {
		t.Fatalf("unexpected costume path: %q", got)
	}
}

func TestBuildCardDetailRequestBuildsCostumePathFromParts(t *testing.T) {
	source := &lookupTestSource{
		region: renderregion.CN,
		card: &masterdata.Card{
			ID:              1001,
			CharacterID:     5,
			CardRarityType:  "rarity_4",
			Attr:            "cute",
			Prefix:          "Test Card",
			AssetBundleName: "card_test",
		},
		characters: map[int]*masterdata.Character{
			5: {ID: 5, FirstName: "花里", GivenName: "实乃理", Unit: "idol"},
		},
		costumesByCard: map[int][]*masterdata.Costume3d{
			1001: {
				{ID: 25000, CharacterID: 5, AssetBundleName: "", PartType: "head", ColorID: 3},
			},
		},
	}

	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardDetailRequest(Query{Query: "1001", Region: "cn"})
	if err != nil {
		t.Fatalf("BuildCardDetailRequest() error = %v", err)
	}

	if len(req.CostumeImagesPath) != 1 {
		t.Fatalf("expected exactly one costume path, got %+v", req.CostumeImagesPath)
	}
	if got := req.CostumeImagesPath[0]; got != "asset/cn-assets/startapp/thumbnail/costume/cos0025_head_02.png" {
		t.Fatalf("unexpected costume path: %q", got)
	}
}
