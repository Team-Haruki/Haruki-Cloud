package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	pjskDB "haruki-cloud/database/pjsk"
	"haruki-cloud/internal/testutil"
)

func runImporterMainForCoverage(t *testing.T, args ...string) {
	t.Helper()
	originalFlags := flag.CommandLine
	originalArgs := os.Args
	flag.CommandLine = flag.NewFlagSet("importer-coverage", flag.ContinueOnError)
	os.Args = append([]string{"importer"}, args...)
	t.Cleanup(func() {
		flag.CommandLine = originalFlags
		os.Args = originalArgs
	})
	main()
}

func TestImporterMainSuccessPaths(t *testing.T) {
	dir := t.TempDir()
	writeImporterFixture(t, dir, "bind.json", `[{"id":1,"im_user_id":"10001","user_id":"20001","is_private":0}]`)
	writeImporterFixture(t, dir, "character_alias.json", `[{"id":2,"alias":"miku","character_id":21}]`)
	writeImporterFixture(t, dir, "music_alias.json", `[{"id":3,"alias":"world","music_id":1}]`)
	writeImporterFixture(t, dir, "group_character_alias.json", `[{"id":4,"group_id":"9","alias":"39","character_id":21}]`)

	t.Run("dry run", func(t *testing.T) {
		runImporterMainForCoverage(t,
			"--exports-dir", dir,
			"--target", " all ",
			"--dry-run",
			"--config", filepath.Join(dir, "missing.yaml"),
		)
	})

	t.Run("configured sqlite databases", func(t *testing.T) {
		configPath := filepath.Join(dir, "importer.yaml")
		pjskPath := filepath.Join(dir, "main-pjsk.db")
		usersPath := filepath.Join(dir, "main-users.db")
		writeImporterFixture(t, dir, "importer.yaml", fmt.Sprintf(`profile: dev
pjsk:
  db_type: sqlite3
  db_url: %q
users_db:
  db_type: sqlite3
  db_url: %q
`, "file:"+pjskPath+"?_fk=1", "file:"+usersPath+"?_fk=1"))
		preflight, err := pjskDB.Open("sqlite3", "file:"+pjskPath+"?_fk=1")
		testutil.Require(t, !(err != nil), "open importer preflight DB: %v", err)
		{

			err := preflight.Schema.Create(context.Background())
			testutil.Require(t, !(err != nil), "migrate importer preflight DB: %v", err)
		}
		{

			err := preflight.Close()
			testutil.Require(t, !(err != nil), "close importer preflight DB: %v", err)
		}

		t.Setenv("HARUKI_PJSK_DB_URL", "")
		t.Setenv("HARUKI_PJSK_DB_TYPE", "")
		t.Setenv("HARUKI_USERS_DB_URL", "")
		t.Setenv("HARUKI_USERS_DB_TYPE", "")
		runImporterMainForCoverage(t,
			"--exports-dir", dir,
			"--target", "bindings, character-aliases,music-aliases, group-aliases,defaults",
			"--config", configPath,
		)
	})
}

func installImporterQueryFailure(client *pjskDB.Client, failAt int, forced error, after func()) {
	calls := 0
	client.Intercept(pjskDB.InterceptFunc(func(next pjskDB.Querier) pjskDB.Querier {
		return pjskDB.QuerierFunc(func(ctx context.Context, query pjskDB.Query) (pjskDB.Value, error) {
			calls++
			if calls == failAt {
				return nil, forced
			}
			value, err := next.Query(ctx, query)
			if after != nil && calls == failAt-1 {
				after()
			}
			return value, err
		})
	}))
}

func installImporterMutationFailure(client *pjskDB.Client, forced error) {
	client.Use(func(pjskDB.Mutator) pjskDB.Mutator {
		return pjskDB.MutateFunc(func(context.Context, pjskDB.Mutation) (pjskDB.Value, error) {
			return nil, forced
		})
	})
}

