package handler_test

import (
	"testing"

	commandhandler "haruki-cloud/internal/pjsk/handler"
	sekaihandler "haruki-cloud/internal/pjsk/handler/sekai"
)

func TestListBotRoutes(t *testing.T) {
	sekaihandler.EnsureCommandHandlersRegistered(nil)

	routes := commandhandler.ListBotRoutes()
	if len(routes) == 0 {
		t.Fatal("expected bot routes to be registered")
	}

	byPath := make(map[string]commandhandler.BotRoute, len(routes))
	for _, route := range routes {
		if route.Path == "" {
			t.Fatal("found route with empty path")
		}
		if route.Module == "" {
			t.Fatalf("route %s has empty module", route.Path)
		}
		if len(route.Commands) == 0 {
			t.Fatalf("route %s has no commands", route.Path)
		}
		byPath[route.Path] = route
	}

	cardDetail, ok := byPath["card/detail"]
	if !ok {
		t.Fatal("expected card/detail route to exist")
	}
	if cardDetail.Module != commandhandler.BotModulePJSK {
		t.Fatalf("expected card/detail module=%s, got %s", commandhandler.BotModulePJSK, cardDetail.Module)
	}
	if !contains(cardDetail.Commands, "/卡面") {
		t.Fatalf("expected card/detail commands to include /卡面, got %v", cardDetail.Commands)
	}

	chartRoute, ok := byPath["music/chart"]
	if !ok {
		t.Fatal("expected music/chart route to exist")
	}
	if !contains(chartRoute.Commands, "/查谱面") {
		t.Fatalf("expected music/chart commands to include /查谱面, got %v", chartRoute.Commands)
	}

	scoreRoute, ok := byPath["score/music-meta"]
	if !ok {
		t.Fatal("expected score/music-meta route to exist")
	}
	if !contains(scoreRoute.Commands, "/曲目meta") {
		t.Fatalf("expected score/music-meta commands to include /曲目meta, got %v", scoreRoute.Commands)
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
