package testutil

import (
	"testing"

	"haruki-cloud/utils/usererror"
)

// RequireUserError fails the test unless err is a typed user error with code
// and catalog message id. Tests assert codes and IDs, never the zh-CN text.
// An empty code or id is not checked.
func RequireUserError(t testing.TB, err error, code usererror.Code, id string) *usererror.Error {
	t.Helper()
	typed, ok := usererror.As(err)
	if !ok {
		t.Fatalf("error %v (%T) is not a typed user error; want %s %s", err, err, code, id)
	}
	if code != "" && typed.Code != code {
		t.Fatalf("user error code = %s (%s), want %s", typed.Code, typed.Message.ID, code)
	}
	if id != "" && typed.Message.ID != id {
		t.Fatalf("user error message = %s (code %s), want %s", typed.Message.ID, typed.Code, id)
	}
	return typed
}

// MessageID is the catalog message ID of err's typed user error, or "".
func MessageID(err error) string {
	if typed, ok := usererror.As(err); ok {
		return typed.Message.ID
	}
	return ""
}

// ErrorDetail is err's user text followed by its code, message ID and
// logged cause (usererror.LogText), for asserting on either.
func ErrorDetail(err error) string {
	if err == nil {
		return ""
	}
	return err.Error() + "\n" + usererror.LogText(err)
}
