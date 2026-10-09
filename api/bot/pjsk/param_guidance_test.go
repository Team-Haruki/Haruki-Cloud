package pjsk

import (
	"strings"
	"testing"

	commandregistry "haruki-cloud/internal/handler"
	commandhandler "haruki-cloud/internal/pjsk/handler"
)

// TestRouteGuidanceKeysAreRegisteredRoutes keeps the guidance table in step
// with the router: a renamed or removed route must not leave guidance that
// can never be shown.
func TestRouteGuidanceKeysAreRegisteredRoutes(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()
	routes := map[string]bool{}
	for _, route := range commandregistry.ListBotRoutes() {
		routes[strings.Trim(route.Path, "/")] = true
	}
	for path := range routeGuidance {
		if !routes[path] {
			t.Errorf("guidance for %q, which is not a registered route", path)
		}
	}
}
