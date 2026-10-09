package server

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	harukiConfig "haruki-cloud/config"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	harukiLogger "haruki-cloud/utils/logger"
)

// The binding visibility bootstrap runs after the PJSK auto-migrate on a
// writable node and is skipped on a read-only one.
func TestBootstrapPJSKDataRunsOnlyOnWritableNodes(t *testing.T) {
	ctx := context.Background()
	client := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:server_visibility_bootstrap_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = client.Close() })
	account, err := client.GameAccount.Create().SetServer("jp").SetUserID("1001").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := client.UserBinding.Create().SetHarukiUserID(1).SetGameAccountID(account.ID).SetVisible(false).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := harukiLogger.NewLogger("Main", "INFO", &logs)

	previous := harukiConfig.Cfg.Node.ReadOnly
	t.Cleanup(func() { harukiConfig.Cfg.Node.ReadOnly = previous })

	harukiConfig.Cfg.Node.ReadOnly = true
	if err := bootstrapPJSKData(ctx, logger, client); err != nil {
		t.Fatalf("read-only bootstrap: %v", err)
	}
	if row, _ := client.UserBinding.Get(ctx, binding.ID); row.UIDVisible != nil {
		t.Fatalf("read-only node wrote flags: %+v", row)
	}

	harukiConfig.Cfg.Node.ReadOnly = false
	if err := bootstrapPJSKData(ctx, logger, client); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	row, _ := client.UserBinding.Get(ctx, binding.ID)
	if row.UIDVisible == nil || *row.UIDVisible || row.SkVisible == nil || *row.SkVisible || row.ProfileVisible == nil || *row.ProfileVisible || row.ArrestVisible == nil || *row.ArrestVisible {
		t.Fatalf("hidden binding not bootstrapped as hidden: %+v", row)
	}
	if !strings.Contains(logs.String(), "bootstrapped per-exposure binding visibility") {
		t.Fatalf("bootstrap not logged: %s", logs.String())
	}
}
