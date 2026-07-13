package handler_test

import (
	"testing"

	commandhandler "haruki-cloud/internal/handler"
	pjskhandler "haruki-cloud/internal/pjsk/handler"
)

func TestListBotRoutes(t *testing.T) {
	pjskhandler.EnsureCommandHandlersRegistered()

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
	if cardDetail.Module != pjskhandler.BotModulePJSK {
		t.Fatalf("expected card/detail module=%s, got %s", pjskhandler.BotModulePJSK, cardDetail.Module)
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

	b30Route, ok := byPath["music/b30"]
	if !ok {
		t.Fatal("expected music/b30 route to exist")
	}
	for _, command := range []string{"/b30", "/pjskb30", "/b39", "/pjskb39"} {
		if !contains(b30Route.Commands, command) {
			t.Fatalf("expected music/b30 commands to include %s, got %v", command, b30Route.Commands)
		}
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

	customProfileRoute, ok := byPath["profile/custom-profile-card"]
	if !ok {
		t.Fatal("expected profile/custom-profile-card route to exist")
	}
	if !contains(customProfileRoute.Commands, "/cp") {
		t.Fatalf("expected profile/custom-profile-card commands to include /cp, got %v", customProfileRoute.Commands)
	}

	profileUIDRoute, ok := byPath["profile/uid"]
	if !ok {
		t.Fatal("expected profile/uid route to exist")
	}
	for _, command := range []string{"/查uid", "/uid"} {
		if !contains(profileUIDRoute.Commands, command) {
			t.Fatalf("expected profile/uid commands to include %s, got %v", command, profileUIDRoute.Commands)
		}
	}

	mysekaiTalkListRoute, ok := byPath["mysekai/talk-list"]
	if !ok {
		t.Fatal("expected mysekai/talk-list route to exist")
	}
	if !contains(mysekaiTalkListRoute.Commands, "/msb") {
		t.Fatalf("expected mysekai/talk-list commands to include /msb, got %v", mysekaiTalkListRoute.Commands)
	}
	if !contains(mysekaiTalkListRoute.Commands, "/烤森对话列表") {
		t.Fatalf("expected mysekai/talk-list commands to include /烤森对话列表, got %v", mysekaiTalkListRoute.Commands)
	}
	if _, ok := byPath["mysekai/blueprint"]; ok {
		t.Fatal("did not expect mysekai/blueprint to remain an active bot route")
	}

	mysekaiOverviewRoute, ok := byPath["mysekai/overview"]
	if !ok {
		t.Fatal("expected mysekai/overview route to exist")
	}
	if !contains(mysekaiOverviewRoute.Commands, "/msam") {
		t.Fatalf("expected mysekai/overview commands to include /msam, got %v", mysekaiOverviewRoute.Commands)
	}

	if _, ok := byPath["mysekai/preview"]; ok {
		t.Fatal("did not expect mysekai/preview to remain an active bot route")
	}

	mysekaiHousingSKRoute, ok := byPath["mysekai/housing-sk"]
	if !ok {
		t.Fatal("expected mysekai/housing-sk route to exist")
	}
	for _, command := range []string{"/bjsk", "/cnbjsk"} {
		if !contains(mysekaiHousingSKRoute.Commands, command) {
			t.Fatalf("expected mysekai/housing-sk commands to include %s, got %v", command, mysekaiHousingSKRoute.Commands)
		}
	}

	costumeListRoute, ok := byPath["costume/list"]
	if !ok {
		t.Fatal("expected costume/list route to exist")
	}
	for _, command := range []string{"/服装列表", "/饰品列表", "/发型列表"} {
		if !contains(costumeListRoute.Commands, command) {
			t.Fatalf("expected costume/list commands to include %s, got %v", command, costumeListRoute.Commands)
		}
	}
	costumeDetailRoute, ok := byPath["costume/detail"]
	if !ok {
		t.Fatal("expected costume/detail route to exist")
	}
	for _, command := range []string{"/查服装", "/查饰品", "/查头饰"} {
		if !contains(costumeDetailRoute.Commands, command) {
			t.Fatalf("expected costume/detail commands to include %s, got %v", command, costumeDetailRoute.Commands)
		}
	}
	if _, ok := byPath["costume/body"]; ok {
		t.Fatal("did not expect costume/body route")
	}
	if _, ok := byPath["costume/accessory"]; ok {
		t.Fatal("did not expect costume/accessory route")
	}
	if _, ok := byPath["costume/hair"]; ok {
		t.Fatal("did not expect costume/hair route")
	}

	costumeComboRoute, ok := byPath["costume/combo"]
	if !ok {
		t.Fatal("expected costume/combo route to exist")
	}
	for _, command := range []string{"/组合", "/试穿", "/3d试穿"} {
		if !contains(costumeComboRoute.Commands, command) {
			t.Fatalf("expected costume/combo commands to include %s, got %v", command, costumeComboRoute.Commands)
		}
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
