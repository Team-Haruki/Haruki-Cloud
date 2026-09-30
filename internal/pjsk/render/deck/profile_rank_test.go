package deck

import (
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
)

func TestSanitizeDeckProfileKeepsRank(t *testing.T) {
	rank := 321
	mode := "suite"
	profile := &drawing.DetailedProfileCardRequest{ID: "1", Source: "suite_dump", Mode: &mode, Rank: &rank}

	got := sanitizeDeckProfile(profile)
	if got.Source != "" || got.Mode != nil {
		t.Fatalf("sanitized profile kept source metadata: %+v", got)
	}
	if got.Rank == nil || *got.Rank != rank || got.Rank == profile.Rank {
		t.Fatalf("sanitized rank = %v, want a copy of %d", got.Rank, rank)
	}
	if sanitizeDeckProfile(&drawing.DetailedProfileCardRequest{ID: "1"}).Rank != nil {
		t.Fatal("rank should stay nil when absent")
	}
}
