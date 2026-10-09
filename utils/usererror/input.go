package usererror

// IsInput reports whether err, or any error it wraps, is a typed Error whose
// code is a user-input code (a mistake in the command's arguments).
func IsInput(err error) bool {
	if typed, ok := As(err); ok {
		return typed.Code.IsInput()
	}
	return false
}