func TestImportBindingsStableDatabaseFailures(t *testing.T) {
	dir := t.TempDir()
	writeImporterFixture(t, dir, "bind.json", `[{"id":1,"im_user_id":"10001","user_id":"20001","is_private":0}]`)
	ctx := context.Background()
	forced := errors.New("forced importer failure")

	t.Run("resolve user", func(t *testing.T) {
		pjsk, users := openImporterTestClients(t)
		{
			err := users.Close()
			testutil.Require(t, !(err != nil), "close users DB: %v", err)
		}
		{

			err := importBindings(ctx, dir, pjsk, users, false)
			testutil.Require(t, !(err != nil), "resolve-user record failure should be skipped: %v", err)
		}

	})

	for _, tc := range []struct {
		name   string
		failAt int
	}{
		{name: "game account query", failAt: 1},
		{name: "binding query", failAt: 2},
		{name: "display order", failAt: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pjsk, users := openImporterTestClients(t)
			installImporterQueryFailure(pjsk, tc.failAt, forced, nil)
			{
				err := importBindings(ctx, dir, pjsk, users, false)
				testutil.Require(t, !(err != nil), "record query failure should be skipped: %v", err)
			}

		})
	}

	for _, tc := range []struct {
		name   string
		forced error
	}{
		{name: "generic create", forced: forced},
		{name: "constraint create", forced: &pjskDB.ConstraintError{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pjsk, users := openImporterTestClients(t)
			installImporterMutationFailure(pjsk, tc.forced)
			{
				err := importBindings(ctx, dir, pjsk, users, false)
				testutil.Require(t, !(err != nil), "game-account create failure should be skipped: %v", err)
			}

		})
	}

	t.Run("binding create", func(t *testing.T) {
		pjsk, users := openImporterTestClients(t)
		{
			_, err := pjsk.GameAccount.Create().SetServer("jp").SetUserID("20001").Save(ctx)
			testutil.Require(t, !(err != nil), "seed game account: %v", err)
		}

		installImporterMutationFailure(pjsk, forced)
		{
			err := importBindings(ctx, dir, pjsk, users, false)
			testutil.Require(t, !(err != nil), "binding create failure should be skipped: %v", err)
		}

	})

	t.Run("binding constraint", func(t *testing.T) {
		pjsk, users := openImporterTestClients(t)
		{
			_, err := pjsk.GameAccount.Create().SetServer("jp").SetUserID("20001").Save(ctx)
			testutil.Require(t, !(err != nil), "seed game account: %v", err)
		}

		installImporterMutationFailure(pjsk, &pjskDB.ConstraintError{})
		{
			err := importBindings(ctx, dir, pjsk, users, false)
			testutil.Require(t, !(err != nil), "binding constraint should be skipped: %v", err)
		}

	})
}

func TestAliasImportsStableDatabaseFailures(t *testing.T) {
	dir := t.TempDir()
	writeImporterFixture(t, dir, "character_alias.json", `[{"id":1,"alias":"miku","character_id":21}]`)
	writeImporterFixture(t, dir, "music_alias.json", `[{"id":2,"alias":"world","music_id":1}]`)
	writeImporterFixture(t, dir, "group_character_alias.json", `[{"id":3,"group_id":"9","alias":"39","character_id":21}]`)
	ctx := context.Background()
	forced := errors.New("forced alias failure")
	runAll := func(t *testing.T, client *pjskDB.Client) {
		t.Helper()
		for name, call := range map[string]func() error{
			"character": func() error { return importCharacterAliases(ctx, dir, client, false) },
			"music":     func() error { return importMusicAliases(ctx, dir, client, false) },
			"group":     func() error { return importGroupAliases(ctx, dir, client, false) },
		} {
			{
				err := call()
				testutil.Require(t, !(err != nil), "%s record failure should be skipped: %v", name, err)
			}

		}
	}

	t.Run("query failures", func(t *testing.T) {
		client, _ := openImporterTestClients(t)
		installImporterQueryFailure(client, 1, forced, nil)
		client.Intercept(pjskDB.InterceptFunc(func(pjskDB.Querier) pjskDB.Querier {
			return pjskDB.QuerierFunc(func(context.Context, pjskDB.Query) (pjskDB.Value, error) {
				return nil, forced
			})
		}))
		runAll(t, client)
	})
	for _, tc := range []struct {
		name   string
		forced error
	}{
		{name: "insert failures", forced: forced},
		{name: "constraint failures", forced: &pjskDB.ConstraintError{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := openImporterTestClients(t)
			installImporterMutationFailure(client, tc.forced)
			runAll(t, client)
		})
	}

	logRecordFailure(ctx, "test", "jp", "test", 1, forced)
}

