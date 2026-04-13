package card

import (
	"fmt"
	"testing"

	eventrender "haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type detailEventSource struct {
	cardID       int
	event        *masterdata.Event
	bannerCharID int
	bonuses      []*masterdata.EventDeckBonus
	gcuByID      map[int]*masterdata.GameCharacterUnit
}

func (s *detailEventSource) DefaultRegion() renderregion.Value { return renderregion.JP }

func (s *detailEventSource) GetEventByID(id int) (*masterdata.Event, error) {
	if s.event != nil && s.event.ID == id {
		copy := *s.event
		return &copy, nil
	}
	return nil, fmt.Errorf("event %d not found", id)
}

func (s *detailEventSource) GetEventByCardID(cardID int) (*masterdata.Event, error) {
	if s.event != nil && s.cardID == cardID {
		copy := *s.event
		return &copy, nil
	}
	return nil, fmt.Errorf("event for card %d not found", cardID)
}

func (s *detailEventSource) GetEvents() []*masterdata.Event { return nil }

func (s *detailEventSource) GetEventCards(eventID int) ([]*masterdata.Card, error) {
	return nil, nil
}

func (s *detailEventSource) GetEventBannerCharacterID(eventID int) (int, error) {
	if s.event == nil || s.event.ID != eventID {
		return 0, fmt.Errorf("event %d not found", eventID)
	}
	return s.bannerCharID, nil
}

func (s *detailEventSource) GetEventDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	if s.event == nil || s.event.ID != eventID {
		return nil, fmt.Errorf("event %d not found", eventID)
	}
	out := make([]*masterdata.EventDeckBonus, 0, len(s.bonuses))
	for _, item := range s.bonuses {
		if item == nil {
			continue
		}
		copy := *item
		out = append(out, &copy)
	}
	return out, nil
}

func (s *detailEventSource) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	if item := s.gcuByID[id]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, fmt.Errorf("game character unit %d not found", id)
}

func (s *detailEventSource) GetBanEvents(charID int) []*masterdata.Event { return nil }

func (s *detailEventSource) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom { return nil }

func (s *detailEventSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	return nil, fmt.Errorf("character %d not found", id)
}

var _ eventrender.DataSource = (*detailEventSource)(nil)

func TestBuildCardDetailRequestUsesLunabotStaticPathsForEventIcons(t *testing.T) {
	source := &lookupTestSource{
		card: &masterdata.Card{
			ID:              190,
			CharacterID:     6,
			CardRarityType:  "rarity_4",
			Attr:            "cute",
			Prefix:          "Test Card",
			AssetBundleName: "card_test",
		},
		characters: map[int]*masterdata.Character{
			6: {ID: 6, FirstName: "桐谷", GivenName: "遥", Unit: "idol"},
		},
	}
	eventSource := &detailEventSource{
		cardID:       190,
		bannerCharID: 6,
		event: &masterdata.Event{
			ID:              17,
			Name:            "Test Event",
			AssetBundleName: "event_test",
			StartAt:         1700000000000,
			AggregateAt:     1700003600000,
		},
		bonuses: []*masterdata.EventDeckBonus{
			{EventID: 17, GameCharacterUnitID: 501, CardAttr: "cute"},
		},
		gcuByID: map[int]*masterdata.GameCharacterUnit{
			501: {ID: 501, GameCharacterID: 6, Unit: "idol"},
		},
	}

	controller := NewController(source, eventSource, nil, nil)

	req, err := controller.BuildCardDetailRequest(Query{Query: "190", Region: "jp"})
	if err != nil {
		t.Fatalf("BuildCardDetailRequest() error = %v", err)
	}
	if req.EventUnitIconPath == nil {
		t.Fatal("expected event unit icon path")
	}
	if got := *req.EventUnitIconPath; got != "static_images/icon_idol.png" {
		t.Fatalf("expected lunabot unit icon path, got %q", got)
	}
	if req.EventCharaIconPath == nil {
		t.Fatal("expected event character icon path")
	}
	if got := *req.EventCharaIconPath; got != "static_images/chara_icon/hrk.png" {
		t.Fatalf("expected lunabot character icon path, got %q", got)
	}
}

func TestBuildCardDetailRequestKeepsBaseMikuIconForCharacter21(t *testing.T) {
	source := &lookupTestSource{
		card: &masterdata.Card{
			ID:              191,
			CharacterID:     21,
			CardRarityType:  "rarity_4",
			Attr:            "cool",
			Prefix:          "Miku Card",
			AssetBundleName: "card_miku",
		},
		characters: map[int]*masterdata.Character{
			21: {ID: 21, FirstName: "初音", GivenName: "未来", Unit: "piapro"},
		},
		unitByCard: map[int]string{
			191: "school_refusal",
		},
	}

	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardDetailRequest(Query{Query: "191", Region: "jp"})
	if err != nil {
		t.Fatalf("BuildCardDetailRequest() error = %v", err)
	}
	if req.CharacterIconPath != "static_images/chara_icon/miku.png" {
		t.Fatalf("unexpected miku icon path: %q", req.CharacterIconPath)
	}
}
