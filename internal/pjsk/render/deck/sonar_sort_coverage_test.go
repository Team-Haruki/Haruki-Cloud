package deck

import (
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
)

func TestSortRecommendDeckTeammatesTieBreakers(t *testing.T) {
	tests := []struct {
		name  string
		left  RecommendCard
		right RecommendCard
	}{
		{name: "event bonus", left: RecommendCard{EventBonusRate: 1}, right: RecommendCard{EventBonusRate: 2}},
		{name: "master rank", left: RecommendCard{MasterRank: 1}, right: RecommendCard{MasterRank: 2}},
		{name: "level", left: RecommendCard{Level: 1}, right: RecommendCard{Level: 2}},
		{name: "card id", left: RecommendCard{CardID: 1}, right: RecommendCard{CardID: 2}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := []drawing.DeckCardData{{}, {CharaID: 1}, {CharaID: 2}}
			sortRecommendDeckTeammates(data, []RecommendCard{{}, test.left, test.right})
			if data[1].CharaID != 2 {
				t.Fatalf("sorted card data = %#v", data)
			}
		})
	}
}
