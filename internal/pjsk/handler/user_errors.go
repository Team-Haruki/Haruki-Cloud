package handler

import (
	"context"
	"strings"

	"haruki-cloud/internal/i18n"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/usererror"
)

// isUserFacingError reports whether err already carries text meant for the
// user: a typed usererror.Error (catalog message). Error normalizers pass
// such errors through unchanged.
func isUserFacingError(err error) bool {
	_, ok := usererror.As(err)
	return ok
}

// unrecognizedUnlessTyped keeps a typed parse error (it says what is wrong)
// and turns any other one into the generic "unrecognized arguments" error,
// which the reply layer completes with the route's guidance.
func unrecognizedUnlessTyped(err error) error {
	if err == nil || isUserFacingError(err) {
		return err
	}
	return usererror.Unrecognized().WithCause(err)
}

// eventLabel names an event of region in error replies, e.g. "日服(JP)活动 123".
func eventLabel(region string, eventID int) i18n.Message {
	return i18n.M("common.event_label", i18n.Data{"Region": i18n.RegionLabel(strings.ToLower(strings.TrimSpace(region))), "UserID": i18n.UserNumber(eventID)})
}

// characterLabel names a game character in replies: its name from game
// data, or the placeholder name when game data has none. Use it where the
// user's own words for the character are no longer known.
func characterLabel(ctx context.Context, app *renderapp.App, characterID int) i18n.Message {
	return arrestChallengeCharacterLabel(characterID, resolveArrestChallengeCharacterName(ctx, app, characterID))
}
