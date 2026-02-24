package query

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	chunithmMainDB "haruki-cloud/database/chunithm/maindb"
	chunithmMusicDB "haruki-cloud/database/chunithm/music"
	pjskDB "haruki-cloud/database/pjsk"
	usersDB "haruki-cloud/database/users"

	_ "github.com/mattn/go-sqlite3"
)

type queryTestClients struct {
	main  *chunithmMainDB.Client
	music *chunithmMusicDB.Client
	pjsk  *pjskDB.Client
	users *usersDB.Client
}

func TestClient_ConfigValidation(t *testing.T) {
	ctx := context.Background()
	client := NewClient(nil, nil, nil, nil)

	if _, err := client.GetChunithmMusicIDByAlias(ctx, "alias"); !errors.Is(err, ErrChunithmNotConfigured) {
		t.Fatalf("expected ErrChunithmNotConfigured, got %v", err)
	}
	if _, err := client.GetPJSKGlobalAliasToID(ctx, "music", "alias"); !errors.Is(err, ErrPJSKNotConfigured) {
		t.Fatalf("expected ErrPJSKNotConfigured, got %v", err)
	}
	if _, err := client.GetUserByID(ctx, 1); !errors.Is(err, ErrUsersNotConfigured) {
		t.Fatalf("expected ErrUsersNotConfigured, got %v", err)
	}
}

func TestClient_ChunithmQueries(t *testing.T) {
	ctx := context.Background()
	qc, raw := setupQueryClient(t)
	defer closeQueryClients(raw)

	respByAlias, err := qc.GetChunithmMusicIDByAlias(ctx, "test-song")
	if err != nil {
		t.Fatalf("GetChunithmMusicIDByAlias: %v", err)
	}
	if len(respByAlias.MatchIDs) != 1 || respByAlias.MatchIDs[0] != 1001 {
		t.Fatalf("unexpected alias->id result: %+v", respByAlias)
	}

	aliasesByID, err := qc.GetChunithmAliasesByMusicID(ctx, 1001)
	if err != nil {
		t.Fatalf("GetChunithmAliasesByMusicID: %v", err)
	}
	if len(aliasesByID.Aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %+v", aliasesByID)
	}

	allMusic, err := qc.GetAllChunithmMusic(ctx)
	if err != nil {
		t.Fatalf("GetAllChunithmMusic: %v", err)
	}
	if len(allMusic) != 1 || allMusic[0].MusicID != 1001 {
		t.Fatalf("expected one released music 1001, got %+v", allMusic)
	}

	basic, err := qc.GetChunithmMusicBasicInfo(ctx, 1001)
	if err != nil {
		t.Fatalf("GetChunithmMusicBasicInfo: %v", err)
	}
	if basic.Title != "Test Song" {
		t.Fatalf("unexpected basic info: %+v", basic)
	}

	diffExact, err := qc.GetChunithmMusicDifficultyInfo(ctx, 1001, "v1")
	if err != nil {
		t.Fatalf("GetChunithmMusicDifficultyInfo(v1): %v", err)
	}
	if diffExact.Version != "v1" {
		t.Fatalf("expected version v1, got %+v", diffExact)
	}

	diffFallback, err := qc.GetChunithmMusicDifficultyInfo(ctx, 1001, "not-exists")
	if err != nil {
		t.Fatalf("GetChunithmMusicDifficultyInfo(fallback): %v", err)
	}
	if diffFallback.Version != "v2" {
		t.Fatalf("expected fallback to latest version v2, got %+v", diffFallback)
	}

	chartData, err := qc.GetChunithmChartData(ctx, 1001)
	if err != nil {
		t.Fatalf("GetChunithmChartData: %v", err)
	}
	if len(chartData) != 1 || chartData[0].Difficulty != 3 {
		t.Fatalf("unexpected chart data: %+v", chartData)
	}

	batch, err := qc.QueryChunithmMusicDataBatch(ctx, []int{1001, 9999}, "v1")
	if err != nil {
		t.Fatalf("QueryChunithmMusicDataBatch: %v", err)
	}
	if batch[1001].Info.Title != "Test Song" {
		t.Fatalf("expected known music title, got %+v", batch[1001])
	}
	if batch[9999].Info.Title != "Unknown" {
		t.Fatalf("expected unknown placeholder for missing music, got %+v", batch[9999])
	}
	for _, d := range batch[9999].Difficulty {
		if d != nil {
			t.Fatalf("expected nil difficulties for unknown music, got %+v", batch[9999].Difficulty)
		}
	}

	defaultServer, err := qc.GetChunithmDefaultServer(ctx, 500)
	if err != nil {
		t.Fatalf("GetChunithmDefaultServer: %v", err)
	}
	if defaultServer.Server != "jp" {
		t.Fatalf("unexpected default server: %+v", defaultServer)
	}

	binding, err := qc.GetChunithmBinding(ctx, 500, "jp")
	if err != nil {
		t.Fatalf("GetChunithmBinding: %v", err)
	}
	if binding.AimeID == nil || *binding.AimeID != "AIME-1001" {
		t.Fatalf("unexpected chunithm binding: %+v", binding)
	}
}

