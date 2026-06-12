package deck

import (
	"fmt"
	"strings"
)

func deckHash(deck RecommendDeck) string {
	cardIDs := make([]string, 0, len(deck.Cards))
	for _, card := range deck.Cards {
		cardIDs = append(cardIDs, fmt.Sprintf("%d", card.CardID))
	}
	return fmt.Sprintf("%d_%d_%d_%g_%g_%g_%s",
		deck.Score,
		deck.LiveScore,
		deck.TotalPower,
		deck.EventBonusRate,
		deck.SupportDeckBonusRate,
		deck.MultiLiveScoreUp,
		strings.Join(cardIDs, ","),
	)
}
