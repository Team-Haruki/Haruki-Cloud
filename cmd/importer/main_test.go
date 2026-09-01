package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	pjskDB "haruki-cloud/database/pjsk"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	"haruki-cloud/database/pjsk/userdefaultbinding"
	usersDB "haruki-cloud/database/users"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/testutil"

	_ "github.com/mattn/go-sqlite3"
)

func openImporterTestClients(t *testing.T) (*pjskDB.Client, *usersDB.Client) {
	t.Helper()
	suffix := time.Now().UnixNano()
	pjsk := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:importer_pjsk_%d?mode=memory&cache=shared&_fk=1", suffix))
	users := usersenttest.Open(t, "sqlite3", fmt.Sprintf("file:importer_users_%d?mode=memory&cache=shared&_fk=1", suffix))
	return pjsk, users
}

func writeImporterFixture(t *testing.T, dir, name, body string) {
	t.Helper()
	{
		err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600)
		testutil.Require(t, !(err != nil), "write %s: %v", name, err)
	}

}

func TestImporterHelpers(t *testing.T) {
	qqbotID := "2e2344faaf7a342ee92d883caae14cf25e31345ff64db4bb1e05004ef8f901f6_102070411"
	for _, tc := range []struct {
		input        string
		wantPlatform string
	}{
		{input: "123456", wantPlatform: "qq"},
		{input: qqbotID, wantPlatform: "qqbot"},
	} {
		platform, userID := detectPlatform(tc.input)
		{
			testutil.Require(t, !(platform != tc.wantPlatform), "detectPlatform(%q) = (%q, %q)", tc.input, platform, userID)
			testutil.Require(t, !(userID != tc.input), "detectPlatform(%q) = (%q, %q)", tc.input, platform, userID)
		}

	}
	{

		got := serverFromFilename("bind.json")
		testutil.Require(t, !(got != "jp"), "serverFromFilename(bind.json) = %q", got)
	}
	{

		got := serverFromFilename("tw_bind.json")
		testutil.Require(t, !(got != "tw"), "serverFromFilename(tw_bind.json) = %q", got)
	}
	{
		testutil.RequireArgs(t, !(importErrorType(nil) != ""), "importErrorType did not distinguish nil and non-nil errors")
		testutil.RequireArgs(t, !(importErrorType(context.Canceled) == ""), "importErrorType did not distinguish nil and non-nil errors")
	}

	t.Setenv("IMPORTER_TEST_DB_URL", "sqlite://from-env")
	t.Setenv("IMPORTER_TEST_DB_TYPE", "")
	dbType, dsn, err := resolveDBConfig("IMPORTER_TEST_DB_URL", "IMPORTER_TEST_DB_TYPE", "config", "mysql", "sqlite3")
	{
		testutil.Require(t, !(err != nil), "resolve env config = (%q, %q, %v)", dbType, dsn, err)
		testutil.Require(t, !(dbType != "sqlite3"), "resolve env config = (%q, %q, %v)", dbType, dsn, err)
		testutil.Require(t, !(dsn != "sqlite://from-env"), "resolve env config = (%q, %q, %v)", dbType, dsn, err)
	}

	t.Setenv("IMPORTER_TEST_DB_URL", "")
	dbType, dsn, err = resolveDBConfig("IMPORTER_TEST_DB_URL", "IMPORTER_TEST_DB_TYPE", "config-dsn", "", "postgres")
	{
		testutil.Require(t, !(err != nil), "resolve file config = (%q, %q, %v)", dbType, dsn, err)
		testutil.Require(t, !(dbType != "postgres"), "resolve file config = (%q, %q, %v)", dbType, dsn, err)
		testutil.Require(t, !(dsn != "config-dsn"), "resolve file config = (%q, %q, %v)", dbType, dsn, err)
	}
	{

		_, _, err := resolveDBConfig("IMPORTER_TEST_DB_URL", "IMPORTER_TEST_DB_TYPE", "", "", "postgres")
		testutil.RequireArgs(t, !(err == nil), "missing DB config unexpectedly succeeded")
	}

}

