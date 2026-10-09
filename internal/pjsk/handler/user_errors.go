package handler

import (
	"strings"

	"haruki-cloud/internal/i18n"
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
	return i18n.M("common.event_label", i18n.Data{"Region": i18n.RegionLabel(strings.ToLower(strings.TrimSpace(region))), "ID": eventID})
}
