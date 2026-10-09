package deck

import (
	"errors"
	"fmt"
	"testing"

	sekaiDB "haruki-cloud/database/sekai"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
)

func TestRestoreFixedCardUnknownTypedIDIsUserInput(t *testing.T) {
	notFound := fmt.Errorf("query card 1378: %w", &sekaiDB.NotFoundError{})
	controller := NewController(&additionalBasicCardSource{region: renderregion.CN, err: notFound}, nil, nil, nil, nil, renderregion.CN)
	option := map[string]any{"fixed_cards": []int{1378}}

	err := controller.restoreFixedCards(renderregion.CN, &snapshot.RawUserData{}, &snapshot.RawUserData{}, option, false)
	if err == nil || !usererror.IsInput(err) {
		t.Fatalf("typed unknown card error = %v (input %v)", err, usererror.IsInput(err))
	}
	testutil.RequireUserError(t, err, usererror.CodeInput, "deck.fixed.card_not_in_region")

	// With "当前" the IDs come from the game's own deck: a missing card is a
	// master-data gap and must stay an internal error.
	err = controller.restoreFixedCards(renderregion.CN, &snapshot.RawUserData{}, &snapshot.RawUserData{}, option, true)
	if !errors.Is(err, notFound) || usererror.IsInput(err) {
		t.Fatalf("current-deck unknown card error = %v (input %v)", err, usererror.IsInput(err))
	}

	// Lookup failures other than not-found are not the user's input either.
	dbDown := errors.New("query card 1378: database unavailable")
	failing := NewController(&additionalBasicCardSource{region: renderregion.CN, err: dbDown}, nil, nil, nil, nil, renderregion.CN)
	err = failing.restoreFixedCards(renderregion.CN, &snapshot.RawUserData{}, &snapshot.RawUserData{}, option, false)
	if !errors.Is(err, dbDown) || usererror.IsInput(err) {
		t.Fatalf("database error = %v (input %v)", err, usererror.IsInput(err))
	}
}
