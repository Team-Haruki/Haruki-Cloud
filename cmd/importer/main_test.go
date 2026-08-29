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
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
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
		if platform != tc.wantPlatform || userID != tc.input {
			t.Fatalf("detectPlatform(%q) = (%q, %q)", tc.input, platform, userID)
		}
	}

	if got := serverFromFilename("bind.json"); got != "jp" {
		t.Fatalf("serverFromFilename(bind.json) = %q", got)
	}
	if got := serverFromFilename("tw_bind.json"); got != "tw" {
		t.Fatalf("serverFromFilename(tw_bind.json) = %q", got)
	}
	if importErrorType(nil) != "" || importErrorType(context.Canceled) == "" {
		t.Fatal("importErrorType did not distinguish nil and non-nil errors")
	}

	t.Setenv("IMPORTER_TEST_DB_URL", "sqlite://from-env")
	t.Setenv("IMPORTER_TEST_DB_TYPE", "")
	dbType, dsn, err := resolveDBConfig("IMPORTER_TEST_DB_URL", "IMPORTER_TEST_DB_TYPE", "config", "mysql", "sqlite3")
	if err != nil || dbType != "sqlite3" || dsn != "sqlite://from-env" {
		t.Fatalf("resolve env config = (%q, %q, %v)", dbType, dsn, err)
	}
	t.Setenv("IMPORTER_TEST_DB_URL", "")
	dbType, dsn, err = resolveDBConfig("IMPORTER_TEST_DB_URL", "IMPORTER_TEST_DB_TYPE", "config-dsn", "", "postgres")
	if err != nil || dbType != "postgres" || dsn != "config-dsn" {
		t.Fatalf("resolve file config = (%q, %q, %v)", dbType, dsn, err)
	}
	if _, _, err := resolveDBConfig("IMPORTER_TEST_DB_URL", "IMPORTER_TEST_DB_TYPE", "", "", "postgres"); err == nil {
		t.Fatal("missing DB config unexpectedly succeeded")
	}
}