func TestSetDefaultBindingsRemainingBranches(t *testing.T) {
	ctx := context.Background()
	forced := errors.New("forced defaults failure")

	t.Run("binding without account", func(t *testing.T) {
		client, _ := openImporterTestClients(t)
		{
			_, err := client.UserBinding.Create().SetHarukiUserID(1).Save(ctx)
			testutil.Require(t, !(err != nil), "create edge-less binding: %v", err)
		}
		{

			err := setDefaultBindings(ctx, client, false)
			testutil.Require(t, !(err != nil), "skip edge-less binding: %v", err)
		}

	})

	t.Run("existing defaults query", func(t *testing.T) {
		client, _ := openImporterTestClients(t)
		installImporterQueryFailure(client, 2, forced, nil)
		{
			err := setDefaultBindings(ctx, client, false)
			testutil.Require(t, errors.Is(err, forced), "existing-default query error = %v", err)
		}

	})

	for _, tc := range []struct {
		name   string
		forced error
	}{
		{name: "create failure", forced: forced},
		{name: "create constraint", forced: &pjskDB.ConstraintError{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := openImporterTestClients(t)
			account, err := client.GameAccount.Create().SetServer("jp").SetUserID("30001").Save(ctx)
			testutil.Require(t, !(err != nil), "create account: %v", err)
			{

				_, err := client.UserBinding.Create().SetHarukiUserID(1).SetGameAccountID(account.ID).Save(ctx)
				testutil.Require(t, !(err != nil), "create binding: %v", err)
			}

			installImporterMutationFailure(client, tc.forced)
			{
				err := setDefaultBindings(ctx, client, false)
				testutil.Require(t, !(err != nil), "default creation failures are per-record: %v", err)
			}

		})
	}

	t.Run("context cancellation after loading", func(t *testing.T) {
		client, _ := openImporterTestClients(t)
		account, err := client.GameAccount.Create().SetServer("jp").SetUserID("30002").Save(ctx)
		testutil.Require(t, !(err != nil), "create account: %v", err)
		{

			_, err := client.UserBinding.Create().SetHarukiUserID(1).SetGameAccountID(account.ID).Save(ctx)
			testutil.Require(t, !(err != nil), "create binding: %v", err)
		}

		cancelCtx, cancel := context.WithCancel(ctx)
		calls := 0
		client.Intercept(pjskDB.InterceptFunc(func(next pjskDB.Querier) pjskDB.Querier {
			return pjskDB.QuerierFunc(func(queryCtx context.Context, query pjskDB.Query) (pjskDB.Value, error) {
				calls++
				value, err := next.Query(queryCtx, query)
				if calls == 2 {
					cancel()
				}
				return value, err
			})
		}))
		{
			err := setDefaultBindings(cancelCtx, client, false)
			testutil.Require(t, errors.Is(err, context.Canceled), "post-load cancellation error = %v", err)
		}

	})
}

func TestImportBindingsDryRunProgressBranch(t *testing.T) {
	dir := t.TempDir()
	records := make([]byte, 0, 400_000)
	records = append(records, '[')
	for i := 0; i < 5000; i++ {
		if i > 0 {
			records = append(records, ',')
		}
		records = append(records, fmt.Sprintf(`{"id":%d,"im_user_id":"%d","user_id":"%d","is_private":0}`, i, i, i)...)
	}
	records = append(records, ']')
	{
		err := os.WriteFile(filepath.Join(dir, "bind.json"), records, 0o600)
		testutil.Require(t, !(err != nil), "write large dry-run fixture: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	{
		err := importBindings(ctx, dir, nil, nil, true)
		testutil.Require(t, !(err != nil), "large dry-run import: %v", err)
	}

}
