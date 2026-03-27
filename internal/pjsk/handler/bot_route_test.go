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
	if !contains(cardDetail.Commands, "/查卡") {
		t.Fatalf("expected card/detail commands to include /查卡, got %v", cardDetail.Commands)
	}

	cardImage, ok := byPath["card/image"]
	if !ok {
		t.Fatal("expected card/image route to exist")
	}
	if !contains(cardImage.Commands, "/卡面") {
		t.Fatalf("expected card/image commands to include /卡面, got %v", cardImage.Commands)
	}

	cardBox, ok := byPath["card/box"]
	if !ok {
		t.Fatal("expected card/box route to exist")
	}
	if !contains(cardBox.Commands, "/卡面一览") {
		t.Fatalf("expected card/box commands to include /卡面一览, got %v", cardBox.Commands)
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

	musicAliasRoute, ok := byPath["alias/music"]
	if !ok {
		t.Fatal("expected alias/music route to exist")
	}
	if !contains(musicAliasRoute.Commands, "/歌曲别名") {
		t.Fatalf("expected alias/music commands to include /歌曲别名, got %v", musicAliasRoute.Commands)
	}

	characterAliasRoute, ok := byPath["alias/character"]
	if !ok {
		t.Fatal("expected alias/character route to exist")
	}
	if !contains(characterAliasRoute.Commands, "/角色别名") {
		t.Fatalf("expected alias/character commands to include /角色别名, got %v", characterAliasRoute.Commands)
	}

	mysekaiBlueprintRoute, ok := byPath["mysekai/blueprint"]
	if !ok {
		t.Fatal("expected mysekai/blueprint route to exist")
	}
	if !contains(mysekaiBlueprintRoute.Commands, "/msb") {
		t.Fatalf("expected mysekai/blueprint commands to include /msb, got %v", mysekaiBlueprintRoute.Commands)
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
