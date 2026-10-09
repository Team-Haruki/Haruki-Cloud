package handler

import (
	"context"
	"testing"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
)

// A WL chapter miss names the character, never echoes its internal ID as if
// the user had typed it.
func TestDeckWorldBloomChapterMissNamesTheCharacter(t *testing.T) {
	r := &deckEventSelectionResolver{
		ctx:     context.Background(),
		query:   &deck.AutoQuery{WorldBloomCharacterID: drawing.IntPtr(21)},
		region:  renderregion.JP,
		eventID: 120,
	}
	typed := testutil.RequireUserError(t, r.resolveCharacterID(), usererror.CodeInput, "sk.wl.no_character_chapter")
	character, ok := typed.Message.Data["Character"].(i18n.Message)
	if !ok || character.ID != "common.fallback.character" {
		t.Fatalf("Character = %#v, want the character label message", typed.Message.Data["Character"])
	}
}
