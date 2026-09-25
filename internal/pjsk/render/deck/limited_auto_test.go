package deck

import (
	"haruki-cloud/internal/pjsk/drawing"
	"strings"
	"testing"
)

func TestLimitedAutoScoreMetadata(t *testing.T) {
	for _, coefficient := range []float64{0, 0.7, 1.7999999523162842} {
		decks := convertRemoteDecks([]remoteRecommendDeck{{LimitedAutoScoreCoefficient: coefficient}})
		request := &drawing.DeckRequest{}
		applyLimitedAutoScoreNotice(request, decks)
		if coefficient > 0.7 {
			if request.AutoScoreNotice == nil || !strings.Contains(*request.AutoScoreNotice, "终章期间限定 AUTO 数值") || !strings.Contains(*request.AutoScoreNotice, "1.8") {
				t.Fatalf("missing applied coefficient notice: %+v", request.AutoScoreNotice)
			}
		} else if request.AutoScoreNotice != nil {
			t.Fatal("ordinary auto must not carry a limited-time notice")
		}
	}
}
