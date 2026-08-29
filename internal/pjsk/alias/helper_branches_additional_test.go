package alias

import (
	"context"
	"strings"
	"testing"

	sekaidb "haruki-cloud/database/sekai"
)

func TestAliasServiceReadOnlyAndNameLoaders(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)
	deps.service.SetReadOnly(true)
	if err := deps.service.requireWritable(); err == nil {
		t.Fatal("read-only service accepted a mutation")
	}
	deps.service.SetReadOnly(false)
	if err := deps.service.requireWritable(); err != nil {
		t.Fatalf("writable service rejected mutation: %v", err)
	}
	(*Service)(nil).SetReadOnly(true)
	if err := (*Service)(nil).requireWritable(); err == nil {
		t.Fatal("nil service was writable")
	}

	if _, err := deps.sekai.Music.Create().SetServerRegion("cn").SetGameID(10).SetTitle("国服曲名").Save(ctx); err != nil {
		t.Fatalf("create CN music: %v", err)
	}
	if _, err := deps.sekai.Music.Create().SetServerRegion("jp").SetGameID(10).SetTitle("日服曲名").Save(ctx); err != nil {
		t.Fatalf("create JP music: %v", err)
	}
	titles, err := deps.service.loadMusicTitles(ctx, []int{0, 10, 10, 99})
	if err != nil || titles[10] != "日服曲名" || !strings.Contains(titles[99], "99") {
		t.Fatalf("music titles = %#v, %v", titles, err)
	}
	if empty, err := deps.service.loadMusicTitles(ctx, nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty music titles = %#v, %v", empty, err)
	}

	if _, err := deps.sekai.Gamecharacter.Create().SetServerRegion("en").SetGameID(20).SetFirstNameEnglish("English").SetGivenNameEnglish("Name").Save(ctx); err != nil {
		t.Fatalf("create EN character: %v", err)
	}
	if _, err := deps.sekai.Gamecharacter.Create().SetServerRegion("jp").SetGameID(20).SetFirstName("日服").SetGivenName("角色").Save(ctx); err != nil {
		t.Fatalf("create JP character: %v", err)
	}
	names, err := deps.service.loadCharacterNames(ctx, []int{0, 20, 20, 98})
	if err != nil || names[20] != "日服角色" || !strings.Contains(names[98], "98") {
		t.Fatalf("character names = %#v, %v", names, err)
	}
	if empty, err := deps.service.loadCharacterNames(ctx, nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty character names = %#v, %v", empty, err)
	}
}

func TestAliasPureHelperBranches(t *testing.T) {
	for region, want := range map[string]int{"jp": 0, "cn": 1, "tw": 2, "kr": 3, "en": 4, "xx": 5} {
		if got := serverRegionRank(region); got != want {
			t.Errorf("serverRegionRank(%q) = %d", region, got)
		}
	}
	if got := preferredMusicTitle([]*sekaidb.Music{{ServerRegion: "cn", Title: "CN"}, {ServerRegion: "jp", Title: "JP"}, {Title: " "}}, 1); got != "JP" {
		t.Fatalf("preferred music title = %q", got)
	}
	if got := preferredMusicTitle(nil, 7); !strings.Contains(got, "7") {
		t.Fatalf("fallback music title = %q", got)
	}
	characterRows := []*sekaidb.Gamecharacter{
		{ServerRegion: "en", FirstNameEnglish: "English", GivenNameEnglish: "Name"},
		{ServerRegion: "jp", FirstName: "日本", GivenName: "角色"},
	}
	if got := preferredCharacterName(characterRows, 2); got != "日本角色" {
		t.Fatalf("preferred character name = %q", got)
	}
	if got := preferredCharacterName(nil, 8); !strings.Contains(got, "8") {
		t.Fatalf("fallback character name = %q", got)
	}
	if characterMatchesName(characterRows[0], "") || !characterMatchesName(characterRows[0], "englishname") || characterMatchesName(characterRows[0], "missing") {
		t.Fatal("character name matching failed")
	}

	values := []string{"Alpha"}
	appendUniqueString(&values, " ")
	appendUniqueString(&values, " alpha ")
	appendUniqueString(&values, "Beta")
	if len(values) != 2 {
		t.Fatalf("unique strings = %#v", values)
	}
	if _, err := normalizeSubmittedAliases(nil); err == nil {
		t.Fatal("empty submitted aliases were accepted")
	}
	if _, err := normalizeSubmittedAliases([]string{"Same", " same "}); err == nil {
		t.Fatal("duplicate submitted aliases were accepted")
	}
	if got, err := normalizeSubmittedAliases([]string{" A ", "B"}); err != nil || len(got) != 2 {
		t.Fatalf("submitted aliases = %#v, %v", got, err)
	}
	if _, err := normalizeReviewIDs(nil); err == nil {
		t.Fatal("empty review IDs were accepted")
	}
	if _, err := normalizeReviewIDs([]int64{0}); err == nil {
		t.Fatal("nonpositive review ID was accepted")
	}
	if got, err := normalizeReviewIDs([]int64{2, 2, 3}); err != nil || len(got) != 2 {
		t.Fatalf("review IDs = %#v, %v", got, err)
	}

	for aliasType, label := range map[string]string{PjskAliasTypeMusic: "歌曲", PjskAliasTypeCharacter: "角色", "x": "未知类型"} {
		if got := aliasTypeLabel(aliasType); got != label {
			t.Errorf("aliasTypeLabel(%q) = %q", aliasType, got)
		}
	}
	if aliasTypeIDLabel("x") != "目标ID" || aliasTypeNameLabel("x") != "名称" || entityTokenPrompt("x") != "ID、名称或已审核别名" {
		t.Fatal("unknown alias labels are incorrect")
	}
	for _, tc := range []struct{ platform, id, want string }{
		{"", "", "unknown"}, {"", "1", "1"}, {"qq", "", "qq"}, {"qq", "1", "qq:1"},
	} {
		if got := buildActorLabel(tc.platform, tc.id); got != tc.want {
			t.Errorf("buildActorLabel(%q, %q) = %q", tc.platform, tc.id, got)
		}
	}
	if shouldTryPartialMusicAlias("") || shouldTryPartialMusicAlias("a") || !shouldTryPartialMusicAlias("ab") {
		t.Fatal("partial music alias threshold failed")
	}
	aliases := []string{"beta", "Alpha", "alpha"}
	sortAliasTexts(aliases)
	if len(aliases) != 3 || aliases[0] != "Alpha" || aliases[1] != "alpha" {
		t.Fatalf("sorted aliases = %#v", aliases)
	}
}
