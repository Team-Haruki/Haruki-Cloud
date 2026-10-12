package pjsk

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	botDB "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/commandmanifest"
	botenttest "haruki-cloud/database/bot/enttest"
	commandhandler "haruki-cloud/internal/handler"
	pjskhandler "haruki-cloud/internal/pjsk/handler"

	"entgo.io/ent"
	_ "github.com/mattn/go-sqlite3"
)

func TestSeedCommandManifestsIncludesBirthdayMonitor(t *testing.T) {
	ctx := context.Background()
	client := botenttest.Open(t, "sqlite3", fmt.Sprintf("file:bot_manifest_birthday_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	defer client.Close()
	pjskhandler.EnsureCommandHandlersRegistered()

	if err := SeedCommandManifests(ctx, client); err != nil {
		t.Fatalf("seed manifests: %v", err)
	}

	row, err := client.CommandManifest.Query().
		Where(
			commandmanifest.CommandModule(pjskhandler.BotModulePJSK),
			commandmanifest.CommandPath(birthdayMonitorCommandPath),
		).
		Only(ctx)
	if err != nil {
		t.Fatalf("query birthday monitor manifest: %v", err)
	}

	if row.CommandMode != "POST" {
		t.Fatalf("unexpected command mode: %s", row.CommandMode)
	}
	for _, command := range birthdayMonitorCommandPrefixes {
		if !slices.Contains(row.CommandPrefixes, command) {
			t.Fatalf("expected birthday monitor manifest to include %q, got %v", command, row.CommandPrefixes)
		}
	}
	for _, command := range []string{"/jp烤森生日监听", "/tw烤森生日取消监听", "/enmysekai birthday monitor"} {
		if !slices.Contains(row.CommandPrefixes, command) {
			t.Fatalf("expected birthday monitor manifest to include region-prefixed %q, got %v", command, row.CommandPrefixes)
		}
	}
}

func TestCommandManifestRoutesMarksFeaturePolicyScopes(t *testing.T) {
	pjskhandler.EnsureCommandHandlersRegistered()

	scopes := commandManifestClientPolicyScopes()
	want := map[string]string{
		manifestKey(pjskhandler.BotModulePJSK, customProfileCommandPath):   customProfileClientPolicyScope,
		manifestKey(pjskhandler.BotModulePJSK, birthdayMonitorCommandPath): birthdayMonitorClientPolicyScope,
	}
	for key, scope := range want {
		if scopes[key] != scope {
			t.Fatalf("%s client policy scope = %q", key, scopes[key])
		}
	}
}

// countingManifestHook counts command_manifests mutations by operation.
func countingManifestHook(counts map[string]int) ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			counts[m.Op().String()]++
			return next.Mutate(ctx, m)
		})
	}
}

func TestSeedCommandManifestsWritesOnlyChangedRows(t *testing.T) {
	ctx := context.Background()
	client := botenttest.Open(t, "sqlite3", fmt.Sprintf("file:bot_manifest_diff_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	defer client.Close()
	pjskhandler.EnsureCommandHandlersRegistered()
	counts := map[string]int{}
	client.CommandManifest.Use(countingManifestHook(counts))

	first, err := seedCommandManifests(ctx, client)
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if first.created == 0 || first.updated != 0 || first.unchanged != 0 {
		t.Fatalf("first seed stats = %+v", first)
	}
	// The bulk insert is one statement, but ent runs the hook once per entity.
	if counts["OpCreate"] != first.created || counts["OpUpdateOne"] != 0 {
		t.Fatalf("first seed mutations = %v, want %d creates and no updates", counts, first.created)
	}

	clear(counts)
	second, err := seedCommandManifests(ctx, client)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if second.created != 0 || second.updated != 0 || second.unchanged != first.created {
		t.Fatalf("second seed stats = %+v, want %d unchanged", second, first.created)
	}
	if len(counts) != 0 {
		t.Fatalf("steady-state seed issued mutations: %v", counts)
	}

	// A drifted row is refreshed, keeping its priority; the rest stay untouched.
	row, err := client.CommandManifest.Query().
		Where(commandmanifest.CommandModule(pjskhandler.BotModulePJSK), commandmanifest.CommandPath(birthdayMonitorCommandPath)).
		Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := row.Update().SetCommandPrefixes([]string{"/stale"}).SetCommandPriority(7).Save(ctx); err != nil {
		t.Fatal(err)
	}
	clear(counts)
	third, err := seedCommandManifests(ctx, client)
	if err != nil {
		t.Fatalf("third seed: %v", err)
	}
	if third.updated != 1 || third.created != 0 || third.unchanged != first.created-1 {
		t.Fatalf("third seed stats = %+v", third)
	}
	if counts["OpUpdateOne"] != 1 {
		t.Fatalf("third seed mutations = %v, want one update", counts)
	}
	refreshed, err := client.CommandManifest.Get(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.CommandPriority != 7 || !slices.Contains(refreshed.CommandPrefixes, "/jp烤森生日监听") {
		t.Fatalf("refreshed row = %+v", refreshed)
	}
}

func TestManifestRowMatchesRouteTreatsNilAndEmptyAlike(t *testing.T) {
	row := &botDB.CommandManifest{CommandMode: "POST", CommandPrefixes: []string{"/a"}, CommandAdditionalParams: []string{}}
	route := commandhandler.BotRoute{CommandMode: "POST", Commands: []string{"/a"}}
	if !manifestRowMatchesRoute(row, route) {
		t.Fatal("nil and empty additional params should match")
	}
	route.Commands = []string{"/a", "/b"}
	if manifestRowMatchesRoute(row, route) {
		t.Fatal("different prefixes should not match")
	}
	route.Commands = []string{"/a"}
	route.CommandMode = "GET"
	if manifestRowMatchesRoute(row, route) {
		t.Fatal("different mode should not match")
	}
}
