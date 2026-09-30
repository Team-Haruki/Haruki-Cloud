package music

import (
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
)

func TestConvertDetailedProfileToCardKeepsRank(t *testing.T) {
	rank := 321
	card := convertDetailedProfileToCard(drawing.DetailedProfileCardRequest{ID: "1", Rank: &rank})
	if card.Rank == nil || *card.Rank != rank || card.Rank == &rank {
		t.Fatalf("converted rank = %v, want a copy of %d", card.Rank, rank)
	}
	if convertDetailedProfileToCard(drawing.DetailedProfileCardRequest{ID: "1"}).Rank != nil {
		t.Fatal("rank should stay nil when absent")
	}
}