func TestClient_PJSKQueries(t *testing.T) {
	ctx := context.Background()
	qc, raw := setupQueryClient(t)
	defer closeQueryClients(raw)

	globalAliasToID, err := qc.GetPJSKGlobalAliasToID(ctx, "music", "sekai-song")
	if err != nil {
		t.Fatalf("GetPJSKGlobalAliasToID: %v", err)
	}
	if len(globalAliasToID.MatchIDs) != 1 || globalAliasToID.MatchIDs[0] != 2001 {
		t.Fatalf("unexpected global alias->id: %+v", globalAliasToID)
	}

	globalAliases, err := qc.GetPJSKGlobalAliasesByID(ctx, "music", 2001)
	if err != nil {
		t.Fatalf("GetPJSKGlobalAliasesByID: %v", err)
	}
	if len(globalAliases.Aliases) != 2 {
		t.Fatalf("expected 2 global aliases, got %+v", globalAliases)
	}

	groupAliasToID, err := qc.GetPJSKGroupAliasToID(ctx, "qq", "g1", "character", "miku")
	if err != nil {
		t.Fatalf("GetPJSKGroupAliasToID: %v", err)
	}
	if len(groupAliasToID.MatchIDs) != 1 || groupAliasToID.MatchIDs[0] != 3001 {
		t.Fatalf("unexpected group alias->id: %+v", groupAliasToID)
	}

	groupAliases, err := qc.GetPJSKGroupAliasesByID(ctx, "qq", "g1", "character", 3001)
	if err != nil {
		t.Fatalf("GetPJSKGroupAliasesByID: %v", err)
	}
	if len(groupAliases.Aliases) != 1 || groupAliases.Aliases[0] != "miku" {
		t.Fatalf("unexpected group aliases: %+v", groupAliases)
	}

	bindingsAll, err := qc.GetPJSKBindings(ctx, 500, "")
	if err != nil {
		t.Fatalf("GetPJSKBindings(all): %v", err)
	}
	if len(bindingsAll.Bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %+v", bindingsAll)
	}

	bindingsJP, err := qc.GetPJSKBindings(ctx, 500, "jp")
	if err != nil {
		t.Fatalf("GetPJSKBindings(jp): %v", err)
	}
	if len(bindingsJP.Bindings) != 1 || bindingsJP.Bindings[0].Server != "jp" {
		t.Fatalf("unexpected jp bindings: %+v", bindingsJP)
	}

	defaultBinding, err := qc.GetPJSKDefaultBinding(ctx, 500, "")
	if err != nil {
		t.Fatalf("GetPJSKDefaultBinding: %v", err)
	}
	if defaultBinding.Binding == nil || defaultBinding.Binding.UserID != "pjsk-jp-user" {
		t.Fatalf("unexpected default binding: %+v", defaultBinding)
	}

	preferences, err := qc.GetPJSKPreferences(ctx, 500)
	if err != nil {
		t.Fatalf("GetPJSKPreferences: %v", err)
	}
	if len(preferences.Options) != 2 {
		t.Fatalf("expected 2 preferences, got %+v", preferences)
	}

	preference, err := qc.GetPJSKPreference(ctx, 500, "theme")
	if err != nil {
		t.Fatalf("GetPJSKPreference: %v", err)
	}
	if preference.Option == nil || preference.Option.Value != "light" {
		t.Fatalf("unexpected preference response: %+v", preference)
	}
}

func TestClient_UserQueries(t *testing.T) {
	ctx := context.Background()
	qc, raw := setupQueryClient(t)
	defer closeQueryClients(raw)

	byPlatform, err := qc.GetUserByPlatform(ctx, "qq", "10001")
	if err != nil {
		t.Fatalf("GetUserByPlatform: %v", err)
	}
	if byPlatform.ID != 500 {
		t.Fatalf("unexpected user by platform: %+v", byPlatform)
	}

	byID, err := qc.GetUserByID(ctx, 500)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if byID.Platform != "qq" || byID.UserID != "10001" {
		t.Fatalf("unexpected user by id: %+v", byID)
	}
}

func setupQueryClient(t *testing.T) (*Client, *queryTestClients) {
	t.Helper()
	ctx := context.Background()
	raw := &queryTestClients{}

	raw.main = openMainClient(t)
	raw.music = openMusicClient(t)
	raw.pjsk = openPJSKClient(t)
	raw.users = openUsersClient(t)

	if err := raw.main.Schema.Create(ctx); err != nil {
		t.Fatalf("create chunithm main schema: %v", err)
	}
	if err := raw.music.Schema.Create(ctx); err != nil {
		t.Fatalf("create chunithm music schema: %v", err)
	}
	if err := raw.pjsk.Schema.Create(ctx); err != nil {
		t.Fatalf("create pjsk schema: %v", err)
	}
	if err := raw.users.Schema.Create(ctx); err != nil {
		t.Fatalf("create users schema: %v", err)
	}

	seedQueryData(t, raw)

	return NewClient(raw.main, raw.music, raw.pjsk, raw.users), raw
}