func TestImporterDryRunAndInputErrors(t *testing.T) {
	dir := t.TempDir()
	writeImporterFixture(t, dir, "bind.json", `[{"id":1,"im_user_id":"10001","user_id":"20001","is_private":0}]`)
	writeImporterFixture(t, dir, "character_alias.json", `[{"id":1,"alias":"miku","character_id":21}]`)
	writeImporterFixture(t, dir, "music_alias.json", `[{"id":2,"alias":"tell your world","music_id":1}]`)
	writeImporterFixture(t, dir, "group_character_alias.json", `[{"id":3,"group_id":"9","alias":"39","character_id":21}]`)

	ctx := context.Background()
	if err := importBindings(ctx, dir, nil, nil, true); err != nil {
		t.Fatalf("dry-run bindings: %v", err)
	}
	if err := importCharacterAliases(ctx, dir, nil, true); err != nil {
		t.Fatalf("dry-run character aliases: %v", err)
	}
	if err := importMusicAliases(ctx, dir, nil, true); err != nil {
		t.Fatalf("dry-run music aliases: %v", err)
	}
	if err := importGroupAliases(ctx, dir, nil, true); err != nil {
		t.Fatalf("dry-run group aliases: %v", err)
	}
	if err := setDefaultBindings(ctx, nil, true); err != nil {
		t.Fatalf("dry-run defaults: %v", err)
	}

	writeImporterFixture(t, dir, "bad.json", `{`)
	if _, err := loadJSON[bindRecord](filepath.Join(dir, "bad.json")); err == nil {
		t.Fatal("malformed JSON unexpectedly decoded")
	}
	if _, err := loadJSON[bindRecord](filepath.Join(dir, "missing.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing JSON error = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := importBindings(canceled, dir, nil, nil, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled bindings error = %v", err)
	}
	if err := importCharacterAliases(canceled, dir, nil, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled character aliases error = %v", err)
	}
	if err := importMusicAliases(canceled, dir, nil, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled music aliases error = %v", err)
	}
	if err := importGroupAliases(canceled, dir, nil, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled group aliases error = %v", err)
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
		if err := importBindings(ctx, dir, pjsk, users, false); err != nil {
			t.Fatalf("import bindings attempt %d: %v", attempt, err)
		}
		if err := importCharacterAliases(ctx, dir, pjsk, false); err != nil {
			t.Fatalf("import character aliases attempt %d: %v", attempt, err)
		}
		if err := importMusicAliases(ctx, dir, pjsk, false); err != nil {
			t.Fatalf("import music aliases attempt %d: %v", attempt, err)
		}
		if err := importGroupAliases(ctx, dir, pjsk, false); err != nil {
			t.Fatalf("import group aliases attempt %d: %v", attempt, err)
		}
		if err := setDefaultBindings(ctx, pjsk, false); err != nil {
			t.Fatalf("set defaults attempt %d: %v", attempt, err)
		}
	}

	if got, err := pjsk.GameAccount.Query().Count(ctx); err != nil || got != 3 {
		t.Fatalf("game accounts = %d, %v", got, err)
	}
	if got, err := pjsk.UserBinding.Query().Count(ctx); err != nil || got != 3 {
		t.Fatalf("bindings = %d, %v", got, err)
	}
	if got, err := pjsk.Alias.Query().Count(ctx); err != nil || got != 2 {
		t.Fatalf("aliases = %d, %v", got, err)
	}
	if got, err := pjsk.GroupAlias.Query().Count(ctx); err != nil || got != 1 {
		t.Fatalf("group aliases = %d, %v", got, err)
	}
	if got, err := pjsk.UserDefaultBinding.Query().Count(ctx); err != nil || got != 5 {
		t.Fatalf("defaults = %d, %v", got, err)
	}

	globalDefaults, err := pjsk.UserDefaultBinding.Query().
		Where(userdefaultbinding.Server("default")).
		WithBinding(func(query *pjskDB.UserBindingQuery) { query.WithGameAccount() }).
		All(ctx)
	if err != nil || len(globalDefaults) != 2 {
		t.Fatalf("global defaults = %d, %v", len(globalDefaults), err)
	}
	for _, item := range globalDefaults {
		binding := item.Edges.Binding
		if binding == nil || binding.Edges.GameAccount == nil {
			t.Fatalf("global default missing binding edges: %#v", item)
		}
	}

	bindings, err := pjsk.UserBinding.Query().All(ctx)
	if err != nil {
		t.Fatalf("query imported bindings: %v", err)
	}
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
	if multiAccountUserID == 0 {
		t.Fatalf("multi-account user not found: %#v", bindingsPerUser)
	}
	global, err := pjsk.UserDefaultBinding.Query().
		Where(userdefaultbinding.HarukiUserID(multiAccountUserID), userdefaultbinding.Server("default")).
		WithBinding(func(query *pjskDB.UserBindingQuery) { query.WithGameAccount() }).
		Only(ctx)
	if err != nil || global.Edges.Binding == nil || global.Edges.Binding.Edges.GameAccount == nil {
		t.Fatalf("query multi-account global default: %#v, %v", global, err)
	}
	if got := global.Edges.Binding.Edges.GameAccount.Server; got != "jp" {
		t.Fatalf("multi-account global default server = %q", got)
	}

	if order, err := nextDisplayOrder(ctx, pjsk, multiAccountUserID); err != nil || order != 2 {
		t.Fatalf("next display order = %d, %v", order, err)
	}
}

func TestImporterClosedDatabaseErrors(t *testing.T) {
	pjsk, _ := openImporterTestClients(t)
	if err := pjsk.Close(); err != nil {
		t.Fatalf("close pjsk client: %v", err)
	}
	ctx := context.Background()
	if _, err := nextDisplayOrder(ctx, pjsk, 1); err == nil {
		t.Fatal("nextDisplayOrder on closed DB unexpectedly succeeded")
	}
	if err := setDefaultBindings(ctx, pjsk, false); err == nil {
		t.Fatal("setDefaultBindings on closed DB unexpectedly succeeded")
	}
}
