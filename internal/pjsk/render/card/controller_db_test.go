package card

import (
	"encoding/json"
	"fmt"
	"testing"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type dbBackedLookupTestSource struct {
	lookupTestSource
	entity         *sekaiDB.Card
	filterEntities []*sekaiDB.Card
}

func (s *dbBackedLookupTestSource) GetCardByID(id int) (*masterdata.Card, error) {
	if s.entity == nil || int(s.entity.GameID) != id {
		return nil, fmt.Errorf("card %d not found", id)
	}
	return common.ConvertCardEntity(s.entity)
}

func (s *dbBackedLookupTestSource) FilterCards(info *PjskCardQueryInfo) ([]*masterdata.Card, error) {
	if len(s.filterEntities) == 0 {
		return s.lookupTestSource.FilterCards(info)
	}
	items := make([]*masterdata.Card, 0, len(s.filterEntities))
	for _, entity := range s.filterEntities {
		card, err := common.ConvertCardEntity(entity)
		if err != nil {
			return nil, err
		}
		items = append(items, card)
	}
	return items, nil
}

func TestBuildCardBoxRequestAcceptsDBObjectCardParameters(t *testing.T) {
	source := &dbBackedLookupTestSource{
		entity: &sekaiDB.Card{
			GameID:          1001,
			CharacterID:     5,
			CardRarityType:  "rarity_4",
			Attr:            "cute",
			Prefix:          "Card A",
			AssetbundleName: "card_a",
			CardParameters: json.RawMessage(`{
				"param2": {"power": 220},
				"param1": {"power": 110},
				"param3": 330
			}`),
		},
	}
	controller := NewController(source, nil, nil, nil)

	req, err := controller.BuildCardBoxRequest([]Query{{Query: "1001", Region: "jp"}})
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if len(req.Cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(req.Cards))
	}
	if req.Cards[0].Card.Power == nil {
		t.Fatal("expected power info to be populated")
	}
	if req.Cards[0].Card.Power.Power1 != 110 || req.Cards[0].Card.Power.Power2 != 220 || req.Cards[0].Card.Power.Power3 != 330 {
		t.Fatalf("unexpected card power: %+v", req.Cards[0].Card.Power)
	}
	if req.Cards[0].Card.Power.PowerTotal != 660 {
		t.Fatalf("unexpected power total: %+v", req.Cards[0].Card.Power)
	}
}

func TestBuildCardBoxRequestAcceptsDBObjectCardParametersOnFilterQuery(t *testing.T) {
	source := &dbBackedLookupTestSource{
		filterEntities: []*sekaiDB.Card{
			{
				GameID:          1002,
				CharacterID:     5,
				CardRarityType:  "rarity_4",
				Attr:            "cool",
				Prefix:          "Card B",
				AssetbundleName: "card_b",
				CardParameters: json.RawMessage(`{
					"param3": 450,
					"param1": {"power": 120},
					"param2": {"power": 230}
				}`),
			},
		},
	}
	controller := NewController(source, nil, nil, nil)

	req, err := controller.BuildCardBoxRequest([]Query{{Query: "mnr 4星", Region: "jp"}})
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if len(req.Cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(req.Cards))
	}
	if req.Cards[0].Card.Power == nil {
		t.Fatal("expected power info to be populated")
	}
	if req.Cards[0].Card.Power.Power1 != 120 || req.Cards[0].Card.Power.Power2 != 230 || req.Cards[0].Card.Power.Power3 != 450 {
		t.Fatalf("unexpected card power: %+v", req.Cards[0].Card.Power)
	}
	if req.Cards[0].Card.Power.PowerTotal != 800 {
		t.Fatalf("unexpected power total: %+v", req.Cards[0].Card.Power)
	}
}
