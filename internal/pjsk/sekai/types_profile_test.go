package sekai

import (
	"testing"

	json "haruki-cloud/internal/jsonutil"
)

func TestProfileCardDataDecodesV67ImageBuckets(t *testing.T) {
	var card UserCustomProfileCard
	err := json.Unmarshal([]byte(`{
		"customProfileCard": {
			"characterIcons": [{"id": 21}],
			"materials": [{"id": 1}],
			"userInterfaceIcons": [{"id": 42}]
		}
	}`), &card)
	if err != nil {
		t.Fatalf("unmarshal custom profile card: %v", err)
	}
	if got := card.CustomProfileCard.CharacterIcons; len(got) != 1 || got[0].ID != 21 {
		t.Fatalf("character icons = %+v", got)
	}
	if got := card.CustomProfileCard.Materials; len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("materials = %+v", got)
	}
	if got := card.CustomProfileCard.UserInterfaceIcons; len(got) != 1 || got[0].ID != 42 {
		t.Fatalf("user interface icons = %+v", got)
	}
}
