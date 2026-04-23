// Package version provides the server version string, injectable via build-time
// ldflags or the HARUKI_VERSION environment variable at runtime.
//
// To set version at build time:
//
//	go build -ldflags "-X haruki-cloud/version.Version=v1.2.3" .
//
// To override at runtime (takes priority over the compiled value):
//
//	HARUKI_VERSION=v1.2.3 ./server
package version

import (
	"fmt"
	"os"
)

// Version is the compiled-in version string. Override with:
//
//	-ldflags "-X haruki-cloud/version.Version=v1.2.3"
var Version = "dev"

// Get returns the effective version: HARUKI_VERSION env var overrides the
// compiled-in value when set.
func Get() string {
	if v := os.Getenv("HARUKI_VERSION"); v != "" {
		return v
	}
	return Version
}

// UserAgent returns the standard "Haruki-Cloud/{version}" User-Agent string.
func UserAgent() string {
	return fmt.Sprintf("Haruki-Cloud/%s", Get())
}
