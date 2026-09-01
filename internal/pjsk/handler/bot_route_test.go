package handler_test

import (
	"testing"

	commandhandler "haruki-cloud/internal/handler"
	pjskhandler "haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/testutil"
)

func TestListBotRoutes(t *testing.T) {
	pjskhandler.EnsureCommandHandlersRegistered()

	routes := commandhandler.ListBotRoutes()
	testutil.RequireArgs(t, !(len(routes) == 0), "expected bot routes to be registered")

	byPath := make(map[string]commandhandler.BotRoute, len(routes))
	for _, route := range routes {
		testutil.RequireArgs(t, !(route.Path == ""), "found route with empty path")

		testutil.Require(t, !(route.Module == ""), "route %s has empty module", route.Path)
		testutil.Require(t, !(len(route.Commands) == 0), "route %s has no commands", route.Path)

		byPath[route.Path] = route
	}

	cardDetail, ok := byPath["card/detail"]
	testutil.RequireArgs(t, ok, "expected card/detail route to exist")

	testutil.Require(t, !(cardDetail.Module != pjskhandler.BotModulePJSK), "expected card/detail module=%s, got %s", pjskhandler.BotModulePJSK, cardDetail.Module)
	testutil.Require(t, contains(cardDetail.Commands, "/查卡"), "expected card/detail commands to include /查卡, got %v", cardDetail.Commands)

	cardImage, ok := byPath["card/image"]
	testutil.RequireArgs(t, ok, "expected card/image route to exist")

	testutil.Require(t, contains(cardImage.Commands, "/卡面"), "expected card/image commands to include /卡面, got %v", cardImage.Commands)

	cardBox, ok := byPath["card/box"]
	testutil.RequireArgs(t, ok, "expected card/box route to exist")

	testutil.Require(t, contains(cardBox.Commands, "/卡面一览"), "expected card/box commands to include /卡面一览, got %v", cardBox.Commands)

	chartRoute, ok := byPath["music/chart"]
	testutil.RequireArgs(t, ok, "expected music/chart route to exist")

	testutil.Require(t, contains(chartRoute.Commands, "/查谱面"), "expected music/chart commands to include /查谱面, got %v", chartRoute.Commands)

	scoreRoute, ok := byPath["score/music-meta"]
	testutil.RequireArgs(t, ok, "expected score/music-meta route to exist")

	testutil.Require(t, contains(scoreRoute.Commands, "/曲目meta"), "expected score/music-meta commands to include /曲目meta, got %v", scoreRoute.Commands)

	b30Route, ok := byPath["music/b30"]
	testutil.RequireArgs(t, ok, "expected music/b30 route to exist")

	for _, command := range []string{"/b30", "/pjskb30", "/b39", "/pjskb39"} {
		testutil.Require(t, contains(b30Route.Commands, command), "expected music/b30 commands to include %s, got %v", command, b30Route.Commands)

	}

	musicAliasRoute, ok := byPath["alias/music"]
	testutil.RequireArgs(t, ok, "expected alias/music route to exist")

	testutil.Require(t, contains(musicAliasRoute.Commands, "/歌曲别名"), "expected alias/music commands to include /歌曲别名, got %v", musicAliasRoute.Commands)

	characterAliasRoute, ok := byPath["alias/character"]
	testutil.RequireArgs(t, ok, "expected alias/character route to exist")

	testutil.Require(t, contains(characterAliasRoute.Commands, "/角色别名"), "expected alias/character commands to include /角色别名, got %v", characterAliasRoute.Commands)

	customProfileRoute, ok := byPath["profile/custom-profile-card"]
	testutil.RequireArgs(t, ok, "expected profile/custom-profile-card route to exist")

	testutil.Require(t, contains(customProfileRoute.Commands, "/cp"), "expected profile/custom-profile-card commands to include /cp, got %v", customProfileRoute.Commands)

	profileUIDRoute, ok := byPath["profile/uid"]
	testutil.RequireArgs(t, ok, "expected profile/uid route to exist")

	for _, command := range []string{"/查uid", "/uid"} {
		testutil.Require(t, contains(profileUIDRoute.Commands, command), "expected profile/uid commands to include %s, got %v", command, profileUIDRoute.Commands)

	}

	mysekaiTalkListRoute, ok := byPath["mysekai/talk-list"]
	testutil.RequireArgs(t, ok, "expected mysekai/talk-list route to exist")

	testutil.Require(t, contains(mysekaiTalkListRoute.Commands, "/msb"), "expected mysekai/talk-list commands to include /msb, got %v", mysekaiTalkListRoute.Commands)
	testutil.Require(t, contains(mysekaiTalkListRoute.Commands, "/烤森对话列表"), "expected mysekai/talk-list commands to include /烤森对话列表, got %v", mysekaiTalkListRoute.Commands)
	{

		_, ok := byPath["mysekai/blueprint"]
		testutil.RequireArgs(t, !(ok), "did not expect mysekai/blueprint to remain an active bot route")
	}

	mysekaiOverviewRoute, ok := byPath["mysekai/overview"]
	testutil.RequireArgs(t, ok, "expected mysekai/overview route to exist")

	testutil.Require(t, contains(mysekaiOverviewRoute.Commands, "/msam"), "expected mysekai/overview commands to include /msam, got %v", mysekaiOverviewRoute.Commands)
	{

		_, ok := byPath["mysekai/preview"]
		testutil.RequireArgs(t, !(ok), "did not expect mysekai/preview to remain an active bot route")
	}

	mysekaiHousingSKRoute, ok := byPath["mysekai/housing-sk"]
	testutil.RequireArgs(t, ok, "expected mysekai/housing-sk route to exist")

	for _, command := range []string{"/bjsk", "/cnbjsk"} {
		testutil.Require(t, contains(mysekaiHousingSKRoute.Commands, command), "expected mysekai/housing-sk commands to include %s, got %v", command, mysekaiHousingSKRoute.Commands)

	}

	costumeListRoute, ok := byPath["costume/list"]
	testutil.RequireArgs(t, ok, "expected costume/list route to exist")

	for _, command := range []string{"/服装列表", "/饰品列表", "/发型列表"} {
		testutil.Require(t, contains(costumeListRoute.Commands, command), "expected costume/list commands to include %s, got %v", command, costumeListRoute.Commands)

	}
	costumeDetailRoute, ok := byPath["costume/detail"]
	testutil.RequireArgs(t, ok, "expected costume/detail route to exist")

	for _, command := range []string{"/查服装", "/查饰品", "/查头饰"} {
		testutil.Require(t, contains(costumeDetailRoute.Commands, command), "expected costume/detail commands to include %s, got %v", command, costumeDetailRoute.Commands)

	}
	{
		_, ok := byPath["costume/body"]
		testutil.RequireArgs(t, !(ok), "did not expect costume/body route")
	}
	{

		_, ok := byPath["costume/accessory"]
		testutil.RequireArgs(t, !(ok), "did not expect costume/accessory route")
	}
	{

		_, ok := byPath["costume/hair"]
		testutil.RequireArgs(t, !(ok), "did not expect costume/hair route")
	}

	costumeComboRoute, ok := byPath["costume/combo"]
	testutil.RequireArgs(t, ok, "expected costume/combo route to exist")

	for _, command := range []string{"/组合", "/试穿", "/3d试穿"} {
		testutil.Require(t, contains(costumeComboRoute.Commands, command), "expected costume/combo commands to include %s, got %v", command, costumeComboRoute.Commands)

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
