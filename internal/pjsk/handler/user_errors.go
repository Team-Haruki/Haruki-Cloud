package handler

import (
	"errors"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/utils/usererror"
)

// isUserFacingError reports whether err already carries text meant for the
// user: a typed usererror.Error (catalog message) or a legacy ReplayError.
// Error normalizers pass such errors through unchanged.
func isUserFacingError(err error) bool {
	if _, ok := usererror.As(err); ok {
		return true
	}
	_, ok := errors.AsType[onebot11.ReplayError](err)
	return ok
}
