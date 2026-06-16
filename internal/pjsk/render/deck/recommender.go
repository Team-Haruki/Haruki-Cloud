package deck

import (
	"fmt"
	"sort"
	"strings"
)

func deckHash(deck RecommendDeck) string {
	cardKeys := make([]string, 0, len(deck.Cards))
	for _, card := range deck.Cards {
		cardKeys = append(cardKeys, fmt.Sprintf("%d:%d:%d:%s:%d:%g:%g:%t:%t:%t:%t",
			card.CardID,
			card.Level,
			card.MasterRank,
			card.DefaultImage,
			card.SkillLevel,
			card.SkillRate,
			card.EventBonusRate,
			card.IsBeforeStory,
			card.IsAfterStory,
			card.IsAfterTraining,
			card.HasCanvasBonus,
		))
	}
	sort.Strings(cardKeys)
	return strings.Join(cardKeys, ",")
}
