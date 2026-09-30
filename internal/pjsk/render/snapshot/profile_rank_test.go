package snapshot

import (
	"context"
	"fmt"
	"strings"
	"testing"

	json "haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func buildRankTestSnapshot(t *testing.T, gamedata string) *Service {
	t.Helper()
	snap, err := NewDefaultSnapshotFactory(nil, nil).Build(context.TODO(), BuildInput{
		Region:    renderregion.JP,
		Source:    "toolbox",
		SuiteJSON: fmt.Appendf(nil, `{"now":1710000000,"userGamedata":%s}`, gamedata),
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	return snap.(*Service)
}

func TestSnapshotProfilesCarrySuiteGameRank(t *testing.T) {
	service := buildRankTestSnapshot(t, `{"userId":123456789,"name":"RankUser","deck":1,"rank":321}`)

	detail := service.DetailedProfile(renderregion.JP)
	if detail.Rank == nil || *detail.Rank != 321 {
		t.Fatalf("detailed profile rank = %v", detail.Rank)
	}
	card := service.ProfileCard(renderregion.JP)
	if card.Rank == nil || *card.Rank != 321 {
		t.Fatalf("profile card rank = %v", card.Rank)
	}

	*detail.Rank = 1
	*card.Rank = 2
	if again := service.DetailedProfile(renderregion.JP); again.Rank == nil || *again.Rank != 321 {
		t.Fatalf("returned profiles alias the cached rank: %v", again.Rank)
	}
}

func TestSnapshotProfilesOmitMissingGameRank(t *testing.T) {
	for _, gamedata := range []string{
		`{"userId":123456789,"name":"RankUser","deck":1}`,
		`{"userId":123456789,"name":"RankUser","deck":1,"rank":0}`,
	} {
		service := buildRankTestSnapshot(t, gamedata)
		detail := service.DetailedProfile(renderregion.JP)
		card := service.ProfileCard(renderregion.JP)
		if detail.Rank != nil || card.Rank != nil {
			t.Fatalf("%s: rank = %v / %v, want nil", gamedata, detail.Rank, card.Rank)
		}
		for _, payload := range []any{detail, card} {
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), `"rank"`) {
				t.Fatalf("%s: rank should be omitted: %s", gamedata, encoded)
			}
		}
	}
}
