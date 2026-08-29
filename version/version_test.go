package version

import "testing"

func TestGetAndUserAgent(t *testing.T) {
	previous := Version
	Version = "compiled"
	t.Cleanup(func() { Version = previous })
	t.Setenv("HARUKI_VERSION", "")
	if got := Get(); got != "compiled" {
		t.Fatalf("compiled version = %q", got)
	}
	if got := UserAgent(); got != "Haruki-Cloud/compiled" {
		t.Fatalf("compiled user agent = %q", got)
	}
	t.Setenv("HARUKI_VERSION", "runtime")
	if got := Get(); got != "runtime" {
		t.Fatalf("runtime version = %q", got)
	}
	if got := UserAgent(); got != "Haruki-Cloud/runtime" {
		t.Fatalf("runtime user agent = %q", got)
	}
}