func seedQueryData(t *testing.T, raw *queryTestClients) {
	t.Helper()
	ctx := context.Background()

	releasedAt := time.Now().Add(-time.Hour)
	futureAt := time.Now().Add(24 * time.Hour)

	raw.music.ChunithmMusic.Create().
		SetMusicID(1001).
		SetTitle("Test Song").
		SetArtist("Haruki Artist").
		SetCategory("POPS").
		SetVersion("v2").
		SetReleaseDate(releasedAt).
		SaveX(ctx)

	raw.music.ChunithmMusic.Create().
		SetMusicID(1002).
		SetTitle("Future Song").
		SetArtist("Future Artist").
		SetCategory("VARIETY").
		SetVersion("v9").
		SetReleaseDate(futureAt).
		SaveX(ctx)

	raw.music.ChunithmMusicDifficulty.Create().
		SetMusicID(1001).
		SetVersion("v1").
		SetDiff0Const(12.1).
		SetDiff1Const(12.8).
		SetDiff2Const(13.4).
		SetDiff3Const(13.9).
		SetDiff4Const(14.6).
		SaveX(ctx)

	raw.music.ChunithmMusicDifficulty.Create().
		SetMusicID(1001).
		SetVersion("v2").
		SetDiff0Const(12.2).
		SetDiff1Const(13.0).
		SetDiff2Const(13.7).
		SetDiff3Const(14.2).
		SetDiff4Const(14.8).
		SaveX(ctx)

	raw.music.ChunithmChartData.Create().
		SetMusicID(1001).
		SetDifficulty(3).
		SetCreator("ChartMaster").
		SetBpm(180).
		SetTapCount(500).
		SetHoldCount(120).
		SetSlideCount(80).
		SetAirCount(70).
		SetFlickCount(30).
		SetTotalCount(800).
		SaveX(ctx)

	raw.main.ChunithmMusicAlias.Create().SetMusicID(1001).SetAlias("test-song").SaveX(ctx)
	raw.main.ChunithmMusicAlias.Create().SetMusicID(1001).SetAlias("ts").SaveX(ctx)
	raw.main.ChunithmDefaultServer.Create().SetHarukiUserID(500).SetServer("jp").SaveX(ctx)
	raw.main.ChunithmBinding.Create().SetHarukiUserID(500).SetServer("jp").SetAimeID("AIME-1001").SaveX(ctx)

	raw.pjsk.Alias.Create().SetAliasType("music").SetAliasTypeID(2001).SetAlias("sekai-song").SaveX(ctx)
	raw.pjsk.Alias.Create().SetAliasType("music").SetAliasTypeID(2001).SetAlias("ss").SaveX(ctx)
	raw.pjsk.GroupAlias.Create().SetPlatform("qq").SetGroupID("g1").SetAliasType("character").SetAliasTypeID(3001).SetAlias("miku").SaveX(ctx)

	bindingJP := raw.pjsk.UserBinding.Create().
		SetHarukiUserID(500).
		SetServer("jp").
		SetUserID("pjsk-jp-user").
		SetVisible(true).
		SaveX(ctx)

	raw.pjsk.UserBinding.Create().
		SetHarukiUserID(500).
		SetServer("en").
		SetUserID("pjsk-en-user").
		SetVisible(false).
		SaveX(ctx)

	raw.pjsk.UserDefaultBinding.Create().
		SetHarukiUserID(500).
		SetServer("default").
		SetBinding(bindingJP).
		SaveX(ctx)

	raw.pjsk.UserPreference.Create().SetHarukiUserID(500).SetOption("theme").SetValue("light").SaveX(ctx)
	raw.pjsk.UserPreference.Create().SetHarukiUserID(500).SetOption("difficulty").SetValue("expert").SaveX(ctx)

	raw.users.User.Create().
		SetID(500).
		SetPlatform("qq").
		SetUserID("10001").
		SetBanState(false).
		SaveX(ctx)
}

func closeQueryClients(raw *queryTestClients) {
	if raw == nil {
		return
	}
	if raw.main != nil {
		_ = raw.main.Close()
	}
	if raw.music != nil {
		_ = raw.music.Close()
	}
	if raw.pjsk != nil {
		_ = raw.pjsk.Close()
	}
	if raw.users != nil {
		_ = raw.users.Close()
	}
}

func openMainClient(t *testing.T) *chunithmMainDB.Client {
	t.Helper()
	dsn := fmt.Sprintf("file:query_chunithm_main_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	client, err := chunithmMainDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open chunithm main db: %v", err)
	}
	return client
}

func openMusicClient(t *testing.T) *chunithmMusicDB.Client {
	t.Helper()
	dsn := fmt.Sprintf("file:query_chunithm_music_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	client, err := chunithmMusicDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open chunithm music db: %v", err)
	}
	return client
}

func openPJSKClient(t *testing.T) *pjskDB.Client {
	t.Helper()
	dsn := fmt.Sprintf("file:query_pjsk_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	client, err := pjskDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open pjsk db: %v", err)
	}
	return client
}

func openUsersClient(t *testing.T) *usersDB.Client {
	t.Helper()
	dsn := fmt.Sprintf("file:query_users_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	client, err := usersDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open users db: %v", err)
	}
	return client
}
