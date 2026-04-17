package deck

import (
	"fmt"
)

func deckHash(deck RecommendDeck) string {
	first := 0
	if len(deck.Cards) > 0 {
		first = deck.Cards[0].CardID
	}
	return fmt.Sprintf("%d_%d_%d", deck.Score, deck.TotalPower, first)
}
