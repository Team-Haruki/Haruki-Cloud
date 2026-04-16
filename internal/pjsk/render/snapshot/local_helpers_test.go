package snapshot

import (
	"strings"
	"testing"
)

func TestEncodeRawUserDataOmitsNilCollections(t *testing.T) {
	raw := &RawUserData{
		Now:          1,
		UserGamedata: RawUserGamedata{UserID: 123, Name: "deck-user"},
		UserProfile:  RawUserProfile{ProfileImageType: "default"},
		UserCards: []RawUserCard{
			{
				CardID:                1001,
				Level:                 60,
				MasterRank:            5,
				SpecialTrainingStatus: "done",
				DefaultImage:          "special_training",
			},
		},
	}

	data, err := EncodeRawUserData(raw)
	if err != nil {
		t.Fatalf("EncodeRawUserData() error = %v", err)
	}

	text := string(data)
	for _, unexpected := range []string{
		`"userDecks":null`,
		`"userBonds":null`,
		`"userMusicResults":null`,
		`"compactUserMusicResults":null`,
		`"userMusics":null`,
		`"userChallengeLiveSoloDecks":null`,
		`"episodes":null`,
	} {
		if strings.Contains(text, unexpected) {
			t.Fatalf("encoded userdata unexpectedly contains %s: %s", unexpected, text)
		}
	}
}
