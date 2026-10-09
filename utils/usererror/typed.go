package usererror

import (
	"errors"

	"haruki-cloud/internal/i18n"
)

// Code classifies a typed user error. Callers and tests branch on the code,
// never on the message text.
type Code string

const (
	// CodeInput is a user-input mistake without a more specific code.
	CodeInput Code = "input"
	// CodeBadParam is a malformed parameter (i18n.BadParam).
	CodeBadParam Code = "bad_param"
	// CodeUsage is a command used with the wrong shape (i18n.WithUsage).
	CodeUsage Code = "usage"
	// CodeNotFound is a query that matches nothing (i18n.NotFound).
	CodeNotFound Code = "not_found"
	// CodeAmbiguous is a query that matches several things (i18n.Ambiguous).
	CodeAmbiguous Code = "ambiguous"
	// CodeOutOfRange is a number outside its range (i18n.OutOfRange).
	CodeOutOfRange Code = "out_of_range"
	// CodeForbidden is a request the user is not allowed to make (banned,
	// not an admin, self-ban).
	CodeForbidden Code = "forbidden"
	// CodeReadOnly is a write refused because the service is read-only.
	CodeReadOnly Code = "read_only"
	// CodeSetup is a request the user can make only after a setup step of
	// their own: binding a game account, uploading data in the Toolbox,
	// verifying an account. It is an expected outcome, not an input mistake.
	CodeSetup Code = "setup"
	// CodeUnavailable is a feature that cannot serve right now (i18n.Unavailable).
	CodeUnavailable Code = "unavailable"
	// CodeTimeout is a feature that did not answer in time (i18n.Timeout).
	CodeTimeout Code = "timeout"
	// CodeMisconfigured is a problem only the bot owner can fix (i18n.Misconfigured).
	CodeMisconfigured Code = "misconfigured"
	// CodeInternal is any other failure (i18n.RequestFailed).
	CodeInternal Code = "internal"
)

// IsInput reports whether the code describes a mistake in the user's input.
func (c Code) IsInput() bool {
	switch c {
	case CodeInput, CodeBadParam, CodeUsage, CodeNotFound, CodeAmbiguous, CodeOutOfRange:
		return true
	}
	return false
}

// Expected reports whether an error with this code is a normal outcome of a
// request (logged at a low level) rather than a service failure.
func (c Code) Expected() bool {
	return c.IsInput() || c == CodeForbidden || c == CodeReadOnly || c == CodeSetup
}

// Error is a typed user error: a Code for control flow and tests, a catalog
// Message for the user, and an optional Cause that only goes to logs.
//
// Error() returns the user text (DefaultLocale) and nothing else, so an
// Error can be shown to a user on any path without leaking its cause.
// Unwrap returns the cause, so errors.Is/As still see through it.
type Error struct {
	Code    Code
	Message i18n.Message
	Cause   error
}

// New builds a typed user error.
func New(code Code, message i18n.Message) *Error {
	return &Error{Code: code, Message: message}
}