func TestImporterDryRunAndInputErrors(t *testing.T) {
	dir := t.TempDir()
	writeImporterFixture(t, dir, "bind.json", `[{"id":1,"im_user_id":"10001","user_id":"20001","is_private":0}]`)
	writeImporterFixture(t, dir, "character_alias.json", `[{"id":1,"alias":"miku","character_id":21}]`)
	writeImporterFixture(t, dir, "music_alias.json", `[{"id":2,"alias":"tell your world","music_id":1}]`)
	writeImporterFixture(t, dir, "group_character_alias.json", `[{"id":3,"group_id":"9","alias":"39","character_id":21}]`)

	ctx := context.Background()
	{
		err := importBindings(ctx, dir, nil, nil, true)
		testutil.Require(t, !(err != nil), "dry-run bindings: %v", err)
	}
	{

		err := importCharacterAliases(ctx, dir, nil, true)
		testutil.Require(t, !(err != nil), "dry-run character aliases: %v", err)
	}
	{

		err := importMusicAliases(ctx, dir, nil, true)
		testutil.Require(t, !(err != nil), "dry-run music aliases: %v", err)
	}
	{

		err := importGroupAliases(ctx, dir, nil, true)
		testutil.Require(t, !(err != nil), "dry-run group aliases: %v", err)
	}
	{

		err := setDefaultBindings(ctx, nil, true)
		testutil.Require(t, !(err != nil), "dry-run defaults: %v", err)
	}

	writeImporterFixture(t, dir, "bad.json", `{`)
	{
		_, err := loadJSON[bindRecord](filepath.Join(dir, "bad.json"))
		testutil.RequireArgs(t, !(err == nil), "malformed JSON unexpectedly decoded")
	}
	{

		_, err := loadJSON[bindRecord](filepath.Join(dir, "missing.json"))
		testutil.Require(t, errors.Is(err, os.ErrNotExist), "missing JSON error = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	{
		err := importBindings(canceled, dir, nil, nil, true)
		testutil.Require(t, errors.Is(err, context.Canceled), "canceled bindings error = %v", err)
	}
	{

		err := importCharacterAliases(canceled, dir, nil, true)
		testutil.Require(t, errors.Is(err, context.Canceled), "canceled character aliases error = %v", err)
	}
	{

		err := importMusicAliases(canceled, dir, nil, true)
		testutil.Require(t, errors.Is(err, context.Canceled), "canceled music aliases error = %v", err)
	}
	{

		err := importGroupAliases(canceled, dir, nil, true)
		testutil.Require(t, errors.Is(err, context.Canceled), "canceled group aliases error = %v", err)
	}

}

func TestImporterDatabaseRoundTripIsIdempotent(t *testing.T) {
	pjsk, users := openImporterTestClients(t)
	dir := t.TempDir()
	writeImporterFixture(t, dir, "bind.json", `[
		{"id":1,"im_user_id":"10001","user_id":"20001","is_private":0}
	]`)
	writeImporterFixture(t, dir, "cn_bind.json", `[
		{"id":2,"im_user_id":"10001","user_id":"20002","is_private":1}
	]`)
	writeImporterFixture(t, dir, "tw_bind.json", `[
		{"id":3,"im_user_id":"10002","user_id":"20003","is_private":0}
	]`)
	writeImporterFixture(t, dir, "character_alias.json", `[{"id":1,"alias":"miku","character_id":21}]`)
	writeImporterFixture(t, dir, "music_alias.json", `[{"id":2,"alias":"tell your world","music_id":1}]`)
	writeImporterFixture(t, dir, "group_character_alias.json", `[{"id":3,"group_id":"9","alias":"39","character_id":21}]`)

	ctx := context.Background()
	for attempt := 0; attempt < 2; attempt++ {
		{
			err := importBindings(ctx, dir, pjsk, users, false)
			testutil.Require(t, !(err != nil), "import bindings attempt %d: %v", attempt, err)
		}
		{

			err := importCharacterAliases(ctx, dir, pjsk, false)
			testutil.Require(t, !(err != nil), "import character aliases attempt %d: %v", attempt, err)
		}
		{

			err := importMusicAliases(ctx, dir, pjsk, false)
			testutil.Require(t, !(err != nil), "import music aliases attempt %d: %v", attempt, err)
		}
		{

			err := importGroupAliases(ctx, dir, pjsk, false)
			testutil.Require(t, !(err != nil), "import group aliases attempt %d: %v", attempt, err)
		}
		{

			err := setDefaultBindings(ctx, pjsk, false)
			testutil.Require(t, !(err != nil), "set defaults attempt %d: %v", attempt, err)
		}

	}
	{

		got, err := pjsk.GameAccount.Query().Count(ctx)
		{
			testutil.Require(t, !(err != nil), "game accounts = %d, %v", got, err)
			testutil.Require(t, !(got != 3), "game accounts = %d, %v", got, err)
		}
	}
	{

		got, err := pjsk.UserBinding.Query().Count(ctx)
		{
			testutil.Require(t, !(err != nil), "bindings = %d, %v", got, err)
			testutil.Require(t, !(got != 3), "bindings = %d, %v", got, err)
		}
	}
	{

		got, err := pjsk.Alias.Query().Count(ctx)
		{
			testutil.Require(t, !(err != nil), "aliases = %d, %v", got, err)
			testutil.Require(t, !(got != 2), "aliases = %d, %v", got, err)
		}
	}
	{

		got, err := pjsk.GroupAlias.Query().Count(ctx)
		{
			testutil.Require(t, !(err != nil), "group aliases = %d, %v", got, err)
			testutil.Require(t, !(got != 1), "group aliases = %d, %v", got, err)
		}
	}
	{

		got, err := pjsk.UserDefaultBinding.Query().Count(ctx)
		{
			testutil.Require(t, !(err != nil), "defaults = %d, %v", got, err)
			testutil.Require(t, !(got != 5), "defaults = %d, %v", got, err)
		}
	}

	globalDefaults, err := pjsk.UserDefaultBinding.Query().
		Where(userdefaultbinding.Server("default")).
		WithBinding(func(query *pjskDB.UserBindingQuery) { query.WithGameAccount() }).
		All(ctx)
	{
		testutil.Require(t, !(err != nil), "global defaults = %d, %v", len(globalDefaults), err)
		testutil.Require(t, !(len(globalDefaults) != 2), "global defaults = %d, %v", len(globalDefaults), err)
	}

	for _, item := range globalDefaults {
		binding := item.Edges.Binding
		{
			testutil.Require(t, !(binding == nil), "global default missing binding edges: %#v", item)
			testutil.Require(t, !(binding.Edges.GameAccount == nil), "global default missing binding edges: %#v", item)
		}

	}

	bindings, err := pjsk.UserBinding.Query().All(ctx)
	testutil.Require(t, !(err != nil), "query imported bindings: %v", err)

	bindingsPerUser := make(map[int]int)
	for _, binding := range bindings {
		bindingsPerUser[binding.HarukiUserID]++
	}
	var multiAccountUserID int
	for userID, count := range bindingsPerUser {
		if count == 2 {
			multiAccountUserID = userID
			break
		}
	}
	testutil.Require(t, !(multiAccountUserID == 0), "multi-account user not found: %#v", bindingsPerUser)

	global, err := pjsk.UserDefaultBinding.Query().
		Where(userdefaultbinding.HarukiUserID(multiAccountUserID), userdefaultbinding.Server("default")).
		WithBinding(func(query *pjskDB.UserBindingQuery) { query.WithGameAccount() }).
		Only(ctx)
	{
		testutil.Require(t, !(err != nil), "query multi-account global default: %#v, %v", global, err)
		testutil.Require(t, !(global.Edges.Binding == nil), "query multi-account global default: %#v, %v", global, err)
		testutil.Require(t, !(global.Edges.Binding.Edges.GameAccount == nil), "query multi-account global default: %#v, %v", global, err)
	}
	{

		got := global.Edges.Binding.Edges.GameAccount.Server
		testutil.Require(t, !(got != "jp"), "multi-account global default server = %q", got)
	}
	{

		order, err := nextDisplayOrder(ctx, pjsk, multiAccountUserID)
		{
			testutil.Require(t, !(err != nil), "next display order = %d, %v", order, err)
			testutil.Require(t, !(order != 2), "next display order = %d, %v", order, err)
		}
	}

}

func TestImporterClosedDatabaseErrors(t *testing.T) {
	pjsk, _ := openImporterTestClients(t)
	{
		err := pjsk.Close()
		testutil.Require(t, !(err != nil), "close pjsk client: %v", err)
	}

	ctx := context.Background()
	{
		_, err := nextDisplayOrder(ctx, pjsk, 1)
		testutil.RequireArgs(t, !(err == nil), "nextDisplayOrder on closed DB unexpectedly succeeded")
	}
	{

		err := setDefaultBindings(ctx, pjsk, false)
		testutil.RequireArgs(t, !(err == nil), "setDefaultBindings on closed DB unexpectedly succeeded")
	}

}
