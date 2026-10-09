package handler

import (
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
)

func TestProfileClonesKeepRank(t *testing.T) {
	rank := 321
	target := ResolvedGameTarget{UIDVisible: true, PJSKUserID: "12345"}

	detail := cloneDetailedProfileForTarget(&drawing.DetailedProfileCardRequest{ID: "1", Rank: &rank}, target, "jp")
	if detail.Rank == nil || *detail.Rank != rank || detail.Rank == &rank {
		t.Fatalf("detailed clone rank = %v, want a copy of %d", detail.Rank, rank)
	}

	card := &drawing.ProfileCardRequest{Profile: &drawing.BasicProfile{ID: "1"}, Rank: &rank}
	cloned := cloneProfileCardForTarget(card, target, "jp")
	if cloned.Rank == nil || *cloned.Rank != rank || cloned.Rank == &rank {
		t.Fatalf("profile card clone rank = %v, want a copy of %d", cloned.Rank, rank)
	}
	forced := forceMySekaiProfileBindingID(card, target, "jp")
	if forced.Rank == nil || *forced.Rank != rank || forced.Profile.ID != "12345" {
		t.Fatalf("forced MySekai profile = %+v", forced)
	}

	if cloneProfileCardForTarget(&drawing.ProfileCardRequest{}, target, "jp").Rank != nil ||
		cloneDetailedProfileForTarget(&drawing.DetailedProfileCardRequest{}, target, "jp").Rank != nil {
		t.Fatal("rank should stay nil when absent")
	}
}
