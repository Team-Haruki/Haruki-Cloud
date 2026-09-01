package testutil

import "testing"

// Require fails the current test immediately when condition is false.
func Require(t testing.TB, condition bool, format string, args ...any) {
	t.Helper()
	if !condition {
		t.Fatalf(format, args...)
	}
}

// Check records a test failure when condition is false and continues execution.
func Check(t testing.TB, condition bool, format string, args ...any) {
	t.Helper()
	if !condition {
		t.Errorf(format, args...)
	}
}

// RequireArgs is the non-formatting variant of Require.
func RequireArgs(t testing.TB, condition bool, args ...any) {
	t.Helper()
	if !condition {
		t.Fatal(args...)
	}
}

// CheckArgs is the non-formatting variant of Check.
func CheckArgs(t testing.TB, condition bool, args ...any) {
	t.Helper()
	if !condition {
		t.Error(args...)
	}
}
