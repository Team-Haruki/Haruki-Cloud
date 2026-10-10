package server

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	harukiConfig "haruki-cloud/config"
	pjskDB "haruki-cloud/database/pjsk"
	"haruki-cloud/internal/core/dbpool"
	harukiLogger "haruki-cloud/utils/logger"
)

// The legacy visible column is backfilled into NULL per-exposure flags and
// dropped after the PJSK auto-migrate, only on a writable node.
func TestMigratePJSKDataDropsLegacyVisibleOnlyOnWritableNodes(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("file:server_visible_drop_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	drv, err := openEntDriver("visible_drop_test", "sqlite3", dsn, dbpool.Config{MaxOpen: 2, MaxIdle: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	client := pjskDB.NewClient(pjskDB.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	account, err := client.GameAccount.Create().SetServer("jp").SetUserID("1001").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := client.UserBinding.Create().SetHarukiUserID(1).SetGameAccountID(account.ID).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The table as a pre-3.9.0 binary left it: a hidden binding, flags NULL.
	if _, err := drv.DB().ExecContext(ctx, "ALTER TABLE user_bindings ADD COLUMN visible bool NOT NULL DEFAULT true"); err != nil {
		t.Fatal(err)
	}
	if _, err := drv.DB().ExecContext(ctx, "UPDATE user_bindings SET visible = false WHERE id = ?", binding.ID); err != nil {
		t.Fatal(err)
	}
	columnCount := func() int {
		var n int
		if err := drv.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_table_info('user_bindings') WHERE name = 'visible'").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	var logs bytes.Buffer
	logger := harukiLogger.NewLogger("Main", "INFO", &logs)

	previous := harukiConfig.Cfg.Node.ReadOnly
	t.Cleanup(func() { harukiConfig.Cfg.Node.ReadOnly = previous })

	harukiConfig.Cfg.Node.ReadOnly = true
	if err := migratePJSKData(ctx, logger, drv); err != nil {
		t.Fatalf("read-only migrate: %v", err)
	}
	if row, _ := client.UserBinding.Get(ctx, binding.ID); row.UIDVisible != nil || columnCount() != 1 {
		t.Fatalf("read-only node changed the table: %+v", row)
	}

	harukiConfig.Cfg.Node.ReadOnly = false
	if err := migratePJSKData(ctx, logger, drv); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	row, _ := client.UserBinding.Get(ctx, binding.ID)
	if row.UIDVisible == nil || *row.UIDVisible || row.SkVisible == nil || *row.SkVisible || row.ProfileVisible == nil || *row.ProfileVisible || row.ArrestVisible == nil || *row.ArrestVisible {
		t.Fatalf("hidden binding not backfilled as hidden: %+v", row)
	}
	if columnCount() != 0 {
		t.Fatal("visible column not dropped")
	}
	if !strings.Contains(logs.String(), "dropped the legacy user_bindings.visible column") {
		t.Fatalf("drop not logged: %s", logs.String())
	}
	if err := migratePJSKData(ctx, logger, drv); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if err := migratePJSKData(ctx, logger, nil); err != nil {
		t.Fatalf("nil driver: %v", err)
	}
	_ = drv.DB().Close()
	if err := migratePJSKData(ctx, logger, drv); err == nil {
		t.Fatal("closed database not reported")
	}
}
