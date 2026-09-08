package card

import (
	"fmt"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"testing"
)

type countedCardListSource struct {
	*lookupTestSource
	idCalls int
}

func (s *countedCardListSource) GetCardByID(id int) (*masterdata.Card, error) {
	s.idCalls++
	return s.lookupTestSource.GetCardByID(id)
}

func TestCardListReusesResolvedCardsAtBoxBoundary(t *testing.T) {
	for _, count := range []int{89, 90} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			calls := 0
			source := &countedCardListSource{lookupTestSource: &lookupTestSource{}}
			for i := range count {
				source.cards = append(source.cards, &masterdata.Card{ID: i + 1, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute"})
			}
			source.filterFunc = func(*PjskCardQueryInfo) ([]*masterdata.Card, error) { calls++; return source.cards, nil }
			controller := NewController(source, nil, nil, nil)
			title := "Selected cards"
			payload, box, err := controller.buildCardListRenderRequest(ListRequest{Query: "mnr 4星", Region: "jp", Title: &title})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || source.idCalls != 0 {
				t.Fatalf("filterCalls=%d idCalls=%d", calls, source.idCalls)
			}
			if box != (count >= cardListAutoBoxThreshold) {
				t.Fatalf("box=%v count=%d", box, count)
			}
			switch request := payload.(type) {
			case *drawing.CardListRequest:
				if len(request.Cards) != count || request.Title != &title {
					t.Fatalf("list=%+v", request)
				}
			case *drawing.CardBoxRequest:
				if len(request.Cards) != count || request.Title != &title {
					t.Fatalf("box=%+v", request)
				}
			default:
				t.Fatalf("unexpected %T", payload)
			}
		})
	}
}
