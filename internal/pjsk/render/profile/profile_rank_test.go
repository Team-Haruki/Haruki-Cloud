package profile

import (
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/sekai"
)

func newRankTestController() *Controller {
	source := &testProfileSource{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			1001: {ID: 1001, CharacterID: 1, AssetBundleName: "res001_no001"},
		},
		honors:      map[int]*masterdata.Honor{},
		honorGroups: map[int]*masterdata.HonorGroup{},
	}
	return NewController(source, nil, assets.NewAssetHelper("", nil), nil)
}

func rankTestResponse() *sekai.GetAnotherProfileResponse {
	return &sekai.GetAnotherProfileResponse{
		User:     sekai.AnotherUser{UserID: 12345, Name: "API User", Rank: 100},
		UserDeck: sekai.UserDeck{DeckID: 1, Leader: 1001, Member1: 1001},
		UserCards: []sekai.AnotherUserCard{
			{CardID: 1001, Level: 60, DefaultImage: "original"},
		},
	}
}

func TestPublicProfileCardsTakeRankFromSuiteSnapshot(t *testing.T) {
	controller := newRankTestController()
	rank := 321
	snap := &profileSnapshotStub{
		detail: &drawing.DetailedProfileCardRequest{ID: "12345", Source: "suite_dump", UpdateTime: 1, Rank: &rank},
	}
	query := Query{Region: "jp", Visible: true}

	detail, err := controller.BuildDetailedProfileCardFromAPIWithSnapshot(query, rankTestResponse(), snap)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Rank == nil || *detail.Rank != rank || detail.Rank == &rank {
		t.Fatalf("detailed profile rank = %v, want a copy of %d", detail.Rank, rank)
	}
	card, err := controller.BuildProfileCardFromAPIWithSnapshot(query, rankTestResponse(), snap)
	if err != nil {
		t.Fatal(err)
	}
	if card.Rank == nil || *card.Rank != rank {
		t.Fatalf("profile card rank = %v, want %d", card.Rank, rank)
	}
}

func TestPublicOnlyProfileCardsOmitRank(t *testing.T) {
	controller := newRankTestController()
	query := Query{Region: "jp", Visible: true}

	detail, err := controller.BuildDetailedProfileCardFromAPIWithSnapshot(query, rankTestResponse(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Rank != nil {
		t.Fatalf("public-only detailed profile rank = %v, want nil", *detail.Rank)
	}
	card, err := controller.BuildProfileCardFromAPI(query, rankTestResponse(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if card.Rank != nil {
		t.Fatalf("public-only profile card rank = %v, want nil", *card.Rank)
	}
	snap := &profileSnapshotStub{detail: &drawing.DetailedProfileCardRequest{ID: "12345", Source: "suite_dump"}}
	card, err = controller.BuildProfileCardFromAPIWithSnapshot(query, rankTestResponse(), snap)
	if err != nil {
		t.Fatal(err)
	}
	if card.Rank != nil {
		t.Fatalf("snapshot without rank produced %v, want nil", *card.Rank)
	}
}