// Wrap builds a typed user error that keeps cause for logs.
func Wrap(code Code, message i18n.Message, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func (e *Error) Error() string { return e.Message.String() }

// Unwrap returns the logged-only cause.
func (e *Error) Unwrap() error { return e.Cause }

// Text renders the user text in locale.
func (e *Error) Text(locale i18n.Locale) string { return e.Message.In(locale) }

// WithCause returns a copy of e that carries cause.
func (e *Error) WithCause(cause error) *Error {
	clone := *e
	clone.Cause = cause
	return &clone
}

// As returns the outermost typed user error in err's chain.
func As(err error) (*Error, bool) {
	return errors.AsType[*Error](err)
}

// CodeOf returns the code of the outermost typed user error in err's chain,
// or "" when there is none.
func CodeOf(err error) Code {
	if e, ok := As(err); ok {
		return e.Code
	}
	return ""
}

// IsExpected reports whether err is a normal outcome of a request: a typed
// error whose code is Expected (user input, setup, forbidden, read-only).
func IsExpected(err error) bool {
	if e, ok := As(err); ok {
		return e.Code.Expected()
	}
	return false
}

// LogText describes err for a log record. For a typed user error it names
// the code and message ID and appends the cause, which Error() hides.
func LogText(err error) string {
	if err == nil {
		return ""
	}
	e, ok := As(err)
	if !ok {
		return err.Error()
	}
	text := string(e.Code) + " " + e.Message.ID
	if e.Cause != nil {
		text += ": " + e.Cause.Error()
	}
	// A domain error that unwraps to the typed error (or a wrap around it)
	// carries its own detail in its text.
	if outer := err.Error(); error(e) != err && outer != e.Error() {
		text += " (" + outer + ")"
	}
	return text
}

// Invalid is a user-input error with a domain message.
func Invalid(message i18n.Message) *Error { return New(CodeInput, message) }

// Forbidden is a refused request with a domain message.
func Forbidden(message i18n.Message) *Error { return New(CodeForbidden, message) }

// Setup is a request that needs a setup step of the user's own first (bind,
// upload, verify); see CodeSetup.
func Setup(message i18n.Message) *Error { return New(CodeSetup, message) }

// Misuse is a command used with the wrong shape, given as one reason line.
// The reply layer adds the help pointer for the command the user actually
// typed, so the producer does not need to know the trigger.
func Misuse(reason i18n.Message) *Error { return New(CodeUsage, reason) }

// Unrecognized is a command whose arguments could not be understood at all.
// The reply layer replaces the generic reason with the route's own guidance
// (what the command expects) and adds the help pointer.
func Unrecognized() *Error { return New(CodeUsage, i18n.Unrecognized()) }

// BadParam is a malformed parameter; see i18n.BadParam.
func BadParam(param string, reason i18n.Message) *Error {
	return New(CodeBadParam, i18n.BadParam(param, reason))
}

// Usage is a command used with the wrong shape: one reason line plus the
// help pointer for trigger; see i18n.WithUsage.
func Usage(reason i18n.Message, trigger string) *Error {
	return New(CodeUsage, i18n.WithUsage(reason, trigger))
}

// NotFound is a query that matches nothing; see i18n.NotFound.
func NotFound(kind i18n.Message, query string) *Error {
	return New(CodeNotFound, i18n.NotFound(kind, query))
}

// Ambiguous is a query that matches several things; see i18n.Ambiguous.
func Ambiguous(kind i18n.Message, query string) *Error {
	return New(CodeAmbiguous, i18n.Ambiguous(kind, query))
}

// OutOfRange is a number outside minValue~maxValue; see i18n.OutOfRange.
func OutOfRange(param i18n.Message, minValue, maxValue int) *Error {
	return New(CodeOutOfRange, i18n.OutOfRange(param, minValue, maxValue))
}

// Unavailable is a feature that cannot serve right now; cause is logged only.
func Unavailable(feature i18n.Message, cause error) *Error {
	return Wrap(CodeUnavailable, i18n.Unavailable(feature), cause)
}

// Timeout is a feature that did not answer in time; cause is logged only.
func Timeout(feature i18n.Message, cause error) *Error {
	return Wrap(CodeTimeout, i18n.Timeout(feature), cause)
}

// Misconfigured is a problem only the bot owner can fix; cause is logged only.
func Misconfigured(cause error) *Error {
	return Wrap(CodeMisconfigured, i18n.Misconfigured(), cause)
}

// Internal is any other failure; cause is logged only.
func Internal(cause error) *Error {
	return Wrap(CodeInternal, i18n.RequestFailed(), cause)
}

// ReadOnly is a write refused because the service is read-only.
func ReadOnly() *Error {
	return New(CodeReadOnly, i18n.ReadOnly())
}
