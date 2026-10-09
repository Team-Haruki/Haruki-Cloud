package usererror

import (
	"errors"
	"fmt"
)

// inputError marks an error caused by the user's command input (an
// unparsable query, an unknown name, an out-of-range index). Its message is
// the wrapped error's message, unchanged, so replies built from the text stay
// identical.
type inputError struct{ err error }

func (e *inputError) Error() string { return e.err.Error() }
func (e *inputError) Unwrap() error { return e.err }

// Input marks err as a user-input error. A nil err stays nil.
func Input(err error) error {
	if err == nil || IsInput(err) {
		return err
	}
	return &inputError{err: err}
}

// Inputf is fmt.Errorf marked as a user-input error.
func Inputf(format string, args ...any) error {
	return &inputError{err: fmt.Errorf(format, args...)}
}

// IsInput reports whether err, or any error it wraps, was marked with Input
// or Inputf, or is a typed Error whose code is a user-input code.
func IsInput(err error) bool {
	if _, ok := errors.AsType[*inputError](err); ok {
		return true
	}
	if typed, ok := As(err); ok {
		return typed.Code.IsInput()
	}
	return false
}
