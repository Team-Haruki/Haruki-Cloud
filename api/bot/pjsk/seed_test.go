package pjsk

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"haruki-cloud/database/bot/commandmanifest"
	botenttest "haruki-cloud/database/bot/enttest"
	pjskhandler "haruki-cloud/internal/pjsk/handler"

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
