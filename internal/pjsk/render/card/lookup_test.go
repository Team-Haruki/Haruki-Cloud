package card

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type lookupTestSource struct {
	card       *masterdata.Card
	cards      []*masterdata.Card
	characters map[int]*masterdata.Character
	filterFunc func(*CardQueryInfo) ([]*masterdata.Card, error)
}

func (s *lookupTestSource) DefaultRegion() renderregion.Value { return renderregion.JP }

func (s *lookupTestSource) GetCardByID(id int) (*masterdata.Card, error) {
	if s.card != nil && s.card.ID == id {
		copy := *s.card
		if s.card.CardParameters != nil {
			copy.CardParameters = append([]masterdata.CardParameter(nil), s.card.CardParameters...)
		}
		return &copy, nil
	}
	for _, item := range s.cards {
		if item != nil && item.ID == id {
			copy := *item
			if item.CardParameters != nil {
				copy.CardParameters = append([]masterdata.CardParameter(nil), item.CardParameters...)
			}
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("card %d not found", id)
}

func (s *lookupTestSource) GetCardByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error) {
	items := make([]*masterdata.Card, 0, len(s.cards))
	for _, item := range s.cards {
		if item != nil && item.CharacterID == characterID {
			copy := *item
			items = append(items, &copy)
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

func (s *lookupTestSource) FilterCards(info *CardQueryInfo) ([]*masterdata.Card, error) {
	if s.filterFunc != nil {
		return s.filterFunc(info)
	}
	if info == nil {
		return nil, fmt.Errorf("filter not supported: %+v", info)
	}
	if info.CharacterID != 5 || info.Rarity != "rarity_4" {
		return nil, fmt.Errorf("filter not supported: %+v", info)
	}
	out := make([]*masterdata.Card, 0, len(s.cards))
	for _, item := range s.cards {
		if item == nil {
			continue
		}
		copy := *item
		out = append(out, &copy)
	}
	return out, nil
}

func (s *lookupTestSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	if s.characters != nil {
		if item := s.characters[id]; item != nil {
			copy := *item
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("character %d not found", id)
}

func (s *lookupTestSource) GetCharacterColorCode(id int) (string, bool) {
	return "", false
}

func (s *lookupTestSource) GetUnitByCardID(cardID int) (string, error) { return "", nil }

func (s *lookupTestSource) GetCardSupplyType(card *masterdata.Card) string { return "" }

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
